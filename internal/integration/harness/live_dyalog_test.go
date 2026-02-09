package harness

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stefan/lsp-dap/internal/ride/protocol"
	"github.com/stefan/lsp-dap/internal/ride/sessionstate"
)

func TestLiveDyalog_GetThreadsAndExecuteRoundTrip(t *testing.T) {
	cfg := ConfigFromEnv()
	required := os.Getenv("DYALOG_LIVE_REQUIRE") == "1"
	if cfg.RideAddr == "" {
		if required {
			t.Fatal("DYALOG_RIDE_ADDR must be set when DYALOG_LIVE_REQUIRE=1")
		}
		t.Skip("DYALOG_RIDE_ADDR is not set; skipping live Dyalog smoke test")
	}

	if cfg.LaunchCommand == "" {
		if dyalogBin := os.Getenv("DYALOG_BIN"); dyalogBin != "" {
			cmd, err := DyalogServeLaunchCommand(cfg.RideAddr, dyalogBin)
			if err != nil {
				t.Fatalf("build launch command from DYALOG_BIN failed: %v", err)
			}
			cfg.LaunchCommand = cmd
		}
	}

	h := New(cfg)
	client, err := h.Start(context.Background(), t.Name())
	if err != nil {
		t.Fatalf("live harness start failed: %v", err)
	}

	dispatcher := sessionstate.NewDispatcher(client, protocol.NewCodec())
	events, unsubscribe := dispatcher.Subscribe(256)
	defer unsubscribe()

	dispatcherCtx, cancelDispatcher := context.WithCancel(context.Background())
	dispatcherDone := make(chan struct{})
	go func() {
		dispatcher.Run(dispatcherCtx)
		close(dispatcherDone)
	}()

	if err := dispatcher.SendCommand("GetThreads", protocol.GetThreadsArgs{}); err != nil {
		t.Fatalf("SendCommand GetThreads failed: %v", err)
	}
	if err := waitForReplyGetThreads(events, 15*time.Second); err != nil {
		t.Fatalf("did not receive ReplyGetThreads from live interpreter: %v", err)
	}

	if err := dispatcher.SendCommand("Execute", protocol.ExecuteArgs{Text: "1+1\n", Trace: 0}); err != nil {
		t.Fatalf("SendCommand Execute failed: %v", err)
	}
	output, err := waitForExecuteCompletion(events, 15*time.Second)
	if err != nil {
		t.Fatalf("did not observe Execute completion in live interpreter: %v", err)
	}
	if !strings.Contains(output, "2") {
		t.Fatalf("expected Execute output to contain 2, got %q", output)
	}

	cancelDispatcher()
	if err := h.Close(); err != nil {
		t.Fatalf("harness close failed: %v", err)
	}
	select {
	case <-dispatcherDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("dispatcher did not stop")
	}

	transcriptPath := h.TranscriptPath()
	contents, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatalf("read transcript failed: %v", err)
	}
	text := string(contents)
	if !strings.Contains(text, "GetThreads") {
		t.Fatalf("expected transcript to contain GetThreads traffic, got: %s", text)
	}
	if !strings.Contains(text, "Execute") {
		t.Fatalf("expected transcript to contain Execute traffic, got: %s", text)
	}
}

func waitForReplyGetThreads(events <-chan protocol.DecodedPayload, timeout time.Duration) error {
	deadline := time.After(timeout)
	for {
		select {
		case event := <-events:
			if event.Command != "ReplyGetThreads" {
				continue
			}
			if _, ok := event.Args.(protocol.ReplyGetThreadsArgs); !ok {
				return &liveAssertionError{message: "ReplyGetThreads args were not decoded into typed struct"}
			}
			return nil
		case <-deadline:
			return &liveAssertionError{message: "timeout waiting for ReplyGetThreads"}
		}
	}
}

func waitForExecuteCompletion(events <-chan protocol.DecodedPayload, timeout time.Duration) (string, error) {
	var output strings.Builder
	sawBusy := false
	deadline := time.After(timeout)

	for {
		select {
		case event := <-events:
			switch event.Command {
			case "AppendSessionOutput":
				args, ok := event.Args.(protocol.AppendSessionOutputArgs)
				if !ok {
					return "", &liveAssertionError{message: "AppendSessionOutput args were not decoded into typed struct"}
				}
				// type 14 is command echo (input line), which gritt filters out.
				if args.Type == 14 {
					continue
				}
				output.WriteString(args.Result)
			case "SetPromptType":
				args, ok := event.Args.(protocol.SetPromptTypeArgs)
				if !ok {
					return "", &liveAssertionError{message: "SetPromptType args were not decoded into typed struct"}
				}
				if args.Type == 0 {
					sawBusy = true
					continue
				}
				if args.Type > 0 && (sawBusy || output.Len() > 0) {
					return output.String(), nil
				}
			}
		case <-deadline:
			return "", &liveAssertionError{
				message: "timeout waiting for Execute completion signal",
			}
		}
	}
}

type liveAssertionError struct {
	message string
}

func (e *liveAssertionError) Error() string {
	return e.message
}
