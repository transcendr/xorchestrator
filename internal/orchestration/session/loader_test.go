package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/xorchestrator/internal/orchestration/message"
	"github.com/zjrosen/xorchestrator/internal/ui/shared/chatrender"
)

// Helper function to create test session directory structure
func createTestSessionDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create coordinator directory
	coordDir := filepath.Join(dir, coordinatorDir)
	require.NoError(t, os.MkdirAll(coordDir, 0750))

	// Create workers directory
	workersPath := filepath.Join(dir, workersDir)
	require.NoError(t, os.MkdirAll(workersPath, 0750))

	return dir
}

// Helper to write JSONL file
func writeJSONLFile(t *testing.T, path string, lines []string) {
	t.Helper()
	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
}

// --- LoadCoordinatorMessages Tests ---

func TestLoadCoordinatorMessages_HappyPath(t *testing.T) {
	dir := createTestSessionDir(t)

	// Create test messages
	now := time.Now().UTC().Truncate(time.Millisecond)
	msg1 := chatrender.Message{Role: "coordinator", Content: "Hello", Timestamp: &now}
	msg2 := chatrender.Message{Role: "user", Content: "Hi there", Timestamp: &now}
	msg3 := chatrender.Message{Role: "coordinator", Content: "🔧 spawn_worker", IsToolCall: true, Timestamp: &now}

	// Write messages to file
	path := filepath.Join(dir, coordinatorDir, chatMessagesFile)
	lines := make([]string, 3)
	for i, msg := range []chatrender.Message{msg1, msg2, msg3} {
		data, err := json.Marshal(msg)
		require.NoError(t, err)
		lines[i] = string(data)
	}
	writeJSONLFile(t, path, lines)

	// Load and verify
	messages, err := LoadCoordinatorMessages(dir)
	require.NoError(t, err)
	require.Len(t, messages, 3)

	require.Equal(t, "coordinator", messages[0].Role)
	require.Equal(t, "Hello", messages[0].Content)
	require.False(t, messages[0].IsToolCall)

	require.Equal(t, "user", messages[1].Role)
	require.Equal(t, "Hi there", messages[1].Content)

	require.Equal(t, "coordinator", messages[2].Role)
	require.Equal(t, "🔧 spawn_worker", messages[2].Content)
	require.True(t, messages[2].IsToolCall)
}

func TestLoadCoordinatorMessages_FileNotExist(t *testing.T) {
	dir := t.TempDir()

	// File doesn't exist - should return empty slice, not error
	messages, err := LoadCoordinatorMessages(dir)
	require.NoError(t, err)
	require.Empty(t, messages)
	require.NotNil(t, messages) // Should be empty slice, not nil
}

func TestLoadCoordinatorMessages_MalformedLines(t *testing.T) {
	dir := createTestSessionDir(t)

	now := time.Now().UTC().Truncate(time.Millisecond)
	validMsg := chatrender.Message{Role: "coordinator", Content: "Valid message", Timestamp: &now}
	validData, err := json.Marshal(validMsg)
	require.NoError(t, err)

	// Write file with mix of valid and invalid JSON lines
	path := filepath.Join(dir, coordinatorDir, chatMessagesFile)
	lines := []string{
		string(validData), // Valid
		"{ invalid json",  // Malformed - missing closing brace
		`{"role": "user", "content": "Also valid"}`, // Valid
		"not json at all", // Malformed - plain text
		`{"role": 123}`,   // Malformed - wrong type for role
	}
	writeJSONLFile(t, path, lines)

	// Load - should skip malformed lines
	messages, err := LoadCoordinatorMessages(dir)
	require.NoError(t, err)
	require.Len(t, messages, 2) // Only 2 valid messages

	require.Equal(t, "coordinator", messages[0].Role)
	require.Equal(t, "Valid message", messages[0].Content)

	require.Equal(t, "user", messages[1].Role)
	require.Equal(t, "Also valid", messages[1].Content)
}

func TestLoadCoordinatorMessages_EmptyFile(t *testing.T) {
	dir := createTestSessionDir(t)

	// Create empty file
	path := filepath.Join(dir, coordinatorDir, chatMessagesFile)
	require.NoError(t, os.WriteFile(path, []byte(""), 0600))

	messages, err := LoadCoordinatorMessages(dir)
	require.NoError(t, err)
	require.Empty(t, messages)
	require.NotNil(t, messages)
}

func TestLoadCoordinatorMessages_LargeLine(t *testing.T) {
	dir := createTestSessionDir(t)

	// Create a message with content larger than typical buffer (but under 1MB limit)
	largeContent := strings.Repeat("x", 100000) // 100KB of content
	now := time.Now().UTC().Truncate(time.Millisecond)
	msg := chatrender.Message{Role: "coordinator", Content: largeContent, Timestamp: &now}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	path := filepath.Join(dir, coordinatorDir, chatMessagesFile)
	writeJSONLFile(t, path, []string{string(data)})

	messages, err := LoadCoordinatorMessages(dir)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, largeContent, messages[0].Content)
}

func TestLoadCoordinatorMessages_EmptyLinesSkipped(t *testing.T) {
	dir := createTestSessionDir(t)

	now := time.Now().UTC().Truncate(time.Millisecond)
	msg := chatrender.Message{Role: "coordinator", Content: "Test", Timestamp: &now}
	data, err := json.Marshal(msg)
	require.NoError(t, err)

	// Write file with empty lines
	path := filepath.Join(dir, coordinatorDir, chatMessagesFile)
	content := "\n" + string(data) + "\n\n\n" + string(data) + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))

	messages, err := LoadCoordinatorMessages(dir)
	require.NoError(t, err)
	require.Len(t, messages, 2)
}

func TestLoadCoordinatorMessages_NilTimestamp(t *testing.T) {
	dir := createTestSessionDir(t)

	// Message without timestamp (nil)
	msg := chatrender.Message{Role: "coordinator", Content: "No timestamp"}
	data, err := json.Marshal(msg)
	require.NoError(t, err)

	path := filepath.Join(dir, coordinatorDir, chatMessagesFile)
	writeJSONLFile(t, path, []string{string(data)})

	messages, err := LoadCoordinatorMessages(dir)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "No timestamp", messages[0].Content)
	require.Nil(t, messages[0].Timestamp)
}

// --- LoadWorkerMessages Tests ---

func TestLoadWorkerMessages_HappyPath(t *testing.T) {
	dir := createTestSessionDir(t)

	// Create worker directory
	workerDir := filepath.Join(dir, workersDir, "worker-1")
	require.NoError(t, os.MkdirAll(workerDir, 0750))

	now := time.Now().UTC().Truncate(time.Millisecond)
	msg1 := chatrender.Message{Role: "system", Content: "Worker spawned", Timestamp: &now}
	msg2 := chatrender.Message{Role: "assistant", Content: "Working on task", Timestamp: &now}

	path := filepath.Join(workerDir, chatMessagesFile)
	lines := make([]string, 2)
	for i, msg := range []chatrender.Message{msg1, msg2} {
		data, err := json.Marshal(msg)
		require.NoError(t, err)
		lines[i] = string(data)
	}
	writeJSONLFile(t, path, lines)

	messages, err := LoadWorkerMessages(dir, "worker-1")
	require.NoError(t, err)
	require.Len(t, messages, 2)

	require.Equal(t, "system", messages[0].Role)
	require.Equal(t, "Worker spawned", messages[0].Content)

	require.Equal(t, "assistant", messages[1].Role)
	require.Equal(t, "Working on task", messages[1].Content)
}

func TestLoadWorkerMessages_MultipleWorkers(t *testing.T) {
	dir := createTestSessionDir(t)

	now := time.Now().UTC().Truncate(time.Millisecond)

	// Create multiple worker directories with different messages
	for i, workerID := range []string{"worker-1", "worker-2", "worker-3"} {
		workerDir := filepath.Join(dir, workersDir, workerID)
		require.NoError(t, os.MkdirAll(workerDir, 0750))

		msg := chatrender.Message{
			Role:      "assistant",
			Content:   "Message from " + workerID,
			Timestamp: &now,
		}
		data, err := json.Marshal(msg)
		require.NoError(t, err)

		path := filepath.Join(workerDir, chatMessagesFile)
		// Write different number of messages per worker
		lines := make([]string, i+1)
		for j := range lines {
			lines[j] = string(data)
		}
		writeJSONLFile(t, path, lines)
	}

	// Load each worker's messages
	msgs1, err := LoadWorkerMessages(dir, "worker-1")
	require.NoError(t, err)
	require.Len(t, msgs1, 1)

	msgs2, err := LoadWorkerMessages(dir, "worker-2")
	require.NoError(t, err)
	require.Len(t, msgs2, 2)

	msgs3, err := LoadWorkerMessages(dir, "worker-3")
	require.NoError(t, err)
	require.Len(t, msgs3, 3)

	// Verify content
	require.Equal(t, "Message from worker-1", msgs1[0].Content)
	require.Equal(t, "Message from worker-2", msgs2[0].Content)
	require.Equal(t, "Message from worker-3", msgs3[0].Content)
}

func TestLoadWorkerMessages_WorkerNotExist(t *testing.T) {
	dir := createTestSessionDir(t)

	// Worker directory doesn't exist - should return empty slice
	messages, err := LoadWorkerMessages(dir, "nonexistent-worker")
	require.NoError(t, err)
	require.Empty(t, messages)
	require.NotNil(t, messages)
}

// --- LoadInterAgentMessages Tests ---

func TestLoadInterAgentMessages_HappyPath(t *testing.T) {
	dir := createTestSessionDir(t)

	now := time.Now().UTC().Truncate(time.Millisecond)
	entry1 := message.Entry{
		ID:        "msg-001",
		Timestamp: now,
		From:      "COORDINATOR",
		To:        "WORKER.1",
		Content:   "Please process this task",
		Type:      message.MessageRequest,
	}
	entry2 := message.Entry{
		ID:        "msg-002",
		Timestamp: now.Add(time.Second),
		From:      "WORKER.1",
		To:        "COORDINATOR",
		Content:   "Task completed",
		Type:      message.MessageCompletion,
		ReadBy:    []string{"COORDINATOR"},
	}

	path := filepath.Join(dir, messagesJSONLFile)
	lines := make([]string, 2)
	for i, entry := range []message.Entry{entry1, entry2} {
		data, err := json.Marshal(entry)
		require.NoError(t, err)
		lines[i] = string(data)
	}
	writeJSONLFile(t, path, lines)

	entries, err := LoadInterAgentMessages(dir)
	require.NoError(t, err)
	require.Len(t, entries, 2)

	require.Equal(t, "msg-001", entries[0].ID)
	require.Equal(t, "COORDINATOR", entries[0].From)
	require.Equal(t, "WORKER.1", entries[0].To)
	require.Equal(t, "Please process this task", entries[0].Content)
	require.Equal(t, message.MessageRequest, entries[0].Type)

	require.Equal(t, "msg-002", entries[1].ID)
	require.Equal(t, "WORKER.1", entries[1].From)
	require.Equal(t, "COORDINATOR", entries[1].To)
	require.Equal(t, "Task completed", entries[1].Content)
	require.Equal(t, message.MessageCompletion, entries[1].Type)
	require.Contains(t, entries[1].ReadBy, "COORDINATOR")
}

func TestLoadInterAgentMessages_FileNotExist(t *testing.T) {
	dir := t.TempDir()

	// File doesn't exist - should return empty slice, not error
	entries, err := LoadInterAgentMessages(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
	require.NotNil(t, entries)
}

func TestLoadInterAgentMessages_MalformedLines(t *testing.T) {
	dir := createTestSessionDir(t)

	now := time.Now().UTC().Truncate(time.Millisecond)
	validEntry := message.Entry{
		ID:        "valid-msg",
		Timestamp: now,
		From:      "COORDINATOR",
		To:        "ALL",
		Content:   "Broadcast message",
		Type:      message.MessageInfo,
	}
	validData, err := json.Marshal(validEntry)
	require.NoError(t, err)

	// Write file with mix of valid and invalid JSON lines
	path := filepath.Join(dir, messagesJSONLFile)
	lines := []string{
		string(validData), // Valid
		"{ broken json",   // Malformed
		`{"id": 123}`,     // Malformed - wrong type for id
		string(validData), // Valid (duplicate)
	}
	writeJSONLFile(t, path, lines)

	entries, err := LoadInterAgentMessages(dir)
	require.NoError(t, err)
	require.Len(t, entries, 2) // Only valid entries

	require.Equal(t, "valid-msg", entries[0].ID)
	require.Equal(t, "valid-msg", entries[1].ID)
}

func TestLoadInterAgentMessages_EmptyFile(t *testing.T) {
	dir := createTestSessionDir(t)

	path := filepath.Join(dir, messagesJSONLFile)
	require.NoError(t, os.WriteFile(path, []byte(""), 0600))

	entries, err := LoadInterAgentMessages(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
	require.NotNil(t, entries)
}

func TestLoadInterAgentMessages_EmptyLinesSkipped(t *testing.T) {
	dir := createTestSessionDir(t)

	now := time.Now().UTC().Truncate(time.Millisecond)
	entry := message.Entry{
		ID:        "msg-123",
		Timestamp: now,
		From:      "USER",
		To:        "COORDINATOR",
		Content:   "Hello",
		Type:      message.MessageInfo,
	}
	data, err := json.Marshal(entry)
	require.NoError(t, err)

	// Write file with empty lines
	path := filepath.Join(dir, messagesJSONLFile)
	content := "\n\n" + string(data) + "\n\n" + string(data) + "\n\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))

	entries, err := LoadInterAgentMessages(dir)
	require.NoError(t, err)
	require.Len(t, entries, 2)
}

func TestLoadInterAgentMessages_LargeLine(t *testing.T) {
	dir := createTestSessionDir(t)

	largeContent := strings.Repeat("y", 100000) // 100KB
	now := time.Now().UTC().Truncate(time.Millisecond)
	entry := message.Entry{
		ID:        "large-msg",
		Timestamp: now,
		From:      "COORDINATOR",
		To:        "WORKER.1",
		Content:   largeContent,
		Type:      message.MessageInfo,
	}
	data, err := json.Marshal(entry)
	require.NoError(t, err)

	path := filepath.Join(dir, messagesJSONLFile)
	writeJSONLFile(t, path, []string{string(data)})

	entries, err := LoadInterAgentMessages(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, largeContent, entries[0].Content)
}

func TestLoadInterAgentMessages_AllMessageTypes(t *testing.T) {
	dir := createTestSessionDir(t)

	now := time.Now().UTC().Truncate(time.Millisecond)
	types := []message.MessageType{
		message.MessageInfo,
		message.MessageRequest,
		message.MessageResponse,
		message.MessageCompletion,
		message.MessageError,
		message.MessageHandoff,
		message.MessageWorkerReady,
	}

	var lines []string
	for i, msgType := range types {
		entry := message.Entry{
			ID:        "msg-" + string(rune('a'+i)),
			Timestamp: now.Add(time.Duration(i) * time.Second),
			From:      "COORDINATOR",
			To:        "WORKER.1",
			Content:   "Type: " + string(msgType),
			Type:      msgType,
		}
		data, err := json.Marshal(entry)
		require.NoError(t, err)
		lines = append(lines, string(data))
	}

	path := filepath.Join(dir, messagesJSONLFile)
	writeJSONLFile(t, path, lines)

	entries, err := LoadInterAgentMessages(dir)
	require.NoError(t, err)
	require.Len(t, entries, len(types))

	for i, entry := range entries {
		require.Equal(t, types[i], entry.Type)
	}
}

// --- ValidateForResumption Tests ---

func TestValidateForResumption_Valid(t *testing.T) {
	// Valid session: resumable=true, has coordinator ref, status=completed
	metadata := &Metadata{
		SessionID:             "test-session",
		Status:                StatusCompleted,
		Resumable:             true,
		CoordinatorSessionRef: "claude-session-ref-123",
	}

	err := ValidateForResumption(metadata)
	require.NoError(t, err)
}

func TestValidateForResumption_NotResumable(t *testing.T) {
	// Session not marked as resumable
	metadata := &Metadata{
		SessionID:             "test-session",
		Status:                StatusCompleted,
		Resumable:             false,
		CoordinatorSessionRef: "claude-session-ref-123",
	}

	err := ValidateForResumption(metadata)
	require.Error(t, err)

	var validationErr *ValidationError
	require.ErrorAs(t, err, &validationErr)
	require.Equal(t, "Resumable", validationErr.Field)
	require.Contains(t, validationErr.Reason, "not marked as resumable")
}

func TestValidateForResumption_MissingRef(t *testing.T) {
	// Missing coordinator session reference
	metadata := &Metadata{
		SessionID:             "test-session",
		Status:                StatusCompleted,
		Resumable:             true,
		CoordinatorSessionRef: "", // Empty
	}

	err := ValidateForResumption(metadata)
	require.Error(t, err)

	var validationErr *ValidationError
	require.ErrorAs(t, err, &validationErr)
	require.Equal(t, "CoordinatorSessionRef", validationErr.Field)
	require.Contains(t, validationErr.Reason, "coordinator session reference is missing")
}

func TestValidateForResumption_RunningStatus(t *testing.T) {
	// Cannot resume a running session
	metadata := &Metadata{
		SessionID:             "test-session",
		Status:                StatusRunning,
		Resumable:             true,
		CoordinatorSessionRef: "claude-session-ref-123",
	}

	err := ValidateForResumption(metadata)
	require.Error(t, err)

	var validationErr *ValidationError
	require.ErrorAs(t, err, &validationErr)
	require.Equal(t, "Status", validationErr.Field)
	require.Contains(t, validationErr.Reason, "cannot resume a running session")
}

func TestValidateForResumption_CompletedStatus(t *testing.T) {
	// Completed session can be resumed
	metadata := &Metadata{
		SessionID:             "test-session",
		Status:                StatusCompleted,
		Resumable:             true,
		CoordinatorSessionRef: "claude-session-ref-123",
	}

	err := ValidateForResumption(metadata)
	require.NoError(t, err)
}

func TestValidateForResumption_FailedStatus(t *testing.T) {
	// Failed session can be resumed
	metadata := &Metadata{
		SessionID:             "test-session",
		Status:                StatusFailed,
		Resumable:             true,
		CoordinatorSessionRef: "claude-session-ref-123",
	}

	err := ValidateForResumption(metadata)
	require.NoError(t, err)
}

func TestValidateForResumption_TimedOutStatus(t *testing.T) {
	// Timed out session can be resumed
	metadata := &Metadata{
		SessionID:             "test-session",
		Status:                StatusTimedOut,
		Resumable:             true,
		CoordinatorSessionRef: "claude-session-ref-123",
	}

	err := ValidateForResumption(metadata)
	require.NoError(t, err)
}

func TestValidateForResumption_NilMetadata(t *testing.T) {
	// Nil metadata should fail validation
	err := ValidateForResumption(nil)
	require.Error(t, err)

	var validationErr *ValidationError
	require.ErrorAs(t, err, &validationErr)
	require.Equal(t, "metadata", validationErr.Field)
	require.Contains(t, validationErr.Reason, "metadata is nil")
}

// --- IsResumable Tests ---

func TestIsResumable_ReturnsTrueWhenValid(t *testing.T) {
	metadata := &Metadata{
		SessionID:             "test-session",
		Status:                StatusCompleted,
		Resumable:             true,
		CoordinatorSessionRef: "claude-session-ref-123",
	}

	require.True(t, IsResumable(metadata))
}

func TestIsResumable_ReturnsFalseWhenInvalid(t *testing.T) {
	testCases := []struct {
		name     string
		metadata *Metadata
	}{
		{
			name:     "nil metadata",
			metadata: nil,
		},
		{
			name: "not resumable",
			metadata: &Metadata{
				Status:                StatusCompleted,
				Resumable:             false,
				CoordinatorSessionRef: "ref-123",
			},
		},
		{
			name: "missing coordinator ref",
			metadata: &Metadata{
				Status:                StatusCompleted,
				Resumable:             true,
				CoordinatorSessionRef: "",
			},
		},
		{
			name: "running status",
			metadata: &Metadata{
				Status:                StatusRunning,
				Resumable:             true,
				CoordinatorSessionRef: "ref-123",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.False(t, IsResumable(tc.metadata))
		})
	}
}

// --- ValidationError Tests ---

func TestValidationError_ErrorMessage(t *testing.T) {
	err := &ValidationError{
		Reason: "test reason",
		Field:  "TestField",
	}

	errMsg := err.Error()
	require.Contains(t, errMsg, "session validation failed")
	require.Contains(t, errMsg, "test reason")
	require.Contains(t, errMsg, "TestField")
}

func TestValidationError_ImplementsErrorInterface(t *testing.T) {
	var err error = &ValidationError{
		Reason: "test",
		Field:  "test",
	}

	// Verify it implements error interface and can be used as such
	require.NotNil(t, err)
	require.Contains(t, err.Error(), "session validation failed")
}

// --- LoadResumableSession Tests ---

// Helper to create a valid resumable metadata file
func createResumableMetadata(t *testing.T, dir string, workers []WorkerMetadata) *Metadata {
	t.Helper()
	metadata := &Metadata{
		SessionID:             "test-session-123",
		Status:                StatusCompleted,
		Resumable:             true,
		CoordinatorSessionRef: "claude-ref-abc",
		Workers:               workers,
	}
	err := metadata.Save(dir)
	require.NoError(t, err)
	return metadata
}

func TestLoadResumableSession_FullLoad(t *testing.T) {
	dir := createTestSessionDir(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	// Create workers with different states
	workers := []WorkerMetadata{
		{ID: "worker-1", SpawnedAt: now.Add(-time.Hour)},                                            // Active
		{ID: "worker-2", SpawnedAt: now.Add(-2 * time.Hour), RetiredAt: now.Add(-30 * time.Minute)}, // Retired
	}
	createResumableMetadata(t, dir, workers)

	// Create coordinator messages
	coordMsg := chatrender.Message{Role: "coordinator", Content: "Hello world", Timestamp: &now}
	coordData, err := json.Marshal(coordMsg)
	require.NoError(t, err)
	coordPath := filepath.Join(dir, coordinatorDir, chatMessagesFile)
	writeJSONLFile(t, coordPath, []string{string(coordData)})

	// Create worker messages
	for _, w := range workers {
		workerDir := filepath.Join(dir, workersDir, w.ID)
		require.NoError(t, os.MkdirAll(workerDir, 0750))
		workerMsg := chatrender.Message{Role: "assistant", Content: "Working on " + w.ID, Timestamp: &now}
		workerData, err := json.Marshal(workerMsg)
		require.NoError(t, err)
		workerPath := filepath.Join(workerDir, chatMessagesFile)
		writeJSONLFile(t, workerPath, []string{string(workerData)})
	}

	// Create inter-agent messages
	interAgentEntry := message.Entry{
		ID:        "msg-001",
		Timestamp: now,
		From:      "COORDINATOR",
		To:        "WORKER.1",
		Content:   "Process this",
		Type:      message.MessageRequest,
	}
	interAgentData, err := json.Marshal(interAgentEntry)
	require.NoError(t, err)
	interAgentPath := filepath.Join(dir, messagesJSONLFile)
	writeJSONLFile(t, interAgentPath, []string{string(interAgentData)})

	// Load resumable session
	session, err := LoadResumableSession(dir)
	require.NoError(t, err)
	require.NotNil(t, session)

	// Verify metadata
	require.Equal(t, "test-session-123", session.Metadata.SessionID)
	require.True(t, session.Metadata.Resumable)

	// Verify coordinator messages
	require.Len(t, session.CoordinatorMessages, 1)
	require.Equal(t, "Hello world", session.CoordinatorMessages[0].Content)

	// Verify worker messages
	require.Len(t, session.WorkerMessages, 2)
	require.Len(t, session.WorkerMessages["worker-1"], 1)
	require.Equal(t, "Working on worker-1", session.WorkerMessages["worker-1"][0].Content)
	require.Len(t, session.WorkerMessages["worker-2"], 1)
	require.Equal(t, "Working on worker-2", session.WorkerMessages["worker-2"][0].Content)

	// Verify inter-agent messages
	require.Len(t, session.InterAgentMessages, 1)
	require.Equal(t, "msg-001", session.InterAgentMessages[0].ID)

	// Verify worker partitioning
	require.Len(t, session.ActiveWorkers, 1)
	require.Equal(t, "worker-1", session.ActiveWorkers[0].ID)
	require.Len(t, session.RetiredWorkers, 1)
	require.Equal(t, "worker-2", session.RetiredWorkers[0].ID)
}

func TestLoadResumableSession_ValidationFails(t *testing.T) {
	dir := createTestSessionDir(t)

	// Create non-resumable metadata
	metadata := &Metadata{
		SessionID:             "test-session",
		Status:                StatusRunning, // Running sessions can't be resumed
		Resumable:             true,
		CoordinatorSessionRef: "claude-ref",
	}
	err := metadata.Save(dir)
	require.NoError(t, err)

	// Load should fail due to validation
	session, err := LoadResumableSession(dir)
	require.Error(t, err)
	require.Nil(t, session)

	var validationErr *ValidationError
	require.ErrorAs(t, err, &validationErr)
	require.Equal(t, "Status", validationErr.Field)
}

func TestLoadResumableSession_WorkerPartitioning(t *testing.T) {
	dir := createTestSessionDir(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	// Create workers with mix of active and retired
	workers := []WorkerMetadata{
		{ID: "worker-1", SpawnedAt: now.Add(-time.Hour)},                                            // Active
		{ID: "worker-2", SpawnedAt: now.Add(-2 * time.Hour), RetiredAt: now.Add(-30 * time.Minute)}, // Retired
		{ID: "worker-3", SpawnedAt: now.Add(-3 * time.Hour)},                                        // Active
		{ID: "worker-4", SpawnedAt: now.Add(-4 * time.Hour), RetiredAt: now.Add(-10 * time.Minute)}, // Retired (more recent)
	}
	createResumableMetadata(t, dir, workers)

	session, err := LoadResumableSession(dir)
	require.NoError(t, err)

	// Verify active workers
	require.Len(t, session.ActiveWorkers, 2)
	activeIDs := []string{session.ActiveWorkers[0].ID, session.ActiveWorkers[1].ID}
	require.Contains(t, activeIDs, "worker-1")
	require.Contains(t, activeIDs, "worker-3")

	// Verify retired workers
	require.Len(t, session.RetiredWorkers, 2)
	retiredIDs := []string{session.RetiredWorkers[0].ID, session.RetiredWorkers[1].ID}
	require.Contains(t, retiredIDs, "worker-2")
	require.Contains(t, retiredIDs, "worker-4")
}

func TestLoadResumableSession_RetiredOrdering(t *testing.T) {
	dir := createTestSessionDir(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	// Create workers retired at different times
	workers := []WorkerMetadata{
		{ID: "worker-newest", SpawnedAt: now.Add(-time.Hour), RetiredAt: now.Add(-5 * time.Minute)},      // Retired most recently
		{ID: "worker-oldest", SpawnedAt: now.Add(-4 * time.Hour), RetiredAt: now.Add(-60 * time.Minute)}, // Retired first
		{ID: "worker-middle", SpawnedAt: now.Add(-2 * time.Hour), RetiredAt: now.Add(-30 * time.Minute)}, // Retired in middle
	}
	createResumableMetadata(t, dir, workers)

	session, err := LoadResumableSession(dir)
	require.NoError(t, err)

	// Retired workers should be sorted by RetiredAt ascending (oldest first)
	require.Len(t, session.RetiredWorkers, 3)
	require.Equal(t, "worker-oldest", session.RetiredWorkers[0].ID)
	require.Equal(t, "worker-middle", session.RetiredWorkers[1].ID)
	require.Equal(t, "worker-newest", session.RetiredWorkers[2].ID)
}

func TestLoadResumableSession_EmptyWorkerMessages(t *testing.T) {
	dir := createTestSessionDir(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	// Create a worker but don't create any messages for it
	workers := []WorkerMetadata{
		{ID: "worker-no-msgs", SpawnedAt: now},
	}
	createResumableMetadata(t, dir, workers)

	session, err := LoadResumableSession(dir)
	require.NoError(t, err)

	// Worker messages map should have the worker but with empty messages
	require.Contains(t, session.WorkerMessages, "worker-no-msgs")
	require.Empty(t, session.WorkerMessages["worker-no-msgs"])
	require.NotNil(t, session.WorkerMessages["worker-no-msgs"])
}

func TestLoadResumableSession_NoWorkers(t *testing.T) {
	dir := createTestSessionDir(t)

	// Create metadata with no workers (coordinator-only session)
	createResumableMetadata(t, dir, []WorkerMetadata{})

	// Create coordinator messages
	now := time.Now().UTC().Truncate(time.Millisecond)
	coordMsg := chatrender.Message{Role: "coordinator", Content: "Solo coordinator", Timestamp: &now}
	coordData, err := json.Marshal(coordMsg)
	require.NoError(t, err)
	coordPath := filepath.Join(dir, coordinatorDir, chatMessagesFile)
	writeJSONLFile(t, coordPath, []string{string(coordData)})

	session, err := LoadResumableSession(dir)
	require.NoError(t, err)

	// Should have empty worker-related fields
	require.Empty(t, session.WorkerMessages)
	require.Empty(t, session.ActiveWorkers)
	require.Empty(t, session.RetiredWorkers)
	require.NotNil(t, session.WorkerMessages)
	require.NotNil(t, session.ActiveWorkers)
	require.NotNil(t, session.RetiredWorkers)

	// Should still have coordinator messages
	require.Len(t, session.CoordinatorMessages, 1)
	require.Equal(t, "Solo coordinator", session.CoordinatorMessages[0].Content)
}

func TestLoadResumableSession_AllWorkersRetired(t *testing.T) {
	dir := createTestSessionDir(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	// All workers are retired
	workers := []WorkerMetadata{
		{ID: "worker-1", SpawnedAt: now.Add(-2 * time.Hour), RetiredAt: now.Add(-30 * time.Minute)},
		{ID: "worker-2", SpawnedAt: now.Add(-time.Hour), RetiredAt: now.Add(-10 * time.Minute)},
	}
	createResumableMetadata(t, dir, workers)

	session, err := LoadResumableSession(dir)
	require.NoError(t, err)

	require.Empty(t, session.ActiveWorkers)
	require.NotNil(t, session.ActiveWorkers)
	require.Len(t, session.RetiredWorkers, 2)
}

func TestLoadResumableSession_RoundTrip(t *testing.T) {
	// This test verifies that data written by a Session can be read back
	// by LoadResumableSession

	dir := createTestSessionDir(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	// Create metadata with workers
	workers := []WorkerMetadata{
		{ID: "worker-1", SpawnedAt: now.Add(-time.Hour), HeadlessSessionRef: "worker-1-ref"},
		{ID: "worker-2", SpawnedAt: now.Add(-2 * time.Hour), RetiredAt: now.Add(-30 * time.Minute), HeadlessSessionRef: "worker-2-ref"},
	}
	originalMetadata := &Metadata{
		SessionID:             "round-trip-session",
		StartTime:             now.Add(-3 * time.Hour),
		Status:                StatusCompleted,
		SessionDir:            dir,
		Resumable:             true,
		CoordinatorSessionRef: "coord-ref-123",
		Workers:               workers,
		ClientType:            "claude",
		Model:                 "sonnet",
	}
	err := originalMetadata.Save(dir)
	require.NoError(t, err)

	// Write coordinator messages
	coordMsgs := []chatrender.Message{
		{Role: "system", Content: "Session started", Timestamp: &now},
		{Role: "user", Content: "Do something", Timestamp: &now},
		{Role: "coordinator", Content: "On it", Timestamp: &now},
	}
	coordPath := filepath.Join(dir, coordinatorDir, chatMessagesFile)
	var coordLines []string
	for _, msg := range coordMsgs {
		data, err := json.Marshal(msg)
		require.NoError(t, err)
		coordLines = append(coordLines, string(data))
	}
	writeJSONLFile(t, coordPath, coordLines)

	// Write worker messages
	for _, w := range workers {
		workerDir := filepath.Join(dir, workersDir, w.ID)
		require.NoError(t, os.MkdirAll(workerDir, 0750))

		workerMsgs := []chatrender.Message{
			{Role: "system", Content: "Worker " + w.ID + " spawned", Timestamp: &now},
			{Role: "assistant", Content: "Task done for " + w.ID, Timestamp: &now},
		}
		var workerLines []string
		for _, msg := range workerMsgs {
			data, err := json.Marshal(msg)
			require.NoError(t, err)
			workerLines = append(workerLines, string(data))
		}
		writeJSONLFile(t, filepath.Join(workerDir, chatMessagesFile), workerLines)
	}

	// Write inter-agent messages
	interAgentEntries := []message.Entry{
		{ID: "msg-1", Timestamp: now, From: "COORDINATOR", To: "WORKER.1", Content: "Task 1", Type: message.MessageRequest},
		{ID: "msg-2", Timestamp: now.Add(time.Second), From: "WORKER.1", To: "COORDINATOR", Content: "Done", Type: message.MessageCompletion},
	}
	var interAgentLines []string
	for _, entry := range interAgentEntries {
		data, err := json.Marshal(entry)
		require.NoError(t, err)
		interAgentLines = append(interAgentLines, string(data))
	}
	writeJSONLFile(t, filepath.Join(dir, messagesJSONLFile), interAgentLines)

	// Load and verify everything matches
	session, err := LoadResumableSession(dir)
	require.NoError(t, err)

	// Verify metadata
	require.Equal(t, originalMetadata.SessionID, session.Metadata.SessionID)
	require.Equal(t, originalMetadata.Resumable, session.Metadata.Resumable)
	require.Equal(t, originalMetadata.CoordinatorSessionRef, session.Metadata.CoordinatorSessionRef)
	require.Equal(t, originalMetadata.ClientType, session.Metadata.ClientType)
	require.Equal(t, originalMetadata.Model, session.Metadata.Model)

	// Verify coordinator messages
	require.Len(t, session.CoordinatorMessages, 3)
	require.Equal(t, "Session started", session.CoordinatorMessages[0].Content)
	require.Equal(t, "Do something", session.CoordinatorMessages[1].Content)
	require.Equal(t, "On it", session.CoordinatorMessages[2].Content)

	// Verify worker messages
	require.Len(t, session.WorkerMessages, 2)
	require.Len(t, session.WorkerMessages["worker-1"], 2)
	require.Len(t, session.WorkerMessages["worker-2"], 2)

	// Verify inter-agent messages
	require.Len(t, session.InterAgentMessages, 2)
	require.Equal(t, "msg-1", session.InterAgentMessages[0].ID)
	require.Equal(t, "msg-2", session.InterAgentMessages[1].ID)

	// Verify worker partitioning
	require.Len(t, session.ActiveWorkers, 1)
	require.Equal(t, "worker-1", session.ActiveWorkers[0].ID)
	require.Len(t, session.RetiredWorkers, 1)
	require.Equal(t, "worker-2", session.RetiredWorkers[0].ID)
}

func TestLoadResumableSession_MetadataNotFound(t *testing.T) {
	dir := t.TempDir()

	// No metadata file - should error
	session, err := LoadResumableSession(dir)
	require.Error(t, err)
	require.Nil(t, session)
	require.Contains(t, err.Error(), "loading metadata")
}
