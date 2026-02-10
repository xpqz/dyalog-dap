package adapter

import (
	"testing"
	"time"

	"github.com/stefan/lsp-dap/internal/ride/protocol"
)

func TestHandleRidePayload_OpenWindowDeferredApplyExecutesSendOutsideLock(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	server.pendingByPath["/ws/src/deferred-lock-check.apl"] = []int{7}

	sendObserved := make(chan struct{}, 1)
	ride.onSend = func(command string, _ map[string]any) {
		if command != "SetLineAttributes" {
			return
		}
		// This takes the server mutex. If SendCommand is called while the mutex is held,
		// this callback would deadlock.
		_ = server.CanLaunchOrAttach()
		sendObserved <- struct{}{}
	}

	done := make(chan []Event, 1)
	go func() {
		done <- server.HandleRidePayload(protocol.DecodedPayload{
			Kind:    protocol.KindCommand,
			Command: "OpenWindow",
			Args: protocol.WindowContentArgs{
				Token:    702,
				Filename: "/ws/src/deferred-lock-check.apl",
			},
		})
	}()

	select {
	case events := <-done:
		if len(events) != 1 || events[0].Event != "output" {
			t.Fatalf("expected one deferred apply output event, got %#v", events)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HandleRidePayload deadlocked while applying deferred breakpoints")
	}

	select {
	case <-sendObserved:
	case <-time.After(2 * time.Second):
		t.Fatal("expected SetLineAttributes send callback to run")
	}
}
