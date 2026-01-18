package texthistory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeHistory_LoadsTextFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	lines := []string{
		`{"display":"hello","pastedContents":{},"timestamp":123,"project":"/tmp"}`,
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

	// Parse the JSONL line
	line := strings.TrimSpace(string(data))
	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("failed to parse JSONL: %v", err)
	}

	// Assert Claude Code format fields
	if display, ok := entry["display"].(string); !ok || display != "hello" {
		t.Fatalf("expected display='hello', got %v", entry["display"])
	}
	if _, ok := entry["pastedContents"].(map[string]any); !ok {
		t.Fatalf("expected pastedContents to be a map, got %v", entry["pastedContents"])
	}
	if _, ok := entry["timestamp"].(float64); !ok {
		t.Fatalf("expected timestamp to be numeric, got %v", entry["timestamp"])
	}
	if project, ok := entry["project"].(string); !ok || project == "" {
		t.Fatalf("expected project to be non-empty string, got %v", entry["project"])
	}
}
