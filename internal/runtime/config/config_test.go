package config

import (
	"strings"
	"testing"
)

func TestFromRequest_AttachClearsEnvLaunchCommand(t *testing.T) {
	t.Setenv("DYALOG_RIDE_ADDR", "127.0.0.1:4502")
	t.Setenv("DYALOG_RIDE_LAUNCH", "RIDE_INIT=SERVE:*:4502 dyalog +s -q")

	cfg, err := FromRequest("attach", nil)
	if err != nil {
		t.Fatalf("FromRequest attach failed: %v", err)
	}
	if cfg.LaunchCommand != "" {
		t.Fatalf("expected attach config to clear launch command ownership, got %q", cfg.LaunchCommand)
	}
}

func TestFromRequest_AttachRejectsExplicitLaunchSettings(t *testing.T) {
	t.Setenv("DYALOG_RIDE_ADDR", "127.0.0.1:4502")

	_, err := FromRequest("attach", map[string]any{
		"rideLaunchCommand": "RIDE_INIT=SERVE:*:4502 dyalog +s -q",
	})
	if err == nil {
		t.Fatal("expected attach with rideLaunchCommand to fail ownership policy")
	}
	if !strings.Contains(err.Error(), "attach does not support adapter-owned launch") {
		t.Fatalf("unexpected error: %v", err)
	}
}
