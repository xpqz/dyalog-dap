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
