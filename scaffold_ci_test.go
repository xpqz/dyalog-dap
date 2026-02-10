package lspdap_test

import (
	"os/exec"
	"testing"

	dapadapter "github.com/stefan/lsp-dap/internal/dap/adapter"
	rideprotocol "github.com/stefan/lsp-dap/internal/ride/protocol"
	ridesession "github.com/stefan/lsp-dap/internal/ride/sessionstate"
	ridetransport "github.com/stefan/lsp-dap/internal/ride/transport"
)

func TestScaffold_PackagesExposeEntryPoints(t *testing.T) {
	if ridetransport.NewClient() == nil {
		t.Fatal("transport.NewClient must return a client")
	}
	if rideprotocol.NewCodec() == nil {
		t.Fatal("protocol.NewCodec must return a codec")
	}
	if ridesession.NewState() == nil {
		t.Fatal("sessionstate.NewState must return state")
	}
	if dapadapter.NewServer() == nil {
		t.Fatal("adapter.NewServer must return a server")
	}
}

func TestScaffold_CommandBuilds(t *testing.T) {
	outPath := t.TempDir() + "/dap-adapter"
	cmd := exec.Command("go", "build", "-o", outPath, "./cmd/dap-adapter")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected cmd/dap-adapter to build: %v\n%s", err, output)
	}
}

func TestScaffold_HasLintConfig(t *testing.T) {
	mustFileExists(t, ".golangci.yml")
}

func TestScaffold_HasVSCodeLaunchSmokeConfig(t *testing.T) {
	launch := mustUnmarshalJSONFile[struct {
		Configurations []struct {
			Name    string `json:"name"`
			Type    string `json:"type"`
			Request string `json:"request"`
		} `json:"configurations"`
	}](t, ".vscode/launch.json")

	found := false
	for _, cfg := range launch.Configurations {
		if cfg.Name == "DAP Adapter Smoke (Go)" && cfg.Type == "go" && cfg.Request == "launch" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected VS Code launch config 'DAP Adapter Smoke (Go)'")
	}
}

func TestScaffold_HasVSCodeSmokeTasks(t *testing.T) {
	tasks := mustUnmarshalJSONFile[struct {
		Tasks []struct {
			Label string `json:"label"`
			Type  string `json:"type"`
		} `json:"tasks"`
	}](t, ".vscode/tasks.json")

	found := false
	for _, task := range tasks.Tasks {
		if task.Label == "build dap-adapter" && task.Type == "shell" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected VS Code task 'build dap-adapter'")
	}
}

func TestScaffold_HasCIWorkflowWithCriticalAndLiveJobs(t *testing.T) {
	text := mustReadFile(t, ".github/workflows/ci.yml")
	requireSnippets(t, text,
		"name: ci",
		"critical-gate:",
		"live-dyalog:",
		"DYALOG_RIDE_ADDR",
		"go test ./...",
	)
}

func TestScaffold_HasAssumptionsTraceabilityDoc(t *testing.T) {
	text := mustReadFile(t, "docs/traceability/assumptions.md")
	requireSnippets(t, text,
		"# Implementation Assumptions and Plan Traceability",
		"## Plan References",
		"## Feature Traceability Matrix",
		"## Validated Deviations",
	)
}

func TestScaffold_HasCIExtensionPackageJob(t *testing.T) {
	text := mustReadFile(t, ".github/workflows/ci.yml")
	requireSnippets(t, text,
		"extension-package:",
		"working-directory: vscode-extension",
		"npm run lint",
		"npm run test",
		"npm run build",
	)
}

func TestScaffold_HasLiveInteractiveE2EAutomationAndFlakePolicy(t *testing.T) {
	ciText := mustReadFile(t, ".github/workflows/ci.yml")
	requireSnippets(t, ciText,
		"DYALOG_E2E_REQUIRE",
		"DYALOG_E2E_TIMEOUT",
		"Upload live integration artifacts",
		"artifacts/integration",
		"TestLiveDAPAdapter_",
	)

	mustFileExists(t, "cmd/dap-adapter/live_e2e_test.go")
	liveE2EText := mustReadFile(t, "cmd/dap-adapter/live_e2e_test.go")
	requireSnippets(t, liveE2EText, "TestLiveDAPAdapter_InteractiveWorkflow")

	policyText := mustReadFile(t, "docs/validations/54-live-e2e-flake-policy.md")
	requireSnippets(t, policyText,
		"infrastructure-flake",
		"scenario-precondition",
		"product-defect",
	)
}

func TestScaffold_HasLiveCompatibilityMatrixPolicyAndReleaseGate(t *testing.T) {
	policyText := mustReadFile(t, "docs/validations/55-live-ci-matrix.md")
	requireSnippetsFold(t, policyText,
		"# Live CI Matrix Policy",
		"OS",
		"Dyalog",
		"promotion",
		"self-hosted",
		"reliability",
	)

	liveMatrixText := mustReadFile(t, ".github/workflows/live-matrix.yml")
	requireSnippetsFold(t, liveMatrixText,
		"name: live-matrix",
		"workflow_dispatch:",
		"matrix:",
		"profile",
		"linux",
		"macos",
		"windows",
		"self-hosted",
	)

	releaseText := mustReadFile(t, ".github/workflows/release.yml")
	requireSnippetsFold(t, releaseText,
		"LIVE_MATRIX_REQUIRED",
		"live-matrix.yml",
		"release-readiness",
	)
}
