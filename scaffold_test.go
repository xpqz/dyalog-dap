package lspdap_test

import (
	"os"
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
	if _, err := os.Stat(".golangci.yml"); err != nil {
		t.Fatalf("missing .golangci.yml: %v", err)
	}
}
