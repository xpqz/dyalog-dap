package adapter

import "testing"

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
