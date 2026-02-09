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
