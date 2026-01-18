package texthistory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeHistory_LoadsTextFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	lines := []string{
		`{"text":"hello"}`,
		`{"prompt":"world"}`,
		`{"messages":[{"role":"user","content":"from-messages"}]}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	h, err := LoadClaudeHistory(path, 10)
	if err != nil {
		t.Fatalf("LoadClaudeHistory error: %v", err)
	}

	entries := h.Entries()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0] != "hello" || entries[1] != "world" || entries[2] != "from-messages" {
		t.Fatalf("unexpected entries: %#v", entries)
	}
}

func TestClaudeHistory_Append(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	if err := AppendClaudeHistory(path, "hello"); err != nil {
		t.Fatalf("AppendClaudeHistory error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	if !strings.Contains(string(data), `"text":"hello"`) {
		t.Fatalf("expected appended text in file: %s", string(data))
	}
}
