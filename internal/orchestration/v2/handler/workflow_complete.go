// Package handler provides command handlers for the v2 orchestration architecture.
// This file contains the handler for workflow completion signals.
package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/sound"
)

// ===========================================================================
// Session Metadata Provider Interface
// ===========================================================================

// SessionMetadataProvider provides access to session metadata for workflow completion.
// Implementations must be thread-safe.
type SessionMetadataProvider interface {
	// GetWorkflowCompletedAt returns the workflow completion timestamp.
	// Returns zero time if workflow has not been completed.
	GetWorkflowCompletedAt() time.Time

	// UpdateWorkflowCompletion updates the workflow completion fields in session metadata.
	// If WorkflowCompletedAt is already set (non-zero), it should be preserved for idempotency.
	UpdateWorkflowCompletion(status, summary string, completedAt time.Time) error
}

// ===========================================================================
// SignalWorkflowCompleteHandler
// ===========================================================================

// SignalWorkflowCompleteHandler handles CmdSignalWorkflowComplete commands.
// It updates session metadata with workflow completion status and publishes events.
type SignalWorkflowCompleteHandler struct {
	sessionProvider SessionMetadataProvider
	soundService    sound.SoundService
}

// SignalWorkflowCompleteHandlerOption configures SignalWorkflowCompleteHandler.
type SignalWorkflowCompleteHandlerOption func(*SignalWorkflowCompleteHandler)

// WithSessionMetadataProvider sets the session metadata provider for workflow completion.
func WithSessionMetadataProvider(provider SessionMetadataProvider) SignalWorkflowCompleteHandlerOption {
	return func(h *SignalWorkflowCompleteHandler) {
		h.sessionProvider = provider
	}
}

// WithWorkflowSoundService sets the sound service for audio feedback on workflow completion.
// If svc is nil, the handler keeps its default NoopSoundService.
func WithWorkflowSoundService(svc sound.SoundService) SignalWorkflowCompleteHandlerOption {
	return func(h *SignalWorkflowCompleteHandler) {
		if svc != nil {
			h.soundService = svc
		}
	}
}

// NewSignalWorkflowCompleteHandler creates a new SignalWorkflowCompleteHandler.
func NewSignalWorkflowCompleteHandler(opts ...SignalWorkflowCompleteHandlerOption) *SignalWorkflowCompleteHandler {
	h := &SignalWorkflowCompleteHandler{
		soundService: sound.NoopSoundService{},
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Handle processes a SignalWorkflowCompleteCommand.
// 1. Validates status is one of "success", "partial", or "aborted"
// 2. Updates session metadata with completion fields (preserving original timestamp for idempotency)
// 3. Publishes ProcessWorkflowComplete event to event bus
func (h *SignalWorkflowCompleteHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	workflowCmd := cmd.(*command.SignalWorkflowCompleteCommand)

	// 1. Validate the command (status and summary validation handled by command.Validate())
	if err := workflowCmd.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// 2. Update session metadata with completion fields
	// For idempotency, preserve the original completion timestamp if already set
	var completedAt time.Time
	isFirstCall := true

	if h.sessionProvider != nil {
		existingTimestamp := h.sessionProvider.GetWorkflowCompletedAt()
		if !existingTimestamp.IsZero() {
			// Subsequent call - preserve original timestamp
			completedAt = existingTimestamp
			isFirstCall = false
		} else {
			// First call - set new timestamp
			completedAt = time.Now()
		}

		if err := h.sessionProvider.UpdateWorkflowCompletion(
			string(workflowCmd.Status),
			workflowCmd.Summary,
			completedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to update session metadata: %w", err)
		}
	} else {
		// No session provider - use current time for event
		completedAt = time.Now()
	}

	// 3. Build ProcessWorkflowComplete event
	// Always emit event (including on duplicate calls) for audit trail
	event := events.ProcessEvent{
		Type:      events.ProcessWorkflowComplete,
		ProcessID: "coordinator",
		Role:      events.RoleCoordinator,
	}

	result := &SignalWorkflowCompleteResult{
		Status:      workflowCmd.Status,
		Summary:     workflowCmd.Summary,
		CompletedAt: completedAt,
		IsFirstCall: isFirstCall,
	}

	// 4. Play completion sound only on first call (not on duplicate signals)
	if isFirstCall {
		h.soundService.Play("complete", "workflow_complete")
	}

	return SuccessWithEvents(result, event), nil
}

// SignalWorkflowCompleteResult contains the result of signaling workflow completion.
type SignalWorkflowCompleteResult struct {
	Status      command.WorkflowStatus
	Summary     string
	CompletedAt time.Time
	IsFirstCall bool // True if this is the first completion signal (timestamp was set)
}
