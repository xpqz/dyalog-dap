package diagbundle

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSummarizeBundle_ClassifiesAndProducesActions(t *testing.T) {
	input := map[string]any{
		"schemaVersion": "1",
		"generatedAt":   "2026-02-09T00:00:00Z",
		"extension": map[string]any{
			"name":    "dyalog-dap",
			"version": "0.0.1",
		},
		"workspace": map[string]any{
			"name": "sample",
		},
		"diagnostics": map[string]any{
			"recent": []string{
				"2026-02-09T00:00:00Z [error] setup.validateRideAddr.failed rideAddr=\"bad\"",
			},
		},
		"environment": map[string]any{
			"DYALOG_RIDE_TRANSCRIPTS_DIR": "<redacted-path>",
		},
		"configSnapshot": map[string]any{
			"launchConfigurations": []any{},
		},
		"transcripts": map[string]any{
			"pointers": []string{"/tmp/transcript.jsonl"},
		},
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	summary, err := SummarizeBundle(data)
	if err != nil {
		t.Fatalf("SummarizeBundle: %v", err)
	}
	if summary.ProblemClass != "setup/configuration" {
		t.Fatalf("unexpected problem class: %q", summary.ProblemClass)
	}
	if !summary.RedactionDetected {
		t.Fatalf("expected redaction marker detection")
	}
	if len(summary.NextActions) == 0 {
		t.Fatalf("expected next actions")
	}
	joined := strings.ToLower(strings.Join(summary.NextActions, " "))
	if !strings.Contains(joined, "adapter path") {
		t.Fatalf("expected setup/configuration recommendation, got: %v", summary.NextActions)
	}
}

func TestSummarizeBundle_MissingTranscriptsAndNoRedaction(t *testing.T) {
	input := map[string]any{
		"schemaVersion": "1",
		"generatedAt":   "2026-02-09T00:00:00Z",
		"extension": map[string]any{
			"name":    "dyalog-dap",
			"version": "0.0.1",
		},
		"workspace": map[string]any{
			"name": "sample",
		},
		"diagnostics": map[string]any{
			"recent": []string{"2026-02-09T00:00:00Z [info] extension.activate"},
		},
		"environment": map[string]any{
			"PATH": "/usr/bin",
		},
		"configSnapshot": map[string]any{
			"launchConfigurations": []any{},
		},
		"transcripts": map[string]any{
			"pointers": []string{},
		},
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	summary, err := SummarizeBundle(data)
	if err != nil {
		t.Fatalf("SummarizeBundle: %v", err)
	}
	if summary.RedactionDetected {
		t.Fatalf("expected no redaction markers in summary")
	}
	joined := strings.ToLower(strings.Join(summary.NextActions, " "))
	if !strings.Contains(joined, "transcript") {
		t.Fatalf("expected transcript action in %v", summary.NextActions)
	}
	if !strings.Contains(joined, "redacted") {
		t.Fatalf("expected redaction reminder in %v", summary.NextActions)
	}
}
