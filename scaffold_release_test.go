package lspdap_test

import (
	"strings"
	"testing"
)

func TestScaffold_HasGoreleaserConfig(t *testing.T) {
	text := mustReadFile(t, ".goreleaser.yaml")
	requireSnippets(t, text,
		"project_name: dyalog-dap",
		"main: ./cmd/dap-adapter",
		"checksum:",
	)
}

func TestScaffold_HasReleaseWorkflow(t *testing.T) {
	text := mustReadFile(t, ".github/workflows/release.yml")
	requireSnippets(t, text,
		"name: release",
		"goreleaser/goreleaser-action",
		"tags:",
		"v*",
	)
}

func TestScaffold_HasExtensionVSIXPackagingScripts(t *testing.T) {
	pkg := mustUnmarshalJSONFile[struct {
		Scripts         map[string]string `json:"scripts"`
		DevDependencies map[string]string `json:"devDependencies"`
	}](t, "vscode-extension/package.json")

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
	text := mustReadFile(t, ".github/workflows/extension-release.yml")
	requireSnippets(t, text,
		"name: extension-release",
		"tags:",
		"v*",
		"working-directory: vscode-extension",
		"npm run package:vsix",
		"actions/upload-artifact",
		".vsix",
	)
}

func TestScaffold_HasExtensionReleaseChecklistAndCompatibilityGuidance(t *testing.T) {
	checklistText := mustReadFile(t, "docs/releases/extension-vsix.md")
	requireSnippets(t, checklistText,
		"# Extension VSIX Release Checklist",
		"Marketplace",
		"GitHub",
		"adapter",
	)

	readmeText := mustReadFile(t, "vscode-extension/README.md")
	requireSnippetsFold(t, readmeText,
		"dap-adapter",
		"DYALOG_DAP_ADAPTER_PATH",
		"compatibility",
	)
}

func TestScaffold_HasBetaReadinessPolicyAndSupportMatrix(t *testing.T) {
	text := mustReadFile(t, "docs/validations/beta-readiness.md")
	requireSnippetsFold(t, text,
		"beta readiness",
		"release gate",
		"support matrix",
		"dyalog",
		"vscode extension host",
	)

	readmeText := mustReadFile(t, "README.md")
	requireSnippetsFold(t, readmeText,
		"beta readiness",
		"support matrix",
	)
}

func TestScaffold_HasReleaseChecklistTemplateAndMetadataValidation(t *testing.T) {
	checklistText := mustReadFile(t, "docs/releases/release-checklist.md")
	requireSnippetsFold(t, checklistText,
		"adapter artifacts",
		"extension artifacts",
		"checksums",
		"install",
	)

	templateText := mustReadFile(t, "docs/releases/release-notes-template.md")
	requireSnippetsFold(t, templateText,
		"installation",
		"checksums",
		"support",
	)

	workflowText := mustReadFile(t, ".github/workflows/release.yml")
	requireSnippets(t, workflowText,
		"Validate release artifact metadata",
		"dist/checksums.txt",
		"release-notes-template.md",
	)
}
