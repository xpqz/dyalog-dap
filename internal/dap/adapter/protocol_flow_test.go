package adapter

import (
	"testing"

	"github.com/stefan/lsp-dap/internal/ride/protocol"
)

func TestDAPProtocol_Phase1DeterministicFlow(t *testing.T) {
	ride := &mockRideController{}
	server := NewServer()
	server.SetRideController(ride)

	initResp, initEvents := server.HandleRequest(Request{Seq: 1, Command: "initialize"})
	if !initResp.Success {
		t.Fatalf("initialize failed: %s", initResp.Message)
	}
	if len(initEvents) != 1 || initEvents[0].Event != "initialized" {
		t.Fatalf("expected initialized event, got %#v", initEvents)
	}

	launchResp, _ := server.HandleRequest(Request{Seq: 2, Command: "launch"})
	if !launchResp.Success {
		t.Fatalf("launch failed: %s", launchResp.Message)
	}

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "ReplyGetThreads",
		Args: protocol.ReplyGetThreadsArgs{
			Threads: []protocol.ThreadInfo{{Tid: 1, Description: "Main"}},
		},
	})

	threadsResp, _ := server.HandleRequest(Request{Seq: 3, Command: "threads"})
	if !threadsResp.Success {
		t.Fatalf("threads failed: %s", threadsResp.Message)
	}
	threadsBody := threadsResp.Body.(ThreadsResponseBody)
	if len(threadsBody.Threads) != 1 || threadsBody.Threads[0].ID != 1 {
		t.Fatalf("unexpected threads body: %#v", threadsBody)
	}
	if ride.lastCall().command != "GetThreads" {
		t.Fatalf("expected GetThreads poll, got %#v", ride.lastCall())
	}

	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "OpenWindow",
		Args: protocol.WindowContentArgs{
			Token:         42,
			Debugger:      true,
			Tid:           1,
			Name:          "trace",
			Filename:      "/ws/src/smoke.apl",
			CurrentRow:    8,
			CurrentColumn: 2,
		},
	})
	server.HandleRidePayload(protocol.DecodedPayload{
		Kind:    protocol.KindCommand,
		Command: "SetHighlightLine",
		Args: protocol.SetHighlightLineArgs{
			Win:      42,
			Line:     9,
			StartCol: 3,
		},
	})

	stackResp, _ := server.HandleRequest(Request{
		Seq:     4,
		Command: "stackTrace",
		Arguments: map[string]any{
			"threadId": 1,
		},
	})
	if !stackResp.Success {
		t.Fatalf("stackTrace failed: %s", stackResp.Message)
	}
	stackBody := stackResp.Body.(StackTraceResponseBody)
	if len(stackBody.StackFrames) != 1 {
		t.Fatalf("expected 1 stack frame, got %#v", stackBody)
	}
	if stackBody.StackFrames[0].Line != 10 || stackBody.StackFrames[0].Column != 4 {
		t.Fatalf("expected one-based highlight coordinates line=10 col=4, got %#v", stackBody.StackFrames[0])
	}

	nextResp, _ := server.HandleRequest(Request{Seq: 5, Command: "next"})
	if !nextResp.Success {
		t.Fatalf("next failed: %s", nextResp.Message)
	}
	if ride.lastCall().command != "RunCurrentLine" || ride.lastCall().args["win"] != 42 {
		t.Fatalf("expected RunCurrentLine win=42, got %#v", ride.lastCall())
	}

	continueResp, _ := server.HandleRequest(Request{Seq: 6, Command: "continue"})
	if !continueResp.Success {
		t.Fatalf("continue failed: %s", continueResp.Message)
	}
	if ride.lastCall().command != "Continue" || ride.lastCall().args["win"] != 42 {
		t.Fatalf("expected Continue win=42, got %#v", ride.lastCall())
	}

	setBreakpointsResp, _ := server.HandleRequest(Request{
		Seq:     7,
		Command: "setBreakpoints",
		Arguments: map[string]any{
			"source": map[string]any{"path": "/ws/src/smoke.apl"},
			"breakpoints": []any{
				map[string]any{"line": 5},
			},
		},
	})
	if !setBreakpointsResp.Success {
		t.Fatalf("setBreakpoints failed: %s", setBreakpointsResp.Message)
	}
	bpBody := setBreakpointsResp.Body.(SetBreakpointsResponseBody)
	if len(bpBody.Breakpoints) != 1 || !bpBody.Breakpoints[0].Verified {
		t.Fatalf("unexpected breakpoint response: %#v", bpBody)
	}
	if ride.lastCall().command != "SetLineAttributes" || ride.lastCall().args["win"] != 42 {
		t.Fatalf("expected SetLineAttributes win=42, got %#v", ride.lastCall())
	}
	stop := ride.lastCall().args["stop"].([]int)
	if len(stop) != 1 || stop[0] != 4 {
		t.Fatalf("expected stop=[4], got %#v", stop)
	}
}
