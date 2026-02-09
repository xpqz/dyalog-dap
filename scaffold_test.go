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

func TestScaffold_HasExtensionVSIXPackagingScripts(t *testing.T) {
	data, err := os.ReadFile("vscode-extension/package.json")
	if err != nil {
		t.Fatalf("missing vscode-extension/package.json: %v", err)
	}
	var pkg struct {
		Scripts         map[string]string `json:"scripts"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatalf("vscode-extension/package.json is not valid JSON: %v", err)
	}
	requiredScripts := []string{"package:vsix", "check:manifest"}
	for _, script := range requiredScripts {
		if strings.TrimSpace(pkg.Scripts[script]) == "" {
			t.Fatalf("expected vscode-extension package script %q", script)
		}
	}
	if strings.TrimSpace(pkg.DevDependencies["@vscode/vsce"]) == "" {
		t.Fatal("expected vscode-extension devDependency @vscode/vsce")
	}
}

func TestScaffold_HasExtensionReleaseWorkflow(t *testing.T) {
	data, err := os.ReadFile(".github/workflows/extension-release.yml")
	if err != nil {
		t.Fatalf("missing .github/workflows/extension-release.yml: %v", err)
	}
	text := string(data)
	requiredSnippets := []string{
		"name: extension-release",
		"tags:",
		"v*",
		"working-directory: vscode-extension",
		"npm run package:vsix",
		"actions/upload-artifact",
		".vsix",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(text, snippet) {
			t.Fatalf("expected extension release workflow to contain %q", snippet)
		}
	}
}

func TestScaffold_HasExtensionReleaseChecklistAndCompatibilityGuidance(t *testing.T) {
	checklist, err := os.ReadFile("docs/releases/extension-vsix.md")
	if err != nil {
		t.Fatalf("missing docs/releases/extension-vsix.md: %v", err)
	}
	checklistText := string(checklist)
	requiredChecklistSnippets := []string{
		"# Extension VSIX Release Checklist",
		"Marketplace",
		"GitHub",
		"adapter",
	}
	for _, snippet := range requiredChecklistSnippets {
		if !strings.Contains(checklistText, snippet) {
			t.Fatalf("expected extension checklist to contain %q", snippet)
		}
	}

	extensionReadme, err := os.ReadFile("vscode-extension/README.md")
	if err != nil {
		t.Fatalf("missing vscode-extension/README.md: %v", err)
	}
	readmeText := string(extensionReadme)
	requiredReadmeSnippets := []string{
		"dap-adapter",
		"DYALOG_DAP_ADAPTER_PATH",
		"compatibility",
	}
	for _, snippet := range requiredReadmeSnippets {
		if !strings.Contains(strings.ToLower(readmeText), strings.ToLower(snippet)) {
			t.Fatalf("expected vscode-extension/README.md to contain %q", snippet)
		}
	}
}

func TestScaffold_HasExtensionSetupAndDiagnosticsCommands(t *testing.T) {
	data, err := os.ReadFile("vscode-extension/package.json")
	if err != nil {
		t.Fatalf("missing vscode-extension/package.json: %v", err)
	}

	var pkg struct {
		ActivationEvents []string `json:"activationEvents"`
		Contributes      struct {
			Commands []struct {
				Command string `json:"command"`
				Title   string `json:"title"`
			} `json:"commands"`
			Configuration struct {
				Properties map[string]struct {
					Type        string `json:"type"`
					Default     bool   `json:"default"`
					Description string `json:"description"`
				} `json:"properties"`
			} `json:"configuration"`
		} `json:"contributes"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatalf("vscode-extension/package.json is not valid JSON: %v", err)
	}

	requiredCommands := []string{
		"dyalogDap.setupLaunchConfig",
		"dyalogDap.validateAdapterPath",
		"dyalogDap.validateRideAddr",
		"dyalogDap.toggleDiagnosticsVerbose",
	}
	for _, commandID := range requiredCommands {
		found := false
		for _, command := range pkg.Contributes.Commands {
			if command.Command == commandID && strings.TrimSpace(command.Title) != "" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected contributes.commands entry for %q", commandID)
		}

		activationEvent := "onCommand:" + commandID
		eventFound := false
		for _, event := range pkg.ActivationEvents {
			if event == activationEvent {
				eventFound = true
				break
			}
		}
		if !eventFound {
			t.Fatalf("expected activation event %q", activationEvent)
		}
	}

	verboseSetting := pkg.Contributes.Configuration.Properties["dyalogDap.diagnostics.verbose"]
	if verboseSetting.Type != "boolean" {
		t.Fatal("expected configuration property dyalogDap.diagnostics.verbose of type boolean")
	}
	if !strings.Contains(strings.ToLower(verboseSetting.Description), "diagnostic") {
		t.Fatal("expected diagnostics verbosity setting description")
	}
}

func TestScaffold_HasExtensionDiagnosticsOutputChannel(t *testing.T) {
	data, err := os.ReadFile("vscode-extension/src/extension.ts")
	if err != nil {
		t.Fatalf("missing vscode-extension/src/extension.ts: %v", err)
	}
	text := string(data)
	requiredSnippets := []string{
		"createOutputChannel(\"Dyalog DAP\")",
		"dyalogDap.setupLaunchConfig",
		"dyalogDap.validateAdapterPath",
		"dyalogDap.validateRideAddr",
		"dyalogDap.toggleDiagnosticsVerbose",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(text, snippet) {
			t.Fatalf("expected extension source to contain %q", snippet)
		}
	}
}

func TestScaffold_HasExtensionContractTestsAndGate(t *testing.T) {
	data, err := os.ReadFile("vscode-extension/package.json")
	if err != nil {
		t.Fatalf("missing vscode-extension/package.json: %v", err)
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatalf("vscode-extension/package.json is not valid JSON: %v", err)
	}
	if strings.TrimSpace(pkg.Scripts["test:contracts"]) == "" {
		t.Fatal("expected vscode-extension package script test:contracts")
	}

	if _, err := os.Stat("vscode-extension/src/test/contracts.test.ts"); err != nil {
		t.Fatalf("missing vscode-extension/src/test/contracts.test.ts: %v", err)
	}

	ci, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("missing .github/workflows/ci.yml: %v", err)
	}
	if !strings.Contains(string(ci), "npm run test:contracts") {
		t.Fatal("expected CI extension package job to run npm run test:contracts")
	}
}

func TestScaffold_HasLiveInteractiveE2EAutomationAndFlakePolicy(t *testing.T) {
	ci, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("missing .github/workflows/ci.yml: %v", err)
	}
	ciText := string(ci)
	requiredCISnippets := []string{
		"DYALOG_E2E_REQUIRE",
		"DYALOG_E2E_TIMEOUT",
		"Upload live integration artifacts",
		"artifacts/integration",
		"TestLiveDAPAdapter_",
	}
	for _, snippet := range requiredCISnippets {
		if !strings.Contains(ciText, snippet) {
			t.Fatalf("expected live CI workflow snippet %q", snippet)
		}
	}

	if _, err := os.Stat("cmd/dap-adapter/live_e2e_test.go"); err != nil {
		t.Fatalf("missing cmd/dap-adapter/live_e2e_test.go: %v", err)
	}
	liveE2E, err := os.ReadFile("cmd/dap-adapter/live_e2e_test.go")
	if err != nil {
		t.Fatalf("read cmd/dap-adapter/live_e2e_test.go failed: %v", err)
	}
	if !strings.Contains(string(liveE2E), "TestLiveDAPAdapter_InteractiveWorkflow") {
		t.Fatal("expected live E2E interactive workflow test in cmd/dap-adapter/live_e2e_test.go")
	}

	policy, err := os.ReadFile("docs/validations/54-live-e2e-flake-policy.md")
	if err != nil {
		t.Fatalf("missing docs/validations/54-live-e2e-flake-policy.md: %v", err)
	}
	policyText := string(policy)
	requiredPolicySnippets := []string{
		"infrastructure-flake",
		"scenario-precondition",
		"product-defect",
	}
	for _, snippet := range requiredPolicySnippets {
		if !strings.Contains(policyText, snippet) {
			t.Fatalf("expected flake policy to contain %q", snippet)
		}
	}
}
