package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/stefan/lsp-dap/internal/support/diagbundle"
)

func main() {
	var jsonOutput bool
	flag.BoolVar(&jsonOutput, "json", false, "print summary as JSON")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: diagnostic-summary [--json] <bundle.json>")
		os.Exit(2)
	}
	path := flag.Arg(0)
	data, err := os.ReadFile(path)
	if err != nil {
		exitErr(fmt.Errorf("read bundle: %w", err))
	}

	summary, err := diagbundle.SummarizeBundle(data)
	if err != nil {
		exitErr(fmt.Errorf("summarize bundle: %w", err))
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(summary); err != nil {
			exitErr(fmt.Errorf("encode summary: %w", err))
		}
		return
	}

	fmt.Printf("Diagnostic Bundle Summary\n")
	fmt.Printf("schema: %s\n", summary.SchemaVersion)
	fmt.Printf("generated: %s\n", summary.GeneratedAt)
	fmt.Printf("extension version: %s\n", summary.ExtensionVersion)
	fmt.Printf("workspace: %s\n", summary.WorkspaceName)
	fmt.Printf("diagnostics count: %d\n", summary.DiagnosticsCount)
	fmt.Printf("problem class: %s\n", summary.ProblemClass)
	if summary.LastDiagnosticLine != "" {
		fmt.Printf("last diagnostic: %s\n", summary.LastDiagnosticLine)
	}
	if len(summary.TranscriptPointers) > 0 {
		fmt.Printf("transcript pointers: %s\n", strings.Join(summary.TranscriptPointers, ", "))
	} else {
		fmt.Printf("transcript pointers: (none)\n")
	}
	fmt.Printf("redaction detected: %t\n", summary.RedactionDetected)
	fmt.Printf("next actions:\n")
	for _, action := range summary.NextActions {
		fmt.Printf("- %s\n", action)
	}
}

func exitErr(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
