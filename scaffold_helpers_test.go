package lspdap_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func mustFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("missing %s: %v", path, err)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s failed: %v", path, err)
	}
	return string(data)
}

func mustUnmarshalJSONFile[T any](t *testing.T, path string) T {
	t.Helper()
	var out T
	data := mustReadFile(t, path)
	if err := json.Unmarshal([]byte(data), &out); err != nil {
		t.Fatalf("%s is not valid JSON: %v", path, err)
	}
	return out
}

func requireSnippets(t *testing.T, text string, snippets ...string) {
	t.Helper()
	for _, snippet := range snippets {
		if !strings.Contains(text, snippet) {
			t.Fatalf("expected text to contain %q", snippet)
		}
	}
}

func requireSnippetsFold(t *testing.T, text string, snippets ...string) {
	t.Helper()
	folded := strings.ToLower(text)
	for _, snippet := range snippets {
		if !strings.Contains(folded, strings.ToLower(snippet)) {
			t.Fatalf("expected text to contain %q (case-insensitive)", snippet)
		}
	}
}
