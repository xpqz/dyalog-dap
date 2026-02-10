package lspdap_test

import (
	"strings"
	"testing"
)

func TestScaffold_HasDiagnosticBundleWorkflowAndTriageDocs(t *testing.T) {
	pkg := mustUnmarshalJSONFile[struct {
		ActivationEvents []string `json:"activationEvents"`
		Contributes      struct {
			Commands []struct {
				Command string `json:"command"`
				Title   string `json:"title"`
			} `json:"commands"`
		} `json:"contributes"`
	}](t, "vscode-extension/package.json")

	commandID := "dyalogDap.generateDiagnosticBundle"
	foundCommand := false
	for _, command := range pkg.Contributes.Commands {
		if command.Command == commandID && strings.TrimSpace(command.Title) != "" {
			foundCommand = true
			break
		}
	}
	if !foundCommand {
		t.Fatalf("expected contributes.commands entry for %q", commandID)
	}

	activationEvent := "onCommand:" + commandID
	foundActivation := false
	for _, event := range pkg.ActivationEvents {
		if event == activationEvent {
			foundActivation = true
			break
		}
	}
	if !foundActivation {
		t.Fatalf("expected activation event %q", activationEvent)
	}

	mustFileExists(t, "vscode-extension/src/diagnosticsBundle.ts")
	mustFileExists(t, "vscode-extension/src/test/diagnosticsBundle.test.ts")

	triageText := mustReadFile(t, "docs/support/triage.md")
	requireSnippetsFold(t, triageText,
		"diagnostic bundle",
		"redact",
		"first-pass",
		"artifacts",
	)
}

func TestScaffold_HasDiagnosticBundleIntegrationCommandTest(t *testing.T) {
	path := "vscode-extension/src/test/extensionBundleCommand.integration.test.ts"
	text := mustReadFile(t, path)
	requireSnippets(t, text,
		"dyalogDap.generateDiagnosticBundle",
		"diagnostic-bundle-",
		"redacted",
	)
}

func TestScaffold_HasSupportIntakeTemplateAndBundleSummarizer(t *testing.T) {
	requiredFiles := []string{
		".github/ISSUE_TEMPLATE/diagnostic-support.yml",
		"cmd/diagnostic-summary/main.go",
		"internal/support/diagbundle/summary.go",
		"internal/support/diagbundle/summary_test.go",
	}
	for _, file := range requiredFiles {
		mustFileExists(t, file)
	}

	triageText := mustReadFile(t, "docs/support/triage.md")
	requireSnippetsFold(t, triageText,
		"diagnostic-summary",
		"issue template",
		"first-pass",
	)
}
