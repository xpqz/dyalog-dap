package adapter

import (
	"errors"
	"testing"
)

func TestHandleRequest_InitializeReturnsCapabilitiesAndInitializedEvent(t *testing.T) {
	server := NewServer()

	resp, events := server.HandleRequest(Request{Seq: 1, Command: "initialize"})
	if !resp.Success {
		t.Fatalf("expected initialize success, got failure: %s", resp.Message)
	}
	if resp.Command != "initialize" {
		t.Fatalf("unexpected command in response: %q", resp.Command)
	}
	body, ok := resp.Body.(Capabilities)
	if !ok {
		t.Fatalf("expected Capabilities body, got %T", resp.Body)
	}
	if !body.SupportsConfigurationDoneRequest {
		t.Fatal("expected supportsConfigurationDoneRequest=true")
	}
	if len(events) != 1 || events[0].Event != "initialized" {
		t.Fatalf("expected single initialized event, got %#v", events)
	}
}

func TestHandleRequest_InitializeTwiceFails(t *testing.T) {
	server := NewServer()
	_, _ = server.HandleRequest(Request{Seq: 1, Command: "initialize"})

	resp, _ := server.HandleRequest(Request{Seq: 2, Command: "initialize"})
	if resp.Success {
		t.Fatal("expected second initialize to fail")
	}
}

func TestHandleRequest_LaunchRequiresInitialize(t *testing.T) {
	server := NewServer()
	resp, _ := server.HandleRequest(Request{Seq: 1, Command: "launch"})
	if resp.Success {
		t.Fatal("expected launch before initialize to fail")
	}
}

func TestHandleRequest_ConfigurationDoneRequiresLaunchOrAttach(t *testing.T) {
	server := NewServer()
	_, _ = server.HandleRequest(Request{Seq: 1, Command: "initialize"})

	resp, _ := server.HandleRequest(Request{Seq: 2, Command: "configurationDone"})
	if resp.Success {
		t.Fatal("expected configurationDone before launch/attach to fail")
	}

	resp, _ = server.HandleRequest(Request{Seq: 3, Command: "attach"})
	if !resp.Success {
		t.Fatalf("expected attach to succeed, got: %s", resp.Message)
	}

	resp, _ = server.HandleRequest(Request{Seq: 4, Command: "configurationDone"})
	if !resp.Success {
		t.Fatalf("expected configurationDone to succeed after attach, got: %s", resp.Message)
	}
}

func TestHandleRequest_DisconnectTerminatesSession(t *testing.T) {
	server := NewServer()
	_, _ = server.HandleRequest(Request{Seq: 1, Command: "initialize"})
	_, _ = server.HandleRequest(Request{Seq: 2, Command: "launch"})

	resp, _ := server.HandleRequest(Request{Seq: 3, Command: "disconnect"})
	if !resp.Success {
		t.Fatalf("expected disconnect success, got: %s", resp.Message)
	}

	resp, _ = server.HandleRequest(Request{Seq: 4, Command: "configurationDone"})
	if resp.Success {
		t.Fatal("expected requests after disconnect to fail")
	}
}

func TestHandleRequest_UnsupportedCommandFails(t *testing.T) {
	server := NewServer()
	resp, _ := server.HandleRequest(Request{Seq: 99, Command: "frobnicate"})
	if resp.Success {
		t.Fatal("expected unsupported command to fail")
	}
}

func TestHandleRequest_ContinueMapsToRideContinue(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	server.SetActiveTracerWindow(42)
	enterRunningState(t, server)

	resp, _ := server.HandleRequest(Request{Seq: 10, Command: "continue"})
	if !resp.Success {
		t.Fatalf("expected continue success, got %s", resp.Message)
	}

	call := ride.lastCall()
	if call.command != "Continue" {
		t.Fatalf("expected Continue, got %q", call.command)
	}
	if got := call.args["win"]; got != 42 {
		t.Fatalf("expected win=42, got %#v", got)
	}
}

func TestHandleRequest_StepCommandsMapToRideCommands(t *testing.T) {
	cases := []struct {
		dapCommand  string
		rideCommand string
	}{
		{dapCommand: "next", rideCommand: "RunCurrentLine"},
		{dapCommand: "stepIn", rideCommand: "StepInto"},
		{dapCommand: "stepOut", rideCommand: "ContinueTrace"},
	}

	for _, tc := range cases {
		t.Run(tc.dapCommand, func(t *testing.T) {
			ride := &mockRideController{}
			server := NewServer()
			server.SetRideController(ride)
			server.SetActiveTracerWindow(7)
			enterRunningState(t, server)

			resp, _ := server.HandleRequest(Request{Seq: 10, Command: tc.dapCommand})
			if !resp.Success {
				t.Fatalf("expected %s success, got %s", tc.dapCommand, resp.Message)
			}
			call := ride.lastCall()
			if call.command != tc.rideCommand {
				t.Fatalf("expected %s, got %q", tc.rideCommand, call.command)
			}
			if got := call.args["win"]; got != 7 {
				t.Fatalf("expected win=7, got %#v", got)
			}
		})
	}
}

func TestHandleRequest_PauseUsesWeakInterrupt(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	resp, _ := server.HandleRequest(Request{Seq: 11, Command: "pause"})
	if !resp.Success {
		t.Fatalf("expected pause success, got %s", resp.Message)
	}

	call := ride.lastCall()
	if call.command != "WeakInterrupt" {
		t.Fatalf("expected WeakInterrupt, got %q", call.command)
	}
	if len(call.args) != 0 {
		t.Fatalf("expected no args for WeakInterrupt, got %#v", call.args)
	}
}

func TestHandleRequest_PauseWeakInterruptFailureUsesFallbackHook(t *testing.T) {
	ride := &mockRideController{
		sendErr: errors.New("weak failed"),
	}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	fallbackCalls := 0
	server.SetPauseFallback(func() error {
		fallbackCalls++
		return nil
	})

	resp, _ := server.HandleRequest(Request{Seq: 12, Command: "pause"})
	if !resp.Success {
		t.Fatalf("expected pause success with fallback, got %s", resp.Message)
	}
	if fallbackCalls != 1 {
		t.Fatalf("expected fallback to be called once, got %d", fallbackCalls)
	}
}

func TestHandleRequest_ControlCommandsRequireActiveTracerWindow(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	resp, _ := server.HandleRequest(Request{Seq: 13, Command: "continue"})
	if resp.Success {
		t.Fatal("expected continue without active tracer window to fail")
	}
}

func enterRunningState(t *testing.T, server *Server) {
	t.Helper()
	resp, _ := server.HandleRequest(Request{Seq: 1, Command: "initialize"})
	if !resp.Success {
		t.Fatalf("initialize failed: %s", resp.Message)
	}
	resp, _ = server.HandleRequest(Request{Seq: 2, Command: "launch"})
	if !resp.Success {
		t.Fatalf("launch failed: %s", resp.Message)
	}
}

type rideCall struct {
	command string
	args    map[string]any
}

type mockRideController struct {
	calls   []rideCall
	sendErr error
}

func (m *mockRideController) SendCommand(command string, args any) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	typedArgs := map[string]any{}
	if args != nil {
		typedArgs = args.(map[string]any)
	}
	m.calls = append(m.calls, rideCall{
		command: command,
		args:    typedArgs,
	})
	return nil
}

func (m *mockRideController) lastCall() rideCall {
	if len(m.calls) == 0 {
		return rideCall{}
	}
	return m.calls[len(m.calls)-1]
}
