package sessionstate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stefan/lsp-dap/internal/ride/protocol"
)

func TestDispatcher_PublishesIncomingEvents(t *testing.T) {
	transport := newMockTransport()
	dispatcher := NewDispatcher(transport, protocol.NewCodec())
	events, _ := dispatcher.Subscribe(4)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		dispatcher.Run(ctx)
		close(done)
	}()

	transport.push(`["AppendSessionOutput",{"result":"ok","type":14,"group":0}]`)

	select {
	case event := <-events:
		if event.Command != "AppendSessionOutput" {
			t.Fatalf("unexpected command: %q", event.Command)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timed out waiting for published event")
	}

	transport.close()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("dispatcher did not stop")
	}
}

func TestDispatcher_QueuesBusyCommandsAndFlushesWhenReady(t *testing.T) {
	transport := newMockTransport()
	dispatcher := NewDispatcher(transport, protocol.NewCodec())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		dispatcher.Run(ctx)
		close(done)
	}()

	transport.push(`["SetPromptType",{"type":0}]`)
	waitForCondition(t, 250*time.Millisecond, func() bool {
		promptType, known := dispatcher.PromptType()
		return known && promptType == 0
	})

	if err := dispatcher.SendCommand("Execute", protocol.ExecuteArgs{Text: "1+1\n", Trace: 0}); err != nil {
		t.Fatalf("SendCommand Execute failed: %v", err)
	}

	select {
	case payload := <-transport.writeCh:
		t.Fatalf("Execute should be queued while busy, got write %q", payload)
	case <-time.After(50 * time.Millisecond):
	}

	if err := dispatcher.SendCommand("WeakInterrupt", map[string]any{}); err != nil {
		t.Fatalf("SendCommand WeakInterrupt failed: %v", err)
	}

	select {
	case payload := <-transport.writeCh:
		command, err := decodeCommandName(payload)
		if err != nil {
			t.Fatalf("decodeCommandName failed: %v", err)
		}
		if command != "WeakInterrupt" {
			t.Fatalf("unexpected command written while busy: %q", command)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timed out waiting for allow-listed command write")
	}

	transport.push(`["SetPromptType",{"type":1}]`)

	select {
	case payload := <-transport.writeCh:
		command, err := decodeCommandName(payload)
		if err != nil {
			t.Fatalf("decodeCommandName failed: %v", err)
		}
		if command != "Execute" {
			t.Fatalf("expected queued Execute to flush, got %q", command)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timed out waiting for queued command flush")
	}

	transport.close()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("dispatcher did not stop")
	}
}

func TestDispatcher_DoesNotBlockOnSlowSubscriber(t *testing.T) {
	transport := newMockTransport()
	dispatcher := NewDispatcher(transport, protocol.NewCodec())
	_, _ = dispatcher.Subscribe(1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		dispatcher.Run(ctx)
		close(done)
	}()

	transport.push(`["SetPromptType",{"type":0}]`)
	transport.push(`["SetPromptType",{"type":1}]`)

	waitForCondition(t, 250*time.Millisecond, func() bool {
		promptType, known := dispatcher.PromptType()
		return known && promptType == 1
	})

	transport.close()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("dispatcher did not stop")
	}
}

func TestDispatcher_FlushFailureRequeuesRemainingCommandsInOrder(t *testing.T) {
	transport := newMockTransport()
	dispatcher := NewDispatcher(transport, protocol.NewCodec())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		dispatcher.Run(ctx)
		close(done)
	}()

	transport.push(`["SetPromptType",{"type":0}]`)
	waitForCondition(t, 250*time.Millisecond, func() bool {
		promptType, known := dispatcher.PromptType()
		return known && promptType == 0
	})

	if err := dispatcher.SendCommand("Execute", protocol.ExecuteArgs{Text: "cmd1\n", Trace: 0}); err != nil {
		t.Fatalf("queue cmd1 failed: %v", err)
	}
	if err := dispatcher.SendCommand("Execute", protocol.ExecuteArgs{Text: "cmd2\n", Trace: 0}); err != nil {
		t.Fatalf("queue cmd2 failed: %v", err)
	}

	transport.failNextWrite(errors.New("synthetic write failure"))
	transport.push(`["SetPromptType",{"type":1}]`)

	time.Sleep(30 * time.Millisecond)

	transport.push(`["SetPromptType",{"type":0}]`)
	transport.push(`["SetPromptType",{"type":1}]`)

	first := waitForWrite(t, transport.writeCh, 250*time.Millisecond)
	second := waitForWrite(t, transport.writeCh, 250*time.Millisecond)
	firstCmd, err := decodeCommandPayload(first)
	if err != nil {
		t.Fatalf("decode first payload failed: %v", err)
	}
	secondCmd, err := decodeCommandPayload(second)
	if err != nil {
		t.Fatalf("decode second payload failed: %v", err)
	}

	if firstCmd.Name != "Execute" || secondCmd.Name != "Execute" {
		t.Fatalf("expected Execute/Execute, got %s/%s", firstCmd.Name, secondCmd.Name)
	}
	if firstCmd.Text != "cmd1\n" || secondCmd.Text != "cmd2\n" {
		t.Fatalf("expected cmd1 then cmd2, got %q then %q", firstCmd.Text, secondCmd.Text)
	}

	transport.close()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("dispatcher did not stop")
	}
}

func TestDispatcher_CloseWindowWaitsForReplySaveChanges(t *testing.T) {
	transport := newMockTransport()
	dispatcher := NewDispatcher(transport, protocol.NewCodec())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		dispatcher.Run(ctx)
		close(done)
	}()

	transport.push(`["SetPromptType",{"type":0}]`)
	waitForCondition(t, 250*time.Millisecond, func() bool {
		promptType, known := dispatcher.PromptType()
		return known && promptType == 0
	})

	if err := dispatcher.SendCommand("SaveChanges", protocol.SaveChangesArgs{
		Win:  88,
		Text: []string{"a←1"},
	}); err != nil {
		t.Fatalf("SendCommand SaveChanges failed: %v", err)
	}

	firstWrite := waitForWrite(t, transport.writeCh, 250*time.Millisecond)
	firstCommand, err := decodeCommandName(firstWrite)
	if err != nil {
		t.Fatalf("failed decoding first write: %v", err)
	}
	if firstCommand != "SaveChanges" {
		t.Fatalf("expected first write SaveChanges, got %q", firstCommand)
	}

	if err := dispatcher.SendCommand("CloseWindow", protocol.WindowArgs{Win: 88}); err != nil {
		t.Fatalf("SendCommand CloseWindow failed: %v", err)
	}

	select {
	case payload := <-transport.writeCh:
		command, err := decodeCommandName(payload)
		if err != nil {
			t.Fatalf("decodeCommandName failed: %v", err)
		}
		t.Fatalf("CloseWindow should wait for ReplySaveChanges, got immediate write %q", command)
	case <-time.After(50 * time.Millisecond):
	}

	transport.push(`["ReplySaveChanges",{"win":88}]`)

	secondWrite := waitForWrite(t, transport.writeCh, 250*time.Millisecond)
	secondCommand, err := decodeCommandName(secondWrite)
	if err != nil {
		t.Fatalf("failed decoding second write: %v", err)
	}
	if secondCommand != "CloseWindow" {
		t.Fatalf("expected CloseWindow after ReplySaveChanges, got %q", secondCommand)
	}

	transport.close()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("dispatcher did not stop")
	}
}

func TestDispatcher_CloseWindowWithoutPendingSaveWritesImmediately(t *testing.T) {
	transport := newMockTransport()
	dispatcher := NewDispatcher(transport, protocol.NewCodec())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		dispatcher.Run(ctx)
		close(done)
	}()

	transport.push(`["SetPromptType",{"type":0}]`)
	waitForCondition(t, 250*time.Millisecond, func() bool {
		promptType, known := dispatcher.PromptType()
		return known && promptType == 0
	})

	if err := dispatcher.SendCommand("CloseWindow", protocol.WindowArgs{Win: 99}); err != nil {
		t.Fatalf("SendCommand CloseWindow failed: %v", err)
	}

	write := waitForWrite(t, transport.writeCh, 250*time.Millisecond)
	command, err := decodeCommandName(write)
	if err != nil {
		t.Fatalf("decodeCommandName failed: %v", err)
	}
	if command != "CloseWindow" {
		t.Fatalf("expected immediate CloseWindow write, got %q", command)
	}

	transport.close()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("dispatcher did not stop")
	}
}

func TestDispatcher_SaveChangesWriteFailureDoesNotBlockCloseWindow(t *testing.T) {
	transport := newMockTransport()
	dispatcher := NewDispatcher(transport, protocol.NewCodec())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		dispatcher.Run(ctx)
		close(done)
	}()

	transport.push(`["SetPromptType",{"type":0}]`)
	waitForCondition(t, 250*time.Millisecond, func() bool {
		promptType, known := dispatcher.PromptType()
		return known && promptType == 0
	})

	transport.failNextWrite(errors.New("save write failed"))
	err := dispatcher.SendCommand("SaveChanges", protocol.SaveChangesArgs{
		Win:  55,
		Text: []string{"x←1"},
	})
	if err == nil {
		t.Fatal("expected SaveChanges write failure")
	}

	if err := dispatcher.SendCommand("CloseWindow", protocol.WindowArgs{Win: 55}); err != nil {
		t.Fatalf("SendCommand CloseWindow failed: %v", err)
	}
	write := waitForWrite(t, transport.writeCh, 250*time.Millisecond)
	command, err := decodeCommandName(write)
	if err != nil {
		t.Fatalf("decodeCommandName failed: %v", err)
	}
	if command != "CloseWindow" {
		t.Fatalf("expected CloseWindow write after SaveChanges failure, got %q", command)
	}

	transport.close()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("dispatcher did not stop")
	}
}

func TestDispatcher_HadErrorCancelsQueuedExecuteCommands(t *testing.T) {
	transport := newMockTransport()
	dispatcher := NewDispatcher(transport, protocol.NewCodec())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		dispatcher.Run(ctx)
		close(done)
	}()

	transport.push(`["SetPromptType",{"type":0}]`)
	waitForCondition(t, 250*time.Millisecond, func() bool {
		promptType, known := dispatcher.PromptType()
		return known && promptType == 0
	})

	if err := dispatcher.SendCommand("Execute", protocol.ExecuteArgs{Text: "cmd1\n", Trace: 0}); err != nil {
		t.Fatalf("queue Execute cmd1 failed: %v", err)
	}
	if err := dispatcher.SendCommand("GetThreads", protocol.GetThreadsArgs{}); err != nil {
		t.Fatalf("queue GetThreads failed: %v", err)
	}
	if err := dispatcher.SendCommand("Execute", protocol.ExecuteArgs{Text: "cmd2\n", Trace: 0}); err != nil {
		t.Fatalf("queue Execute cmd2 failed: %v", err)
	}

	transport.push(`["HadError",{"error":11,"error_text":"DOMAIN ERROR"}]`)
	transport.push(`["SetPromptType",{"type":1}]`)

	write := waitForWrite(t, transport.writeCh, 250*time.Millisecond)
	command, err := decodeCommandName(write)
	if err != nil {
		t.Fatalf("decodeCommandName failed: %v", err)
	}
	if command != "GetThreads" {
		t.Fatalf("expected only non-Execute command to remain queued, got %q", command)
	}

	select {
	case payload := <-transport.writeCh:
		nextCommand, err := decodeCommandName(payload)
		if err != nil {
			t.Fatalf("decodeCommandName failed: %v", err)
		}
		t.Fatalf("expected no additional queued commands, got %q", nextCommand)
	case <-time.After(50 * time.Millisecond):
	}

	transport.close()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("dispatcher did not stop")
	}
}

func TestDispatcher_DisconnectClearsQueuedCommands(t *testing.T) {
	transport := newMockTransport()
	dispatcher := NewDispatcher(transport, protocol.NewCodec())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		dispatcher.Run(ctx)
		close(done)
	}()

	transport.push(`["SetPromptType",{"type":0}]`)
	waitForCondition(t, 250*time.Millisecond, func() bool {
		promptType, known := dispatcher.PromptType()
		return known && promptType == 0
	})

	if err := dispatcher.SendCommand("Execute", protocol.ExecuteArgs{Text: "cmd1\n", Trace: 0}); err != nil {
		t.Fatalf("queue Execute failed: %v", err)
	}
	if err := dispatcher.SendCommand("GetThreads", protocol.GetThreadsArgs{}); err != nil {
		t.Fatalf("queue GetThreads failed: %v", err)
	}

	transport.push(`["Disconnect",{"message":"socket closed"}]`)
	transport.push(`["SetPromptType",{"type":1}]`)

	select {
	case payload := <-transport.writeCh:
		command, err := decodeCommandName(payload)
		if err != nil {
			t.Fatalf("decodeCommandName failed: %v", err)
		}
		t.Fatalf("expected queue to be cleared on Disconnect, got write %q", command)
	case <-time.After(50 * time.Millisecond):
	}

	transport.close()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("dispatcher did not stop")
	}
}

func TestDispatcher_QuoteQuadPromptTypeDoesNotQueueExecute(t *testing.T) {
	transport := newMockTransport()
	dispatcher := NewDispatcher(transport, protocol.NewCodec())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		dispatcher.Run(ctx)
		close(done)
	}()

	transport.push(`["SetPromptType",{"type":4}]`)
	waitForCondition(t, 250*time.Millisecond, func() bool {
		promptType, known := dispatcher.PromptType()
		return known && promptType == 4
	})

	if err := dispatcher.SendCommand("Execute", protocol.ExecuteArgs{Text: "1+1\n", Trace: 0}); err != nil {
		t.Fatalf("SendCommand Execute failed: %v", err)
	}

	write := waitForWrite(t, transport.writeCh, 250*time.Millisecond)
	command, err := decodeCommandName(write)
	if err != nil {
		t.Fatalf("decodeCommandName failed: %v", err)
	}
	if command != "Execute" {
		t.Fatalf("expected immediate Execute in quote-quad mode, got %q", command)
	}

	transport.close()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("dispatcher did not stop")
	}
}

func TestDispatcher_QueuedExecuteFlushesOnPromptTypeTransitionToQuoteQuad(t *testing.T) {
	transport := newMockTransport()
	dispatcher := NewDispatcher(transport, protocol.NewCodec())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		dispatcher.Run(ctx)
		close(done)
	}()

	transport.push(`["SetPromptType",{"type":0}]`)
	waitForCondition(t, 250*time.Millisecond, func() bool {
		promptType, known := dispatcher.PromptType()
		return known && promptType == 0
	})

	if err := dispatcher.SendCommand("Execute", protocol.ExecuteArgs{Text: "queued\n", Trace: 0}); err != nil {
		t.Fatalf("queue Execute failed: %v", err)
	}
	select {
	case payload := <-transport.writeCh:
		command, _ := decodeCommandName(payload)
		t.Fatalf("expected Execute queued while busy, got %q", command)
	case <-time.After(50 * time.Millisecond):
	}

	transport.push(`["SetPromptType",{"type":4}]`)

	write := waitForWrite(t, transport.writeCh, 250*time.Millisecond)
	command, err := decodeCommandName(write)
	if err != nil {
		t.Fatalf("decodeCommandName failed: %v", err)
	}
	if command != "Execute" {
		t.Fatalf("expected queued Execute flush on promptType=4, got %q", command)
	}

	transport.close()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("dispatcher did not stop")
	}
}

func TestDispatcher_PromptTypeModeSemanticsAcrossInterpreterVersions(t *testing.T) {
	t.Skip("Known limitation: promptType 2/5 interpreter-version semantics are not fully specified; live matrix validation tracked in integration tasks")
}

func TestDispatcher_PublishDoesNotPanicWhenSubscriberChannelIsClosed(t *testing.T) {
	dispatcher := NewDispatcher(newMockTransport(), protocol.NewCodec())
	_, _ = dispatcher.Subscribe(1)

	dispatcher.mu.Lock()
	for _, subscriber := range dispatcher.subscribers {
		close(subscriber)
		break
	}
	dispatcher.mu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("publish panicked with closed subscriber: %v", r)
		}
	}()
	dispatcher.publish(protocol.DecodedPayload{Kind: protocol.KindRaw, Raw: "x"})
}

type readResult struct {
	payload string
	err     error
}

type mockTransport struct {
	readCh  chan readResult
	writeCh chan string

	mu           sync.Mutex
	nextWriteErr error
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		readCh:  make(chan readResult, 16),
		writeCh: make(chan string, 16),
	}
}

func (m *mockTransport) push(payload string) {
	m.readCh <- readResult{payload: payload}
}

func (m *mockTransport) close() {
	close(m.readCh)
}

func (m *mockTransport) ReadPayload() (string, error) {
	item, ok := <-m.readCh
	if !ok {
		return "", io.EOF
	}
	return item.payload, item.err
}

func (m *mockTransport) WritePayload(payload string) error {
	m.mu.Lock()
	err := m.nextWriteErr
	m.nextWriteErr = nil
	m.mu.Unlock()
	if err != nil {
		return err
	}
	m.writeCh <- payload
	return nil
}

func (m *mockTransport) failNextWrite(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextWriteErr = err
}

type decodedCommand struct {
	Name string
	Text string
}

func decodeCommandName(payload string) (string, error) {
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(payload), &arr); err != nil {
		return "", err
	}
	if len(arr) < 1 {
		return "", errors.New("payload did not contain command name")
	}
	var command string
	if err := json.Unmarshal(arr[0], &command); err != nil {
		return "", err
	}
	return command, nil
}

func decodeCommandPayload(payload string) (decodedCommand, error) {
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(payload), &arr); err != nil {
		return decodedCommand{}, err
	}
	if len(arr) < 2 {
		return decodedCommand{}, errors.New("payload did not contain command and args")
	}
	var command string
	if err := json.Unmarshal(arr[0], &command); err != nil {
		return decodedCommand{}, err
	}
	var args map[string]any
	if err := json.Unmarshal(arr[1], &args); err != nil {
		return decodedCommand{}, err
	}
	text, _ := args["text"].(string)
	return decodedCommand{Name: command, Text: text}, nil
}

func waitForWrite(t *testing.T, writes <-chan string, timeout time.Duration) string {
	t.Helper()
	select {
	case payload := <-writes:
		return payload
	case <-time.After(timeout):
		t.Fatal("timed out waiting for write")
		return ""
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}
