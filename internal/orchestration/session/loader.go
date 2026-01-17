// Package session provides session tracking for orchestration mode.
// loader.go provides functions for loading persisted session data from JSONL files.
package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zjrosen/xorchestrator/internal/orchestration/message"
	"github.com/zjrosen/xorchestrator/internal/ui/shared/chatrender"
)

// maxLineSize is the buffer size for reading JSONL lines.
// Set to 1MB to handle long message content.
const maxLineSize = 1024 * 1024

// LoadCoordinatorMessages loads the coordinator's chat history from messages.jsonl.
// Returns an empty slice if the file doesn't exist (session may have no messages yet).
// Malformed JSON lines are skipped gracefully.
func LoadCoordinatorMessages(sessionDir string) ([]chatrender.Message, error) {
	path := filepath.Join(sessionDir, coordinatorDir, chatMessagesFile)
	return loadMessagesJSONL(path)
}

// LoadWorkerMessages loads a worker's chat history from messages.jsonl.
// Returns an empty slice if the file doesn't exist (worker may not have output yet).
// Malformed JSON lines are skipped gracefully.
func LoadWorkerMessages(sessionDir, workerID string) ([]chatrender.Message, error) {
	path := filepath.Join(sessionDir, workersDir, workerID, chatMessagesFile)
	return loadMessagesJSONL(path)
}

// LoadInterAgentMessages loads the inter-agent message log.
// Returns an empty slice if the file doesn't exist.
// Malformed JSON lines are skipped gracefully.
func LoadInterAgentMessages(sessionDir string) ([]message.Entry, error) {
	path := filepath.Join(sessionDir, messagesJSONLFile)
	return loadInterAgentMessagesJSONL(path)
}

// loadMessagesJSONL is the internal implementation for loading chat messages from a JSONL file.
// Returns an empty slice if the file doesn't exist.
// Malformed JSON lines are skipped gracefully to provide resilience against partial writes.
func loadMessagesJSONL(path string) ([]chatrender.Message, error) {
	file, err := os.Open(path) //nolint:gosec // path is constructed internally from session directory
	if err != nil {
		if os.IsNotExist(err) {
			return []chatrender.Message{}, nil
		}
		return nil, fmt.Errorf("opening messages file: %w", err)
	}
	defer func() { _ = file.Close() }()

	var messages []chatrender.Message
	scanner := bufio.NewScanner(file)

	// Increase buffer size for potentially long lines
	buf := make([]byte, maxLineSize)
	scanner.Buffer(buf, maxLineSize)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue // Skip empty lines
		}

		var msg chatrender.Message
		if err := json.Unmarshal(line, &msg); err != nil {
			// Log warning but continue - don't fail on one bad line
			// This provides resilience against partial writes
			continue
		}
		messages = append(messages, msg)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning messages file: %w", err)
	}

	// Ensure we return an empty slice, not nil
	if messages == nil {
		messages = []chatrender.Message{}
	}

	return messages, nil
}

// ValidationError represents an error during session validation for resumption.
// It contains details about which field failed validation and why.
type ValidationError struct {
	// Reason explains why validation failed.
	Reason string
	// Field identifies which metadata field caused the validation failure.
	Field string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return "session validation failed: " + e.Reason + " (field: " + e.Field + ")"
}

// ValidateForResumption checks if a session can safely be resumed.
// Returns a ValidationError if the session cannot be resumed.
//
// Validation rules:
// - metadata.Resumable must be true
// - metadata.CoordinatorSessionRef must not be empty
// - metadata.Status must not be StatusRunning (only Completed, Failed, TimedOut are allowed)
func ValidateForResumption(metadata *Metadata) error {
	if metadata == nil {
		return &ValidationError{
			Reason: "metadata is nil",
			Field:  "metadata",
		}
	}

	if !metadata.Resumable {
		return &ValidationError{
			Reason: "session is not marked as resumable",
			Field:  "Resumable",
		}
	}

	if metadata.CoordinatorSessionRef == "" {
		return &ValidationError{
			Reason: "coordinator session reference is missing",
			Field:  "CoordinatorSessionRef",
		}
	}

	if metadata.Status == StatusRunning {
		return &ValidationError{
			Reason: "cannot resume a running session",
			Field:  "Status",
		}
	}

	return nil
}

// IsResumable is a convenience wrapper that returns true if the session can be resumed.
// It returns false if metadata is nil or validation fails.
func IsResumable(metadata *Metadata) bool {
	return ValidateForResumption(metadata) == nil
}

// ResumableSession contains all data needed to resume an orchestration session.
// It aggregates metadata, chat messages, and inter-agent messages into a single
// structure suitable for populating the TUI.
type ResumableSession struct {
	// Metadata contains the session configuration and state.
	Metadata *Metadata

	// CoordinatorMessages contains the coordinator's chat history.
	CoordinatorMessages []chatrender.Message

	// WorkerMessages maps worker IDs to their chat histories.
	WorkerMessages map[string][]chatrender.Message

	// InterAgentMessages contains the inter-agent communication log.
	InterAgentMessages []message.Entry

	// ActiveWorkers are workers that have not been retired (RetiredAt.IsZero()).
	ActiveWorkers []WorkerMetadata

	// RetiredWorkers are workers that have been retired, sorted by RetiredAt ascending (oldest first).
	RetiredWorkers []WorkerMetadata
}

// LoadResumableSession loads all session data needed for resumption.
// It aggregates metadata, coordinator messages, worker messages, and inter-agent messages.
// Returns an error if metadata cannot be loaded or validation fails.
func LoadResumableSession(sessionDir string) (*ResumableSession, error) {
	// Load and validate metadata
	metadata, err := Load(sessionDir)
	if err != nil {
		return nil, fmt.Errorf("loading metadata: %w", err)
	}

	if err := ValidateForResumption(metadata); err != nil {
		return nil, err
	}

	// Load coordinator messages
	coordMsgs, err := LoadCoordinatorMessages(sessionDir)
	if err != nil {
		return nil, fmt.Errorf("loading coordinator messages: %w", err)
	}

	// Load worker messages for each worker in metadata
	workerMsgs := make(map[string][]chatrender.Message)
	for _, worker := range metadata.Workers {
		msgs, err := LoadWorkerMessages(sessionDir, worker.ID)
		if err != nil {
			return nil, fmt.Errorf("loading worker %s messages: %w", worker.ID, err)
		}
		workerMsgs[worker.ID] = msgs
	}

	// Load inter-agent messages
	interAgentMsgs, err := LoadInterAgentMessages(sessionDir)
	if err != nil {
		return nil, fmt.Errorf("loading inter-agent messages: %w", err)
	}

	// Partition workers into active and retired
	var activeWorkers, retiredWorkers []WorkerMetadata
	for _, worker := range metadata.Workers {
		if worker.RetiredAt.IsZero() {
			activeWorkers = append(activeWorkers, worker)
		} else {
			retiredWorkers = append(retiredWorkers, worker)
		}
	}

	// Sort retired workers by RetiredAt ascending (oldest first)
	sortWorkersByRetiredAt(retiredWorkers)

	// Ensure slices are not nil
	if activeWorkers == nil {
		activeWorkers = []WorkerMetadata{}
	}
	if retiredWorkers == nil {
		retiredWorkers = []WorkerMetadata{}
	}

	return &ResumableSession{
		Metadata:            metadata,
		CoordinatorMessages: coordMsgs,
		WorkerMessages:      workerMsgs,
		InterAgentMessages:  interAgentMsgs,
		ActiveWorkers:       activeWorkers,
		RetiredWorkers:      retiredWorkers,
	}, nil
}

// sortWorkersByRetiredAt sorts workers by their RetiredAt time in ascending order (oldest first).
func sortWorkersByRetiredAt(workers []WorkerMetadata) {
	for i := 0; i < len(workers)-1; i++ {
		for j := i + 1; j < len(workers); j++ {
			if workers[j].RetiredAt.Before(workers[i].RetiredAt) {
				workers[i], workers[j] = workers[j], workers[i]
			}
		}
	}
}

// loadInterAgentMessagesJSONL loads inter-agent message entries from a JSONL file.
// Returns an empty slice if the file doesn't exist.
// Malformed JSON lines are skipped gracefully to provide resilience against partial writes.
func loadInterAgentMessagesJSONL(path string) ([]message.Entry, error) {
	file, err := os.Open(path) //nolint:gosec // path is constructed internally from session directory
	if err != nil {
		if os.IsNotExist(err) {
			return []message.Entry{}, nil
		}
		return nil, fmt.Errorf("opening inter-agent messages file: %w", err)
	}
	defer func() { _ = file.Close() }()

	var entries []message.Entry
	scanner := bufio.NewScanner(file)

	// Increase buffer size for potentially long lines
	buf := make([]byte, maxLineSize)
	scanner.Buffer(buf, maxLineSize)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue // Skip empty lines
		}

		var entry message.Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			// Skip malformed lines
			continue
		}
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning inter-agent messages file: %w", err)
	}

	// Ensure we return an empty slice, not nil
	if entries == nil {
		entries = []message.Entry{}
	}

	return entries, nil
}
