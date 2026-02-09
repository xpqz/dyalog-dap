package sessionstate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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

type readResult struct {
	payload string
	err     error
}

type mockTransport struct {
	readCh  chan readResult
	writeCh chan string
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
	m.writeCh <- payload
	return nil
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
