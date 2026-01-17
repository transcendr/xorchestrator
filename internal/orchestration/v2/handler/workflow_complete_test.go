package handler_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/xorchestrator/internal/mocks"
	"github.com/zjrosen/xorchestrator/internal/orchestration/events"
	"github.com/zjrosen/xorchestrator/internal/orchestration/v2/command"
	"github.com/zjrosen/xorchestrator/internal/orchestration/v2/handler"
	"github.com/zjrosen/xorchestrator/internal/sound"
)

// ===========================================================================
// Mock Session Metadata Provider
// ===========================================================================

type mockSessionMetadataProvider struct {
	workflowCompletedAt time.Time
	status              string
	summary             string
	updateError         error
	updateCalled        bool
}

func (m *mockSessionMetadataProvider) GetWorkflowCompletedAt() time.Time {
	return m.workflowCompletedAt
}

func (m *mockSessionMetadataProvider) UpdateWorkflowCompletion(status, summary string, completedAt time.Time) error {
	m.updateCalled = true
	if m.updateError != nil {
		return m.updateError
	}
	m.status = status
	m.summary = summary
	// Only update timestamp if not already set (simulating idempotency behavior)
	if m.workflowCompletedAt.IsZero() {
		m.workflowCompletedAt = completedAt
	}
	return nil
}

// ===========================================================================
// SignalWorkflowCompleteHandler Tests
// ===========================================================================

func TestSignalWorkflowCompleteHandler_SuccessStatus(t *testing.T) {
	sessionProvider := &mockSessionMetadataProvider{}

	h := handler.NewSignalWorkflowCompleteHandler(
		handler.WithSessionMetadataProvider(sessionProvider),
	)

	cmd := command.NewSignalWorkflowCompleteCommand(
		command.SourceMCPTool,
		command.WorkflowStatusSuccess,
		"Completed all tasks successfully",
		"epic-123",
		5,
	)

	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify session metadata was updated
	assert.True(t, sessionProvider.updateCalled)
	assert.Equal(t, "success", sessionProvider.status)
	assert.Equal(t, "Completed all tasks successfully", sessionProvider.summary)
	assert.False(t, sessionProvider.workflowCompletedAt.IsZero())

	// Verify result
	workflowResult := result.Data.(*handler.SignalWorkflowCompleteResult)
	assert.Equal(t, command.WorkflowStatusSuccess, workflowResult.Status)
	assert.Equal(t, "Completed all tasks successfully", workflowResult.Summary)
	assert.True(t, workflowResult.IsFirstCall)

	// Verify event was emitted
	require.Len(t, result.Events, 1)
	event := result.Events[0].(events.ProcessEvent)
	assert.Equal(t, events.ProcessWorkflowComplete, event.Type)
	assert.Equal(t, events.RoleCoordinator, event.Role)
}

func TestSignalWorkflowCompleteHandler_PartialStatus(t *testing.T) {
	sessionProvider := &mockSessionMetadataProvider{}

	h := handler.NewSignalWorkflowCompleteHandler(
		handler.WithSessionMetadataProvider(sessionProvider),
	)

	cmd := command.NewSignalWorkflowCompleteCommand(
		command.SourceMCPTool,
		command.WorkflowStatusPartial,
		"Completed 3 of 5 tasks",
		"",
		3,
	)

	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify session metadata was updated with partial status
	assert.Equal(t, "partial", sessionProvider.status)
	assert.Equal(t, "Completed 3 of 5 tasks", sessionProvider.summary)
}

func TestSignalWorkflowCompleteHandler_AbortedStatus(t *testing.T) {
	sessionProvider := &mockSessionMetadataProvider{}

	h := handler.NewSignalWorkflowCompleteHandler(
		handler.WithSessionMetadataProvider(sessionProvider),
	)

	cmd := command.NewSignalWorkflowCompleteCommand(
		command.SourceMCPTool,
		command.WorkflowStatusAborted,
		"User requested abort",
		"epic-456",
		0,
	)

	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify session metadata was updated with aborted status
	assert.Equal(t, "aborted", sessionProvider.status)
	assert.Equal(t, "User requested abort", sessionProvider.summary)
}

func TestSignalWorkflowCompleteHandler_InvalidStatus(t *testing.T) {
	sessionProvider := &mockSessionMetadataProvider{}

	h := handler.NewSignalWorkflowCompleteHandler(
		handler.WithSessionMetadataProvider(sessionProvider),
	)

	// Create command with invalid status
	cmd := command.NewSignalWorkflowCompleteCommand(
		command.SourceMCPTool,
		command.WorkflowStatus("invalid"),
		"Some summary",
		"",
		0,
	)

	result, err := h.Handle(context.Background(), cmd)

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")

	// Verify session metadata was NOT updated
	assert.False(t, sessionProvider.updateCalled)
}

func TestSignalWorkflowCompleteHandler_EmptySummary(t *testing.T) {
	sessionProvider := &mockSessionMetadataProvider{}

	h := handler.NewSignalWorkflowCompleteHandler(
		handler.WithSessionMetadataProvider(sessionProvider),
	)

	// Create command with empty summary
	cmd := command.NewSignalWorkflowCompleteCommand(
		command.SourceMCPTool,
		command.WorkflowStatusSuccess,
		"", // Empty summary
		"",
		0,
	)

	result, err := h.Handle(context.Background(), cmd)

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
	assert.Contains(t, err.Error(), "summary is required")

	// Verify session metadata was NOT updated
	assert.False(t, sessionProvider.updateCalled)
}

func TestSignalWorkflowCompleteHandler_EventPublished(t *testing.T) {
	sessionProvider := &mockSessionMetadataProvider{}

	h := handler.NewSignalWorkflowCompleteHandler(
		handler.WithSessionMetadataProvider(sessionProvider),
	)

	cmd := command.NewSignalWorkflowCompleteCommand(
		command.SourceMCPTool,
		command.WorkflowStatusSuccess,
		"All done",
		"",
		0,
	)

	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify ProcessWorkflowComplete event was emitted
	require.Len(t, result.Events, 1)
	event := result.Events[0].(events.ProcessEvent)
	assert.Equal(t, events.ProcessWorkflowComplete, event.Type)
	assert.Equal(t, "coordinator", event.ProcessID)
	assert.Equal(t, events.RoleCoordinator, event.Role)
}

func TestSignalWorkflowCompleteHandler_Idempotency_FirstCall(t *testing.T) {
	sessionProvider := &mockSessionMetadataProvider{
		// No existing completion - first call
		workflowCompletedAt: time.Time{},
	}

	h := handler.NewSignalWorkflowCompleteHandler(
		handler.WithSessionMetadataProvider(sessionProvider),
	)

	cmd := command.NewSignalWorkflowCompleteCommand(
		command.SourceMCPTool,
		command.WorkflowStatusSuccess,
		"First completion",
		"",
		0,
	)

	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify timestamp was set (first call)
	workflowResult := result.Data.(*handler.SignalWorkflowCompleteResult)
	assert.True(t, workflowResult.IsFirstCall)
	assert.False(t, workflowResult.CompletedAt.IsZero())
}

func TestSignalWorkflowCompleteHandler_Idempotency_SubsequentCall(t *testing.T) {
	// Set an existing completion timestamp to simulate a previous call
	originalTimestamp := time.Date(2026, 1, 13, 10, 0, 0, 0, time.UTC)
	sessionProvider := &mockSessionMetadataProvider{
		workflowCompletedAt: originalTimestamp,
	}

	h := handler.NewSignalWorkflowCompleteHandler(
		handler.WithSessionMetadataProvider(sessionProvider),
	)

	cmd := command.NewSignalWorkflowCompleteCommand(
		command.SourceMCPTool,
		command.WorkflowStatusSuccess,
		"Subsequent completion",
		"",
		0,
	)

	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify timestamp was preserved (subsequent call)
	workflowResult := result.Data.(*handler.SignalWorkflowCompleteResult)
	assert.False(t, workflowResult.IsFirstCall)
	assert.Equal(t, originalTimestamp, workflowResult.CompletedAt)

	// Verify event is STILL emitted (for audit trail)
	require.Len(t, result.Events, 1)
	event := result.Events[0].(events.ProcessEvent)
	assert.Equal(t, events.ProcessWorkflowComplete, event.Type)
}

func TestSignalWorkflowCompleteHandler_DuplicateCallsEmitEvents(t *testing.T) {
	// This test verifies that duplicate calls still emit events for audit trail
	originalTimestamp := time.Date(2026, 1, 13, 10, 0, 0, 0, time.UTC)
	sessionProvider := &mockSessionMetadataProvider{
		workflowCompletedAt: originalTimestamp,
	}

	h := handler.NewSignalWorkflowCompleteHandler(
		handler.WithSessionMetadataProvider(sessionProvider),
	)

	// Call twice
	for i := 0; i < 2; i++ {
		cmd := command.NewSignalWorkflowCompleteCommand(
			command.SourceMCPTool,
			command.WorkflowStatusSuccess,
			"Duplicate call",
			"",
			0,
		)

		result, err := h.Handle(context.Background(), cmd)
		require.NoError(t, err)
		assert.True(t, result.Success)

		// Both calls should emit events
		require.Len(t, result.Events, 1)
		event := result.Events[0].(events.ProcessEvent)
		assert.Equal(t, events.ProcessWorkflowComplete, event.Type)
	}
}

func TestSignalWorkflowCompleteHandler_NoSessionProvider(t *testing.T) {
	// Test that handler works even without a session provider
	h := handler.NewSignalWorkflowCompleteHandler()

	cmd := command.NewSignalWorkflowCompleteCommand(
		command.SourceMCPTool,
		command.WorkflowStatusSuccess,
		"No session provider",
		"",
		0,
	)

	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Event should still be emitted
	require.Len(t, result.Events, 1)
}

// ===========================================================================
// Sound Service Tests
// ===========================================================================

func TestSignalWorkflowCompleteHandler_PlaysCompletionSound(t *testing.T) {
	sessionProvider := &mockSessionMetadataProvider{}
	soundService := mocks.NewMockSoundService(t)

	// Expect sound to be played on first completion
	soundService.EXPECT().Play("complete", "workflow_complete").Once()

	h := handler.NewSignalWorkflowCompleteHandler(
		handler.WithSessionMetadataProvider(sessionProvider),
		handler.WithWorkflowSoundService(soundService),
	)

	cmd := command.NewSignalWorkflowCompleteCommand(
		command.SourceMCPTool,
		command.WorkflowStatusSuccess,
		"Completed all tasks",
		"epic-123",
		5,
	)

	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)
	// Sound service mock expectations are automatically verified on cleanup
}

func TestSignalWorkflowCompleteHandler_NoSoundOnDuplicateCall(t *testing.T) {
	// Set an existing completion timestamp to simulate a previous call
	originalTimestamp := time.Date(2026, 1, 13, 10, 0, 0, 0, time.UTC)
	sessionProvider := &mockSessionMetadataProvider{
		workflowCompletedAt: originalTimestamp,
	}
	soundService := mocks.NewMockSoundService(t)

	// Sound should NOT be played on duplicate call - no expectations set

	h := handler.NewSignalWorkflowCompleteHandler(
		handler.WithSessionMetadataProvider(sessionProvider),
		handler.WithWorkflowSoundService(soundService),
	)

	cmd := command.NewSignalWorkflowCompleteCommand(
		command.SourceMCPTool,
		command.WorkflowStatusSuccess,
		"Duplicate completion",
		"",
		0,
	)

	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify it was a subsequent call
	workflowResult := result.Data.(*handler.SignalWorkflowCompleteResult)
	assert.False(t, workflowResult.IsFirstCall)
	// If Play was called, the mock would fail due to unexpected call
}

func TestSignalWorkflowCompleteHandler_DefaultNoopSoundService(t *testing.T) {
	sessionProvider := &mockSessionMetadataProvider{}

	// Create handler WITHOUT sound service option - should use NoopSoundService
	h := handler.NewSignalWorkflowCompleteHandler(
		handler.WithSessionMetadataProvider(sessionProvider),
	)

	cmd := command.NewSignalWorkflowCompleteCommand(
		command.SourceMCPTool,
		command.WorkflowStatusSuccess,
		"Using default sound service",
		"",
		0,
	)

	result, err := h.Handle(context.Background(), cmd)

	// Should succeed without panic - NoopSoundService handles the Play call
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestWithWorkflowSoundService_SetsService(t *testing.T) {
	soundService := mocks.NewMockSoundService(t)

	h := handler.NewSignalWorkflowCompleteHandler(
		handler.WithWorkflowSoundService(soundService),
	)

	// Verify the handler was created with the sound service
	require.NotNil(t, h)
	// The soundService field is not exported, so we verify by testing behavior
	// through the PlaysCompletionSound test instead
}

func TestWithWorkflowSoundService_NilIgnored(t *testing.T) {
	h := handler.NewSignalWorkflowCompleteHandler(
		handler.WithWorkflowSoundService(nil), // nil should be ignored
	)

	// Should still have NoopSoundService as the default
	// We verify by testing that Play doesn't panic
	require.NotNil(t, h)

	// Test that the handler works with a successful completion
	sessionProvider := &mockSessionMetadataProvider{}
	h2 := handler.NewSignalWorkflowCompleteHandler(
		handler.WithSessionMetadataProvider(sessionProvider),
		handler.WithWorkflowSoundService(nil), // nil should be ignored
	)

	cmd := command.NewSignalWorkflowCompleteCommand(
		command.SourceMCPTool,
		command.WorkflowStatusSuccess,
		"Testing nil sound service",
		"",
		0,
	)

	result, err := h2.Handle(context.Background(), cmd)

	// Should succeed without panic - NoopSoundService handles the Play call
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestSignalWorkflowCompleteHandler_SoundPlaysForAllStatuses(t *testing.T) {
	// Sound should play on first completion regardless of status
	tests := []struct {
		name   string
		status command.WorkflowStatus
	}{
		{"success", command.WorkflowStatusSuccess},
		{"partial", command.WorkflowStatusPartial},
		{"aborted", command.WorkflowStatusAborted},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sessionProvider := &mockSessionMetadataProvider{}
			soundService := mocks.NewMockSoundService(t)

			// Expect sound for all statuses
			soundService.EXPECT().Play("complete", "workflow_complete").Once()

			h := handler.NewSignalWorkflowCompleteHandler(
				handler.WithSessionMetadataProvider(sessionProvider),
				handler.WithWorkflowSoundService(soundService),
			)

			cmd := command.NewSignalWorkflowCompleteCommand(
				command.SourceMCPTool,
				tc.status,
				"Workflow finished",
				"",
				0,
			)

			result, err := h.Handle(context.Background(), cmd)

			require.NoError(t, err)
			assert.True(t, result.Success)
		})
	}
}

func TestSignalWorkflowCompleteHandler_SoundServiceTypeCheck(t *testing.T) {
	// Verify default constructor uses NoopSoundService
	h := handler.NewSignalWorkflowCompleteHandler()

	// Access soundService field to verify type (exported for testing)
	// Since soundService is not exported, we verify through behavior
	require.NotNil(t, h)

	// The handler should work without any sound service configuration
	cmd := command.NewSignalWorkflowCompleteCommand(
		command.SourceMCPTool,
		command.WorkflowStatusSuccess,
		"Type check test",
		"",
		0,
	)

	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)
	require.True(t, result.Success)
}

// Helper to verify soundService is indeed NoopSoundService when not provided
func TestNewSignalWorkflowCompleteHandler_DefaultSoundService(t *testing.T) {
	sessionProvider := &mockSessionMetadataProvider{}

	// Create handler without sound service option - should use NoopSoundService by default
	h := handler.NewSignalWorkflowCompleteHandler(
		handler.WithSessionMetadataProvider(sessionProvider),
	)

	require.NotNil(t, h)

	// Verify by behavior: a first completion should trigger sound play,
	// but NoopSoundService does nothing (doesn't panic), so this test
	// just confirms the handler works correctly with defaults
	cmd := command.NewSignalWorkflowCompleteCommand(
		command.SourceMCPTool,
		command.WorkflowStatusSuccess,
		"Default sound service test",
		"",
		0,
	)

	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)
	require.True(t, result.Success)

	// Verify it was first call (sound would have played if not NoopSoundService)
	workflowResult := result.Data.(*handler.SignalWorkflowCompleteResult)
	require.True(t, workflowResult.IsFirstCall)
}

// Ensure the sound package import is used to satisfy LSP
var _ = sound.NoopSoundService{}
