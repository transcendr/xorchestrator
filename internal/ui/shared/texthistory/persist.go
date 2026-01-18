package texthistory

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const historySnapshotVersion = 1

type historySnapshot struct {
	Version   int      `json:"version"`
	MaxSize   int      `json:"max_size"`
	Entries   []string `json:"entries"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

// LoadHistory loads history entries from a JSON snapshot file.
// If the file does not exist, returns an empty HistoryManager.
func LoadHistory(path string, fallbackMax int) (*HistoryManager, error) {
	if fallbackMax <= 0 {
		fallbackMax = DefaultMaxHistory
	}
	h := NewHistoryManager(fallbackMax)

	if path == "" {
		return h, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return h, nil
		}
		return nil, err
	}

	var snap historySnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	if snap.Version != 0 && snap.Version != historySnapshotVersion {
		return nil, errors.New("unsupported history snapshot version")
	}

	if snap.MaxSize > 0 {
		h = NewHistoryManager(snap.MaxSize)
	}

	for _, entry := range snap.Entries {
		h.Add(entry)
	}

	return h, nil
}

// SaveHistory writes history entries to a JSON snapshot file.
// Uses an atomic temp file + rename for safety.
func SaveHistory(path string, history *HistoryManager) error {
	if history == nil {
		return errors.New("history manager is nil")
	}
	if path == "" {
		return errors.New("history path is empty")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	snap := historySnapshot{
		Version:   historySnapshotVersion,
		MaxSize:   history.maxSize,
		Entries:   history.Entries(),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}

	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, path)
}
