package harness

import "testing"

func TestDyalogServeLaunchCommand_DefaultExecutable(t *testing.T) {
	cmd, err := DyalogServeLaunchCommand("127.0.0.1:4502", "")
	if err != nil {
		t.Fatalf("DyalogServeLaunchCommand returned error: %v", err)
	}

	const expected = "RIDE_INIT=SERVE:*:4502 dyalog +s -q"
	if cmd != expected {
		t.Fatalf("command mismatch: got %q want %q", cmd, expected)
	}
}

func TestDyalogServeLaunchCommand_QuotesExecutablePath(t *testing.T) {
	cmd, err := DyalogServeLaunchCommand("localhost:14502", "/Applications/Dyalog APL-18.2/mapl")
	if err != nil {
		t.Fatalf("DyalogServeLaunchCommand returned error: %v", err)
	}

	const expected = "RIDE_INIT=SERVE:*:14502 '/Applications/Dyalog APL-18.2/mapl' +s -q"
	if cmd != expected {
		t.Fatalf("command mismatch: got %q want %q", cmd, expected)
	}
}

func TestDyalogServeLaunchCommand_RejectsMissingPort(t *testing.T) {
	if _, err := DyalogServeLaunchCommand("127.0.0.1", "dyalog"); err == nil {
		t.Fatal("expected error for address without port")
	}
}
