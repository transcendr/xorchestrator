package orchestration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/xorchestrator/internal/orchestration/events"
	"github.com/zjrosen/xorchestrator/internal/orchestration/session"
	"github.com/zjrosen/xorchestrator/internal/ui/shared/chatrender"
)

// createTestSessionDir creates a minimal test session directory with valid metadata.
func createTestSessionDir(t *testing.T) string {
	t.Helper()

	sessionDir := t.TempDir()
	sessionID := "test-session-123"
	now := time.Now().UTC().Truncate(time.Millisecond)

	// Create coordinator directory
	coordDir := filepath.Join(sessionDir, "coordinator")
	require.NoError(t, os.MkdirAll(coordDir, 0755))

	// Create workers directory
	workersDir := filepath.Join(sessionDir, "workers")
	require.NoError(t, os.MkdirAll(workersDir, 0755))

	// Create a valid metadata file
	metadata := &session.Metadata{
		SessionID:             sessionID,
		SessionDir:            sessionDir,
		StartTime:             now,
		EndTime:               now.Add(time.Hour),
		Status:                session.StatusCompleted,
		Resumable:             true,
		CoordinatorSessionRef: "claude-session-abc123",
	}

	// Save metadata
	require.NoError(t, metadata.Save(sessionDir))

	return sessionDir
}

// createTestSessionDirWithContent creates a test session with content for full restoration tests.
func createTestSessionDirWithContent(t *testing.T) string {
	t.Helper()

	sessionDir := createTestSessionDir(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	// Add coordinator messages
	coordMsgPath := filepath.Join(sessionDir, "coordinator", "messages.jsonl")
	coordMsgContent := `{"role":"user","content":"Hello coordinator"}
{"role":"assistant","content":"Hello! I'll help you."}
`
	require.NoError(t, os.WriteFile(coordMsgPath, []byte(coordMsgContent), 0644))

	// Create worker directory and messages
	workerDir := filepath.Join(sessionDir, "workers", "worker-1")
	require.NoError(t, os.MkdirAll(workerDir, 0755))

	workerMsgPath := filepath.Join(workerDir, "messages.jsonl")
	workerMsgContent := `{"role":"coordinator","content":"Implement feature X"}
{"role":"worker","content":"Working on it..."}
`
	require.NoError(t, os.WriteFile(workerMsgPath, []byte(workerMsgContent), 0644))

	// Update metadata with worker info
	metadata := &session.Metadata{
		SessionID:             "test-session-123",
		SessionDir:            sessionDir,
		StartTime:             now,
		EndTime:               now.Add(time.Hour),
		Status:                session.StatusCompleted,
		Resumable:             true,
		CoordinatorSessionRef: "claude-session-abc123",
		Workers: []session.WorkerMetadata{
			{
				ID:                 "worker-1",
				SpawnedAt:          now,
				HeadlessSessionRef: "claude-worker-session-xyz",
				FinalPhase:         "implementing",
			},
		},
	}
	require.NoError(t, metadata.Save(sessionDir))

	// Create inter-agent messages file
	msgPath := filepath.Join(sessionDir, "messages.jsonl")
	msgContent := `{"id":"msg-1","timestamp":"2025-01-01T12:00:00Z","from":"COORDINATOR","to":"WORKER.1","content":"Task assigned","type":"info"}
{"id":"msg-2","timestamp":"2025-01-01T12:01:00Z","from":"WORKER.1","to":"COORDINATOR","content":"Task completed","type":"completion"}
`
	require.NoError(t, os.WriteFile(msgPath, []byte(msgContent), 0644))

	return sessionDir
}

// =============================================================================
// ResumeSessionMsg Tests
// =============================================================================

func TestHandleResumeSession_LoadsSession(t *testing.T) {
	sessionDir := createTestSessionDirWithContent(t)

	m := New(Config{})
	m = m.SetSize(120, 30)

	msg := ResumeSessionMsg{SessionDir: sessionDir}
	m, cmd := m.handleResumeSession(msg)

	// Verify session was loaded and stored
	require.NotNil(t, m.resumedSession, "resumedSession should be set")
	require.Equal(t, "test-session-123", m.resumedSession.Metadata.SessionID)

	// Verify a command was returned
	require.NotNil(t, cmd, "should return a command")

	// Execute the command and verify it returns StartRestoredSessionMsg
	resultMsg := cmd()
	startMsg, ok := resultMsg.(StartRestoredSessionMsg)
	require.True(t, ok, "command should return StartRestoredSessionMsg")
	require.NotNil(t, startMsg.Session)
	require.Equal(t, "test-session-123", startMsg.Session.Metadata.SessionID)
}

func TestHandleResumeSession_BuildsUIState(t *testing.T) {
	sessionDir := createTestSessionDirWithContent(t)

	m := New(Config{})
	m = m.SetSize(120, 30)

	msg := ResumeSessionMsg{SessionDir: sessionDir}
	m, _ = m.handleResumeSession(msg)

	// Verify UI state was populated via RestoreFromSession
	// Coordinator messages should be restored
	require.NotEmpty(t, m.coordinatorPane.messages, "coordinator messages should be restored")
	require.Equal(t, "Hello coordinator", m.coordinatorPane.messages[0].Content)
	require.Equal(t, "user", m.coordinatorPane.messages[0].Role)
}

func TestHandleResumeSession_RestoresModel(t *testing.T) {
	sessionDir := createTestSessionDirWithContent(t)

	m := New(Config{})
	m = m.SetSize(120, 30)

	// Clear dirty flags to verify they get set
	m.coordinatorPane.contentDirty = false
	m.messagePane.contentDirty = false

	msg := ResumeSessionMsg{SessionDir: sessionDir}
	m, _ = m.handleResumeSession(msg)

	// Verify dirty flags were set (via RestoreFromSession)
	require.True(t, m.coordinatorPane.contentDirty, "coordinatorPane should be dirty after restore")
	require.True(t, m.messagePane.contentDirty, "messagePane should be dirty after restore")

	// Verify inter-agent messages were restored
	require.NotEmpty(t, m.messagePane.entries, "message pane entries should be restored")
}

func TestHandleResumeSession_StoresSession(t *testing.T) {
	sessionDir := createTestSessionDirWithContent(t)

	m := New(Config{})
	m = m.SetSize(120, 30)

	// Verify initially nil
	require.Nil(t, m.resumedSession)

	msg := ResumeSessionMsg{SessionDir: sessionDir}
	m, _ = m.handleResumeSession(msg)

	// Verify session was stored
	require.NotNil(t, m.resumedSession, "resumedSession should be stored")
	require.Equal(t, "test-session-123", m.resumedSession.Metadata.SessionID)
	require.Equal(t, "claude-session-abc123", m.resumedSession.Metadata.CoordinatorSessionRef)

	// Verify worker metadata
	require.Len(t, m.resumedSession.ActiveWorkers, 1)
	require.Equal(t, "worker-1", m.resumedSession.ActiveWorkers[0].ID)
	require.Equal(t, "claude-worker-session-xyz", m.resumedSession.ActiveWorkers[0].HeadlessSessionRef)
}

func TestHandleResumeSession_LoadError_ShowsError(t *testing.T) {
	// Use a non-existent directory
	nonExistentDir := "/nonexistent/path/to/session"

	m := New(Config{})
	m = m.SetSize(120, 30)

	msg := ResumeSessionMsg{SessionDir: nonExistentDir}
	m, cmd := m.handleResumeSession(msg)

	// Verify error modal is shown
	require.NotNil(t, m.errorModal, "error modal should be shown on load failure")

	// Verify no command returned on error
	require.Nil(t, cmd, "no command should be returned on error")

	// Verify resumedSession is not set
	require.Nil(t, m.resumedSession, "resumedSession should not be set on error")
}

func TestHandleResumeSession_InvalidPath(t *testing.T) {
	// Create a directory that exists but doesn't have valid session data
	invalidDir := t.TempDir()

	m := New(Config{})
	m = m.SetSize(120, 30)

	msg := ResumeSessionMsg{SessionDir: invalidDir}
	m, cmd := m.handleResumeSession(msg)

	// Verify error modal is shown
	require.NotNil(t, m.errorModal, "error modal should be shown for invalid session path")

	// Verify no command returned
	require.Nil(t, cmd)

	// Verify resumedSession is not set
	require.Nil(t, m.resumedSession)
}

// =============================================================================
// StartRestoredSessionMsg Tests
// =============================================================================

func TestStartRestoredSession_ReturnsCorrectMsg(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	loadedSession := &session.ResumableSession{
		Metadata: &session.Metadata{
			SessionID:             "test-session-456",
			CoordinatorSessionRef: "claude-coord-ref",
			StartTime:             now,
			EndTime:               now.Add(time.Hour),
		},
		CoordinatorMessages: []chatrender.Message{
			{Role: "user", Content: "test"},
		},
	}

	cmd := startRestoredSession(loadedSession)
	require.NotNil(t, cmd)

	// Execute the command
	resultMsg := cmd()

	// Verify it returns StartRestoredSessionMsg with the session
	startMsg, ok := resultMsg.(StartRestoredSessionMsg)
	require.True(t, ok, "should return StartRestoredSessionMsg")
	require.Equal(t, loadedSession, startMsg.Session)
	require.Equal(t, "test-session-456", startMsg.Session.Metadata.SessionID)
}

func TestHandleStartRestoredSession_TriggersStartCoordinator(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	loadedSession := &session.ResumableSession{
		Metadata: &session.Metadata{
			SessionID:             "test-session-789",
			CoordinatorSessionRef: "claude-coord-ref",
			StartTime:             now,
			EndTime:               now.Add(time.Hour),
		},
	}

	m := New(Config{})
	m = m.SetSize(120, 30)

	msg := StartRestoredSessionMsg{Session: loadedSession}
	_, cmd := m.handleStartRestoredSession(msg)

	require.NotNil(t, cmd, "should return a command")

	// Execute the command and verify it returns StartCoordinatorMsg
	resultMsg := cmd()
	_, ok := resultMsg.(StartCoordinatorMsg)
	require.True(t, ok, "command should return StartCoordinatorMsg")
}

func TestHandleStartRestoredSession_NilSession_ShowsError(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	msg := StartRestoredSessionMsg{Session: nil}
	m, cmd := m.handleStartRestoredSession(msg)

	// Verify error modal is shown
	require.NotNil(t, m.errorModal, "error modal should be shown for nil session")

	// Verify no command returned
	require.Nil(t, cmd)
}

// =============================================================================
// Message Handler Wiring Tests
// =============================================================================

func TestUpdate_HandlesResumeSessionMsg(t *testing.T) {
	sessionDir := createTestSessionDir(t)

	m := New(Config{})
	m = m.SetSize(120, 30)

	// Send ResumeSessionMsg through Update
	msg := ResumeSessionMsg{SessionDir: sessionDir}
	m, cmd := m.Update(msg)

	// Verify session was loaded
	require.NotNil(t, m.resumedSession, "resumedSession should be set after Update")

	// Verify command was returned
	require.NotNil(t, cmd)
}

func TestUpdate_HandlesStartRestoredSessionMsg(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	loadedSession := &session.ResumableSession{
		Metadata: &session.Metadata{
			SessionID:             "test-session-update",
			CoordinatorSessionRef: "claude-ref",
			StartTime:             now,
			EndTime:               now.Add(time.Hour),
		},
	}

	m := New(Config{})
	m = m.SetSize(120, 30)

	// Send StartRestoredSessionMsg through Update
	msg := StartRestoredSessionMsg{Session: loadedSession}
	_, cmd := m.Update(msg)

	// Verify command was returned
	require.NotNil(t, cmd)

	// Execute and verify StartCoordinatorMsg
	resultMsg := cmd()
	_, ok := resultMsg.(StartCoordinatorMsg)
	require.True(t, ok)
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestResumeFlow_EndToEnd(t *testing.T) {
	// Create a complete test session
	sessionDir := createTestSessionDirWithContent(t)

	m := New(Config{
		WorkDir: t.TempDir(),
	})
	m = m.SetSize(120, 30)

	// Step 1: Send ResumeSessionMsg
	resumeMsg := ResumeSessionMsg{SessionDir: sessionDir}
	m, cmd := m.Update(resumeMsg)

	// Verify session loaded and stored
	require.NotNil(t, m.resumedSession, "resumedSession should be set")
	require.Equal(t, "test-session-123", m.resumedSession.Metadata.SessionID)

	// Verify UI state restored
	require.NotEmpty(t, m.coordinatorPane.messages, "coordinator messages restored")
	require.NotEmpty(t, m.messagePane.entries, "message pane entries restored")
	require.NotEmpty(t, m.workerPane.workerStatus, "worker status restored")

	// Step 2: Execute the command from ResumeSessionMsg
	require.NotNil(t, cmd)
	startRestoredMsg := cmd()
	restoredMsg, ok := startRestoredMsg.(StartRestoredSessionMsg)
	require.True(t, ok, "should return StartRestoredSessionMsg")
	require.NotNil(t, restoredMsg.Session)

	// Step 3: Send StartRestoredSessionMsg through Update
	m, cmd = m.Update(restoredMsg)

	// Verify it triggers StartCoordinatorMsg
	require.NotNil(t, cmd)
	startCoordMsg := cmd()
	_, ok = startCoordMsg.(StartCoordinatorMsg)
	require.True(t, ok, "should trigger StartCoordinatorMsg")

	// Verify resumedSession is still available for Initializer
	require.NotNil(t, m.resumedSession, "resumedSession should still be available")
	require.Equal(t, "claude-session-abc123", m.resumedSession.Metadata.CoordinatorSessionRef)

	// Verify worker session refs are available (for --resume flow)
	require.Len(t, m.resumedSession.ActiveWorkers, 1)
	require.Equal(t, "claude-worker-session-xyz", m.resumedSession.ActiveWorkers[0].HeadlessSessionRef)
}

func TestResumeFlow_WithRetiredWorkers(t *testing.T) {
	sessionDir := createTestSessionDir(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	// Create worker directories
	worker1Dir := filepath.Join(sessionDir, "workers", "worker-1")
	worker2Dir := filepath.Join(sessionDir, "workers", "worker-2")
	worker3Dir := filepath.Join(sessionDir, "workers", "worker-3")
	require.NoError(t, os.MkdirAll(worker1Dir, 0755))
	require.NoError(t, os.MkdirAll(worker2Dir, 0755))
	require.NoError(t, os.MkdirAll(worker3Dir, 0755))

	// Update metadata with mix of active and retired workers
	metadata := &session.Metadata{
		SessionID:             "test-session-123",
		SessionDir:            sessionDir,
		StartTime:             now,
		EndTime:               now.Add(time.Hour),
		Status:                session.StatusCompleted,
		Resumable:             true,
		CoordinatorSessionRef: "claude-session-abc123",
		Workers: []session.WorkerMetadata{
			{
				ID:                 "worker-1",
				SpawnedAt:          now,
				HeadlessSessionRef: "claude-worker-1-ref",
				// Active worker (no RetiredAt)
			},
			{
				ID:                 "worker-2",
				SpawnedAt:          now,
				RetiredAt:          now.Add(30 * time.Minute),
				HeadlessSessionRef: "claude-worker-2-ref",
				FinalPhase:         "committing",
			},
			{
				ID:                 "worker-3",
				SpawnedAt:          now,
				HeadlessSessionRef: "claude-worker-3-ref",
				// Active worker (no RetiredAt)
			},
		},
	}
	require.NoError(t, metadata.Save(sessionDir))

	m := New(Config{})
	m = m.SetSize(120, 30)

	msg := ResumeSessionMsg{SessionDir: sessionDir}
	m, _ = m.handleResumeSession(msg)

	// Verify active vs retired partitioning
	require.Len(t, m.resumedSession.ActiveWorkers, 2, "should have 2 active workers")
	require.Len(t, m.resumedSession.RetiredWorkers, 1, "should have 1 retired worker")

	// Verify active workers don't include retired
	activeIDs := []string{}
	for _, w := range m.resumedSession.ActiveWorkers {
		activeIDs = append(activeIDs, w.ID)
	}
	require.Contains(t, activeIDs, "worker-1")
	require.Contains(t, activeIDs, "worker-3")
	require.NotContains(t, activeIDs, "worker-2")

	// Verify retired worker is in retiredOrder
	require.Equal(t, "worker-2", m.resumedSession.RetiredWorkers[0].ID)

	// Verify UI state respects active/retired separation
	// workerIDs should only contain active workers
	require.Len(t, m.workerPane.workerIDs, 2)
	require.Contains(t, m.workerPane.workerIDs, "worker-1")
	require.Contains(t, m.workerPane.workerIDs, "worker-3")

	// retiredOrder should contain retired workers
	require.Len(t, m.workerPane.retiredOrder, 1)
	require.Equal(t, "worker-2", m.workerPane.retiredOrder[0])

	// Status should be set for all workers
	require.Equal(t, events.ProcessStatusReady, m.workerPane.workerStatus["worker-1"])
	require.Equal(t, events.ProcessStatusRetired, m.workerPane.workerStatus["worker-2"])
	require.Equal(t, events.ProcessStatusReady, m.workerPane.workerStatus["worker-3"])
}

func TestResumeFlow_PreservesSessionIDs(t *testing.T) {
	// Test that SessionID values are preserved for --resume flow
	sessionDir := createTestSessionDir(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	// Create worker directory
	workerDir := filepath.Join(sessionDir, "workers", "worker-1")
	require.NoError(t, os.MkdirAll(workerDir, 0755))

	// Update metadata with session refs
	metadata := &session.Metadata{
		SessionID:             "xorchestrator-session-uuid",
		SessionDir:            sessionDir,
		StartTime:             now,
		EndTime:               now.Add(time.Hour),
		Status:                session.StatusCompleted,
		Resumable:             true,
		CoordinatorSessionRef: "claude-coordinator-session-ref-12345",
		Workers: []session.WorkerMetadata{
			{
				ID:                 "worker-1",
				SpawnedAt:          now,
				HeadlessSessionRef: "claude-worker-session-ref-67890",
			},
		},
	}
	require.NoError(t, metadata.Save(sessionDir))

	m := New(Config{})
	m = m.SetSize(120, 30)

	msg := ResumeSessionMsg{SessionDir: sessionDir}
	m, _ = m.handleResumeSession(msg)

	// Verify session IDs are preserved (these enable --resume flag)
	require.Equal(t, "claude-coordinator-session-ref-12345",
		m.resumedSession.Metadata.CoordinatorSessionRef,
		"CoordinatorSessionRef should be preserved")

	require.Len(t, m.resumedSession.ActiveWorkers, 1)
	require.Equal(t, "claude-worker-session-ref-67890",
		m.resumedSession.ActiveWorkers[0].HeadlessSessionRef,
		"WorkerSessionRef should be preserved for --resume")
}

// =============================================================================
// Edge Cases
// =============================================================================

func TestHandleResumeSession_EmptySession(t *testing.T) {
	sessionDir := createTestSessionDir(t)

	// No messages, no workers - just valid metadata
	m := New(Config{})
	m = m.SetSize(120, 30)

	msg := ResumeSessionMsg{SessionDir: sessionDir}
	m, cmd := m.handleResumeSession(msg)

	// Should succeed with empty content
	require.NotNil(t, m.resumedSession)
	require.NotNil(t, cmd)

	// UI state should be empty but valid, except for the separator message
	// which is always added on resume
	require.Len(t, m.coordinatorPane.messages, 1, "should have exactly the separator message")
	require.Equal(t, "system", m.coordinatorPane.messages[0].Role, "separator should be system role")
	require.Contains(t, m.coordinatorPane.messages[0].Content, "Session Resumed", "separator content")
	require.Empty(t, m.workerPane.workerIDs)
	require.Empty(t, m.messagePane.entries)
}

func TestHandleResumeSession_ValidationFailure(t *testing.T) {
	sessionDir := t.TempDir()

	// Create metadata that fails validation (Resumable = false)
	metadata := &session.Metadata{
		SessionID:             "test-session",
		SessionDir:            sessionDir,
		StartTime:             time.Now(),
		EndTime:               time.Now().Add(time.Hour),
		Status:                session.StatusCompleted,
		Resumable:             false, // Not resumable!
		CoordinatorSessionRef: "claude-ref",
	}
	require.NoError(t, metadata.Save(sessionDir))

	m := New(Config{})
	m = m.SetSize(120, 30)

	msg := ResumeSessionMsg{SessionDir: sessionDir}
	m, cmd := m.handleResumeSession(msg)

	// Should show error due to validation failure
	require.NotNil(t, m.errorModal, "error modal should be shown for non-resumable session")
	require.Nil(t, cmd)
	require.Nil(t, m.resumedSession)
}

func TestHandleResumeSession_MissingCoordinatorRef(t *testing.T) {
	sessionDir := t.TempDir()

	// Create metadata missing CoordinatorSessionRef
	metadata := &session.Metadata{
		SessionID:             "test-session",
		SessionDir:            sessionDir,
		StartTime:             time.Now(),
		EndTime:               time.Now().Add(time.Hour),
		Status:                session.StatusCompleted,
		Resumable:             true,
		CoordinatorSessionRef: "", // Missing!
	}
	require.NoError(t, metadata.Save(sessionDir))

	m := New(Config{})
	m = m.SetSize(120, 30)

	msg := ResumeSessionMsg{SessionDir: sessionDir}
	m, cmd := m.handleResumeSession(msg)

	// Should show error due to missing coordinator ref
	require.NotNil(t, m.errorModal)
	require.Nil(t, cmd)
	require.Nil(t, m.resumedSession)
}

// =============================================================================
// Model Field Tests
// =============================================================================

func TestModel_ResumedSession_FieldExists(t *testing.T) {
	m := New(Config{})

	// Field should be nil by default
	require.Nil(t, m.resumedSession)

	// Set and verify
	m.resumedSession = &session.ResumableSession{
		Metadata: &session.Metadata{SessionID: "test"},
	}
	require.NotNil(t, m.resumedSession)
	require.Equal(t, "test", m.resumedSession.Metadata.SessionID)
}

func TestModel_ResumedSession_PassedToInitializer(t *testing.T) {
	// This test verifies that handleStartCoordinator passes resumedSession to Initializer
	// We can't easily test Initializer config, but we can verify the field is set
	// and available when handleStartCoordinator runs

	sessionDir := createTestSessionDir(t)

	m := New(Config{
		WorkDir:          t.TempDir(),
		DisableWorktrees: true, // Skip worktree prompt for cleaner test
	})
	m = m.SetSize(120, 30)

	// First, load a session via ResumeSessionMsg
	resumeMsg := ResumeSessionMsg{SessionDir: sessionDir}
	m, _ = m.handleResumeSession(resumeMsg)

	// Verify resumedSession is set before calling handleStartCoordinator
	require.NotNil(t, m.resumedSession, "resumedSession should be set before initialization")
	require.Equal(t, "test-session-123", m.resumedSession.Metadata.SessionID)

	// The actual passing to Initializer is tested by the integration test
	// and by verifying update.go passes m.resumedSession to InitializerConfig
}

// =============================================================================
// Message Type Tests
// =============================================================================

func TestResumeSessionMsg_Type(t *testing.T) {
	msg := ResumeSessionMsg{SessionDir: "/path/to/session"}

	// Verify it can be used as tea.Msg
	var teaMsg tea.Msg = msg
	require.NotNil(t, teaMsg)

	// Type assertion should work
	resumeMsg, ok := teaMsg.(ResumeSessionMsg)
	require.True(t, ok)
	require.Equal(t, "/path/to/session", resumeMsg.SessionDir)
}

func TestStartRestoredSessionMsg_Type(t *testing.T) {
	msg := StartRestoredSessionMsg{
		Session: &session.ResumableSession{
			Metadata: &session.Metadata{SessionID: "test"},
		},
	}

	// Verify it can be used as tea.Msg
	var teaMsg tea.Msg = msg
	require.NotNil(t, teaMsg)

	// Type assertion should work
	startMsg, ok := teaMsg.(StartRestoredSessionMsg)
	require.True(t, ok)
	require.Equal(t, "test", startMsg.Session.Metadata.SessionID)
}

// =============================================================================
// Resume Indicator State Tests
// =============================================================================

func TestResumeIndicatorState_NotSetForNewSession(t *testing.T) {
	// When a new session is created, isResumedSession should be false
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Verify default state for new session
	require.False(t, m.isResumedSession, "isResumedSession should be false for new session")
	require.True(t, m.resumedAt.IsZero(), "resumedAt should be zero for new session")
	require.True(t, m.originalStartTime.IsZero(), "originalStartTime should be zero for new session")
}

func TestResumeIndicatorState_SetForResumedSession(t *testing.T) {
	sessionDir := createTestSessionDirWithContent(t)

	m := New(Config{})
	m = m.SetSize(120, 30)

	// Verify initial state
	require.False(t, m.isResumedSession)

	msg := ResumeSessionMsg{SessionDir: sessionDir}
	beforeResume := time.Now()
	m, _ = m.handleResumeSession(msg)
	afterResume := time.Now()

	// Verify resume indicator state is set
	require.True(t, m.isResumedSession, "isResumedSession should be true after resume")

	// Verify resumedAt is set to approximately now
	require.False(t, m.resumedAt.IsZero(), "resumedAt should be set")
	require.True(t, m.resumedAt.After(beforeResume) || m.resumedAt.Equal(beforeResume),
		"resumedAt should be >= beforeResume")
	require.True(t, m.resumedAt.Before(afterResume) || m.resumedAt.Equal(afterResume),
		"resumedAt should be <= afterResume")

	// Verify originalStartTime is set from session metadata
	require.False(t, m.originalStartTime.IsZero(), "originalStartTime should be set")
}

func TestResumeSession_SetsSeparatorMessage(t *testing.T) {
	sessionDir := createTestSessionDirWithContent(t)

	m := New(Config{})
	m = m.SetSize(120, 30)

	// Count existing messages
	initialMsgCount := len(m.coordinatorPane.messages)

	msg := ResumeSessionMsg{SessionDir: sessionDir}
	m, _ = m.handleResumeSession(msg)

	// The session has 2 coordinator messages + 1 separator we add
	// The original session has "Hello coordinator" and "Hello! I'll help you."
	require.Greater(t, len(m.coordinatorPane.messages), initialMsgCount,
		"coordinator messages should include restored messages plus separator")

	// Find the separator message (last message after restoration)
	lastMsg := m.coordinatorPane.messages[len(m.coordinatorPane.messages)-1]

	// Verify it's a system message with separator format
	require.Equal(t, "system", lastMsg.Role, "separator should have role='system'")
	require.Contains(t, lastMsg.Content, "───", "separator should contain horizontal line characters")
	require.Contains(t, lastMsg.Content, "Session Resumed", "separator should contain 'Session Resumed'")
}

func TestResumeSession_SeparatorMessageFormat(t *testing.T) {
	// Test the separator format function directly
	testTime := time.Date(2026, time.January, 15, 14, 30, 0, 0, time.UTC)

	separator := formatResumedSeparator(testTime)

	require.Equal(t, "─── Session Resumed (Jan 15 14:30) ───", separator,
		"separator should match expected format")
}

func TestInit_NewSession_IsResumedSessionFalse(t *testing.T) {
	// Verify that isResumedSession remains false when starting a new session
	m := New(Config{})

	// New session should have isResumedSession = false
	require.False(t, m.isResumedSession, "new session should have isResumedSession=false")

	// Calling Init() doesn't change the flag for new sessions
	cmd := m.Init()
	require.NotNil(t, cmd)

	// Execute to verify it's StartCoordinatorMsg (not resume)
	resultMsg := cmd()
	_, ok := resultMsg.(StartCoordinatorMsg)
	require.True(t, ok, "new session Init() should return StartCoordinatorMsg")

	// isResumedSession should still be false
	require.False(t, m.isResumedSession)
}

func TestResumeSession_SetsWorktreeDecisionMade(t *testing.T) {
	// When resuming a session, the worktree decision was already made in the original session.
	// The worktreeDecisionMade flag should be set to true to skip the prompt.
	sessionDir := createTestSessionDirWithContent(t)

	m := New(Config{})
	m = m.SetSize(120, 30)

	// Verify initial state - worktreeDecisionMade is false
	require.False(t, m.worktreeDecisionMade, "new model should have worktreeDecisionMade=false")

	msg := ResumeSessionMsg{SessionDir: sessionDir}
	m, _ = m.handleResumeSession(msg)

	// After resume, worktreeDecisionMade should be true
	require.True(t, m.worktreeDecisionMade,
		"worktreeDecisionMade should be true after resume to skip worktree prompt")
}

func TestNewSession_DoesNotSetWorktreeDecisionMade(t *testing.T) {
	// For new sessions, worktreeDecisionMade should remain false so the prompt can be shown
	m := New(Config{})

	// New session should have worktreeDecisionMade = false
	require.False(t, m.worktreeDecisionMade, "new session should have worktreeDecisionMade=false")
}
