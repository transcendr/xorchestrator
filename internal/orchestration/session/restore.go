// Package session provides session tracking for orchestration mode.
// restore.go provides functions for restoring repository state from session data.
package session

import (
	"fmt"
	"time"

	"github.com/zjrosen/xorchestrator/internal/orchestration/events"
	"github.com/zjrosen/xorchestrator/internal/orchestration/message"
	"github.com/zjrosen/xorchestrator/internal/orchestration/v2/process"
	"github.com/zjrosen/xorchestrator/internal/orchestration/v2/repository"
	"github.com/zjrosen/xorchestrator/internal/pubsub"
	"github.com/zjrosen/xorchestrator/internal/ui/shared/chatrender"
)

// RestoreProcessRepository populates a ProcessRepository from loaded session data.
// Creates process entities for coordinator and all workers (active and retired).
//
// CRITICAL: The SessionID field is populated from saved session refs - this enables
// seamless resume when messages start flowing (spawn uses --resume flag).
// - Coordinator gets SessionID from Metadata.CoordinatorSessionRef
// - Workers get SessionID from WorkerMetadata.HeadlessSessionRef
func RestoreProcessRepository(repo repository.ProcessRepository, session *ResumableSession) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}

	meta := session.Metadata
	if meta == nil {
		return fmt.Errorf("session metadata is nil")
	}

	// Restore coordinator with its session ref
	coordinator := &repository.Process{
		ID:               repository.CoordinatorID,
		Role:             repository.RoleCoordinator,
		Status:           repository.StatusReady, // Ready for user input (not running yet)
		SessionID:        meta.CoordinatorSessionRef,
		CreatedAt:        meta.StartTime,
		LastActivityAt:   meta.EndTime,
		HasCompletedTurn: true, // Had previous turns
	}
	if err := repo.Save(coordinator); err != nil {
		return fmt.Errorf("failed to save coordinator: %w", err)
	}

	// Restore active workers with their session refs
	for _, w := range session.ActiveWorkers {
		proc := workerMetadataToProcess(w, repository.StatusReady)
		if err := repo.Save(proc); err != nil {
			return fmt.Errorf("failed to save worker %s: %w", w.ID, err)
		}
	}

	// Restore retired workers
	for _, w := range session.RetiredWorkers {
		proc := workerMetadataToProcess(w, repository.StatusRetired)
		if err := repo.Save(proc); err != nil {
			return fmt.Errorf("failed to save retired worker %s: %w", w.ID, err)
		}
	}

	return nil
}

// RestoreProcessRegistry populates a ProcessRegistry with dormant processes from session data.
// Creates dormant process.Process instances for coordinator and active workers only.
// Retired workers are NOT added to the registry (they can't receive messages).
//
// Dormant processes:
// - Have session IDs set (for --resume flag when spawning)
// - Have no live AI subprocess attached
// - Can be activated via Resume() when a message is delivered
//
// Parameters:
//   - registry: ProcessRegistry to populate with dormant processes
//   - session: loaded session data containing coordinator and worker metadata
//   - submitter: CommandSubmitter for processes to submit commands on state transitions
//   - eventBus: event bus for processes to publish events
func RestoreProcessRegistry(
	registry *process.ProcessRegistry,
	session *ResumableSession,
	submitter process.CommandSubmitter,
	eventBus *pubsub.Broker[any],
) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}

	meta := session.Metadata
	if meta == nil {
		return fmt.Errorf("session metadata is nil")
	}

	// Restore coordinator as dormant process
	coordinator := process.NewDormant(
		repository.CoordinatorID,
		repository.RoleCoordinator,
		meta.CoordinatorSessionRef,
		submitter,
		eventBus,
	)
	registry.Register(coordinator)

	// Restore active workers as dormant processes
	// Retired workers are NOT added - they can't receive messages
	for _, w := range session.ActiveWorkers {
		worker := process.NewDormant(
			w.ID,
			repository.RoleWorker,
			w.HeadlessSessionRef,
			submitter,
			eventBus,
		)
		registry.Register(worker)
	}

	return nil
}

// workerMetadataToProcess converts WorkerMetadata to a repository.Process entity.
// The status parameter determines whether this is an active or retired worker.
//
// Key points:
// - SessionID is populated from HeadlessSessionRef to enable --resume
// - Phase is restored from FinalPhase if available
// - RetiredAt is preserved for retired workers
func workerMetadataToProcess(w WorkerMetadata, status repository.ProcessStatus) *repository.Process {
	proc := &repository.Process{
		ID:               w.ID,
		Role:             repository.RoleWorker,
		Status:           status,
		SessionID:        w.HeadlessSessionRef, // KEY: enables --resume for workers
		CreatedAt:        w.SpawnedAt,
		LastActivityAt:   w.SpawnedAt,
		HasCompletedTurn: true, // Had previous turns
		RetiredAt:        w.RetiredAt,
	}

	// Restore phase if available
	if w.FinalPhase != "" {
		phase := events.ProcessPhase(w.FinalPhase)
		proc.Phase = &phase
	}

	return proc
}

// RestoreMessageRepository populates a MessageRepository from loaded inter-agent messages.
// Uses AppendRestored to preserve existing IDs and timestamps without triggering broker events.
//
// The MessageRepository interface includes AppendRestored which is specifically designed
// for session restoration - it preserves existing IDs and timestamps, and critically
// does NOT publish to the broker (to avoid duplicate display in TUI).
func RestoreMessageRepository(repo repository.MessageRepository, messages []message.Entry) error {
	for _, entry := range messages {
		if _, err := repo.AppendRestored(entry); err != nil {
			return fmt.Errorf("failed to restore message %s: %w", entry.ID, err)
		}
	}
	return nil
}

// RestoredUIState contains all TUI-ready state extracted from a loaded session.
// This is the output of BuildRestoredUIState and is used by the orchestration model
// to populate its panes on session resume.
type RestoredUIState struct {
	// CoordinatorMessages contains the coordinator's chat history ready for display.
	CoordinatorMessages []chatrender.Message

	// WorkerIDs is the ordered list of worker IDs for display.
	// Order: active workers first, then retired workers (sorted by RetiredAt).
	WorkerIDs []string

	// WorkerStatus maps worker IDs to their current status (Ready or Retired).
	WorkerStatus map[string]events.ProcessStatus

	// WorkerPhases maps worker IDs to their current/final phase.
	WorkerPhases map[string]events.ProcessPhase

	// WorkerMessages maps worker IDs to their chat histories.
	WorkerMessages map[string][]chatrender.Message

	// RetiredOrder contains retired worker IDs sorted by RetiredAt (oldest first).
	RetiredOrder []string

	// MessageLogEntries contains the inter-agent communication log.
	MessageLogEntries []message.Entry

	// SessionID is the unique identifier for this session.
	SessionID string

	// StartTime is when the session was originally started.
	StartTime time.Time

	// EndTime is when the session was last saved.
	EndTime time.Time

	// IsResumed indicates this is a restored session.
	IsResumed bool
}

// BuildRestoredUIState extracts TUI-ready state from a loaded ResumableSession.
// It transforms the session data into a format suitable for directly populating
// the orchestration model's panes.
//
// Worker display order:
// - Active workers come first (in their original order)
// - Retired workers come after (sorted by RetiredAt, oldest first)
//
// Returns nil if the session is nil.
func BuildRestoredUIState(session *ResumableSession) *RestoredUIState {
	if session == nil {
		return nil
	}

	state := &RestoredUIState{
		WorkerStatus:   make(map[string]events.ProcessStatus),
		WorkerPhases:   make(map[string]events.ProcessPhase),
		WorkerMessages: make(map[string][]chatrender.Message),
		IsResumed:      true,
	}

	// Extract metadata
	if session.Metadata != nil {
		state.SessionID = session.Metadata.SessionID
		state.StartTime = session.Metadata.StartTime
		state.EndTime = session.Metadata.EndTime
	}

	// Extract coordinator messages
	state.CoordinatorMessages = session.CoordinatorMessages

	// Build worker IDs list: active workers first
	for _, w := range session.ActiveWorkers {
		state.WorkerIDs = append(state.WorkerIDs, w.ID)
		state.WorkerStatus[w.ID] = events.ProcessStatusReady

		// Extract phase from FinalPhase if available
		if w.FinalPhase != "" {
			state.WorkerPhases[w.ID] = events.ProcessPhase(w.FinalPhase)
		}

		// Copy messages
		if msgs, ok := session.WorkerMessages[w.ID]; ok {
			state.WorkerMessages[w.ID] = msgs
		}
	}

	// Build retired order sorted by RetiredAt (oldest first)
	// Note: ResumableSession.RetiredWorkers is already sorted by RetiredAt per loader.go
	state.RetiredOrder = make([]string, 0, len(session.RetiredWorkers))
	for _, w := range session.RetiredWorkers {
		state.RetiredOrder = append(state.RetiredOrder, w.ID)
	}

	// Add retired workers to WorkerIDs (after active workers)
	for _, w := range session.RetiredWorkers {
		state.WorkerIDs = append(state.WorkerIDs, w.ID)
		state.WorkerStatus[w.ID] = events.ProcessStatusRetired

		// Extract phase from FinalPhase if available
		if w.FinalPhase != "" {
			state.WorkerPhases[w.ID] = events.ProcessPhase(w.FinalPhase)
		}

		// Copy messages
		if msgs, ok := session.WorkerMessages[w.ID]; ok {
			state.WorkerMessages[w.ID] = msgs
		}
	}

	// Extract inter-agent messages
	state.MessageLogEntries = session.InterAgentMessages

	return state
}
