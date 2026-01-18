package texthistory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHistoryPersistence_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")

	h := NewHistoryManager(5)
	h.Add("one")
	h.Add("two")

	if err := SaveHistory(path, h); err != nil {
		t.Fatalf("SaveHistory error: %v", err)
	}

	loaded, err := LoadHistory(path, 5)
	if err != nil {
		t.Fatalf("LoadHistory error: %v", err)
	}

	entries := loaded.Entries()
	if len(entries) != 2 || entries[0] != "one" || entries[1] != "two" {
		t.Fatalf("entries mismatch: %#v", entries)
	}
}

func TestHistoryPersistence_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")

	h, err := LoadHistory(path, 3)
	if err != nil {
		t.Fatalf("LoadHistory error: %v", err)
	}
	if h.Len() != 0 {
		t.Fatalf("expected empty history, got %d", h.Len())
	}
}

func TestHistoryPersistence_InvalidPath(t *testing.T) {
	if err := SaveHistory("", NewHistoryManager(3)); err == nil {
		t.Fatalf("expected error for empty path")
	}
}

func TestHistoryPersistence_UsesSnapshotMaxSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")

	h := NewHistoryManager(2)
	h.Add("a")
	h.Add("b")
	if err := SaveHistory(path, h); err != nil {
		t.Fatalf("SaveHistory error: %v", err)
	}

	loaded, err := LoadHistory(path, 10)
	if err != nil {
		t.Fatalf("LoadHistory error: %v", err)
	}

	loaded.Add("c")
	entries := loaded.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected max size 2, got %d", len(entries))
	}
}

func TestHistoryPersistence_Mkdirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "history.json")

	h := NewHistoryManager(3)
	h.Add("x")

	if err := SaveHistory(path, h); err != nil {
		t.Fatalf("SaveHistory error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}
