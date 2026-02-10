package lspdap_test

import (
	"strings"
	"testing"
)

func TestScaffold_HasVSCodeExtensionDebuggerContribution(t *testing.T) {
	pkg := mustUnmarshalJSONFile[struct {
		Main    string `json:"main"`
		Engines struct {
			VSCode string `json:"vscode"`
		} `json:"engines"`
		ActivationEvents []string `json:"activationEvents"`
		Contributes      struct {
			Debuggers []struct {
				Type    string `json:"type"`
				Label   string `json:"label"`
				Program string `json:"program"`
			} `json:"debuggers"`
			Breakpoints []struct {
				Language string `json:"language"`
			} `json:"breakpoints"`
		} `json:"contributes"`
	}](t, "vscode-extension/package.json")

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

	hasAPLBreakpoints := false
	for _, contribution := range pkg.Contributes.Breakpoints {
		if contribution.Language == "apl" {
			hasAPLBreakpoints = true
			break
		}
	}
	if !hasAPLBreakpoints {
		t.Fatal("expected breakpoints contribution for language apl")
	}
}

func TestScaffold_HasVSCodeExtensionEntrypoint(t *testing.T) {
	mustFileExists(t, "vscode-extension/out/extension.js")
}

func TestScaffold_HasVSCodeExtensionTypeScriptPipeline(t *testing.T) {
	requiredFiles := []string{
		"vscode-extension/src/extension.ts",
		"vscode-extension/src/activation.ts",
		"vscode-extension/tsconfig.json",
	}
	for _, file := range requiredFiles {
		mustFileExists(t, file)
	}

	pkg := mustUnmarshalJSONFile[struct {
		Main    string            `json:"main"`
		Scripts map[string]string `json:"scripts"`
	}](t, "vscode-extension/package.json")

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

func TestScaffold_HasExtensionSetupAndDiagnosticsCommands(t *testing.T) {
	pkg := mustUnmarshalJSONFile[struct {
		ActivationEvents []string `json:"activationEvents"`
		Contributes      struct {
			Commands []struct {
				Command string `json:"command"`
				Title   string `json:"title"`
			} `json:"commands"`
			Configuration struct {
				Properties map[string]struct {
					Type        string `json:"type"`
					Default     any    `json:"default"`
					Description string `json:"description"`
				} `json:"properties"`
			} `json:"configuration"`
		} `json:"contributes"`
	}](t, "vscode-extension/package.json")

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
	text := mustReadFile(t, "vscode-extension/src/activation.ts")
	requireSnippets(t, text,
		"createOutputChannel(\"Dyalog DAP\")",
		"dyalogDap.setupLaunchConfig",
		"dyalogDap.validateAdapterPath",
		"dyalogDap.validateRideAddr",
		"dyalogDap.toggleDiagnosticsVerbose",
	)
}

func TestScaffold_HasExtensionContractTestsAndGate(t *testing.T) {
	pkg := mustUnmarshalJSONFile[struct {
		Scripts map[string]string `json:"scripts"`
	}](t, "vscode-extension/package.json")
	if strings.TrimSpace(pkg.Scripts["test:contracts"]) == "" {
		t.Fatal("expected vscode-extension package script test:contracts")
	}

	mustFileExists(t, "vscode-extension/src/test/contracts.test.ts")

	ciText := mustReadFile(t, ".github/workflows/ci.yml")
	requireSnippets(t, ciText, "npm run test:contracts")
}

func TestScaffold_HasExtensionHostTestHarnessForDiagnosticBundle(t *testing.T) {
	pkg := mustUnmarshalJSONFile[struct {
		Scripts map[string]string `json:"scripts"`
	}](t, "vscode-extension/package.json")
	if _, ok := pkg.Scripts["test:exthost"]; !ok {
		t.Fatalf("expected vscode-extension/package.json to define scripts.test:exthost")
	}

	requiredFiles := []string{
		"vscode-extension/src/test/exthost/runTest.ts",
		"vscode-extension/src/test/exthost/suite/index.ts",
		"vscode-extension/src/test/exthost/suite/diagnosticBundle.exthost.test.ts",
		".github/workflows/vscode-extension-host.yml",
	}
	for _, file := range requiredFiles {
		mustFileExists(t, file)
	}

	text := mustReadFile(t, "vscode-extension/README.md")
	requireSnippetsFold(t, text,
		"extension host",
		"npm run test:exthost",
		"ci",
	)
}

func TestScaffold_HasAdapterInstallCommandAndInstallerTests(t *testing.T) {
	pkg := mustUnmarshalJSONFile[struct {
		ActivationEvents []string `json:"activationEvents"`
		Contributes      struct {
			Commands []struct {
				Command string `json:"command"`
				Title   string `json:"title"`
			} `json:"commands"`
		} `json:"contributes"`
	}](t, "vscode-extension/package.json")

	commandID := "dyalogDap.installAdapter"
	hasCommand := false
	for _, command := range pkg.Contributes.Commands {
		if command.Command == commandID && strings.TrimSpace(command.Title) != "" {
			hasCommand = true
			break
		}
	}
	if !hasCommand {
		t.Fatalf("expected contributes.commands entry for %q", commandID)
	}
	activation := "onCommand:" + commandID
	hasActivation := false
	for _, event := range pkg.ActivationEvents {
		if event == activation {
			hasActivation = true
			break
		}
	}
	if !hasActivation {
		t.Fatalf("expected activation event %q", activation)
	}

	requiredFiles := []string{
		"vscode-extension/src/adapterInstaller.ts",
		"vscode-extension/src/test/adapterInstaller.test.ts",
	}
	for _, file := range requiredFiles {
		mustFileExists(t, file)
	}

	text := mustReadFile(t, "vscode-extension/README.md")
	requireSnippetsFold(t, text,
		"install/update adapter",
		"checksum",
		"github releases",
	)
}
