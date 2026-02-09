package adapter

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	body, ok := resp.Body.(PauseResponseBody)
	if !ok {
		t.Fatalf("expected PauseResponseBody, got %T", resp.Body)
	}
	if body.InterruptMethod != "weak" {
		t.Fatalf("expected weak interrupt method, got %q", body.InterruptMethod)
	}

	call := ride.lastCall()
	if call.command != "WeakInterrupt" {
		t.Fatalf("expected WeakInterrupt, got %q", call.command)
	}
	if len(call.args) != 0 {
		t.Fatalf("expected no args for WeakInterrupt, got %#v", call.args)
	}
}

func TestHandleRequest_PauseWeakInterruptFailureUsesStrongInterrupt(t *testing.T) {
	ride := &mockRideController{
		sendErrByCommand: map[string]error{
			"WeakInterrupt": errors.New("weak failed"),
		},
	}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	resp, _ := server.HandleRequest(Request{Seq: 12, Command: "pause"})
	if !resp.Success {
		t.Fatalf("expected pause success via strong interrupt, got %s", resp.Message)
	}
	body := resp.Body.(PauseResponseBody)
	if body.InterruptMethod != "strong" {
		t.Fatalf("expected strong interrupt method, got %q", body.InterruptMethod)
	}
	if len(ride.calls) != 2 {
		t.Fatalf("expected weak then strong attempts, got %#v", ride.calls)
	}
	if ride.calls[0].command != "WeakInterrupt" || ride.calls[1].command != "StrongInterrupt" {
		t.Fatalf("unexpected interrupt command sequence: %#v", ride.calls)
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
	body := resp.Body.(PauseResponseBody)
	if body.InterruptMethod != "fallback" {
		t.Fatalf("expected fallback interrupt method, got %q", body.InterruptMethod)
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
	calls            []rideCall
	sendErr          error
	sendErrByCommand map[string]error
	onSend           func(command string, args map[string]any)
}

func (m *mockRideController) SendCommand(command string, args any) error {
	typedArgs := map[string]any{}
	if args != nil {
		typedArgs = args.(map[string]any)
	}
	m.calls = append(m.calls, rideCall{
		command: command,
		args:    typedArgs,
	})
	if m.onSend != nil {
		m.onSend(command, typedArgs)
	}

	if m.sendErrByCommand != nil {
		if err, ok := m.sendErrByCommand[command]; ok {
			return err
		}
	}
	if m.sendErr != nil {
		return m.sendErr
	}
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

func TestHandleRideReconnect_RequestsLayoutAndResetsActiveState(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)
	server.SetActiveTracerWindow(77)

	events := server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "Disconnect",
		Args: protocol.DisconnectArgs{
			Message: "network drop",
		},
	})
	if len(events) == 0 || events[len(events)-1].Event != "terminated" {
		t.Fatalf("expected disconnect to terminate, got %#v", events)
	}

	resp, _ := server.HandleRequest(Request{Seq: 46, Command: "configurationDone"})
	if resp.Success {
		t.Fatal("expected terminated session before reconnect")
	}

	reconnectEvents := server.HandleRideReconnect()
	if len(reconnectEvents) != 1 || reconnectEvents[0].Event != "output" {
		t.Fatalf("expected reconnect output event, got %#v", reconnectEvents)
	}
	if ride.lastCall().command != "GetWindowLayout" {
		t.Fatalf("expected GetWindowLayout after reconnect, got %q", ride.lastCall().command)
	}

	resp, _ = server.HandleRequest(Request{Seq: 47, Command: "configurationDone"})
	if !resp.Success {
		t.Fatalf("expected session to recover after reconnect, got %s", resp.Message)
	}

	resp, _ = server.HandleRequest(Request{Seq: 48, Command: "continue"})
	if resp.Success {
		t.Fatal("expected active tracer window to be reset on reconnect")
	}
}

func TestHandleRideReconnect_RebuildsSourceMappingFromLayoutEvents(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    701,
			Filename: "/ws/src/reconnect.apl",
		},
	})
	originalRef, ok := server.ResolveSourceReferenceForToken(701)
	if !ok {
		t.Fatal("expected sourceRef for initial window")
	}

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "Disconnect",
		Args: protocol.DisconnectArgs{
			Message: "network drop",
		},
	})
	_ = server.HandleRideReconnect()

	if _, ok := server.ResolveSourceReferenceForToken(701); ok {
		t.Fatal("expected old token mapping cleared on reconnect")
	}

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    702,
			Filename: "/ws/src/reconnect.apl",
		},
	})
	rebuiltRef, ok := server.ResolveSourceReferenceForToken(702)
	if !ok {
		t.Fatal("expected rebuilt mapping after reconnect")
	}
	if rebuiltRef != originalRef {
		t.Fatalf("expected stable sourceRef across reconnect, got %d (want %d)", rebuiltRef, originalRef)
	}

	resp, _ := server.HandleRequest(Request{
		Seq:     49,
		Command: "setBreakpoints",
		Arguments: map[string]any{
			"source": map[string]any{
				"path": "/ws/src/reconnect.apl",
			},
			"breakpoints": []any{
				map[string]any{"line": 6},
			},
		},
	})
	if !resp.Success {
		t.Fatalf("expected setBreakpoints success after reconnect rebuild, got %s", resp.Message)
	}
	if ride.lastCall().command != "SetLineAttributes" || ride.lastCall().args["win"] != 702 {
		t.Fatalf("expected SetLineAttributes to rebuilt token, got %#v", ride.lastCall())
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

func TestHandleRequest_ScopesRequiresFrameID(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	resp, _ := server.HandleRequest(Request{Seq: 63, Command: "scopes"})
	if resp.Success {
		t.Fatal("expected scopes without frameId to fail")
	}
}

func TestHandleRequest_ScopesAndVariablesExposeFrameContext(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "ReplyGetThreads",
		Args: protocol.ReplyGetThreadsArgs{
			Threads: []protocol.ThreadInfo{
				{Tid: 8, Description: "Main"},
			},
		},
	})
	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:         210,
			Debugger:      true,
			Tid:           8,
			Name:          "TopFn",
			Filename:      "/ws/src/top.apl",
			CurrentRow:    11,
			CurrentColumn: 2,
		},
	})
	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "ReplyGetSIStack",
		Args: protocol.ReplyGetSIStackArgs{
			Tid: 8,
			Stack: []protocol.SIStackEntry{
				{Description: "TopFn"},
				{Description: "CallerFn"},
			},
		},
	})

	scopesResp, _ := server.HandleRequest(Request{
		Seq:     64,
		Command: "scopes",
		Arguments: map[string]any{
			"frameId": 210,
		},
	})
	if !scopesResp.Success {
		t.Fatalf("expected scopes success, got %s", scopesResp.Message)
	}
	scopesBody, ok := scopesResp.Body.(ScopesResponseBody)
	if !ok {
		t.Fatalf("expected ScopesResponseBody, got %T", scopesResp.Body)
	}
	if len(scopesBody.Scopes) != 1 {
		t.Fatalf("expected one scope, got %d", len(scopesBody.Scopes))
	}
	if scopesBody.Scopes[0].Name != "Locals" {
		t.Fatalf("unexpected scope name: %#v", scopesBody.Scopes[0])
	}
	if scopesBody.Scopes[0].VariablesReference <= 0 {
		t.Fatalf("expected scope variablesReference > 0, got %d", scopesBody.Scopes[0].VariablesReference)
	}
	firstScopeRef := scopesBody.Scopes[0].VariablesReference

	scopesResp2, _ := server.HandleRequest(Request{
		Seq:     65,
		Command: "scopes",
		Arguments: map[string]any{
			"frameId": 210,
		},
	})
	if !scopesResp2.Success {
		t.Fatalf("expected second scopes success, got %s", scopesResp2.Message)
	}
	scopesBody2 := scopesResp2.Body.(ScopesResponseBody)
	if scopesBody2.Scopes[0].VariablesReference != firstScopeRef {
		t.Fatalf("expected stable variablesReference %d, got %d", firstScopeRef, scopesBody2.Scopes[0].VariablesReference)
	}

	varsResp, _ := server.HandleRequest(Request{
		Seq:     66,
		Command: "variables",
		Arguments: map[string]any{
			"variablesReference": firstScopeRef,
		},
	})
	if !varsResp.Success {
		t.Fatalf("expected variables success, got %s", varsResp.Message)
	}
	varsBody, ok := varsResp.Body.(VariablesResponseBody)
	if !ok {
		t.Fatalf("expected VariablesResponseBody, got %T", varsResp.Body)
	}

	var (
		foundLine       bool
		sourceRef       int
		siStackRef      int
		foundThreadName bool
	)
	for _, variable := range varsBody.Variables {
		switch variable.Name {
		case "line":
			foundLine = variable.Value == "12" && variable.VariablesReference == 0
		case "source":
			sourceRef = variable.VariablesReference
		case "siStack":
			siStackRef = variable.VariablesReference
		case "threadName":
			foundThreadName = variable.Value == "Main"
		}
	}
	if !foundLine {
		t.Fatalf("expected scalar line variable in locals scope, got %#v", varsBody.Variables)
	}
	if sourceRef <= 0 {
		t.Fatalf("expected object source variable reference, got %d", sourceRef)
	}
	if siStackRef <= 0 {
		t.Fatalf("expected array siStack variable reference, got %d", siStackRef)
	}
	if !foundThreadName {
		t.Fatalf("expected threadName scalar variable, got %#v", varsBody.Variables)
	}

	sourceVarsResp, _ := server.HandleRequest(Request{
		Seq:     67,
		Command: "variables",
		Arguments: map[string]any{
			"variablesReference": sourceRef,
		},
	})
	if !sourceVarsResp.Success {
		t.Fatalf("expected source variables success, got %s", sourceVarsResp.Message)
	}
	sourceVarsBody := sourceVarsResp.Body.(VariablesResponseBody)
	if len(sourceVarsBody.Variables) == 0 {
		t.Fatal("expected source object children")
	}
	var foundPath bool
	for _, variable := range sourceVarsBody.Variables {
		if variable.Name == "path" && variable.Value == "/ws/src/top.apl" {
			foundPath = true
		}
		if variable.VariablesReference != 0 {
			t.Fatalf("expected scalar source child, got %#v", variable)
		}
	}
	if !foundPath {
		t.Fatalf("expected source.path variable, got %#v", sourceVarsBody.Variables)
	}

	siVarsResp, _ := server.HandleRequest(Request{
		Seq:     68,
		Command: "variables",
		Arguments: map[string]any{
			"variablesReference": siStackRef,
		},
	})
	if !siVarsResp.Success {
		t.Fatalf("expected siStack variables success, got %s", siVarsResp.Message)
	}
	siVarsBody := siVarsResp.Body.(VariablesResponseBody)
	if len(siVarsBody.Variables) != 2 {
		t.Fatalf("expected two siStack entries, got %#v", siVarsBody.Variables)
	}
	if siVarsBody.Variables[0].Name != "[0]" || siVarsBody.Variables[0].Value != "TopFn" {
		t.Fatalf("unexpected first siStack entry: %#v", siVarsBody.Variables[0])
	}
	if siVarsBody.Variables[1].Name != "[1]" || siVarsBody.Variables[1].Value != "CallerFn" {
		t.Fatalf("unexpected second siStack entry: %#v", siVarsBody.Variables[1])
	}
}

func TestHandleRequest_VariablesUnknownReferenceFails(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	resp, _ := server.HandleRequest(Request{
		Seq:     69,
		Command: "variables",
		Arguments: map[string]any{
			"variablesReference": 9999,
		},
	})
	if resp.Success {
		t.Fatal("expected unknown variablesReference to fail")
	}
}

func TestHandleRequest_VariablesEmptyArrayReturnsEmptySlice(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:         211,
			Debugger:      true,
			Tid:           1,
			Name:          "TopFn",
			CurrentRow:    3,
			CurrentColumn: 0,
		},
	})

	scopesResp, _ := server.HandleRequest(Request{
		Seq:     69,
		Command: "scopes",
		Arguments: map[string]any{
			"frameId": 211,
		},
	})
	if !scopesResp.Success {
		t.Fatalf("expected scopes success, got %s", scopesResp.Message)
	}
	scopesBody := scopesResp.Body.(ScopesResponseBody)
	scopeRef := scopesBody.Scopes[0].VariablesReference

	varsResp, _ := server.HandleRequest(Request{
		Seq:     70,
		Command: "variables",
		Arguments: map[string]any{
			"variablesReference": scopeRef,
		},
	})
	if !varsResp.Success {
		t.Fatalf("expected variables success, got %s", varsResp.Message)
	}
	varsBody := varsResp.Body.(VariablesResponseBody)

	siRef := 0
	for _, variable := range varsBody.Variables {
		if variable.Name == "siStack" {
			siRef = variable.VariablesReference
			break
		}
	}
	if siRef <= 0 {
		t.Fatalf("expected siStack child reference, got %#v", varsBody.Variables)
	}

	siResp, _ := server.HandleRequest(Request{
		Seq:     71,
		Command: "variables",
		Arguments: map[string]any{
			"variablesReference": siRef,
		},
	})
	if !siResp.Success {
		t.Fatalf("expected empty siStack variables request to succeed, got %s", siResp.Message)
	}
	siBody := siResp.Body.(VariablesResponseBody)
	if siBody.Variables == nil {
		t.Fatal("expected empty variables slice, got nil")
	}
	if len(siBody.Variables) != 0 {
		t.Fatalf("expected zero siStack children, got %#v", siBody.Variables)
	}
}

func TestHandleRequest_ScopesAndVariablesExposeLocalsAndGlobalsCategories(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	ride := &mockRideController{
		onSend: func(command string, args map[string]any) {
			if command != "GetValueTip" {
				return
			}
			name := args["line"].(string)
			token := args["token"]
			reply := []any{"0"}
			switch name {
			case "a":
				reply = []any{"1"}
			case "b":
				reply = []any{"2 3 4"}
			case "g":
				reply = []any{"99"}
			}
			server.HandleRidePayload(protocol.DecodedPayload{
				Kind:    protocol.KindCommand,
				Command: "ValueTip",
				Args: map[string]any{
					"tip":   reply,
					"class": 2,
					"token": token,
				},
			})
		},
	}
	server.SetRideController(ride)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:         512,
			Debugger:      true,
			Tid:           4,
			Name:          "TopFn",
			Filename:      "/ws/src/top.apl",
			CurrentRow:    3,
			CurrentColumn: 2,
			Text: []string{
				"TopFn;a;b",
				"a←1",
				"g←a+b",
			},
		},
	})

	scopesResp, _ := server.HandleRequest(Request{
		Seq:     200,
		Command: "scopes",
		Arguments: map[string]any{
			"frameId": 512,
		},
	})
	if !scopesResp.Success {
		t.Fatalf("expected scopes success, got %s", scopesResp.Message)
	}
	scopesBody := scopesResp.Body.(ScopesResponseBody)
	scopeRef := scopesBody.Scopes[0].VariablesReference

	varsResp, _ := server.HandleRequest(Request{
		Seq:     201,
		Command: "variables",
		Arguments: map[string]any{
			"variablesReference": scopeRef,
		},
	})
	if !varsResp.Success {
		t.Fatalf("expected variables success, got %s", varsResp.Message)
	}
	body := varsResp.Body.(VariablesResponseBody)
	localsRef := 0
	globalsRef := 0
	for _, variable := range body.Variables {
		if variable.Name == "locals" {
			localsRef = variable.VariablesReference
		}
		if variable.Name == "globals" {
			globalsRef = variable.VariablesReference
		}
	}
	if localsRef <= 0 {
		t.Fatalf("expected locals category variable, got %#v", body.Variables)
	}
	if globalsRef <= 0 {
		t.Fatalf("expected globals category variable, got %#v", body.Variables)
	}

	localsResp, _ := server.HandleRequest(Request{
		Seq:     202,
		Command: "variables",
		Arguments: map[string]any{
			"variablesReference": localsRef,
		},
	})
	if !localsResp.Success {
		t.Fatalf("expected locals variables success, got %s", localsResp.Message)
	}
	localsBody := localsResp.Body.(VariablesResponseBody)
	if len(localsBody.Variables) == 0 {
		t.Fatal("expected local symbols")
	}
	localValues := map[string]string{}
	for _, variable := range localsBody.Variables {
		localValues[variable.Name] = variable.Value
	}
	if localValues["a"] != "1" {
		t.Fatalf("expected a=1, got %#v", localsBody.Variables)
	}
	if localValues["b"] != "2 3 4" {
		t.Fatalf("expected b=2 3 4, got %#v", localsBody.Variables)
	}

	globalsResp, _ := server.HandleRequest(Request{
		Seq:     203,
		Command: "variables",
		Arguments: map[string]any{
			"variablesReference": globalsRef,
		},
	})
	if !globalsResp.Success {
		t.Fatalf("expected globals variables success, got %s", globalsResp.Message)
	}
	globalsBody := globalsResp.Body.(VariablesResponseBody)
	if len(globalsBody.Variables) != 1 || globalsBody.Variables[0].Name != "g" || globalsBody.Variables[0].Value != "99" {
		t.Fatalf("expected one global g=99, got %#v", globalsBody.Variables)
	}
}

func TestHandleRequest_LocalVariableLongValueIsExpandableAndStableAcrossRefresh(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	longLines := make([]string, 0, maxLocalValueChildren+8)
	for i := 0; i < maxLocalValueChildren+8; i++ {
		longLines = append(longLines, strings.Repeat("1234567890", 4))
	}
	longValue := strings.Join(longLines, "\n")

	ride := &mockRideController{
		onSend: func(command string, args map[string]any) {
			if command != "GetValueTip" {
				return
			}
			if args["line"] != "a" {
				return
			}
			server.HandleRidePayload(protocol.DecodedPayload{
				Kind:    protocol.KindCommand,
				Command: "ValueTip",
				Args: map[string]any{
					"tip":   strings.Split(longValue, "\n"),
					"class": 2,
					"token": args["token"],
				},
			})
		},
	}
	server.SetRideController(ride)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:         513,
			Debugger:      true,
			Tid:           4,
			Name:          "TopFn",
			Filename:      "/ws/src/top.apl",
			CurrentRow:    3,
			CurrentColumn: 2,
			Text: []string{
				"TopFn;a",
				"a←1",
			},
		},
	})

	scopesResp, _ := server.HandleRequest(Request{
		Seq:     210,
		Command: "scopes",
		Arguments: map[string]any{
			"frameId": 513,
		},
	})
	if !scopesResp.Success {
		t.Fatalf("expected scopes success, got %s", scopesResp.Message)
	}
	scopeRef := scopesResp.Body.(ScopesResponseBody).Scopes[0].VariablesReference

	scopesResp2, _ := server.HandleRequest(Request{
		Seq:     211,
		Command: "scopes",
		Arguments: map[string]any{
			"frameId": 513,
		},
	})
	if !scopesResp2.Success {
		t.Fatalf("expected second scopes success, got %s", scopesResp2.Message)
	}
	scopeRef2 := scopesResp2.Body.(ScopesResponseBody).Scopes[0].VariablesReference
	if scopeRef2 != scopeRef {
		t.Fatalf("expected stable scope variablesReference %d, got %d", scopeRef, scopeRef2)
	}

	scopeVarsResp, _ := server.HandleRequest(Request{
		Seq:     212,
		Command: "variables",
		Arguments: map[string]any{
			"variablesReference": scopeRef,
		},
	})
	if !scopeVarsResp.Success {
		t.Fatalf("expected scope variables success, got %s", scopeVarsResp.Message)
	}
	localsRef := 0
	for _, variable := range scopeVarsResp.Body.(VariablesResponseBody).Variables {
		if variable.Name == "locals" {
			localsRef = variable.VariablesReference
		}
	}
	if localsRef <= 0 {
		t.Fatalf("expected locals ref in scope vars, got %#v", scopeVarsResp.Body)
	}

	localsResp, _ := server.HandleRequest(Request{
		Seq:     213,
		Command: "variables",
		Arguments: map[string]any{
			"variablesReference": localsRef,
		},
	})
	if !localsResp.Success {
		t.Fatalf("expected locals variables success, got %s", localsResp.Message)
	}
	var target Variable
	found := false
	for _, variable := range localsResp.Body.(VariablesResponseBody).Variables {
		if variable.Name == "a" {
			target = variable
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected local variable a, got %#v", localsResp.Body)
	}
	if target.VariablesReference <= 0 {
		t.Fatalf("expected expandable long value for a, got %#v", target)
	}
	if !strings.Contains(target.Value, "…") {
		t.Fatalf("expected truncated preview with ellipsis, got %#v", target)
	}

	childrenResp, _ := server.HandleRequest(Request{
		Seq:     214,
		Command: "variables",
		Arguments: map[string]any{
			"variablesReference": target.VariablesReference,
		},
	})
	if !childrenResp.Success {
		t.Fatalf("expected child variables success, got %s", childrenResp.Message)
	}
	children := childrenResp.Body.(VariablesResponseBody).Variables
	if len(children) == 0 {
		t.Fatal("expected paginated child entries for long value")
	}
	if len(children) > maxLocalValueChildren+1 {
		t.Fatalf("expected child pagination cap, got %d entries", len(children))
	}
}

func TestHandleRequest_EvaluateWatchUsesValueTipAndReturnsDecodedResult(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	ride := &mockRideController{}
	ride.onSend = func(command string, args map[string]any) {
		if command != "GetValueTip" {
			return
		}
		server.HandleRidePayload(protocol.DecodedPayload{
			Kind:    protocol.KindCommand,
			Command: "ValueTip",
			Args: map[string]any{
				"tip":   []any{"0 1 2", "3 4 5"},
				"class": 2,
				"token": args["token"],
			},
		})
	}
	server.SetRideController(ride)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    801,
			Debugger: true,
			Tid:      5,
		},
	})

	resp, _ := server.HandleRequest(Request{
		Seq:     80,
		Command: "evaluate",
		Arguments: map[string]any{
			"expression": "foo",
			"context":    "watch",
			"frameId":    801,
		},
	})
	if !resp.Success {
		t.Fatalf("expected evaluate success, got %s", resp.Message)
	}
	if len(ride.calls) != 1 || ride.calls[0].command != "GetValueTip" {
		t.Fatalf("expected one GetValueTip command, got %#v", ride.calls)
	}
	if got := ride.calls[0].args["win"]; got != 801 {
		t.Fatalf("expected GetValueTip win=801, got %#v", got)
	}
	if got := ride.calls[0].args["line"]; got != "foo" {
		t.Fatalf("expected GetValueTip line=foo, got %#v", got)
	}

	body, ok := resp.Body.(EvaluateResponseBody)
	if !ok {
		t.Fatalf("expected EvaluateResponseBody, got %T", resp.Body)
	}
	if body.Result != "0 1 2\n3 4 5" {
		t.Fatalf("unexpected evaluate result: %#v", body)
	}
	if body.Type != "nameclass(2)" {
		t.Fatalf("unexpected evaluate type: %#v", body)
	}
	if body.VariablesReference != 0 {
		t.Fatalf("expected scalar evaluate result, got %#v", body)
	}
}

func TestHandleRequest_EvaluateFailsWhenPromptBusy(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)
	ride := &mockRideController{}
	server.SetRideController(ride)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    802,
			Debugger: true,
			Tid:      6,
		},
	})
	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "SetPromptType",
		Args: map[string]any{
			"type": 0,
		},
	})

	resp, _ := server.HandleRequest(Request{
		Seq:     81,
		Command: "evaluate",
		Arguments: map[string]any{
			"expression": "foo",
			"context":    "watch",
			"frameId":    802,
		},
	})
	if resp.Success {
		t.Fatal("expected evaluate to fail while promptType=0")
	}
	if len(ride.calls) != 0 {
		t.Fatalf("expected busy evaluate to avoid RIDE send, got %#v", ride.calls)
	}
}

func TestHandleRequest_EvaluateHoverUsesValueTip(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	ride := &mockRideController{}
	ride.onSend = func(command string, args map[string]any) {
		if command != "GetValueTip" {
			return
		}
		server.HandleRidePayload(protocol.DecodedPayload{
			Kind:    protocol.KindCommand,
			Command: "ValueTip",
			Args: map[string]any{
				"tip":   []any{"hover-value"},
				"class": 2,
				"token": args["token"],
			},
		})
	}
	server.SetRideController(ride)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    804,
			Debugger: true,
			Tid:      6,
		},
	})

	resp, _ := server.HandleRequest(Request{
		Seq:     82,
		Command: "evaluate",
		Arguments: map[string]any{
			"expression": "foo",
			"context":    "HOVER",
			"frameId":    804,
		},
	})
	if !resp.Success {
		t.Fatalf("expected hover evaluate success, got %s", resp.Message)
	}
	body := resp.Body.(EvaluateResponseBody)
	if body.Result != "hover-value" {
		t.Fatalf("expected hover value, got %#v", body)
	}
}

func TestHandleRequest_EvaluateReplFallsBackToCachedSymbolValue(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)
	ride := &mockRideController{
		sendErrByCommand: map[string]error{
			"GetValueTip": errors.New("offline"),
		},
	}
	server.SetRideController(ride)

	server.mu.Lock()
	server.activeTracerSet = true
	server.activeTracerWindow = 900
	server.tracerWindows[900] = tracerWindowState{threadID: 1}
	server.frameSymbols[900] = frameSymbolsState{
		order: []string{"foo"},
		symbols: map[string]frameSymbol{
			"foo": {
				name:     "foo",
				isLocal:  true,
				value:    "42",
				class:    2,
				hasValue: true,
			},
		},
	}
	server.mu.Unlock()

	resp, _ := server.HandleRequest(Request{
		Seq:     83,
		Command: "evaluate",
		Arguments: map[string]any{
			"expression": "foo",
			"context":    "repl",
			"frameId":    900,
		},
	})
	if !resp.Success {
		t.Fatalf("expected repl evaluate fallback success, got %s", resp.Message)
	}
	body := resp.Body.(EvaluateResponseBody)
	if body.Result != "42" || body.Type != "nameclass(2)" {
		t.Fatalf("unexpected repl fallback body: %#v", body)
	}
	if len(ride.calls) != 1 || ride.calls[0].command != "GetValueTip" {
		t.Fatalf("expected repl to attempt GetValueTip before fallback, got %#v", ride.calls)
	}
}

func TestHandleRequest_EvaluateWatchTimeoutReturnsActionableError(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)
	server.evaluateTimeout = 5 * time.Millisecond
	ride := &mockRideController{}
	server.SetRideController(ride)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    805,
			Debugger: true,
			Tid:      7,
		},
	})

	resp, _ := server.HandleRequest(Request{
		Seq:     84,
		Command: "evaluate",
		Arguments: map[string]any{
			"expression": "foo",
			"context":    "watch",
			"frameId":    805,
		},
	})
	if resp.Success {
		t.Fatal("expected watch evaluate timeout to fail")
	}
	if !strings.Contains(strings.ToLower(resp.Message), "watch context") {
		t.Fatalf("expected actionable watch timeout message, got %q", resp.Message)
	}
}

func TestHandleRequest_SourceReturnsMappedWindowText(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    901,
			Filename: "/ws/src/source.apl",
			Text:     []string{"[fn-header]", "r<-x+1", "[fn-end]"},
		},
	})
	sourceRef, ok := server.ResolveSourceReferenceForToken(901)
	if !ok {
		t.Fatal("expected source reference mapping for token")
	}

	resp, _ := server.HandleRequest(Request{
		Seq:     82,
		Command: "source",
		Arguments: map[string]any{
			"sourceReference": sourceRef,
		},
	})
	if !resp.Success {
		t.Fatalf("expected source success, got %s", resp.Message)
	}
	body, ok := resp.Body.(SourceResponseBody)
	if !ok {
		t.Fatalf("expected SourceResponseBody, got %T", resp.Body)
	}
	if body.Content != "[fn-header]\nr<-x+1\n[fn-end]" {
		t.Fatalf("unexpected source content: %#v", body)
	}
}

func TestHandleRequest_SourcePrefersActiveWindowTextOverDisk(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	sourcePath := filepath.Join(t.TempDir(), "active.apl")
	if err := os.WriteFile(sourcePath, []byte("disk-version"), 0o600); err != nil {
		t.Fatalf("write disk source: %v", err)
	}

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    902,
			Filename: sourcePath,
			Text:     []string{"window-version"},
		},
	})
	sourceRef, ok := server.ResolveSourceReferenceForToken(902)
	if !ok {
		t.Fatal("expected source reference mapping for token")
	}

	resp, _ := server.HandleRequest(Request{
		Seq:     85,
		Command: "source",
		Arguments: map[string]any{
			"sourceReference": sourceRef,
		},
	})
	if !resp.Success {
		t.Fatalf("expected source success, got %s", resp.Message)
	}
	body := resp.Body.(SourceResponseBody)
	if body.Content != "window-version" {
		t.Fatalf("expected active window text precedence, got %#v", body)
	}
}

func TestHandleRequest_SourceFallsBackToDiskAfterCloseWindow(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	sourcePath := filepath.Join(t.TempDir(), "closed.apl")
	if err := os.WriteFile(sourcePath, []byte("disk-after-close"), 0o600); err != nil {
		t.Fatalf("write disk source: %v", err)
	}

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    903,
			Filename: sourcePath,
			Text:     []string{"window-before-close"},
		},
	})
	sourceRef, ok := server.ResolveSourceReferenceForToken(903)
	if !ok {
		t.Fatal("expected source reference for token")
	}
	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "CloseWindow",
		Args: protocol.WindowArgs{
			Win: 903,
		},
	})

	resp, _ := server.HandleRequest(Request{
		Seq:     86,
		Command: "source",
		Arguments: map[string]any{
			"sourceReference": sourceRef,
		},
	})
	if !resp.Success {
		t.Fatalf("expected source success from disk fallback, got %s", resp.Message)
	}
	body := resp.Body.(SourceResponseBody)
	if body.Content != "disk-after-close" {
		t.Fatalf("expected disk content after close invalidation, got %#v", body)
	}
}

func TestHandleRequest_SourceMappingChurnDoesNotCrossBindOldReferenceContent(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	tempDir := t.TempDir()
	oldPath := filepath.Join(tempDir, "old.apl")
	newPath := filepath.Join(tempDir, "new.apl")
	if err := os.WriteFile(oldPath, []byte("old-disk"), 0o600); err != nil {
		t.Fatalf("write old source: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new-disk"), 0o600); err != nil {
		t.Fatalf("write new source: %v", err)
	}

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    904,
			Filename: oldPath,
			Text:     []string{"old-window"},
		},
	})
	oldRef, ok := server.ResolveSourceReferenceForToken(904)
	if !ok {
		t.Fatal("expected old source ref")
	}

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "UpdateWindow",
		Args: protocol.WindowContentArgs{
			Token:    904,
			Filename: newPath,
			Text:     []string{"new-window"},
		},
	})
	newRef, ok := server.ResolveSourceReferenceForToken(904)
	if !ok {
		t.Fatal("expected new source ref after update")
	}
	if newRef == oldRef {
		t.Fatalf("expected different source refs after path churn: %d", newRef)
	}

	newResp, _ := server.HandleRequest(Request{
		Seq:     87,
		Command: "source",
		Arguments: map[string]any{
			"sourceReference": newRef,
		},
	})
	if !newResp.Success {
		t.Fatalf("expected source success for new ref, got %s", newResp.Message)
	}
	if body := newResp.Body.(SourceResponseBody); body.Content != "new-window" {
		t.Fatalf("expected new ref to return new window text, got %#v", body)
	}

	oldResp, _ := server.HandleRequest(Request{
		Seq:     88,
		Command: "source",
		Arguments: map[string]any{
			"sourceReference": oldRef,
		},
	})
	if !oldResp.Success {
		t.Fatalf("expected old source ref to resolve from disk, got %s", oldResp.Message)
	}
	if body := oldResp.Body.(SourceResponseBody); body.Content != "old-disk" {
		t.Fatalf("expected old ref to avoid cross-bound text, got %#v", body)
	}
}

func TestHandleRequest_SourceWithoutMappingFails(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	resp, _ := server.HandleRequest(Request{
		Seq:     83,
		Command: "source",
		Arguments: map[string]any{
			"sourceReference": 9999,
		},
	})
	if resp.Success {
		t.Fatal("expected missing source mapping to fail")
	}
}

func TestHandleRequest_SourceFailureIncludesPathDiagnostic(t *testing.T) {
	server := NewServer()
	enterRunningState(t, server)

	sourcePath := filepath.Join(t.TempDir(), "missing.apl")
	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    906,
			Filename: sourcePath,
			Text:     []string{"transient"},
		},
	})
	sourceRef, ok := server.ResolveSourceReferenceForToken(906)
	if !ok {
		t.Fatal("expected source reference")
	}
	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "CloseWindow",
		Args: protocol.WindowArgs{
			Win: 906,
		},
	})

	resp, _ := server.HandleRequest(Request{
		Seq:     89,
		Command: "source",
		Arguments: map[string]any{
			"sourceReference": sourceRef,
		},
	})
	if resp.Success {
		t.Fatal("expected missing disk source to fail after close")
	}
	if !strings.Contains(resp.Message, sourcePath) {
		t.Fatalf("expected source failure message to include path, got %q", resp.Message)
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
	if !strings.Contains(strings.ToLower(body.Breakpoints[0].Message), "active") {
		t.Fatalf("expected active breakpoint message, got %#v", body.Breakpoints[0])
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
	if !strings.Contains(strings.ToLower(body.Breakpoints[0].Message), "pending") {
		t.Fatalf("expected pending breakpoint message, got %#v", body.Breakpoints[0])
	}
}

func TestHandleRequest_SetBreakpointsPendingEmitsDiagnosticOutputEvent(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	resp, events := server.HandleRequest(Request{
		Seq:     72,
		Command: "setBreakpoints",
		Arguments: map[string]any{
			"source": map[string]any{
				"path": "/ws/src/pending.apl",
			},
			"breakpoints": []any{
				map[string]any{"line": 3},
			},
		},
	})
	if !resp.Success {
		t.Fatalf("expected pending setBreakpoints success, got %s", resp.Message)
	}
	if len(events) != 1 || events[0].Event != "output" {
		t.Fatalf("expected one output diagnostic event, got %#v", events)
	}
	output := events[0].Body.(OutputEventBody).Output
	if !strings.Contains(strings.ToLower(output), "breakpoints pending") {
		t.Fatalf("expected pending diagnostic output, got %q", output)
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

	events := server.HandleRidePayload(protocol.DecodedPayload{
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
	if len(events) == 0 || events[0].Event != "output" {
		t.Fatalf("expected output diagnostic event from deferred apply, got %#v", events)
	}
	if !strings.Contains(strings.ToLower(events[0].Body.(OutputEventBody).Output), "deferred apply succeeded") {
		t.Fatalf("expected deferred apply success diagnostic, got %#v", events[0])
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

func TestHandleRidePayload_UpdateWindowAppliesDeferredBreakpointsWithConsistentLineTranslation(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)
	enterRunningState(t, server)

	_, _ = server.HandleRequest(Request{
		Seq:     77,
		Command: "setBreakpoints",
		Arguments: map[string]any{
			"source": map[string]any{
				"path": "/ws/src/update-deferred.apl",
			},
			"breakpoints": []any{
				map[string]any{"line": 1},
				map[string]any{"line": 10},
			},
		},
	})

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:    511,
			Filename: "/ws/src/other.apl",
		},
	})

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "UpdateWindow",
		Args: protocol.WindowContentArgs{
			Token:    511,
			Filename: "/ws/src/update-deferred.apl",
		},
	})

	last := ride.lastCall()
	if last.command != "SetLineAttributes" {
		t.Fatalf("expected SetLineAttributes, got %q", last.command)
	}
	stop := last.args["stop"].([]int)
	if len(stop) != 2 || stop[0] != 0 || stop[1] != 9 {
		t.Fatalf("expected consistent zero-based stops [0 9], got %#v", stop)
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
