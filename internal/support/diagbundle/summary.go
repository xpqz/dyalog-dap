package diagbundle

import (
	"encoding/json"
	"errors"
	"strings"
)

type Bundle struct {
	SchemaVersion string `json:"schemaVersion"`
	GeneratedAt   string `json:"generatedAt"`
	Extension     struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"extension"`
	Workspace struct {
		Name string `json:"name"`
	} `json:"workspace"`
	Diagnostics struct {
		Recent []string `json:"recent"`
	} `json:"diagnostics"`
	Environment    map[string]any `json:"environment"`
	ConfigSnapshot any            `json:"configSnapshot"`
	Transcripts    struct {
		Pointers []string `json:"pointers"`
	} `json:"transcripts"`
}

type Summary struct {
	SchemaVersion      string   `json:"schemaVersion"`
	GeneratedAt        string   `json:"generatedAt"`
	ExtensionVersion   string   `json:"extensionVersion"`
	WorkspaceName      string   `json:"workspaceName"`
	DiagnosticsCount   int      `json:"diagnosticsCount"`
	LastDiagnosticLine string   `json:"lastDiagnosticLine"`
	TranscriptPointers []string `json:"transcriptPointers"`
	ProblemClass       string   `json:"problemClass"`
	RedactionDetected  bool     `json:"redactionDetected"`
	NextActions        []string `json:"nextActions"`
}

func SummarizeBundle(data []byte) (Summary, error) {
	var bundle Bundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return Summary{}, err
	}
	if bundle.SchemaVersion == "" {
		return Summary{}, errors.New("invalid diagnostic bundle: missing schemaVersion")
	}

	lines := bundle.Diagnostics.Recent
	last := ""
	if len(lines) > 0 {
		last = lines[len(lines)-1]
	}

	summary := Summary{
		SchemaVersion:      bundle.SchemaVersion,
		GeneratedAt:        bundle.GeneratedAt,
		ExtensionVersion:   bundle.Extension.Version,
		WorkspaceName:      bundle.Workspace.Name,
		DiagnosticsCount:   len(lines),
		LastDiagnosticLine: last,
		TranscriptPointers: append([]string(nil), bundle.Transcripts.Pointers...),
		ProblemClass:       classify(lines),
		RedactionDetected:  hasRedactionMarker(bundle.Environment) || hasRedactionMarker(bundle.ConfigSnapshot),
	}
	summary.NextActions = recommendedActions(summary)
	return summary, nil
}

func classify(lines []string) string {
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "adapter.install.failed") {
			return "installer/distribution"
		}
		if strings.Contains(lower, "validateadapterpath.failed") ||
			strings.Contains(lower, "adapter.resolve.failed") ||
			strings.Contains(lower, "validaterideaddr.failed") {
			return "setup/configuration"
		}
		if strings.Contains(lower, "support.bundle.noworkspace") {
			return "workspace/setup"
		}
		if strings.Contains(lower, "setbreakpoints") ||
			strings.Contains(lower, "stacktrace") ||
			strings.Contains(lower, "variables") ||
			strings.Contains(lower, "evaluate") {
			return "debug-runtime"
		}
	}
	return "unknown"
}

func recommendedActions(summary Summary) []string {
	actions := []string{
		"Attach diagnostic bundle JSON to the issue.",
	}
	if len(summary.TranscriptPointers) == 0 {
		actions = append(actions, "Collect and attach RIDE transcript artifacts.")
	}
	if !summary.RedactionDetected {
		actions = append(actions, "Confirm sensitive values are redacted before sharing externally.")
	}

	switch summary.ProblemClass {
	case "setup/configuration":
		actions = append(actions, "Verify adapter path and rideAddr settings first.")
	case "installer/distribution":
		actions = append(actions, "Verify release asset availability and checksums for this platform.")
	case "workspace/setup":
		actions = append(actions, "Confirm issue reproduces with an opened workspace folder.")
	case "debug-runtime":
		actions = append(actions, "Request minimal APL repro and transcript excerpt around failing request.")
	default:
		actions = append(actions, "Classify incident manually and request missing reproduction artifacts.")
	}
	return actions
}

func hasRedactionMarker(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.Contains(strings.ToLower(typed), "<redacted")
	case []any:
		for _, item := range typed {
			if hasRedactionMarker(item) {
				return true
			}
		}
		return false
	case map[string]any:
		for _, item := range typed {
			if hasRedactionMarker(item) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
