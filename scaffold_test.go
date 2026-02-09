package lspdap_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
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
	if _, err := os.Stat(".golangci.yml"); err != nil {
		t.Fatalf("missing .golangci.yml: %v", err)
	}
}

func TestScaffold_HasVSCodeLaunchSmokeConfig(t *testing.T) {
	data, err := os.ReadFile(".vscode/launch.json")
	if err != nil {
		t.Fatalf("missing .vscode/launch.json: %v", err)
	}

	var launch struct {
		Configurations []struct {
			Name    string `json:"name"`
			Type    string `json:"type"`
			Request string `json:"request"`
		} `json:"configurations"`
	}
	if err := json.Unmarshal(data, &launch); err != nil {
		t.Fatalf("launch.json is not valid JSON: %v", err)
	}

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
	data, err := os.ReadFile(".vscode/tasks.json")
	if err != nil {
		t.Fatalf("missing .vscode/tasks.json: %v", err)
	}

	var tasks struct {
		Tasks []struct {
			Label string `json:"label"`
			Type  string `json:"type"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(data, &tasks); err != nil {
		t.Fatalf("tasks.json is not valid JSON: %v", err)
	}

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
	data, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("missing .github/workflows/ci.yml: %v", err)
	}
	text := string(data)

	requiredSnippets := []string{
		"name: ci",
		"critical-gate:",
		"live-dyalog:",
		"DYALOG_RIDE_ADDR",
		"go test ./...",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(text, snippet) {
			t.Fatalf("expected CI workflow to contain %q", snippet)
		}
	}
}

func TestScaffold_HasAssumptionsTraceabilityDoc(t *testing.T) {
	data, err := os.ReadFile("docs/traceability/assumptions.md")
	if err != nil {
		t.Fatalf("missing docs/traceability/assumptions.md: %v", err)
	}
	text := string(data)
	requiredSnippets := []string{
		"# Implementation Assumptions and Plan Traceability",
		"## Plan References",
		"## Feature Traceability Matrix",
		"## Validated Deviations",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(text, snippet) {
			t.Fatalf("expected assumptions doc to contain %q", snippet)
		}
	}
}

func TestScaffold_HasGoreleaserConfig(t *testing.T) {
	data, err := os.ReadFile(".goreleaser.yaml")
	if err != nil {
		t.Fatalf("missing .goreleaser.yaml: %v", err)
	}
	text := string(data)
	requiredSnippets := []string{
		"project_name: dyalog-dap",
		"main: ./cmd/dap-adapter",
		"checksum:",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(text, snippet) {
			t.Fatalf("expected goreleaser config to contain %q", snippet)
		}
	}
}

func TestScaffold_HasReleaseWorkflow(t *testing.T) {
	data, err := os.ReadFile(".github/workflows/release.yml")
	if err != nil {
		t.Fatalf("missing .github/workflows/release.yml: %v", err)
	}
	text := string(data)
	requiredSnippets := []string{
		"name: release",
		"goreleaser/goreleaser-action",
		"tags:",
		"v*",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(text, snippet) {
			t.Fatalf("expected release workflow to contain %q", snippet)
		}
	}
}

func TestScaffold_HasVSCodeExtensionDebuggerContribution(t *testing.T) {
	data, err := os.ReadFile("vscode-extension/package.json")
	if err != nil {
		t.Fatalf("missing vscode-extension/package.json: %v", err)
	}

	var pkg struct {
		Main         string `json:"main"`
		Engines      struct {
			VSCode string `json:"vscode"`
		} `json:"engines"`
		ActivationEvents []string `json:"activationEvents"`
		Contributes      struct {
			Debuggers []struct {
				Type    string `json:"type"`
				Label   string `json:"label"`
				Program string `json:"program"`
			} `json:"debuggers"`
		} `json:"contributes"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatalf("vscode-extension/package.json is not valid JSON: %v", err)
	}

	if pkg.Main == "" {
		t.Fatal("expected extension main entry in vscode-extension/package.json")
	}
	if pkg.Engines.VSCode == "" {
		t.Fatal("expected engines.vscode in vscode-extension/package.json")
	}

	hasActivation := false
	for _, event := range pkg.ActivationEvents {
		if event == "onDebug:dyalog-dap" {
			hasActivation = true
			break
		}
	}
	if !hasActivation {
		t.Fatal("expected activation event onDebug:dyalog-dap")
	}

	hasDebugger := false
	for _, debugger := range pkg.Contributes.Debuggers {
		if debugger.Type == "dyalog-dap" && debugger.Label != "" {
			hasDebugger = true
			break
		}
	}
	if !hasDebugger {
		t.Fatal("expected debugger contribution for type dyalog-dap")
	}
}

func TestScaffold_HasVSCodeExtensionEntrypoint(t *testing.T) {
	if _, err := os.Stat("vscode-extension/out/extension.js"); err != nil {
		t.Fatalf("missing vscode-extension/out/extension.js: %v", err)
	}
}

func TestScaffold_HasVSCodeExtensionTypeScriptPipeline(t *testing.T) {
	requiredFiles := []string{
		"vscode-extension/src/extension.ts",
		"vscode-extension/tsconfig.json",
	}
	for _, file := range requiredFiles {
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("missing extension pipeline file %s: %v", file, err)
		}
	}

	data, err := os.ReadFile("vscode-extension/package.json")
	if err != nil {
		t.Fatalf("missing vscode-extension/package.json: %v", err)
	}
	var pkg struct {
		Main    string            `json:"main"`
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatalf("vscode-extension/package.json is not valid JSON: %v", err)
	}
	if pkg.Main != "./out/extension.js" {
		t.Fatalf("expected extension main ./out/extension.js, got %q", pkg.Main)
	}
	requiredScripts := []string{"build", "lint", "test"}
	for _, script := range requiredScripts {
		if strings.TrimSpace(pkg.Scripts[script]) == "" {
			t.Fatalf("expected vscode-extension package script %q", script)
		}
	}
}

func TestScaffold_HasCIExtensionPackageJob(t *testing.T) {
	data, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("missing .github/workflows/ci.yml: %v", err)
	}
	text := string(data)
	requiredSnippets := []string{
		"extension-package:",
		"working-directory: vscode-extension",
		"npm run lint",
		"npm run test",
		"npm run build",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(text, snippet) {
			t.Fatalf("expected CI workflow to contain %q", snippet)
		}
	}
}
