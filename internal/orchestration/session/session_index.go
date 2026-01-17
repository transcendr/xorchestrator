package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// SessionIndexVersion is the current schema version for the session index.
	SessionIndexVersion = "1.0"
)

// SessionIndex tracks all sessions in a xorchestrator sessions directory.
type SessionIndex struct {
	// Version is the schema version for forward compatibility.
	Version string `json:"version"`

	// Sessions is the list of all sessions in chronological order.
	Sessions []SessionIndexEntry `json:"sessions"`
}

// SessionIndexEntry contains summary information about a single session.
type SessionIndexEntry struct {
	// ID is the unique session identifier (UUID).
	ID string `json:"id"`

	// StartTime is when the session was created.
	StartTime time.Time `json:"start_time"`

	// EndTime is when the session ended (zero if still running).
	EndTime time.Time `json:"end_time,omitzero"`

	// Status is the session's final status.
	Status Status `json:"status"`

	// EpicID is the bd epic ID associated with this session (if any).
	EpicID string `json:"epic_id,omitempty"`

	// SessionDir is the session storage directory path.
	SessionDir string `json:"session_dir"`

	// AccountabilitySummaryPath is the path to the aggregated accountability summary.
	AccountabilitySummaryPath string `json:"accountability_summary_path,omitempty"`

	// WorkerCount is the number of workers that participated in this session.
	WorkerCount int `json:"worker_count"`

	// TasksCompleted is the number of tasks completed during this session.
	TasksCompleted int `json:"tasks_completed"`

	// TotalCommits is the number of commits made during this session.
	TotalCommits int `json:"total_commits"`

	// ApplicationName is the derived or configured name for the application.
	// Used for organizing sessions in centralized storage.
	ApplicationName string `json:"application_name,omitempty"`

	// WorkDir is the project working directory where the session was initiated.
	// This preserves the actual project location even when using git worktrees.
	WorkDir string `json:"work_dir,omitempty"`

	// DatePartition is the date-based partition (YYYY-MM-DD format) for organizing sessions.
	DatePartition string `json:"date_partition,omitempty"`
}

// LoadSessionIndex loads an existing session index from the given path.
// If the file doesn't exist, it returns an empty index with the current version.
// If the file exists but contains invalid JSON, it returns an error.
func LoadSessionIndex(path string) (*SessionIndex, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is trusted input from caller
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty index for missing file
			return &SessionIndex{
				Version:  SessionIndexVersion,
				Sessions: []SessionIndexEntry{},
			}, nil
		}
		return nil, fmt.Errorf("reading session index: %w", err)
	}

	var index SessionIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("parsing session index: %w", err)
	}

	return &index, nil
}

// ApplicationSessionIndex tracks sessions for a specific application.
// This provides per-application session organization at {baseDir}/{appName}/sessions.json.
type ApplicationSessionIndex struct {
	// Version is the schema version for forward compatibility.
	Version string `json:"version"`

	// ApplicationName is the name of the application (derived from git remote or directory).
	ApplicationName string `json:"application_name"`

	// Sessions is the list of all sessions for this application in chronological order.
	Sessions []SessionIndexEntry `json:"sessions"`
}

// LoadApplicationIndex loads an existing application session index from the given path.
// If the file doesn't exist, it returns an empty index with the current version.
// If the file exists but contains invalid JSON, it returns an error.
func LoadApplicationIndex(path string) (*ApplicationSessionIndex, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is trusted input from caller
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty index for missing file
			return &ApplicationSessionIndex{
				Version:  SessionIndexVersion,
				Sessions: []SessionIndexEntry{},
			}, nil
		}
		return nil, fmt.Errorf("reading application session index: %w", err)
	}

	var index ApplicationSessionIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("parsing application session index: %w", err)
	}

	return &index, nil
}

// SaveApplicationIndex writes the application session index to the given path using atomic rename.
// It writes to a temporary file first, then renames to the final path to ensure
// the file is never in a partially-written state.
func SaveApplicationIndex(path string, index *ApplicationSessionIndex) error {
	// Marshal to JSON with indentation for readability
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling application session index: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("creating application index directory: %w", err)
	}

	// Write to temporary file in the same directory (required for atomic rename)
	tmpFile, err := os.CreateTemp(dir, "sessions.*.tmp")
	if err != nil {
		return fmt.Errorf("creating temporary application session index file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Write data and close the file
	_, writeErr := tmpFile.Write(data)
	closeErr := tmpFile.Close()
	if writeErr != nil {
		_ = os.Remove(tmpPath) // best effort cleanup
		return fmt.Errorf("writing temporary application session index: %w", writeErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath) // best effort cleanup
		return fmt.Errorf("closing temporary application session index: %w", closeErr)
	}

	// Atomic rename to final path
	if err := os.Rename(tmpPath, path); err != nil {
		// Clean up temp file on rename failure
		_ = os.Remove(tmpPath) // best effort cleanup
		return fmt.Errorf("renaming application session index: %w", err)
	}

	return nil
}

// SaveSessionIndex writes the session index to the given path using atomic rename.
// It writes to a temporary file first, then renames to the final path to ensure
// the file is never in a partially-written state.
func SaveSessionIndex(path string, index *SessionIndex) error {
	// Marshal to JSON with indentation for readability
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling session index: %w", err)
	}

	// Write to temporary file in the same directory (required for atomic rename)
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, "sessions.*.tmp")
	if err != nil {
		return fmt.Errorf("creating temporary session index file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Write data and close the file
	_, writeErr := tmpFile.Write(data)
	closeErr := tmpFile.Close()
	if writeErr != nil {
		_ = os.Remove(tmpPath) // best effort cleanup
		return fmt.Errorf("writing temporary session index: %w", writeErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath) // best effort cleanup
		return fmt.Errorf("closing temporary session index: %w", closeErr)
	}

	// Atomic rename to final path
	if err := os.Rename(tmpPath, path); err != nil {
		// Clean up temp file on rename failure
		_ = os.Remove(tmpPath) // best effort cleanup
		return fmt.Errorf("renaming session index: %w", err)
	}

	return nil
}
