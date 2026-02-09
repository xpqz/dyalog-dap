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

func TestHandleRidePayload_DisconnectEmitsTerminatedAndEndsSession(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	events := server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "Disconnect",
		Args: protocol.DisconnectArgs{
			Message: "peer disconnected",
		},
	})
	if len(events) != 2 {
		t.Fatalf("expected output and terminated events, got %#v", events)
	}
	if events[0].Event != "output" {
		t.Fatalf("expected first event output, got %q", events[0].Event)
	}
	output := events[0].Body.(OutputEventBody)
	if output.Category != "stderr" {
		t.Fatalf("expected stderr category, got %q", output.Category)
	}
	if output.Output == "" {
		t.Fatal("expected disconnect output message")
	}
	if events[1].Event != "terminated" {
		t.Fatalf("expected terminated event, got %q", events[1].Event)
	}

	resp, _ := server.HandleRequest(Request{Seq: 42, Command: "configurationDone"})
	if resp.Success {
		t.Fatal("expected session to be terminated after Disconnect")
	}
}

func TestHandleRidePayload_SysErrorEmitsTerminatedAndEndsSession(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	events := server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "SysError",
		Args: protocol.SysErrorArgs{
			Text:  "SYSTEM ERROR",
			Stack: "stacktrace",
		},
	})
	if len(events) != 2 {
		t.Fatalf("expected output and terminated events, got %#v", events)
	}
	if events[0].Event != "output" || events[1].Event != "terminated" {
		t.Fatalf("unexpected event sequence: %#v", events)
	}
	output := events[0].Body.(OutputEventBody)
	if output.Category != "stderr" {
		t.Fatalf("expected stderr category, got %q", output.Category)
	}
	if output.Output == "" {
		t.Fatal("expected SysError output text")
	}

	resp, _ := server.HandleRequest(Request{Seq: 43, Command: "configurationDone"})
	if resp.Success {
		t.Fatal("expected session to be terminated after SysError")
	}
}

func TestHandleRidePayload_InternalErrorEmitsTerminatedAndEndsSession(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	events := server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "InternalError",
		Args: protocol.InternalErrorArgs{
			ErrorText: "internal fault",
			Message:   "panic in interpreter",
		},
	})
	if len(events) != 2 {
		t.Fatalf("expected output and terminated events, got %#v", events)
	}
	if events[0].Event != "output" || events[1].Event != "terminated" {
		t.Fatalf("unexpected event sequence: %#v", events)
	}
	output := events[0].Body.(OutputEventBody)
	if output.Category != "stderr" {
		t.Fatalf("expected stderr category, got %q", output.Category)
	}
	if output.Output == "" {
		t.Fatal("expected InternalError output text")
	}

	resp, _ := server.HandleRequest(Request{Seq: 44, Command: "configurationDone"})
	if resp.Success {
		t.Fatal("expected session to be terminated after InternalError")
	}
}

func TestHandleRidePayload_UnknownCommandEmitsOutputWithoutTermination(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	events := server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "UnknownCommand",
		Args: protocol.UnknownCommandArgs{
			Name: "BogusCommand",
		},
	})
	if len(events) != 1 {
		t.Fatalf("expected single output event, got %#v", events)
	}
	if events[0].Event != "output" {
		t.Fatalf("expected output event, got %q", events[0].Event)
	}
	output := events[0].Body.(OutputEventBody)
	if output.Category != "console" {
		t.Fatalf("expected console category, got %q", output.Category)
	}
	if output.Output == "" {
		t.Fatal("expected unknown command output text")
	}

	resp, _ := server.HandleRequest(Request{Seq: 45, Command: "configurationDone"})
	if !resp.Success {
		t.Fatalf("expected session to remain active after UnknownCommand, got %s", resp.Message)
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

func TestHandleRidePayload_OpenWindowRegistersSourceReferenceAndToken(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    301,
			Filename: "/ws/src/foo.apl",
			Name:     "foo",
		},
	})

	sourceRef, ok := server.ResolveSourceReferenceForToken(301)
	if !ok {
		t.Fatal("expected source reference for token 301")
	}
	if sourceRef <= 0 {
		t.Fatalf("expected positive sourceRef, got %d", sourceRef)
	}

	token, ok := server.ResolveTokenForSourceReference(sourceRef)
	if !ok || token != 301 {
		t.Fatalf("expected sourceRef %d to resolve to token 301, got %d (ok=%v)", sourceRef, token, ok)
	}
}

func TestHandleRidePayload_CloseWindowRemovesTokenMapping(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    302,
			Filename: "/ws/src/bar.apl",
		},
	})
	sourceRef, ok := server.ResolveSourceReferenceForToken(302)
	if !ok {
		t.Fatal("expected source ref for open window")
	}

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "CloseWindow",
		Args: protocol.WindowArgs{
			Win: 302,
		},
	})

	if _, ok := server.ResolveSourceReferenceForToken(302); ok {
		t.Fatal("expected token mapping removed after close")
	}
	if _, ok := server.ResolveTokenForSourceReference(sourceRef); ok {
		t.Fatal("expected sourceRef token mapping removed after close")
	}
}

func TestHandleRidePayload_ReopenSameSourceReusesSourceReference(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    303,
			Filename: "/ws/src/baz.apl",
		},
	})
	firstRef, ok := server.ResolveSourceReferenceForToken(303)
	if !ok {
		t.Fatal("expected first source ref")
	}

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "CloseWindow",
		Args: protocol.WindowArgs{
			Win: 303,
		},
	})
	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    304,
			Filename: "/ws/src/baz.apl",
		},
	})

	secondRef, ok := server.ResolveSourceReferenceForToken(304)
	if !ok {
		t.Fatal("expected second source ref")
	}
	if firstRef != secondRef {
		t.Fatalf("expected source ref reuse across reopen, got %d then %d", firstRef, secondRef)
	}
}

func TestHandleRidePayload_UpdateWindowRebindsTokenToNewSource(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    305,
			Filename: "/ws/src/old.apl",
		},
	})
	oldRef, ok := server.ResolveSourceReferenceForToken(305)
	if !ok {
		t.Fatal("expected old source ref")
	}

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "UpdateWindow",
		Args: protocol.WindowContentArgs{
			Token:    305,
			Filename: "/ws/src/new.apl",
		},
	})

	newRef, ok := server.ResolveSourceReferenceForToken(305)
	if !ok {
		t.Fatal("expected updated source ref")
	}
	if newRef == oldRef {
		t.Fatal("expected updated source ref to differ after path change")
	}
	if _, ok := server.ResolveTokenForSourceReference(oldRef); ok {
		t.Fatal("expected old source ref to be unbound from token")
	}
	token, ok := server.ResolveTokenForSourceReference(newRef)
	if !ok || token != 305 {
		t.Fatalf("expected new sourceRef to map to token 305, got %d (ok=%v)", token, ok)
	}
}

func TestHandleRidePayload_RebindingPathToNewTokenUnbindsOldToken(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    306,
			Filename: "/ws/src/shared.apl",
		},
	})
	ref, ok := server.ResolveSourceReferenceForToken(306)
	if !ok {
		t.Fatal("expected source ref for first token")
	}

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    307,
			Filename: "/ws/src/shared.apl",
		},
	})

	if _, ok := server.ResolveSourceReferenceForToken(306); ok {
		t.Fatal("expected first token to be unbound after rebind")
	}
	token, ok := server.ResolveTokenForSourceReference(ref)
	if !ok || token != 307 {
		t.Fatalf("expected sourceRef to map to token 307, got %d (ok=%v)", token, ok)
	}
}

func TestHandleRequest_SetBreakpointsTranslatesLinesAndSendsSetLineAttributes(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    401,
			Filename: "/ws/src/break.apl",
		},
	})

	resp, _ := server.HandleRequest(Request{
		Seq:     70,
		Command: "setBreakpoints",
		Arguments: map[string]any{
			"source": map[string]any{
				"path": "/ws/src/break.apl",
			},
			"breakpoints": []any{
				map[string]any{"line": 2},
				map[string]any{"line": 5},
			},
		},
	})
	if !resp.Success {
		t.Fatalf("expected setBreakpoints success, got %s", resp.Message)
	}

	call := ride.lastCall()
	if call.command != "SetLineAttributes" {
		t.Fatalf("expected SetLineAttributes, got %q", call.command)
	}
	if call.args["win"] != 401 {
		t.Fatalf("expected win=401, got %#v", call.args["win"])
	}
	stop, ok := call.args["stop"].([]int)
	if !ok {
		t.Fatalf("expected []int stop array, got %T", call.args["stop"])
	}
	if len(stop) != 2 || stop[0] != 1 || stop[1] != 4 {
		t.Fatalf("expected stop=[1 4], got %#v", stop)
	}

	body, ok := resp.Body.(SetBreakpointsResponseBody)
	if !ok {
		t.Fatalf("expected SetBreakpointsResponseBody, got %T", resp.Body)
	}
	if len(body.Breakpoints) != 2 {
		t.Fatalf("expected 2 breakpoint responses, got %d", len(body.Breakpoints))
	}
	if !body.Breakpoints[0].Verified || body.Breakpoints[0].Line != 2 {
		t.Fatalf("unexpected first breakpoint response: %#v", body.Breakpoints[0])
	}
}

func TestHandleRequest_SetBreakpointsSupportsSourceReferenceLookup(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    402,
			Filename: "/ws/src/ref.apl",
		},
	})
	sourceRef, ok := server.ResolveSourceReferenceForToken(402)
	if !ok {
		t.Fatal("expected sourceRef for token")
	}

	resp, _ := server.HandleRequest(Request{
		Seq:     71,
		Command: "setBreakpoints",
		Arguments: map[string]any{
			"source": map[string]any{
				"sourceReference": sourceRef,
			},
			"breakpoints": []any{
				map[string]any{"line": 10},
			},
		},
	})
	if !resp.Success {
		t.Fatalf("expected setBreakpoints success, got %s", resp.Message)
	}
	call := ride.lastCall()
	if call.command != "SetLineAttributes" || call.args["win"] != 402 {
		t.Fatalf("unexpected SetLineAttributes call: %#v", call)
	}
}

func TestHandleRequest_SetBreakpointsUnverifiedWhenSourceNotMapped(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	resp, _ := server.HandleRequest(Request{
		Seq:     72,
		Command: "setBreakpoints",
		Arguments: map[string]any{
			"source": map[string]any{
				"path": "/ws/src/missing.apl",
			},
			"breakpoints": []any{
				map[string]any{"line": 3},
			},
		},
	})
	if !resp.Success {
		t.Fatalf("expected setBreakpoints success with unverified breakpoints, got %s", resp.Message)
	}
	if len(ride.calls) != 1 {
		t.Fatalf("expected one RIDE command send, got %#v", ride.calls)
	}
	if ride.calls[0].command != "GetWindowLayout" {
		t.Fatalf("expected GetWindowLayout sync request, got %q", ride.calls[0].command)
	}

	body := resp.Body.(SetBreakpointsResponseBody)
	if len(body.Breakpoints) != 1 {
		t.Fatalf("expected one breakpoint response, got %d", len(body.Breakpoints))
	}
	if body.Breakpoints[0].Verified {
		t.Fatal("expected unverified breakpoint for unmapped source")
	}
}

func TestHandleRidePayload_OpenWindowAppliesDeferredBreakpoints(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	resp, _ := server.HandleRequest(Request{
		Seq:     73,
		Command: "setBreakpoints",
		Arguments: map[string]any{
			"source": map[string]any{
				"path": "/ws/src/deferred.apl",
			},
			"breakpoints": []any{
				map[string]any{"line": 4},
				map[string]any{"line": 9},
			},
		},
	})
	if !resp.Success {
		t.Fatalf("expected setBreakpoints success, got %s", resp.Message)
	}

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    501,
			Filename: "/ws/src/deferred.apl",
		},
	})

	if len(ride.calls) < 2 {
		t.Fatalf("expected deferred SetLineAttributes call after sync request, got %#v", ride.calls)
	}
	last := ride.lastCall()
	if last.command != "SetLineAttributes" {
		t.Fatalf("expected SetLineAttributes, got %q", last.command)
	}
	if last.args["win"] != 501 {
		t.Fatalf("expected win=501, got %#v", last.args["win"])
	}
	stop := last.args["stop"].([]int)
	if len(stop) != 2 || stop[0] != 3 || stop[1] != 8 {
		t.Fatalf("expected stop=[3 8], got %#v", stop)
	}
}

func TestHandleRequest_SetBreakpointsUnmappedReplacesDeferredLines(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	_, _ = server.HandleRequest(Request{
		Seq:     74,
		Command: "setBreakpoints",
		Arguments: map[string]any{
			"source": map[string]any{
				"path": "/ws/src/replace.apl",
			},
			"breakpoints": []any{
				map[string]any{"line": 2},
			},
		},
	})
	_, _ = server.HandleRequest(Request{
		Seq:     75,
		Command: "setBreakpoints",
		Arguments: map[string]any{
			"source": map[string]any{
				"path": "/ws/src/replace.apl",
			},
			"breakpoints": []any{
				map[string]any{"line": 7},
			},
		},
	})

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    502,
			Filename: "/ws/src/replace.apl",
		},
	})

	last := ride.lastCall()
	if last.command != "SetLineAttributes" {
		t.Fatalf("expected SetLineAttributes, got %q", last.command)
	}
	stop := last.args["stop"].([]int)
	if len(stop) != 1 || stop[0] != 6 {
		t.Fatalf("expected latest deferred stop=[6], got %#v", stop)
	}
}

func TestHandleRidePayload_ReopenSourceAppliesDeferredBySourceReference(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    503,
			Filename: "/ws/src/source-ref.apl",
		},
	})
	sourceRef, ok := server.ResolveSourceReferenceForToken(503)
	if !ok {
		t.Fatal("expected source reference")
	}
	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "CloseWindow",
		Args: protocol.WindowArgs{
			Win: 503,
		},
	})

	_, _ = server.HandleRequest(Request{
		Seq:     76,
		Command: "setBreakpoints",
		Arguments: map[string]any{
			"source": map[string]any{
				"sourceReference": sourceRef,
			},
			"breakpoints": []any{
				map[string]any{"line": 12},
			},
		},
	})

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    504,
			Filename: "/ws/src/source-ref.apl",
		},
	})

	last := ride.lastCall()
	if last.command != "SetLineAttributes" {
		t.Fatalf("expected SetLineAttributes, got %q", last.command)
	}
	if last.args["win"] != 504 {
		t.Fatalf("expected win=504, got %#v", last.args["win"])
	}
	stop := last.args["stop"].([]int)
	if len(stop) != 1 || stop[0] != 11 {
		t.Fatalf("expected stop=[11], got %#v", stop)
	}
}
