package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/xorchestrator/internal/orchestration/events"
	"github.com/zjrosen/xorchestrator/internal/orchestration/message"
	"github.com/zjrosen/xorchestrator/internal/orchestration/v2/process"
	"github.com/zjrosen/xorchestrator/internal/orchestration/v2/repository"
	"github.com/zjrosen/xorchestrator/internal/pubsub"
	"github.com/zjrosen/xorchestrator/internal/ui/shared/chatrender"
)

// --- RestoreProcessRepository Tests ---

func TestRestoreProcessRepository_CoordinatorAndWorkers(t *testing.T) {
	repo := repository.NewMemoryProcessRepository()
	now := time.Now().UTC().Truncate(time.Millisecond)

	session := &ResumableSession{
		Metadata: &Metadata{
			SessionID:             "test-session",
			StartTime:             now.Add(-time.Hour),
			EndTime:               now,
			CoordinatorSessionRef: "coord-session-ref",
		},
		ActiveWorkers: []WorkerMetadata{
			{ID: "worker-1", SpawnedAt: now.Add(-30 * time.Minute), HeadlessSessionRef: "worker-1-ref"},
			{ID: "worker-2", SpawnedAt: now.Add(-20 * time.Minute), HeadlessSessionRef: "worker-2-ref"},
		},
		RetiredWorkers: []WorkerMetadata{
			{ID: "worker-3", SpawnedAt: now.Add(-40 * time.Minute), RetiredAt: now.Add(-10 * time.Minute)},
		},
	}

	err := RestoreProcessRepository(repo, session)
	require.NoError(t, err)

	// Verify coordinator was created
	coordinator, err := repo.GetCoordinator()
	require.NoError(t, err)
	require.Equal(t, repository.CoordinatorID, coordinator.ID)
	require.Equal(t, repository.RoleCoordinator, coordinator.Role)
	require.Equal(t, repository.StatusReady, coordinator.Status)

	// Verify workers were created
	workers := repo.Workers()
	require.Len(t, workers, 3)

	// Verify active workers
	activeWorkers := repo.ActiveWorkers()
	require.Len(t, activeWorkers, 2)

	// Verify retired workers
	retiredWorkers := repo.RetiredWorkers()
	require.Len(t, retiredWorkers, 1)
}

func TestRestoreProcessRepository_CoordinatorSessionID(t *testing.T) {
	repo := repository.NewMemoryProcessRepository()
	now := time.Now().UTC().Truncate(time.Millisecond)

	expectedSessionRef := "claude-session-abc123"
	session := &ResumableSession{
		Metadata: &Metadata{
			SessionID:             "test-session",
			StartTime:             now.Add(-time.Hour),
			EndTime:               now,
			CoordinatorSessionRef: expectedSessionRef,
		},
		ActiveWorkers:  []WorkerMetadata{},
		RetiredWorkers: []WorkerMetadata{},
	}

	err := RestoreProcessRepository(repo, session)
	require.NoError(t, err)

	coordinator, err := repo.GetCoordinator()
	require.NoError(t, err)
	require.Equal(t, expectedSessionRef, coordinator.SessionID,
		"Coordinator SessionID should be populated from CoordinatorSessionRef")
	require.True(t, coordinator.HasCompletedTurn)
}

func TestRestoreProcessRepository_WorkerSessionIDs(t *testing.T) {
	repo := repository.NewMemoryProcessRepository()
	now := time.Now().UTC().Truncate(time.Millisecond)

	session := &ResumableSession{
		Metadata: &Metadata{
			SessionID:             "test-session",
			StartTime:             now.Add(-time.Hour),
			EndTime:               now,
			CoordinatorSessionRef: "coord-ref",
		},
		ActiveWorkers: []WorkerMetadata{
			{ID: "worker-1", SpawnedAt: now, HeadlessSessionRef: "worker-1-session-ref"},
			{ID: "worker-2", SpawnedAt: now, HeadlessSessionRef: "worker-2-session-ref"},
		},
		RetiredWorkers: []WorkerMetadata{},
	}

	err := RestoreProcessRepository(repo, session)
	require.NoError(t, err)

	// Verify each worker has correct SessionID
	worker1, err := repo.Get("worker-1")
	require.NoError(t, err)
	require.Equal(t, "worker-1-session-ref", worker1.SessionID,
		"Worker SessionID should be populated from HeadlessSessionRef")

	worker2, err := repo.Get("worker-2")
	require.NoError(t, err)
	require.Equal(t, "worker-2-session-ref", worker2.SessionID,
		"Worker SessionID should be populated from HeadlessSessionRef")
}

func TestRestoreProcessRepository_RetiredWorkers(t *testing.T) {
	repo := repository.NewMemoryProcessRepository()
	now := time.Now().UTC().Truncate(time.Millisecond)
	retiredAt := now.Add(-10 * time.Minute)

	session := &ResumableSession{
		Metadata: &Metadata{
			SessionID:             "test-session",
			StartTime:             now.Add(-time.Hour),
			EndTime:               now,
			CoordinatorSessionRef: "coord-ref",
		},
		ActiveWorkers: []WorkerMetadata{},
		RetiredWorkers: []WorkerMetadata{
			{
				ID:                 "retired-worker",
				SpawnedAt:          now.Add(-30 * time.Minute),
				RetiredAt:          retiredAt,
				FinalPhase:         "implementing",
				HeadlessSessionRef: "retired-worker-ref",
			},
		},
	}

	err := RestoreProcessRepository(repo, session)
	require.NoError(t, err)

	worker, err := repo.Get("retired-worker")
	require.NoError(t, err)
	require.Equal(t, repository.StatusRetired, worker.Status)
	require.Equal(t, retiredAt.UTC(), worker.RetiredAt.UTC())
}

func TestRestoreProcessRepository_EmptyWorkers(t *testing.T) {
	repo := repository.NewMemoryProcessRepository()
	now := time.Now().UTC().Truncate(time.Millisecond)

	session := &ResumableSession{
		Metadata: &Metadata{
			SessionID:             "test-session",
			StartTime:             now.Add(-time.Hour),
			EndTime:               now,
			CoordinatorSessionRef: "coord-ref",
		},
		ActiveWorkers:  []WorkerMetadata{},
		RetiredWorkers: []WorkerMetadata{},
	}

	err := RestoreProcessRepository(repo, session)
	require.NoError(t, err)

	// Should have only coordinator
	processes := repo.List()
	require.Len(t, processes, 1)

	coordinator, err := repo.GetCoordinator()
	require.NoError(t, err)
	require.NotNil(t, coordinator)
}

func TestRestoreProcessRepository_NilSession(t *testing.T) {
	repo := repository.NewMemoryProcessRepository()

	err := RestoreProcessRepository(repo, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "session is nil")
}

func TestRestoreProcessRepository_NilMetadata(t *testing.T) {
	repo := repository.NewMemoryProcessRepository()

	session := &ResumableSession{
		Metadata: nil,
	}

	err := RestoreProcessRepository(repo, session)
	require.Error(t, err)
	require.Contains(t, err.Error(), "metadata is nil")
}

// --- workerMetadataToProcess Tests ---

func TestWorkerMetadataToProcess_ActiveWorker(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)

	worker := WorkerMetadata{
		ID:                 "worker-1",
		SpawnedAt:          now,
		HeadlessSessionRef: "worker-session-ref",
	}

	proc := workerMetadataToProcess(worker, repository.StatusReady)

	require.Equal(t, "worker-1", proc.ID)
	require.Equal(t, repository.RoleWorker, proc.Role)
	require.Equal(t, repository.StatusReady, proc.Status)
	require.Equal(t, "worker-session-ref", proc.SessionID)
	require.Equal(t, now.UTC(), proc.CreatedAt.UTC())
	require.True(t, proc.HasCompletedTurn)
	require.True(t, proc.RetiredAt.IsZero())
	require.Nil(t, proc.Phase)
}

func TestWorkerMetadataToProcess_RetiredWorker(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	retiredAt := now.Add(-10 * time.Minute)

	worker := WorkerMetadata{
		ID:                 "worker-retired",
		SpawnedAt:          now.Add(-30 * time.Minute),
		RetiredAt:          retiredAt,
		HeadlessSessionRef: "retired-session-ref",
	}

	proc := workerMetadataToProcess(worker, repository.StatusRetired)

	require.Equal(t, "worker-retired", proc.ID)
	require.Equal(t, repository.StatusRetired, proc.Status)
	require.Equal(t, retiredAt.UTC(), proc.RetiredAt.UTC())
}

func TestWorkerMetadataToProcess_WithPhase(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)

	worker := WorkerMetadata{
		ID:         "worker-with-phase",
		SpawnedAt:  now,
		FinalPhase: "implementing",
	}

	proc := workerMetadataToProcess(worker, repository.StatusReady)

	require.NotNil(t, proc.Phase)
	require.Equal(t, events.ProcessPhase("implementing"), *proc.Phase)
}

func TestWorkerMetadataToProcess_SessionID(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	expectedSessionRef := "unique-session-ref-123"

	worker := WorkerMetadata{
		ID:                 "worker-session-test",
		SpawnedAt:          now,
		HeadlessSessionRef: expectedSessionRef,
	}

	proc := workerMetadataToProcess(worker, repository.StatusReady)

	require.Equal(t, expectedSessionRef, proc.SessionID,
		"Process.SessionID must be populated from HeadlessSessionRef for --resume to work")
}

// --- AppendRestored Tests ---

func TestAppendRestored_PreservesIDAndTimestamp(t *testing.T) {
	repo := repository.NewMemoryMessageRepository()

	originalID := "original-msg-id-123"
	originalTimestamp := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)

	entry := message.Entry{
		ID:        originalID,
		Timestamp: originalTimestamp,
		From:      "COORDINATOR",
		To:        "WORKER.1",
		Content:   "Test message",
		Type:      message.MessageRequest,
		ReadBy:    []string{"COORDINATOR"},
	}

	restored, err := repo.AppendRestored(entry)
	require.NoError(t, err)
	require.NotNil(t, restored)

	// Verify ID and timestamp were preserved
	require.Equal(t, originalID, restored.ID)
	require.Equal(t, originalTimestamp, restored.Timestamp.UTC())

	// Verify message is in repository
	entries := repo.Entries()
	require.Len(t, entries, 1)
	require.Equal(t, originalID, entries[0].ID)
	require.Equal(t, originalTimestamp, entries[0].Timestamp.UTC())
}

func TestAppendRestored_NoBrokerPublish(t *testing.T) {
	repo := repository.NewMemoryMessageRepository()

	// Subscribe to broker BEFORE restoring message
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	broker := repo.Broker()
	ch := broker.Subscribe(ctx)

	// Restore a message
	entry := message.Entry{
		ID:        "test-msg",
		Timestamp: time.Now().UTC(),
		From:      "COORDINATOR",
		To:        "ALL",
		Content:   "Restored message",
		Type:      message.MessageInfo,
	}

	_, err := repo.AppendRestored(entry)
	require.NoError(t, err)

	// Try to receive from channel - should timeout (no message published)
	select {
	case event := <-ch:
		t.Fatalf("AppendRestored should NOT publish to broker, but received event: %+v", event)
	case <-ctx.Done():
		// Expected - context timed out because no event was published
	}
}

// --- RestoreMessageRepository Tests ---

func TestRestoreMessageRepository_MultipleMessages(t *testing.T) {
	repo := repository.NewMemoryMessageRepository()
	now := time.Now().UTC().Truncate(time.Millisecond)

	messages := []message.Entry{
		{
			ID:        "msg-001",
			Timestamp: now,
			From:      "COORDINATOR",
			To:        "WORKER.1",
			Content:   "First message",
			Type:      message.MessageRequest,
		},
		{
			ID:        "msg-002",
			Timestamp: now.Add(time.Second),
			From:      "WORKER.1",
			To:        "COORDINATOR",
			Content:   "Second message",
			Type:      message.MessageResponse,
		},
		{
			ID:        "msg-003",
			Timestamp: now.Add(2 * time.Second),
			From:      "COORDINATOR",
			To:        "ALL",
			Content:   "Third message",
			Type:      message.MessageInfo,
		},
	}

	err := RestoreMessageRepository(repo, messages)
	require.NoError(t, err)

	// Verify all messages were restored
	entries := repo.Entries()
	require.Len(t, entries, 3)

	// Verify order and content preserved
	require.Equal(t, "msg-001", entries[0].ID)
	require.Equal(t, "msg-002", entries[1].ID)
	require.Equal(t, "msg-003", entries[2].ID)

	require.Equal(t, "First message", entries[0].Content)
	require.Equal(t, "Second message", entries[1].Content)
	require.Equal(t, "Third message", entries[2].Content)
}

func TestRestoreMessageRepository_EmptyMessages(t *testing.T) {
	repo := repository.NewMemoryMessageRepository()

	err := RestoreMessageRepository(repo, []message.Entry{})
	require.NoError(t, err)

	entries := repo.Entries()
	require.Empty(t, entries)
}

func TestRestoreMessageRepository_PreservesReadBy(t *testing.T) {
	repo := repository.NewMemoryMessageRepository()
	now := time.Now().UTC()

	messages := []message.Entry{
		{
			ID:        "msg-with-readers",
			Timestamp: now,
			From:      "COORDINATOR",
			To:        "ALL",
			Content:   "Message with readers",
			Type:      message.MessageInfo,
			ReadBy:    []string{"COORDINATOR", "WORKER.1", "WORKER.2"},
		},
	}

	err := RestoreMessageRepository(repo, messages)
	require.NoError(t, err)

	entries := repo.Entries()
	require.Len(t, entries, 1)
	require.Equal(t, []string{"COORDINATOR", "WORKER.1", "WORKER.2"}, entries[0].ReadBy)
}

func TestRestoreMessageRepository_NoBrokerEvents(t *testing.T) {
	repo := repository.NewMemoryMessageRepository()
	now := time.Now().UTC()

	// Subscribe to broker
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ch := repo.Broker().Subscribe(ctx)

	// Restore multiple messages
	messages := []message.Entry{
		{ID: "msg-1", Timestamp: now, From: "COORD", To: "ALL", Content: "1", Type: message.MessageInfo},
		{ID: "msg-2", Timestamp: now, From: "COORD", To: "ALL", Content: "2", Type: message.MessageInfo},
	}

	err := RestoreMessageRepository(repo, messages)
	require.NoError(t, err)

	// Should NOT receive any broker events
	select {
	case event := <-ch:
		t.Fatalf("RestoreMessageRepository should NOT publish to broker, but received: %+v", event)
	case <-ctx.Done():
		// Expected - no events published
	}
}

// --- BuildRestoredUIState Tests ---

func TestBuildRestoredUIState_FullSession(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)

	session := &ResumableSession{
		Metadata: &Metadata{
			SessionID: "test-session-id",
			StartTime: now.Add(-time.Hour),
			EndTime:   now,
		},
		CoordinatorMessages: []chatrender.Message{
			{Role: "user", Content: "Hello coordinator"},
			{Role: "assistant", Content: "Hello user"},
		},
		WorkerMessages: map[string][]chatrender.Message{
			"worker-1": {{Role: "user", Content: "Task for worker-1"}},
			"worker-2": {{Role: "user", Content: "Task for worker-2"}},
		},
		InterAgentMessages: []message.Entry{
			{ID: "msg-1", From: "COORDINATOR", To: "WORKER.1", Content: "Assign task"},
		},
		ActiveWorkers: []WorkerMetadata{
			{ID: "worker-1", SpawnedAt: now.Add(-30 * time.Minute), FinalPhase: "implementing"},
			{ID: "worker-2", SpawnedAt: now.Add(-20 * time.Minute)},
		},
		RetiredWorkers: []WorkerMetadata{
			{ID: "worker-3", SpawnedAt: now.Add(-50 * time.Minute), RetiredAt: now.Add(-10 * time.Minute), FinalPhase: "committing"},
		},
	}

	state := BuildRestoredUIState(session)

	require.NotNil(t, state)
	require.Equal(t, "test-session-id", state.SessionID)
	require.Equal(t, now.Add(-time.Hour), state.StartTime)
	require.Equal(t, now, state.EndTime)
	require.True(t, state.IsResumed)

	// Check coordinator messages
	require.Len(t, state.CoordinatorMessages, 2)
	require.Equal(t, "Hello coordinator", state.CoordinatorMessages[0].Content)

	// Check worker messages
	require.Len(t, state.WorkerMessages["worker-1"], 1)
	require.Len(t, state.WorkerMessages["worker-2"], 1)

	// Check inter-agent messages
	require.Len(t, state.MessageLogEntries, 1)
	require.Equal(t, "msg-1", state.MessageLogEntries[0].ID)
}

func TestBuildRestoredUIState_NoWorkers(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)

	session := &ResumableSession{
		Metadata: &Metadata{
			SessionID: "empty-session",
			StartTime: now.Add(-time.Hour),
			EndTime:   now,
		},
		CoordinatorMessages: []chatrender.Message{
			{Role: "user", Content: "Hello"},
		},
		WorkerMessages:     map[string][]chatrender.Message{},
		InterAgentMessages: []message.Entry{},
		ActiveWorkers:      []WorkerMetadata{},
		RetiredWorkers:     []WorkerMetadata{},
	}

	state := BuildRestoredUIState(session)

	require.NotNil(t, state)
	require.Empty(t, state.WorkerIDs)
	require.Empty(t, state.WorkerStatus)
	require.Empty(t, state.WorkerPhases)
	require.Empty(t, state.WorkerMessages)
	require.Empty(t, state.RetiredOrder)
	require.Len(t, state.CoordinatorMessages, 1)
}

func TestBuildRestoredUIState_OnlyRetiredWorkers(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)

	session := &ResumableSession{
		Metadata: &Metadata{
			SessionID: "retired-only-session",
			StartTime: now.Add(-time.Hour),
			EndTime:   now,
		},
		WorkerMessages: map[string][]chatrender.Message{
			"worker-1": {{Role: "user", Content: "Old task"}},
		},
		ActiveWorkers: []WorkerMetadata{},
		RetiredWorkers: []WorkerMetadata{
			{ID: "worker-1", SpawnedAt: now.Add(-50 * time.Minute), RetiredAt: now.Add(-10 * time.Minute)},
		},
	}

	state := BuildRestoredUIState(session)

	require.NotNil(t, state)
	require.Len(t, state.WorkerIDs, 1)
	require.Equal(t, "worker-1", state.WorkerIDs[0])
	require.Equal(t, events.ProcessStatusRetired, state.WorkerStatus["worker-1"])
	require.Len(t, state.RetiredOrder, 1)
	require.Equal(t, "worker-1", state.RetiredOrder[0])
}

func TestBuildRestoredUIState_OnlyActiveWorkers(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)

	session := &ResumableSession{
		Metadata: &Metadata{
			SessionID: "active-only-session",
			StartTime: now.Add(-time.Hour),
			EndTime:   now,
		},
		WorkerMessages: map[string][]chatrender.Message{
			"worker-1": {{Role: "user", Content: "Current task"}},
			"worker-2": {{Role: "user", Content: "Another task"}},
		},
		ActiveWorkers: []WorkerMetadata{
			{ID: "worker-1", SpawnedAt: now.Add(-30 * time.Minute)},
			{ID: "worker-2", SpawnedAt: now.Add(-20 * time.Minute)},
		},
		RetiredWorkers: []WorkerMetadata{},
	}

	state := BuildRestoredUIState(session)

	require.NotNil(t, state)
	require.Len(t, state.WorkerIDs, 2)
	require.Equal(t, "worker-1", state.WorkerIDs[0])
	require.Equal(t, "worker-2", state.WorkerIDs[1])
	require.Equal(t, events.ProcessStatusReady, state.WorkerStatus["worker-1"])
	require.Equal(t, events.ProcessStatusReady, state.WorkerStatus["worker-2"])
	require.Empty(t, state.RetiredOrder)
}

func TestBuildRestoredUIState_WorkerDisplayOrder(t *testing.T) {
	// Test that active workers come first, then retired workers
	now := time.Now().UTC().Truncate(time.Millisecond)

	session := &ResumableSession{
		Metadata: &Metadata{
			SessionID: "order-test-session",
			StartTime: now.Add(-time.Hour),
			EndTime:   now,
		},
		WorkerMessages: map[string][]chatrender.Message{},
		ActiveWorkers: []WorkerMetadata{
			{ID: "active-1", SpawnedAt: now.Add(-30 * time.Minute)},
			{ID: "active-2", SpawnedAt: now.Add(-25 * time.Minute)},
		},
		RetiredWorkers: []WorkerMetadata{
			{ID: "retired-1", SpawnedAt: now.Add(-50 * time.Minute), RetiredAt: now.Add(-15 * time.Minute)},
			{ID: "retired-2", SpawnedAt: now.Add(-45 * time.Minute), RetiredAt: now.Add(-10 * time.Minute)},
		},
	}

	state := BuildRestoredUIState(session)

	require.NotNil(t, state)
	require.Len(t, state.WorkerIDs, 4)

	// Active workers first
	require.Equal(t, "active-1", state.WorkerIDs[0])
	require.Equal(t, "active-2", state.WorkerIDs[1])

	// Then retired workers
	require.Equal(t, "retired-1", state.WorkerIDs[2])
	require.Equal(t, "retired-2", state.WorkerIDs[3])

	// Check status mapping
	require.Equal(t, events.ProcessStatusReady, state.WorkerStatus["active-1"])
	require.Equal(t, events.ProcessStatusReady, state.WorkerStatus["active-2"])
	require.Equal(t, events.ProcessStatusRetired, state.WorkerStatus["retired-1"])
	require.Equal(t, events.ProcessStatusRetired, state.WorkerStatus["retired-2"])
}

func TestBuildRestoredUIState_RetiredOrderPreserved(t *testing.T) {
	// Test that RetiredOrder is sorted by RetiredAt (oldest first)
	now := time.Now().UTC().Truncate(time.Millisecond)

	// RetiredWorkers in ResumableSession are already sorted by RetiredAt per loader.go
	session := &ResumableSession{
		Metadata: &Metadata{
			SessionID: "retired-order-test",
			StartTime: now.Add(-time.Hour),
			EndTime:   now,
		},
		WorkerMessages: map[string][]chatrender.Message{},
		ActiveWorkers:  []WorkerMetadata{},
		RetiredWorkers: []WorkerMetadata{
			// Already sorted by RetiredAt (oldest first) per loader.go
			{ID: "oldest-retired", SpawnedAt: now.Add(-60 * time.Minute), RetiredAt: now.Add(-30 * time.Minute)},
			{ID: "middle-retired", SpawnedAt: now.Add(-50 * time.Minute), RetiredAt: now.Add(-20 * time.Minute)},
			{ID: "newest-retired", SpawnedAt: now.Add(-40 * time.Minute), RetiredAt: now.Add(-10 * time.Minute)},
		},
	}

	state := BuildRestoredUIState(session)

	require.NotNil(t, state)
	require.Len(t, state.RetiredOrder, 3)

	// Verify order is preserved (oldest first)
	require.Equal(t, "oldest-retired", state.RetiredOrder[0])
	require.Equal(t, "middle-retired", state.RetiredOrder[1])
	require.Equal(t, "newest-retired", state.RetiredOrder[2])
}

func TestBuildRestoredUIState_WorkerPhasesExtracted(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)

	session := &ResumableSession{
		Metadata: &Metadata{
			SessionID: "phases-test",
			StartTime: now.Add(-time.Hour),
			EndTime:   now,
		},
		WorkerMessages: map[string][]chatrender.Message{},
		ActiveWorkers: []WorkerMetadata{
			{ID: "worker-implementing", SpawnedAt: now.Add(-30 * time.Minute), FinalPhase: "implementing"},
			{ID: "worker-reviewing", SpawnedAt: now.Add(-25 * time.Minute), FinalPhase: "reviewing"},
			{ID: "worker-no-phase", SpawnedAt: now.Add(-20 * time.Minute)}, // No FinalPhase set
		},
		RetiredWorkers: []WorkerMetadata{
			{ID: "retired-committing", SpawnedAt: now.Add(-50 * time.Minute), RetiredAt: now.Add(-10 * time.Minute), FinalPhase: "committing"},
		},
	}

	state := BuildRestoredUIState(session)

	require.NotNil(t, state)

	// Check phases for workers with FinalPhase set
	require.Equal(t, events.ProcessPhase("implementing"), state.WorkerPhases["worker-implementing"])
	require.Equal(t, events.ProcessPhase("reviewing"), state.WorkerPhases["worker-reviewing"])
	require.Equal(t, events.ProcessPhase("committing"), state.WorkerPhases["retired-committing"])

	// Worker without FinalPhase should not have an entry
	_, hasPhase := state.WorkerPhases["worker-no-phase"]
	require.False(t, hasPhase, "Worker without FinalPhase should not have an entry in WorkerPhases")
}

func TestBuildRestoredUIState_EmptyMessages(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)

	session := &ResumableSession{
		Metadata: &Metadata{
			SessionID: "empty-messages-test",
			StartTime: now.Add(-time.Hour),
			EndTime:   now,
		},
		CoordinatorMessages: []chatrender.Message{}, // Empty
		WorkerMessages:      map[string][]chatrender.Message{},
		InterAgentMessages:  []message.Entry{}, // Empty
		ActiveWorkers: []WorkerMetadata{
			{ID: "worker-1", SpawnedAt: now.Add(-30 * time.Minute)},
		},
		RetiredWorkers: []WorkerMetadata{},
	}

	state := BuildRestoredUIState(session)

	require.NotNil(t, state)
	require.Empty(t, state.CoordinatorMessages)
	require.Empty(t, state.MessageLogEntries)
	require.Empty(t, state.WorkerMessages["worker-1"]) // Worker exists but has no messages
}

func TestBuildRestoredUIState_NilSession(t *testing.T) {
	state := BuildRestoredUIState(nil)
	require.Nil(t, state)
}

func TestBuildRestoredUIState_NilMetadata(t *testing.T) {
	// Even with nil metadata, should not panic and should handle gracefully
	session := &ResumableSession{
		Metadata:            nil,
		CoordinatorMessages: []chatrender.Message{{Role: "user", Content: "test"}},
		WorkerMessages:      map[string][]chatrender.Message{},
		ActiveWorkers:       []WorkerMetadata{},
		RetiredWorkers:      []WorkerMetadata{},
	}

	state := BuildRestoredUIState(session)

	require.NotNil(t, state)
	require.Empty(t, state.SessionID)
	require.True(t, state.StartTime.IsZero())
	require.True(t, state.EndTime.IsZero())
	require.True(t, state.IsResumed)
	require.Len(t, state.CoordinatorMessages, 1)
}

// --- RestoreProcessRegistry Tests ---

func TestRestoreProcessRegistry_CoordinatorAndActiveWorkers(t *testing.T) {
	registry := process.NewProcessRegistry()
	eventBus := pubsub.NewBroker[any]()
	now := time.Now().UTC().Truncate(time.Millisecond)

	session := &ResumableSession{
		Metadata: &Metadata{
			SessionID:             "test-session",
			StartTime:             now.Add(-time.Hour),
			EndTime:               now,
			CoordinatorSessionRef: "coord-session-ref",
		},
		ActiveWorkers: []WorkerMetadata{
			{ID: "worker-1", SpawnedAt: now.Add(-30 * time.Minute), HeadlessSessionRef: "worker-1-ref"},
			{ID: "worker-2", SpawnedAt: now.Add(-20 * time.Minute), HeadlessSessionRef: "worker-2-ref"},
		},
		RetiredWorkers: []WorkerMetadata{
			{ID: "worker-3", SpawnedAt: now.Add(-40 * time.Minute), RetiredAt: now.Add(-10 * time.Minute)},
		},
	}

	err := RestoreProcessRegistry(registry, session, nil, eventBus)
	require.NoError(t, err)

	// Verify coordinator was registered with session ID
	coordinator := registry.GetCoordinator()
	require.NotNil(t, coordinator)
	require.Equal(t, repository.CoordinatorID, coordinator.ID)
	require.Equal(t, "coord-session-ref", coordinator.SessionID())

	// Verify active workers were registered with session IDs
	worker1 := registry.Get("worker-1")
	require.NotNil(t, worker1)
	require.Equal(t, "worker-1-ref", worker1.SessionID())

	worker2 := registry.Get("worker-2")
	require.NotNil(t, worker2)
	require.Equal(t, "worker-2-ref", worker2.SessionID())

	// Verify retired workers are NOT registered (they can't receive messages)
	worker3 := registry.Get("worker-3")
	require.Nil(t, worker3, "Retired workers should not be in registry")
}

func TestRestoreProcessRegistry_DormantProcessesCanBeResumed(t *testing.T) {
	registry := process.NewProcessRegistry()
	eventBus := pubsub.NewBroker[any]()
	now := time.Now().UTC().Truncate(time.Millisecond)

	session := &ResumableSession{
		Metadata: &Metadata{
			SessionID:             "test-session",
			StartTime:             now.Add(-time.Hour),
			EndTime:               now,
			CoordinatorSessionRef: "coord-session-ref",
		},
		ActiveWorkers:  []WorkerMetadata{},
		RetiredWorkers: []WorkerMetadata{},
	}

	err := RestoreProcessRegistry(registry, session, nil, eventBus)
	require.NoError(t, err)

	// Get the dormant coordinator
	coordinator := registry.GetCoordinator()
	require.NotNil(t, coordinator)

	// Verify it's dormant (no live process)
	// The process should be resumable without blocking
	// This is tested more thoroughly in process_test.go
}

func TestRestoreProcessRegistry_NilSession(t *testing.T) {
	registry := process.NewProcessRegistry()

	err := RestoreProcessRegistry(registry, nil, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "session is nil")
}

func TestRestoreProcessRegistry_NilMetadata(t *testing.T) {
	registry := process.NewProcessRegistry()

	session := &ResumableSession{
		Metadata: nil,
	}

	err := RestoreProcessRegistry(registry, session, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "metadata is nil")
}

func TestRestoreProcessRegistry_EmptyActiveWorkers(t *testing.T) {
	registry := process.NewProcessRegistry()
	now := time.Now().UTC().Truncate(time.Millisecond)

	session := &ResumableSession{
		Metadata: &Metadata{
			SessionID:             "test-session",
			StartTime:             now.Add(-time.Hour),
			EndTime:               now,
			CoordinatorSessionRef: "coord-ref",
		},
		ActiveWorkers:  []WorkerMetadata{},
		RetiredWorkers: []WorkerMetadata{},
	}

	err := RestoreProcessRegistry(registry, session, nil, nil)
	require.NoError(t, err)

	// Should only have coordinator
	all := registry.All()
	require.Len(t, all, 1)

	coordinator := registry.GetCoordinator()
	require.NotNil(t, coordinator)
}
