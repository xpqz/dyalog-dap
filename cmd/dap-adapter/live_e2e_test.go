package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stefan/lsp-dap/internal/integration/harness"
)

func TestLiveDAPAdapter_InteractiveWorkflow(t *testing.T) {
	cfg := harness.ConfigFromEnv()
	liveRequired := os.Getenv("DYALOG_LIVE_REQUIRE") == "1"
	e2eRequired := os.Getenv("DYALOG_E2E_REQUIRE") == "1"

	if cfg.RideAddr == "" {
		if liveRequired || e2eRequired {
			t.Fatal("DYALOG_RIDE_ADDR must be set when live or e2e tests are required")
		}
		t.Skip("DYALOG_RIDE_ADDR is not set; skipping live interactive E2E test")
	}
	if !e2eRequired {
		t.Skip("set DYALOG_E2E_REQUIRE=1 to enable live interactive E2E workflow validation")
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

	e2eTimeout := parseE2ETimeout("DYALOG_E2E_TIMEOUT", 20*time.Second)
	artifactDir := filepath.Join("artifacts", "integration", "live-e2e")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}
	artifactPath := filepath.Join(artifactDir, fmt.Sprintf("%s-%d.jsonl", sanitizeName(t.Name()), time.Now().UnixNano()))
	artifactFile, err := os.Create(artifactPath)
	if err != nil {
		t.Fatalf("create e2e artifact file: %v", err)
	}
	defer artifactFile.Close()
	t.Logf("live E2E DAP artifact: %s", artifactPath)

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	runErr := make(chan error, 1)
	go func() {
		runErr <- run(context.Background(), inR, outW, io.Discard)
		_ = outW.Close()
	}()

	msgs := make(chan map[string]any, 256)
	decoderErr := make(chan error, 1)
	go func() {
		defer close(msgs)
		decoderErr <- decodeAndRecordDAPStream(outR, msgs, artifactFile)
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

	failStep := func(step string, details string) {
		t.Helper()
		classification := classifyE2EFailure(step + ": " + details)
		t.Fatalf("[%s] %s (artifact: %s)", classification, details, artifactPath)
	}

	writeReq(1, "initialize", map[string]any{"adapterID": "dyalog-dap"})
	initResp, err := waitForResponseWithin(msgs, 1, e2eTimeout)
	if err != nil {
		failStep("initialize", err.Error())
	}
	if ok, _ := initResp["success"].(bool); !ok {
		failStep("initialize", fmt.Sprintf("initialize failed: %#v", initResp))
	}
	if _, err := waitForEventWithin(msgs, "initialized", e2eTimeout); err != nil {
		failStep("initialized-event", err.Error())
	}

	launchArgs := map[string]any{
		"rideAddr":           cfg.RideAddr,
		"rideConnectTimeout": cfg.ConnectTimeout.String(),
		"rideTranscriptsDir": artifactDir,
	}
	if cfg.LaunchCommand != "" {
		launchArgs["rideLaunchCommand"] = cfg.LaunchCommand
	}
	writeReq(2, "launch", launchArgs)
	launchResp, err := waitForResponseWithin(msgs, 2, e2eTimeout)
	if err != nil {
		failStep("launch", err.Error())
	}
	if ok, _ := launchResp["success"].(bool); !ok {
		failStep("launch", fmt.Sprintf("launch failed: %#v", launchResp))
	}

	stoppedEvent, err := waitForEventWithin(msgs, "stopped", e2eTimeout)
	if err != nil {
		failStep("stopped-event", err.Error())
	}
	threadID := 0
	if body, ok := stoppedEvent["body"].(map[string]any); ok {
		threadID, _ = asInt(body["threadId"])
	}
	if threadID <= 0 {
		failStep("stopped-event", fmt.Sprintf("missing threadId in stopped event: %#v", stoppedEvent))
	}

	writeReq(3, "threads", nil)
	threadsResp, err := waitForResponseWithin(msgs, 3, e2eTimeout)
	if err != nil {
		failStep("threads", err.Error())
	}
	if ok, _ := threadsResp["success"].(bool); !ok {
		failStep("threads", fmt.Sprintf("threads failed: %#v", threadsResp))
	}

	writeReq(4, "stackTrace", map[string]any{"threadId": threadID})
	stackResp, err := waitForResponseWithin(msgs, 4, e2eTimeout)
	if err != nil {
		failStep("stackTrace", err.Error())
	}
	if ok, _ := stackResp["success"].(bool); !ok {
		failStep("stackTrace", fmt.Sprintf("stackTrace failed: %#v", stackResp))
	}
	frameID := firstFrameID(stackResp)
	if frameID <= 0 {
		failStep("stackTrace", fmt.Sprintf("missing frame id in stackTrace response: %#v", stackResp))
	}

	writeReq(5, "scopes", map[string]any{"frameId": frameID})
	scopesResp, err := waitForResponseWithin(msgs, 5, e2eTimeout)
	if err != nil {
		failStep("scopes", err.Error())
	}
	if ok, _ := scopesResp["success"].(bool); !ok {
		failStep("scopes", fmt.Sprintf("scopes failed: %#v", scopesResp))
	}
	varRef := firstScopeVariablesReference(scopesResp)
	if varRef <= 0 {
		failStep("scopes", fmt.Sprintf("missing scope variablesReference: %#v", scopesResp))
	}

	writeReq(6, "variables", map[string]any{"variablesReference": varRef})
	varsResp, err := waitForResponseWithin(msgs, 6, e2eTimeout)
	if err != nil {
		failStep("variables", err.Error())
	}
	if ok, _ := varsResp["success"].(bool); !ok {
		failStep("variables", fmt.Sprintf("variables failed: %#v", varsResp))
	}

	writeReq(7, "next", nil)
	nextResp, err := waitForResponseWithin(msgs, 7, e2eTimeout)
	if err != nil {
		failStep("next", err.Error())
	}
	if ok, _ := nextResp["success"].(bool); !ok {
		failStep("next", fmt.Sprintf("next failed: %#v", nextResp))
	}

	writeReq(8, "continue", nil)
	continueResp, err := waitForResponseWithin(msgs, 8, e2eTimeout)
	if err != nil {
		failStep("continue", err.Error())
	}
	if ok, _ := continueResp["success"].(bool); !ok {
		failStep("continue", fmt.Sprintf("continue failed: %#v", continueResp))
	}

	writeReq(9, "disconnect", nil)
	disconnectResp, err := waitForResponseWithin(msgs, 9, e2eTimeout)
	if err != nil {
		failStep("disconnect", err.Error())
	}
	if ok, _ := disconnectResp["success"].(bool); !ok {
		failStep("disconnect", fmt.Sprintf("disconnect failed: %#v", disconnectResp))
	}
	_ = inW.Close()

	select {
	case err := <-runErr:
		if err != nil {
			failStep("run-stop", err.Error())
		}
	case <-time.After(5 * time.Second):
		failStep("run-stop", "timed out waiting for adapter run loop to stop")
	}
	if err := <-decoderErr; err != nil {
		failStep("decoder", err.Error())
	}
}

func decodeAndRecordDAPStream(
	r io.Reader,
	out chan<- map[string]any,
	record io.Writer,
) error {
	reader := bufio.NewReader(r)
	for {
		payload, err := readDAPPayload(reader)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		var msg map[string]any
		if err := json.Unmarshal(payload, &msg); err != nil {
			return err
		}
		line, err := json.Marshal(msg)
		if err == nil {
			_, _ = fmt.Fprintf(record, "%s\n", line)
		}
		out <- msg
	}
}

func waitForResponseWithin(
	msgs <-chan map[string]any,
	seq int,
	timeout time.Duration,
) (map[string]any, error) {
	deadline := time.After(timeout)
	for {
		select {
		case msg, ok := <-msgs:
			if !ok {
				return nil, fmt.Errorf("message stream closed while waiting for response %d", seq)
			}
			if msg["type"] != "response" {
				continue
			}
			requestSeq, _ := asInt(msg["request_seq"])
			if requestSeq != seq {
				continue
			}
			return msg, nil
		case <-deadline:
			return nil, fmt.Errorf("timeout waiting for response %d", seq)
		}
	}
}

func waitForEventWithin(
	msgs <-chan map[string]any,
	event string,
	timeout time.Duration,
) (map[string]any, error) {
	deadline := time.After(timeout)
	for {
		select {
		case msg, ok := <-msgs:
			if !ok {
				return nil, fmt.Errorf("message stream closed while waiting for event %q", event)
			}
			if msg["type"] != "event" {
				continue
			}
			name, _ := msg["event"].(string)
			if name == event {
				return msg, nil
			}
		case <-deadline:
			return nil, fmt.Errorf("timeout waiting for event %q", event)
		}
	}
}

func firstFrameID(stackResp map[string]any) int {
	body, _ := stackResp["body"].(map[string]any)
	frames, _ := body["stackFrames"].([]any)
	if len(frames) == 0 {
		return 0
	}
	frame, _ := frames[0].(map[string]any)
	id, _ := asInt(frame["id"])
	return id
}

func firstScopeVariablesReference(scopesResp map[string]any) int {
	body, _ := scopesResp["body"].(map[string]any)
	scopes, _ := body["scopes"].([]any)
	if len(scopes) == 0 {
		return 0
	}
	scope, _ := scopes[0].(map[string]any)
	ref, _ := asInt(scope["variablesReference"])
	return ref
}

func parseE2ETimeout(envKey string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func sanitizeName(name string) string {
	replaced := strings.NewReplacer("/", "-", " ", "-", ":", "-", "\\", "-").Replace(name)
	replaced = strings.Trim(replaced, "-")
	if replaced == "" {
		return "live-e2e"
	}
	return replaced
}

func classifyE2EFailure(details string) string {
	msg := strings.ToLower(details)
	switch {
	case strings.Contains(msg, "timeout"),
		strings.Contains(msg, "connection reset"),
		strings.Contains(msg, "broken pipe"),
		strings.Contains(msg, "stream closed"):
		return "infrastructure-flake"
	case strings.Contains(msg, "no active tracer window"),
		strings.Contains(msg, "missing frame id"),
		strings.Contains(msg, "missing scope"):
		return "scenario-precondition"
	default:
		return "product-defect"
	}
}
