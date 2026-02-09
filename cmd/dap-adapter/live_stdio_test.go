package main

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stefan/lsp-dap/internal/integration/harness"
)

func TestLiveDAPAdapter_InitializeLaunchThreadsDisconnect(t *testing.T) {
	cfg := harness.ConfigFromEnv()
	required := os.Getenv("DYALOG_LIVE_REQUIRE") == "1"
	if cfg.RideAddr == "" {
		if required {
			t.Fatal("DYALOG_RIDE_ADDR must be set when DYALOG_LIVE_REQUIRE=1")
		}
		t.Skip("DYALOG_RIDE_ADDR is not set; skipping live stdio DAP adapter test")
	}

	if cfg.LaunchCommand == "" {
		if dyalogBin := os.Getenv("DYALOG_BIN"); dyalogBin != "" {
			command, err := harness.DyalogServeLaunchCommand(cfg.RideAddr, dyalogBin)
			if err != nil {
				t.Fatalf("build launch command from DYALOG_BIN failed: %v", err)
			}
			cfg.LaunchCommand = command
		}
	}

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	runErr := make(chan error, 1)
	go func() {
		runErr <- run(context.Background(), inR, outW, io.Discard)
		_ = outW.Close()
	}()

	decoderErr := make(chan error, 1)
	msgs := make(chan map[string]any, 128)
	go func() {
		defer close(msgs)
		decoderErr <- decodeDAPStream(outR, msgs)
	}()

	writeReq := func(seq int, command string, args map[string]any) {
		t.Helper()
		req := map[string]any{
			"seq":     seq,
			"type":    "request",
			"command": command,
		}
		if args != nil {
			req["arguments"] = args
		}
		if err := writeDAPFrame(inW, req); err != nil {
			t.Fatalf("write %s failed: %v", command, err)
		}
	}

	writeReq(1, "initialize", map[string]any{"adapterID": "dyalog-dap"})
	if ok, _ := waitForResponse(t, msgs, 1)["success"].(bool); !ok {
		t.Fatal("initialize response was not successful")
	}
	waitForEvent(t, msgs, "initialized")

	launchArgs := map[string]any{
		"rideAddr":           cfg.RideAddr,
		"rideConnectTimeout": cfg.ConnectTimeout.String(),
		"rideTranscriptsDir": t.TempDir(),
	}
	if cfg.LaunchCommand != "" {
		launchArgs["rideLaunchCommand"] = cfg.LaunchCommand
	}
	writeReq(2, "launch", launchArgs)
	launchResp := waitForResponse(t, msgs, 2)
	if ok, _ := launchResp["success"].(bool); !ok {
		t.Fatalf("launch response failed: %#v", launchResp)
	}

	writeReq(3, "threads", nil)
	if ok, _ := waitForResponse(t, msgs, 3)["success"].(bool); !ok {
		t.Fatal("threads response was not successful")
	}

	writeReq(4, "disconnect", nil)
	if ok, _ := waitForResponse(t, msgs, 4)["success"].(bool); !ok {
		t.Fatal("disconnect response was not successful")
	}
	_ = inW.Close()

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("timed out waiting for run to stop")
	}
	if err := <-decoderErr; err != nil {
		t.Fatalf("decode stream failed: %v", err)
	}
}
