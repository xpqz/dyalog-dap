package adapter

import (
	"errors"
	"testing"

	"github.com/stefan/lsp-dap/internal/ride/protocol"
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

func TestHandleRidePayload_OpenWindowDebuggerEmitsStoppedAndUpdatesActiveWindow(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	events := server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    99,
			Debugger: true,
			Tid:      3,
		},
	})

	if len(events) != 1 || events[0].Event != "stopped" {
		t.Fatalf("expected one stopped event, got %#v", events)
	}
	body, ok := events[0].Body.(StoppedEventBody)
	if !ok {
		t.Fatalf("expected StoppedEventBody, got %T", events[0].Body)
	}
	if body.Reason != "entry" {
		t.Fatalf("expected reason=entry, got %q", body.Reason)
	}
	if body.ThreadID != 3 {
		t.Fatalf("expected threadId=3, got %d", body.ThreadID)
	}

	resp, _ := server.HandleRequest(Request{Seq: 40, Command: "continue"})
	if !resp.Success {
		t.Fatalf("expected continue success, got %s", resp.Message)
	}
	if got := ride.lastCall().args["win"]; got != 99 {
		t.Fatalf("expected active win=99, got %#v", got)
	}
}

func TestHandleRidePayload_SetHighlightLineEmitsStepStoppedAndSelectsThread(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    10,
			Debugger: true,
			Tid:      1,
		},
	})
	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    20,
			Debugger: true,
			Tid:      2,
		},
	})

	events := server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "SetHighlightLine",
		Args: protocol.SetHighlightLineArgs{
			Win: 20,
		},
	})
	if len(events) != 1 || events[0].Event != "stopped" {
		t.Fatalf("expected one stopped event, got %#v", events)
	}
	body := events[0].Body.(StoppedEventBody)
	if body.Reason != "step" {
		t.Fatalf("expected reason=step, got %q", body.Reason)
	}
	if body.ThreadID != 2 {
		t.Fatalf("expected threadId=2, got %d", body.ThreadID)
	}

	resp, _ := server.HandleRequest(Request{Seq: 41, Command: "next"})
	if !resp.Success {
		t.Fatalf("expected next success, got %s", resp.Message)
	}
	if got := ride.lastCall().args["win"]; got != 20 {
		t.Fatalf("expected active win=20, got %#v", got)
	}
}

func TestHandleRidePayload_HadErrorEmitsExceptionStopped(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "SetThread",
		Args: protocol.SetThreadArgs{
			Tid: 7,
		},
	})

	events := server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "HadError",
		Args:    protocol.HadErrorArgs{Error: 11, ErrorText: "DOMAIN ERROR"},
	})
	if len(events) != 1 || events[0].Event != "stopped" {
		t.Fatalf("expected one stopped event, got %#v", events)
	}
	body := events[0].Body.(StoppedEventBody)
	if body.Reason != "exception" {
		t.Fatalf("expected reason=exception, got %q", body.Reason)
	}
	if body.ThreadID != 7 {
		t.Fatalf("expected threadId=7, got %d", body.ThreadID)
	}
}

func TestHandleRidePayload_OpenWindowNonDebuggerDoesNotEmitStopped(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	events := server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    5,
			Debugger: false,
		},
	})
	if len(events) != 0 {
		t.Fatalf("expected no events, got %#v", events)
	}
}

func TestHandleRequest_ThreadsPollsGetThreadsAndReturnsCachedThreads(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "ReplyGetThreads",
		Args: protocol.ReplyGetThreadsArgs{
			Threads: []protocol.ThreadInfo{
				{Tid: 1, Description: "Main"},
				{Tid: 2, Description: "Worker"},
			},
		},
	})

	resp, _ := server.HandleRequest(Request{Seq: 50, Command: "threads"})
	if !resp.Success {
		t.Fatalf("expected threads success, got %s", resp.Message)
	}

	call := ride.lastCall()
	if call.command != "GetThreads" {
		t.Fatalf("expected GetThreads poll, got %q", call.command)
	}

	body, ok := resp.Body.(ThreadsResponseBody)
	if !ok {
		t.Fatalf("expected ThreadsResponseBody, got %T", resp.Body)
	}
	if len(body.Threads) != 2 {
		t.Fatalf("expected 2 threads, got %d", len(body.Threads))
	}
	if body.Threads[0].ID != 1 || body.Threads[0].Name != "Main" {
		t.Fatalf("unexpected first thread: %#v", body.Threads[0])
	}
	if body.Threads[1].ID != 2 || body.Threads[1].Name != "Worker" {
		t.Fatalf("unexpected second thread: %#v", body.Threads[1])
	}
}

func TestHandleRidePayload_ReplyGetThreadsMaintainsStableIDsAcrossUpdates(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "ReplyGetThreads",
		Args: protocol.ReplyGetThreadsArgs{
			Threads: []protocol.ThreadInfo{
				{Tid: 9, Description: "Thread A"},
			},
		},
	})

	resp, _ := server.HandleRequest(Request{Seq: 51, Command: "threads"})
	if !resp.Success {
		t.Fatalf("expected threads success, got %s", resp.Message)
	}
	first := resp.Body.(ThreadsResponseBody).Threads[0]
	if first.ID != 9 {
		t.Fatalf("expected first ID=9, got %d", first.ID)
	}

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "ReplyGetThreads",
		Args: protocol.ReplyGetThreadsArgs{
			Threads: []protocol.ThreadInfo{
				{Tid: 9, Description: "Thread A (running)"},
			},
		},
	})

	resp, _ = server.HandleRequest(Request{Seq: 52, Command: "threads"})
	if !resp.Success {
		t.Fatalf("expected threads success, got %s", resp.Message)
	}
	second := resp.Body.(ThreadsResponseBody).Threads[0]
	if second.ID != 9 {
		t.Fatalf("expected stable ID=9, got %d", second.ID)
	}
	if second.Name != "Thread A (running)" {
		t.Fatalf("expected updated name, got %q", second.Name)
	}
}

func TestHandleRequest_ThreadsWithoutRideControllerFails(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	resp, _ := server.HandleRequest(Request{Seq: 53, Command: "threads"})
	if resp.Success {
		t.Fatal("expected threads request without controller to fail")
	}
}

func TestHandleRequest_StackTraceBuildsFramesFromTracerWindows(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:         101,
			Debugger:      true,
			Tid:           2,
			Name:          "Caller",
			CurrentRow:    5,
			CurrentColumn: 1,
		},
	})
	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:         102,
			Debugger:      true,
			Tid:           2,
			Name:          "Top",
			CurrentRow:    6,
			CurrentColumn: 2,
		},
	})
	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "SetHighlightLine",
		Args: protocol.SetHighlightLineArgs{
			Win:      102,
			Line:     7,
			StartCol: 3,
		},
	})

	resp, _ := server.HandleRequest(Request{
		Seq:     60,
		Command: "stackTrace",
		Arguments: map[string]any{
			"threadId": 2,
		},
	})
	if !resp.Success {
		t.Fatalf("expected stackTrace success, got %s", resp.Message)
	}

	body, ok := resp.Body.(StackTraceResponseBody)
	if !ok {
		t.Fatalf("expected StackTraceResponseBody, got %T", resp.Body)
	}
	if len(body.StackFrames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(body.StackFrames))
	}
	if body.StackFrames[0].ID != 102 || body.StackFrames[0].Name != "Top" {
		t.Fatalf("unexpected top frame: %#v", body.StackFrames[0])
	}
	if body.StackFrames[0].Line != 8 || body.StackFrames[0].Column != 4 {
		t.Fatalf("unexpected top frame location: %#v", body.StackFrames[0])
	}
	if body.StackFrames[1].ID != 101 || body.StackFrames[1].Name != "Caller" {
		t.Fatalf("unexpected caller frame: %#v", body.StackFrames[1])
	}
}

func TestHandleRequest_StackTraceUsesReplyGetSIStackDescriptions(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:         201,
			Debugger:      true,
			Tid:           4,
			Name:          "OrigA",
			CurrentRow:    1,
			CurrentColumn: 1,
		},
	})
	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:         202,
			Debugger:      true,
			Tid:           4,
			Name:          "OrigB",
			CurrentRow:    2,
			CurrentColumn: 1,
		},
	})
	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "ReplyGetSIStack",
		Args: protocol.ReplyGetSIStackArgs{
			Tid: 4,
			Stack: []protocol.SIStackEntry{
				{Description: "TopFn"},
				{Description: "CallerFn"},
			},
		},
	})

	resp, _ := server.HandleRequest(Request{
		Seq:     61,
		Command: "stackTrace",
		Arguments: map[string]any{
			"threadId": 4,
		},
	})
	if !resp.Success {
		t.Fatalf("expected stackTrace success, got %s", resp.Message)
	}
	body := resp.Body.(StackTraceResponseBody)
	if len(body.StackFrames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(body.StackFrames))
	}
	if body.StackFrames[0].Name != "TopFn" || body.StackFrames[1].Name != "CallerFn" {
		t.Fatalf("expected SI-enriched names, got %#v", body.StackFrames)
	}
}

func TestHandleRequest_StackTraceWithoutThreadIDFails(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	resp, _ := server.HandleRequest(Request{Seq: 62, Command: "stackTrace"})
	if resp.Success {
		t.Fatal("expected stackTrace without threadId to fail")
	}
}
