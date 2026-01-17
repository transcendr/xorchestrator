// Package session provides session tracking for orchestration mode.
// It persists conversation history, inter-agent messages, and operational logs
// to disk in a UUID-based session folder structure.
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zjrosen/xorchestrator/internal/log"
	"github.com/zjrosen/xorchestrator/internal/orchestration/events"
	"github.com/zjrosen/xorchestrator/internal/orchestration/message"
	"github.com/zjrosen/xorchestrator/internal/pubsub"
	"github.com/zjrosen/xorchestrator/internal/ui/shared/chatrender"
)

// Status represents the session lifecycle state.
type Status string

const (
	// StatusRunning means the session is active.
	StatusRunning Status = "running"
	// StatusCompleted means the session ended normally.
	StatusCompleted Status = "completed"
	// StatusFailed means the session ended due to an error.
	StatusFailed Status = "failed"
	// StatusTimedOut means the session ended due to timeout.
	StatusTimedOut Status = "timed_out"
)

// String returns the string representation of the status.
func (s Status) String() string {
	return string(s)
}

// Session represents an active orchestration session with file handles.
// All writes are centralized through this struct to avoid concurrent file access issues.
type Session struct {
	// ID is the unique session identifier (UUID).
	ID string

	// Dir is the full path to the session folder (.xorchestrator/sessions/{id}).
	Dir string

	// StartTime is when the session was created.
	StartTime time.Time

	// Status is the current session state.
	Status Status

	// BufferedWriters for log files.
	coordRaw       *BufferedWriter            // coordinator/raw.jsonl
	coordMessages  *BufferedWriter            // coordinator/messages.jsonl (structured chat)
	workerRaws     map[string]*BufferedWriter // workerID -> workers/{id}/raw.jsonl
	workerMessages map[string]*BufferedWriter // workerID -> workers/{id}/messages.jsonl
	messageLog     *BufferedWriter            // messages.jsonl (inter-agent messages)
	mcpLog         *BufferedWriter            // mcp_requests.jsonl

	// Metadata for tracking workers and token usage.
	workers    []WorkerMetadata
	tokenUsage TokenUsageSummary

	// Session resumption fields.
	coordinatorSessionRef string
	resumable             bool

	// Application context fields (set via options).
	applicationName string
	workDir         string
	datePartition   string

	// pathBuilder is used for constructing session index paths.
	// Set via WithPathBuilder option.
	pathBuilder *SessionPathBuilder

	// Synchronization.
	mu     sync.Mutex
	closed bool
}

// SessionOption is a functional option for configuring a Session.
type SessionOption func(*Session)

// WithWorkDir sets the project working directory for the session.
// This preserves the actual project location even when using git worktrees.
func WithWorkDir(dir string) SessionOption {
	return func(s *Session) {
		s.workDir = dir
	}
}

// WithApplicationName sets the application name for the session.
// Used for organizing sessions in centralized storage.
func WithApplicationName(name string) SessionOption {
	return func(s *Session) {
		s.applicationName = name
	}
}

// WithDatePartition sets the date partition (YYYY-MM-DD format) for the session.
// Used for organizing sessions by date in centralized storage.
func WithDatePartition(date string) SessionOption {
	return func(s *Session) {
		s.datePartition = date
	}
}

// WithPathBuilder sets the SessionPathBuilder for constructing index paths.
// This enables writing to both application-level and global session indexes.
func WithPathBuilder(pb *SessionPathBuilder) SessionOption {
	return func(s *Session) {
		s.pathBuilder = pb
	}
}

// Directory and file constants for session folder structure.
const (
	// Directory names.
	coordinatorDir = "coordinator"
	workersDir     = "workers"

	// File names.
	rawJSONLFile              = "raw.jsonl"
	messagesJSONLFile         = "messages.jsonl" // Inter-agent messages (root level)
	chatMessagesFile          = "messages.jsonl" // Chat messages (coordinator/worker directories)
	mcpRequestsFile           = "mcp_requests.jsonl"
	summaryFile               = "summary.md"
	accountabilitySummaryFile = "accountability_summary.md"
)

// New creates a new session with the given ID and directory.
// It creates the complete folder structure and initializes the session with status=running.
//
// Optional SessionOption functions can be passed to configure the session with additional
// application context (e.g., WithWorkDir, WithApplicationName, WithDatePartition).
//
// The folder structure created:
//
//	{dir}/
//	├── metadata.json                # Session metadata with status=running
//	├── coordinator/
//	│   ├── messages.jsonl           # Coordinator chat messages (structured JSONL)
//	│   └── raw.jsonl                # Raw Claude API JSON responses
//	├── workers/                     # Worker directories created on demand
//	├── messages.jsonl               # Inter-agent message log
//	├── mcp_requests.jsonl           # MCP tool call requests/responses
//	└── summary.md                   # Post-session summary (created on close)
func New(id, dir string, opts ...SessionOption) (*Session, error) {
	// Create the main session directory
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("creating session directory: %w", err)
	}

	// Validate write permissions by creating a test file
	testFile := filepath.Join(dir, ".write_test")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil { //nolint:gosec // G306: test file is immediately deleted
		return nil, fmt.Errorf("session directory not writable: %w", err)
	}
	_ = os.Remove(testFile)

	// Create coordinator directory
	coordPath := filepath.Join(dir, coordinatorDir)
	if err := os.MkdirAll(coordPath, 0750); err != nil {
		return nil, fmt.Errorf("creating coordinator directory: %w", err)
	}

	// Create workers directory (empty initially, subdirs created on demand)
	workersPath := filepath.Join(dir, workersDir)
	if err := os.MkdirAll(workersPath, 0750); err != nil {
		return nil, fmt.Errorf("creating workers directory: %w", err)
	}

	// Create coordinator raw.jsonl with BufferedWriter
	coordRawPath := filepath.Join(coordPath, rawJSONLFile)
	coordRawFile, err := os.OpenFile(coordRawPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted dir parameter
	if err != nil {
		return nil, fmt.Errorf("creating coordinator raw.jsonl: %w", err)
	}
	coordRaw := NewBufferedWriter(coordRawFile)

	// Create coordinator messages.jsonl with BufferedWriter (structured chat messages)
	coordMsgsPath := filepath.Join(coordPath, chatMessagesFile)
	coordMsgsFile, err := os.OpenFile(coordMsgsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted dir parameter
	if err != nil {
		_ = coordRaw.Close()
		return nil, fmt.Errorf("creating coordinator messages.jsonl: %w", err)
	}
	coordMessages := NewBufferedWriter(coordMsgsFile)

	// Create messages.jsonl with BufferedWriter (inter-agent messages)
	messagesPath := filepath.Join(dir, messagesJSONLFile)
	messageLogFile, err := os.OpenFile(messagesPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted dir parameter
	if err != nil {
		_ = coordRaw.Close()
		_ = coordMessages.Close()
		return nil, fmt.Errorf("creating messages.jsonl: %w", err)
	}
	messageLog := NewBufferedWriter(messageLogFile)

	// Create mcp_requests.jsonl with BufferedWriter
	mcpPath := filepath.Join(dir, mcpRequestsFile)
	mcpLogFile, err := os.OpenFile(mcpPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted dir parameter
	if err != nil {
		_ = coordRaw.Close()
		_ = coordMessages.Close()
		_ = messageLog.Close()
		return nil, fmt.Errorf("creating mcp_requests.jsonl: %w", err)
	}
	mcpLog := NewBufferedWriter(mcpLogFile)

	startTime := time.Now()

	// Create session struct first so we can apply options
	sess := &Session{
		ID:             id,
		Dir:            dir,
		StartTime:      startTime,
		Status:         StatusRunning,
		coordRaw:       coordRaw,
		coordMessages:  coordMessages,
		workerRaws:     make(map[string]*BufferedWriter),
		workerMessages: make(map[string]*BufferedWriter),
		messageLog:     messageLog,
		mcpLog:         mcpLog,
		workers:        []WorkerMetadata{},
		tokenUsage:     TokenUsageSummary{},
		closed:         false,
	}

	// Apply any provided options to set application context fields
	for _, opt := range opts {
		opt(sess)
	}

	// Create initial metadata with status=running and application context fields
	meta := &Metadata{
		SessionID:       id,
		StartTime:       startTime,
		Status:          StatusRunning,
		SessionDir:      dir,
		Workers:         []WorkerMetadata{},
		ApplicationName: sess.applicationName,
		WorkDir:         sess.workDir,
		DatePartition:   sess.datePartition,
	}

	if err := meta.Save(dir); err != nil {
		_ = coordRaw.Close()
		_ = coordMessages.Close()
		_ = messageLog.Close()
		_ = mcpLog.Close()
		return nil, fmt.Errorf("saving initial metadata: %w", err)
	}

	return sess, nil
}

// Reopen reopens an existing session directory for continued writing.
// This is used for session resumption - it opens existing JSONL files in append mode
// so new messages continue writing to the same files without overwriting.
//
// Unlike New(), Reopen:
//   - Does NOT create directories (they must already exist)
//   - Does NOT create/overwrite metadata.json
//   - Opens files in append mode to continue from existing content
//   - Loads existing worker metadata to preserve the workers list
//
// The sessionDir must be an existing session directory with a valid metadata.json.
// Worker files (under workers/{id}/) are still created on-demand when workers are spawned.
func Reopen(sessionID, sessionDir string, opts ...SessionOption) (*Session, error) {
	// Verify the session directory exists
	info, err := os.Stat(sessionDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session directory does not exist: %s", sessionDir)
		}
		return nil, fmt.Errorf("checking session directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("session path is not a directory: %s", sessionDir)
	}

	// Load metadata to get existing workers list and other session state.
	// Per reviewer recommendation: We need to restore workers slice so that
	// AddWorker/UpdateWorker calls don't corrupt the session state.
	meta, err := Load(sessionDir)
	if err != nil {
		return nil, fmt.Errorf("loading session metadata: %w", err)
	}

	// Open coordinator/raw.jsonl in append mode
	coordPath := filepath.Join(sessionDir, coordinatorDir)
	coordRawPath := filepath.Join(coordPath, rawJSONLFile)
	coordRawFile, err := os.OpenFile(coordRawPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted sessionDir parameter
	if err != nil {
		return nil, fmt.Errorf("reopening coordinator raw.jsonl: %w", err)
	}
	coordRaw := NewBufferedWriter(coordRawFile)

	// Open coordinator/messages.jsonl in append mode
	coordMsgsPath := filepath.Join(coordPath, chatMessagesFile)
	coordMsgsFile, err := os.OpenFile(coordMsgsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted sessionDir parameter
	if err != nil {
		_ = coordRaw.Close()
		return nil, fmt.Errorf("reopening coordinator messages.jsonl: %w", err)
	}
	coordMessages := NewBufferedWriter(coordMsgsFile)

	// Open messages.jsonl (inter-agent messages) in append mode
	messagesPath := filepath.Join(sessionDir, messagesJSONLFile)
	messageLogFile, err := os.OpenFile(messagesPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted sessionDir parameter
	if err != nil {
		_ = coordRaw.Close()
		_ = coordMessages.Close()
		return nil, fmt.Errorf("reopening messages.jsonl: %w", err)
	}
	messageLog := NewBufferedWriter(messageLogFile)

	// Open mcp_requests.jsonl in append mode
	mcpPath := filepath.Join(sessionDir, mcpRequestsFile)
	mcpLogFile, err := os.OpenFile(mcpPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted sessionDir parameter
	if err != nil {
		_ = coordRaw.Close()
		_ = coordMessages.Close()
		_ = messageLog.Close()
		return nil, fmt.Errorf("reopening mcp_requests.jsonl: %w", err)
	}
	mcpLog := NewBufferedWriter(mcpLogFile)

	// Create session with current time as start time for this resumed session
	sess := &Session{
		ID:             sessionID,
		Dir:            sessionDir,
		StartTime:      time.Now(),
		Status:         StatusRunning,
		coordRaw:       coordRaw,
		coordMessages:  coordMessages,
		workerRaws:     make(map[string]*BufferedWriter),
		workerMessages: make(map[string]*BufferedWriter),
		messageLog:     messageLog,
		mcpLog:         mcpLog,
		// Restore workers from metadata to preserve existing worker list
		workers:               meta.Workers,
		tokenUsage:            meta.TokenUsage,
		coordinatorSessionRef: meta.CoordinatorSessionRef,
		resumable:             meta.Resumable,
		applicationName:       meta.ApplicationName,
		workDir:               meta.WorkDir,
		datePartition:         meta.DatePartition,
		closed:                false,
	}

	// Apply any provided options
	for _, opt := range opts {
		opt(sess)
	}

	return sess, nil
}

// WriteCoordinatorMessage writes a structured chat message to coordinator/messages.jsonl.
// The message is serialized to JSON and appended as a single JSONL line.
func (s *Session) WriteCoordinatorMessage(msg chatrender.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling coordinator message: %w", err)
	}

	// Append newline for JSONL format
	data = append(data, '\n')
	return s.coordMessages.Write(data)
}

// WriteWorkerMessage writes a structured chat message to workers/{workerID}/messages.jsonl.
// Lazy-creates worker subdirectory and messages.jsonl file if needed.
func (s *Session) WriteWorkerMessage(workerID string, msg chatrender.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	// Get or create worker messages writer
	writer, err := s.getOrCreateWorkerMessages(workerID)
	if err != nil {
		return err
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling worker message: %w", err)
	}

	// Append newline for JSONL format
	data = append(data, '\n')
	return writer.Write(data)
}

// WriteCoordinatorRawJSON appends raw JSON to coordinator/raw.jsonl.
// The rawJSON should be a single JSON object (one line in JSONL format).
func (s *Session) WriteCoordinatorRawJSON(timestamp time.Time, rawJSON []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	// Ensure the line ends with newline
	data := rawJSON
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}

	return s.coordRaw.Write(data)
}

// WriteWorkerRawJSON appends raw JSON to workers/{workerID}/raw.jsonl.
// Lazy-creates worker subdirectory and raw.jsonl file if needed.
func (s *Session) WriteWorkerRawJSON(workerID string, timestamp time.Time, rawJSON []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	// Get or create worker raw log
	writer, err := s.getOrCreateWorkerRaw(workerID)
	if err != nil {
		return err
	}

	// Ensure the line ends with newline
	data := rawJSON
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}

	return writer.Write(data)
}

// WriteMessage appends a message entry to messages.jsonl in JSONL format.
func (s *Session) WriteMessage(entry message.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshaling message entry: %w", err)
	}

	// Append newline for JSONL format
	data = append(data, '\n')
	return s.messageLog.Write(data)
}

// WriteMCPEvent appends an MCP event to mcp_requests.jsonl in JSONL format.
func (s *Session) WriteMCPEvent(event events.MCPEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshaling MCP event: %w", err)
	}

	// Append newline for JSONL format
	data = append(data, '\n')
	return s.mcpLog.Write(data)
}

// Close finalizes the session, flushes all BufferedWriters, updates metadata, and closes file handles.
// After Close returns, no more writes are accepted.
func (s *Session) Close(status Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}
	s.closed = true
	s.Status = status

	var firstErr error

	// Close all worker raw writers
	for _, w := range s.workerRaws {
		if err := w.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Close all worker message writers
	for _, w := range s.workerMessages {
		if err := w.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Close coordinator writers
	if err := s.coordRaw.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := s.coordMessages.Close(); err != nil && firstErr == nil {
		firstErr = err
	}

	// Close message and MCP logs
	if err := s.messageLog.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := s.mcpLog.Close(); err != nil && firstErr == nil {
		firstErr = err
	}

	// Update metadata with end time, final status, workers, and token usage
	meta, err := Load(s.Dir)
	if err != nil {
		// If we can't load metadata, create a new one with application context
		meta = &Metadata{
			SessionID:       s.ID,
			StartTime:       s.StartTime,
			Status:          status,
			SessionDir:      s.Dir,
			Workers:         []WorkerMetadata{},
			ApplicationName: s.applicationName,
			WorkDir:         s.workDir,
			DatePartition:   s.datePartition,
		}
	}
	meta.EndTime = time.Now()
	meta.Status = status
	meta.Workers = s.workers
	meta.TokenUsage = s.tokenUsage

	if err := meta.Save(s.Dir); err != nil && firstErr == nil {
		firstErr = err
	}

	// Generate summary.md
	if err := s.generateSummary(meta); err != nil && firstErr == nil {
		firstErr = err
	}

	// Update sessions.json index
	if err := s.updateSessionIndex(meta); err != nil && firstErr == nil {
		firstErr = err
	}

	return firstErr
}

// getOrCreateWorkerRaw returns the BufferedWriter for a worker's raw.jsonl,
// creating the worker directory and file if needed.
// Caller must hold s.mu.
func (s *Session) getOrCreateWorkerRaw(workerID string) (*BufferedWriter, error) {
	if writer, ok := s.workerRaws[workerID]; ok {
		return writer, nil
	}

	// Create worker directory
	workerPath := filepath.Join(s.Dir, workersDir, workerID)
	if err := os.MkdirAll(workerPath, 0750); err != nil {
		return nil, fmt.Errorf("creating worker directory: %w", err)
	}

	// Create raw.jsonl
	rawPath := filepath.Join(workerPath, rawJSONLFile)
	file, err := os.OpenFile(rawPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted workerID parameter
	if err != nil {
		return nil, fmt.Errorf("creating worker raw.jsonl: %w", err)
	}

	writer := NewBufferedWriter(file)
	s.workerRaws[workerID] = writer
	return writer, nil
}

// getOrCreateWorkerMessages returns the BufferedWriter for a worker's messages.jsonl,
// creating the worker directory and file if needed.
// Caller must hold s.mu.
func (s *Session) getOrCreateWorkerMessages(workerID string) (*BufferedWriter, error) {
	if writer, ok := s.workerMessages[workerID]; ok {
		return writer, nil
	}

	// Create worker directory
	workerPath := filepath.Join(s.Dir, workersDir, workerID)
	if err := os.MkdirAll(workerPath, 0750); err != nil {
		return nil, fmt.Errorf("creating worker directory: %w", err)
	}

	// Create messages.jsonl
	msgsPath := filepath.Join(workerPath, chatMessagesFile)
	file, err := os.OpenFile(msgsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted workerID parameter
	if err != nil {
		return nil, fmt.Errorf("creating worker messages.jsonl: %w", err)
	}

	writer := NewBufferedWriter(file)
	s.workerMessages[workerID] = writer
	return writer, nil
}

// generateSummary writes a summary.md file for the session.
func (s *Session) generateSummary(meta *Metadata) error {
	summaryPath := filepath.Join(s.Dir, summaryFile)

	duration := meta.EndTime.Sub(meta.StartTime)

	var content string
	content = "# Session Summary\n\n"
	content += fmt.Sprintf("**Session ID:** %s\n\n", meta.SessionID)
	content += fmt.Sprintf("**Status:** %s\n\n", meta.Status)
	content += fmt.Sprintf("**Start Time:** %s\n\n", meta.StartTime.Format(time.RFC3339))
	content += fmt.Sprintf("**End Time:** %s\n\n", meta.EndTime.Format(time.RFC3339))
	content += fmt.Sprintf("**Duration:** %s\n\n", duration.Round(time.Second))

	if len(meta.Workers) > 0 {
		content += "## Workers\n\n"
		for _, w := range meta.Workers {
			content += fmt.Sprintf("- **%s**: Spawned at %s", w.ID, w.SpawnedAt.Format(time.RFC3339))
			if !w.RetiredAt.IsZero() {
				content += fmt.Sprintf(", Retired at %s", w.RetiredAt.Format(time.RFC3339))
			}
			if w.FinalPhase != "" {
				content += fmt.Sprintf(" (Final phase: %s)", w.FinalPhase)
			}
			content += "\n"
		}
		content += "\n"
	}

	if meta.TokenUsage.TotalInputTokens > 0 || meta.TokenUsage.TotalOutputTokens > 0 {
		content += "## Token Usage\n\n"
		content += fmt.Sprintf("- **Input Tokens:** %d\n", meta.TokenUsage.TotalInputTokens)
		content += fmt.Sprintf("- **Output Tokens:** %d\n", meta.TokenUsage.TotalOutputTokens)
		if meta.TokenUsage.TotalCostUSD > 0 {
			content += fmt.Sprintf("- **Total Cost:** $%.2f\n", meta.TokenUsage.TotalCostUSD)
		}
		content += "\n"
	}

	return os.WriteFile(summaryPath, []byte(content), 0600)
}

// updateSessionIndex appends this session's entry to the session index files.
//
// When a pathBuilder is configured, the session writes to TWO indexes:
//   - Application index: {baseDir}/{appName}/sessions.json (per-application sessions)
//   - Global index: {baseDir}/sessions.json (all sessions across applications)
//
// When no pathBuilder is configured (legacy mode), writes only to the parent directory:
//   - Legacy index: {parent of session dir}/sessions.json
//
// Uses atomic rename to avoid race conditions on both files.
func (s *Session) updateSessionIndex(meta *Metadata) error {
	// Build accountability summary path relative to session dir if it exists
	var accountabilitySummaryPath string
	summaryPath := filepath.Join(s.Dir, accountabilitySummaryFile)
	if _, statErr := os.Stat(summaryPath); statErr == nil {
		accountabilitySummaryPath = summaryPath
	}

	// Create entry for this session with all metadata fields
	entry := SessionIndexEntry{
		ID:                        s.ID,
		StartTime:                 meta.StartTime,
		EndTime:                   meta.EndTime,
		Status:                    meta.Status,
		SessionDir:                s.Dir,
		AccountabilitySummaryPath: accountabilitySummaryPath,
		WorkerCount:               len(meta.Workers),
		ApplicationName:           s.applicationName,
		WorkDir:                   s.workDir,
		DatePartition:             s.datePartition,
	}

	// If pathBuilder is configured, write to both application and global indexes
	if s.pathBuilder != nil {
		// Update application-level index
		if err := s.updateApplicationIndex(entry); err != nil {
			return fmt.Errorf("updating application index: %w", err)
		}

		// Update global index
		if err := s.updateGlobalIndex(entry); err != nil {
			return fmt.Errorf("updating global index: %w", err)
		}

		return nil
	}

	// Legacy mode: write only to parent directory index
	indexPath := filepath.Join(filepath.Dir(s.Dir), "sessions.json")
	index, err := LoadSessionIndex(indexPath)
	if err != nil {
		return fmt.Errorf("loading session index: %w", err)
	}

	index.Sessions = append(index.Sessions, entry)

	if err := SaveSessionIndex(indexPath, index); err != nil {
		return fmt.Errorf("saving session index: %w", err)
	}

	return nil
}

// updateApplicationIndex updates the per-application sessions.json index.
// Path: {baseDir}/{appName}/sessions.json
func (s *Session) updateApplicationIndex(entry SessionIndexEntry) error {
	indexPath := s.pathBuilder.ApplicationIndexPath()

	// Load existing index or create empty one
	appIndex, err := LoadApplicationIndex(indexPath)
	if err != nil {
		return fmt.Errorf("loading application index: %w", err)
	}

	// Set the application name if not already set
	if appIndex.ApplicationName == "" {
		appIndex.ApplicationName = s.applicationName
	}

	// Append entry
	appIndex.Sessions = append(appIndex.Sessions, entry)

	// Save with atomic rename
	if err := SaveApplicationIndex(indexPath, appIndex); err != nil {
		return fmt.Errorf("saving application index: %w", err)
	}

	return nil
}

// updateGlobalIndex updates the global sessions.json index.
// Path: {baseDir}/sessions.json
func (s *Session) updateGlobalIndex(entry SessionIndexEntry) error {
	indexPath := s.pathBuilder.IndexPath()

	// Load existing index or create empty one
	globalIndex, err := LoadSessionIndex(indexPath)
	if err != nil {
		return fmt.Errorf("loading global index: %w", err)
	}

	// Append entry
	globalIndex.Sessions = append(globalIndex.Sessions, entry)

	// Save with atomic rename
	if err := SaveSessionIndex(indexPath, globalIndex); err != nil {
		return fmt.Errorf("saving global index: %w", err)
	}

	return nil
}

// AttachToBrokers subscribes to all event brokers and spawns goroutines to stream events to disk.
//
// Each broker spawns a dedicated goroutine that reads events from the subscription channel
// and writes them to the appropriate log files via the Write* methods.
//
// Note: Coordinator events should be received via AttachV2EventBus which handles both
// coordinator and worker ProcessEvents from the unified v2EventBus.
//
// Context cancellation stops all subscriber goroutines cleanly.
func (s *Session) AttachToBrokers(
	ctx context.Context,
	processBroker *pubsub.Broker[events.ProcessEvent],
	msgBroker *pubsub.Broker[message.Event],
	mcpBroker *pubsub.Broker[events.MCPEvent],
) {
	// Attach process broker for worker events
	if processBroker != nil {
		s.attachProcessBroker(ctx, processBroker)
	}

	// Attach message broker
	if msgBroker != nil {
		s.attachMessageBroker(ctx, msgBroker)
	}

	// Attach MCP broker
	if mcpBroker != nil {
		s.AttachMCPBroker(ctx, mcpBroker)
	}
}

// handleCoordinatorProcessEvent processes a coordinator ProcessEvent and writes to appropriate logs.
// This replaces the legacy handleCoordinatorEvent function that used CoordinatorEvent type.
//
// Event type mapping from legacy to v2:
//   - CoordinatorChat → ProcessOutput
//   - CoordinatorTokenUsage → ProcessTokenUsage
//   - CoordinatorError → ProcessError
//   - CoordinatorStatusChange/Ready/Working → ProcessStatusChange/Ready/Working
func (s *Session) handleCoordinatorProcessEvent(event events.ProcessEvent) {
	now := time.Now().UTC()

	switch event.Type {
	case events.ProcessOutput:
		// Detect tool calls by 🔧 prefix
		isToolCall := strings.HasPrefix(event.Output, "🔧")
		msg := chatrender.Message{
			Role:       string(event.Role),
			Content:    event.Output,
			IsToolCall: isToolCall,
			Timestamp:  &now,
		}
		if err := s.WriteCoordinatorMessage(msg); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write coordinator message", "error", err)
		}
		// Write raw JSON if present
		if len(event.RawJSON) > 0 {
			if err := s.WriteCoordinatorRawJSON(now, event.RawJSON); err != nil {
				log.Warn(log.CatOrch, "Session: failed to write coordinator raw JSON", "error", err)
			}
		}

	case events.ProcessIncoming:
		// User input - role is "user"
		msg := chatrender.Message{
			Role:      "user",
			Content:   event.Message,
			Timestamp: &now,
		}
		if err := s.WriteCoordinatorMessage(msg); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write coordinator incoming message", "error", err)
		}

	case events.ProcessTokenUsage:
		// Update token usage in metadata
		if event.Metrics != nil {
			s.updateTokenUsage(event.Metrics.TokensUsed, event.Metrics.OutputTokens, event.Metrics.TotalCostUSD)
		}

	case events.ProcessError:
		// Write error as system message
		errMsg := "unknown error"
		if event.Error != nil {
			errMsg = event.Error.Error()
		}
		msg := chatrender.Message{
			Role:      "system",
			Content:   "Error: " + errMsg,
			Timestamp: &now,
		}
		if err := s.WriteCoordinatorMessage(msg); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write coordinator error message", "error", err)
		}

	case events.ProcessStatusChange, events.ProcessReady, events.ProcessWorking:
		// Status changes are not user-visible chat - skip writing to messages.jsonl
	}
}

// attachProcessBroker subscribes to the process event broker for worker events.
//
// The subscriber goroutine handles:
//   - ProcessOutput: writes to worker messages.jsonl and raw.jsonl (if RawJSON present)
//   - ProcessSpawned: updates metadata workers list
//   - ProcessStatusChange: updates worker metadata
//   - ProcessTokenUsage: updates metadata token counts
//   - ProcessError: writes error to worker messages.jsonl
func (s *Session) attachProcessBroker(ctx context.Context, broker *pubsub.Broker[events.ProcessEvent]) {
	sub := broker.Subscribe(ctx)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub:
				if !ok {
					return
				}
				// Only handle worker events, skip coordinator events
				if ev.Payload.IsWorker() {
					s.handleProcessEvent(ev.Payload)
				}
			}
		}
	}()

	log.Debug(log.CatOrch, "Session attached to process broker", "sessionID", s.ID)
}

// AttachV2EventBus subscribes to the unified v2EventBus for all process events.
// This handles both coordinator and worker events via the unified ProcessEvent type.
//
// The subscriber goroutine type-asserts to ProcessEvent and routes events based on Role:
// - RoleCoordinator: routes to handleCoordinatorProcessEvent
// - RoleWorker: routes to handleProcessEvent
//
// This replaces the legacy AttachCoordinatorBroker method that used CoordinatorEvent type.
func (s *Session) AttachV2EventBus(ctx context.Context, broker *pubsub.Broker[any]) {
	sub := broker.Subscribe(ctx)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub:
				if !ok {
					return
				}
				// Type-assert to ProcessEvent and route based on role
				if processEvent, isProcess := ev.Payload.(events.ProcessEvent); isProcess {
					if processEvent.IsCoordinator() {
						s.handleCoordinatorProcessEvent(processEvent)
					} else if processEvent.IsWorker() {
						s.handleProcessEvent(processEvent)
					}
				}
				// Other event types from v2EventBus are ignored by session logger
			}
		}
	}()

	log.Debug(log.CatOrch, "Session attached to v2EventBus", "sessionID", s.ID)
}

// handleProcessEvent processes a process event (worker) and writes to appropriate logs.
func (s *Session) handleProcessEvent(event events.ProcessEvent) {
	now := time.Now().UTC()
	workerID := event.ProcessID

	switch event.Type {
	case events.ProcessSpawned:
		// Add worker to metadata - use session's workDir (same for all processes currently)
		s.addWorker(workerID, now, s.workDir)
		// Log the spawn event as a system message
		msg := chatrender.Message{
			Role:      "system",
			Content:   "Worker spawned",
			Timestamp: &now,
		}
		if err := s.WriteWorkerMessage(workerID, msg); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write worker spawn message", "error", err, "workerID", workerID)
		}

	case events.ProcessOutput:
		// Detect tool calls by 🔧 prefix
		isToolCall := strings.HasPrefix(event.Output, "🔧")
		msg := chatrender.Message{
			Role:       "assistant",
			Content:    event.Output,
			IsToolCall: isToolCall,
			Timestamp:  &now,
		}
		if err := s.WriteWorkerMessage(workerID, msg); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write worker output message", "error", err, "workerID", workerID)
		}
		// Write raw JSON if present
		if len(event.RawJSON) > 0 {
			if err := s.WriteWorkerRawJSON(workerID, now, event.RawJSON); err != nil {
				log.Warn(log.CatOrch, "Session: failed to write worker raw JSON", "error", err, "workerID", workerID)
			}
		}

	case events.ProcessStatusChange:
		// Update worker phase in metadata
		var phaseStr string
		if event.Phase != nil {
			phaseStr = string(*event.Phase)
		}
		s.updateProcessPhase(workerID, phaseStr)
		// If worker is retired, record retirement time
		if event.Status == events.ProcessStatusRetired {
			s.retireWorker(workerID, now, phaseStr)
		}
		// Status changes are not user-visible chat - skip writing to messages.jsonl

	case events.ProcessTokenUsage:
		// Update token usage in metadata
		if event.Metrics != nil {
			s.updateTokenUsage(event.Metrics.TokensUsed, event.Metrics.OutputTokens, event.Metrics.TotalCostUSD)
		}

	case events.ProcessIncoming:
		// Preserve sender role (default to "coordinator" for coordinator→worker messages)
		role := event.Sender
		if role == "" {
			role = "coordinator"
		}
		msg := chatrender.Message{
			Role:      role,
			Content:   event.Message,
			Timestamp: &now,
		}
		if err := s.WriteWorkerMessage(workerID, msg); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write worker incoming message", "error", err, "workerID", workerID)
		}

	case events.ProcessError:
		// Write error as system message
		errMsg := "unknown error"
		if event.Error != nil {
			errMsg = event.Error.Error()
		}
		msg := chatrender.Message{
			Role:      "system",
			Content:   "Error: " + errMsg,
			Timestamp: &now,
		}
		if err := s.WriteWorkerMessage(workerID, msg); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write worker error message", "error", err, "workerID", workerID)
		}
	}
}

// attachMessageBroker subscribes to the message event broker.
//
// The subscriber goroutine handles:
//   - EventPosted: writes the message to messages.jsonl
func (s *Session) attachMessageBroker(ctx context.Context, broker *pubsub.Broker[message.Event]) {
	sub := broker.Subscribe(ctx)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub:
				if !ok {
					return
				}
				s.handleMessageEvent(ev.Payload)
			}
		}
	}()

	log.Debug(log.CatOrch, "Session attached to message broker", "sessionID", s.ID)
}

// handleMessageEvent processes a message event and writes to messages.jsonl.
func (s *Session) handleMessageEvent(event message.Event) {
	if event.Type == message.EventPosted {
		if err := s.WriteMessage(event.Entry); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write message", "error", err, "messageID", event.Entry.ID)
		}
	}
}

// AttachMCPBroker subscribes to the MCP event broker for late binding.
// This is useful when the MCP server starts after the session is created.
//
// The subscriber goroutine handles:
//   - MCPToolCall/MCPToolResult/MCPError: writes to mcp_requests.jsonl
func (s *Session) AttachMCPBroker(ctx context.Context, broker *pubsub.Broker[events.MCPEvent]) {
	sub := broker.Subscribe(ctx)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub:
				if !ok {
					return
				}
				s.handleMCPEvent(ev.Payload)
			}
		}
	}()

	log.Debug(log.CatOrch, "Session attached to MCP broker", "sessionID", s.ID)
}

// handleMCPEvent processes an MCP event and writes to mcp_requests.jsonl.
func (s *Session) handleMCPEvent(event events.MCPEvent) {
	if err := s.WriteMCPEvent(event); err != nil {
		log.Warn(log.CatOrch, "Session: failed to write MCP event", "error", err, "toolName", event.ToolName)
	}
}

// updateTokenUsage atomically updates the session's token usage counters.
func (s *Session) updateTokenUsage(inputTokens, outputTokens int, costUSD float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Context tokens (inputTokens) are cumulative per-turn (includes CacheRead),
	// so we replace rather than accumulate. Output tokens and cost are incremental.
	s.tokenUsage.TotalInputTokens = inputTokens
	s.tokenUsage.TotalOutputTokens += outputTokens
	s.tokenUsage.TotalCostUSD += costUSD
}

// addWorker adds a new worker to the session's metadata.
// workDir is captured at spawn time; sessionRef is set later via SetWorkerSessionRef.
func (s *Session) addWorker(workerID string, spawnedAt time.Time, workDir string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if worker already exists
	for i := range s.workers {
		if s.workers[i].ID == workerID {
			return // Already tracked
		}
	}

	s.workers = append(s.workers, WorkerMetadata{
		ID:        workerID,
		SpawnedAt: spawnedAt,
		WorkDir:   workDir,
	})
}

// updateProcessPhase updates a worker's current phase in the metadata.
func (s *Session) updateProcessPhase(workerID, phase string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.workers {
		if s.workers[i].ID == workerID {
			s.workers[i].FinalPhase = phase
			return
		}
	}
}

// retireWorker marks a worker as retired with the retirement time and final phase.
func (s *Session) retireWorker(workerID string, retiredAt time.Time, finalPhase string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.workers {
		if s.workers[i].ID == workerID {
			s.workers[i].RetiredAt = retiredAt
			s.workers[i].FinalPhase = finalPhase
			return
		}
	}
}

// WriteWorkerAccountabilitySummary writes a worker's accountability summary to their session directory.
// Creates the worker directory if it doesn't exist (follows getOrCreateWorkerLog pattern).
// Returns the full path where the summary was saved.
// Note: taskID is embedded in the YAML frontmatter of the content, not passed as parameter.
func (s *Session) WriteWorkerAccountabilitySummary(workerID string, content []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return "", os.ErrClosed
	}

	// Ensure worker directory exists (lazy creation)
	workerPath := filepath.Join(s.Dir, workersDir, workerID)
	if err := os.MkdirAll(workerPath, 0750); err != nil {
		return "", fmt.Errorf("creating worker directory: %w", err)
	}

	// Write accountability summary file (overwrites if exists - latest summary wins)
	summaryPath := filepath.Join(workerPath, accountabilitySummaryFile)
	if err := os.WriteFile(summaryPath, content, 0600); err != nil {
		return "", fmt.Errorf("writing accountability summary file: %w", err)
	}

	log.Debug(log.CatOrch, "Wrote worker accountability summary", "workerID", workerID, "path", summaryPath)

	return summaryPath, nil
}

// SetCoordinatorSessionRef sets the coordinator's headless session reference.
// This should be called after the coordinator's first successful turn.
// Immediately persists metadata to ensure crash resilience.
func (s *Session) SetCoordinatorSessionRef(ref string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	s.coordinatorSessionRef = ref
	return s.saveMetadataLocked()
}

// SetWorkerSessionRef sets a worker's headless session reference.
// Should be called after the worker's first successful turn.
// Immediately persists metadata to ensure crash resilience.
func (s *Session) SetWorkerSessionRef(workerID, ref, workDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	// Find worker and update
	for i := range s.workers {
		if s.workers[i].ID == workerID {
			s.workers[i].HeadlessSessionRef = ref
			s.workers[i].WorkDir = workDir
			return s.saveMetadataLocked()
		}
	}

	return fmt.Errorf("worker not found: %s", workerID)
}

// MarkResumable marks the session as resumable.
// Called after coordinator session ref is captured.
func (s *Session) MarkResumable() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	s.resumable = true
	return s.saveMetadataLocked()
}

// saveMetadataLocked persists metadata to disk.
// Caller must hold s.mu.
func (s *Session) saveMetadataLocked() error {
	meta, err := Load(s.Dir)
	if err != nil {
		// Create fresh metadata if file doesn't exist or is corrupted
		meta = &Metadata{
			SessionID:       s.ID,
			StartTime:       s.StartTime,
			Status:          s.Status,
			SessionDir:      s.Dir,
			ApplicationName: s.applicationName,
			WorkDir:         s.workDir,
			DatePartition:   s.datePartition,
		}
	}

	// Update with current in-memory state
	meta.CoordinatorSessionRef = s.coordinatorSessionRef
	meta.Resumable = s.resumable
	meta.Workers = s.workers
	meta.TokenUsage = s.tokenUsage

	return meta.Save(s.Dir)
}

// NotifySessionRef implements the SessionRefNotifier interface.
// Called by ProcessTurnCompleteHandler after a process's first successful turn.
func (s *Session) NotifySessionRef(processID, sessionRef, workDir string) error {
	if processID == "coordinator" {
		if err := s.SetCoordinatorSessionRef(sessionRef); err != nil {
			return err
		}
		return s.MarkResumable()
	}

	// Worker session ref
	return s.SetWorkerSessionRef(processID, sessionRef, workDir)
}

// GetWorkflowCompletedAt returns the workflow completion timestamp from session metadata.
// Returns zero time if workflow has not been completed.
// Implements handler.SessionMetadataProvider interface.
func (s *Session) GetWorkflowCompletedAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return time.Time{}
	}

	meta, err := Load(s.Dir)
	if err != nil {
		return time.Time{}
	}
	return meta.WorkflowCompletedAt
}

// UpdateWorkflowCompletion updates the workflow completion fields in session metadata.
// If WorkflowCompletedAt is already set (non-zero), the timestamp is preserved for idempotency.
// Implements handler.SessionMetadataProvider interface.
func (s *Session) UpdateWorkflowCompletion(status, summary string, completedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	meta, err := Load(s.Dir)
	if err != nil {
		// Create fresh metadata if file doesn't exist or is corrupted
		meta = &Metadata{
			SessionID:       s.ID,
			StartTime:       s.StartTime,
			Status:          s.Status,
			SessionDir:      s.Dir,
			ApplicationName: s.applicationName,
			WorkDir:         s.workDir,
			DatePartition:   s.datePartition,
		}
	}

	// Update workflow completion fields
	meta.WorkflowCompletionStatus = status
	meta.WorkflowSummary = summary
	meta.WorkflowCompletedAt = completedAt

	return meta.Save(s.Dir)
}
