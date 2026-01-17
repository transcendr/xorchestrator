package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/xorchestrator/internal/beads"
	"github.com/zjrosen/xorchestrator/internal/mocks"
	"github.com/zjrosen/xorchestrator/internal/orchestration/v2/command"
	"github.com/zjrosen/xorchestrator/internal/orchestration/v2/repository"
)

// ===========================================================================
// MarkTaskCompleteHandler Tests
// ===========================================================================

func TestMarkTaskCompleteHandler_Success(t *testing.T) {
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().UpdateStatus("xorchestrator-abc1.2", beads.StatusClosed).Return(nil)
	bdExecutor.EXPECT().AddComment("xorchestrator-abc1.2", "coordinator", "Task completed").Return(nil)

	handler := NewMarkTaskCompleteHandler(bdExecutor, nil)

	cmd := command.NewMarkTaskCompleteCommand(command.SourceMCPTool, "xorchestrator-abc1.2")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, result.Success, "expected success, got failure: %v", result.Error)
}

func TestMarkTaskCompleteHandler_ReturnsResult(t *testing.T) {
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().UpdateStatus("xorchestrator-abc1.2", beads.StatusClosed).Return(nil)
	bdExecutor.EXPECT().AddComment("xorchestrator-abc1.2", "coordinator", "Task completed").Return(nil)

	handler := NewMarkTaskCompleteHandler(bdExecutor, nil)

	cmd := command.NewMarkTaskCompleteCommand(command.SourceMCPTool, "xorchestrator-abc1.2")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	completeResult, ok := result.Data.(*MarkTaskCompleteResult)
	require.True(t, ok, "expected MarkTaskCompleteResult, got: %T", result.Data)
	require.Equal(t, "xorchestrator-abc1.2", completeResult.TaskID)
}

func TestMarkTaskCompleteHandler_FailsOnUpdateStatusError(t *testing.T) {
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().UpdateStatus(mock.Anything, mock.Anything).Return(errors.New("bd database locked"))

	handler := NewMarkTaskCompleteHandler(bdExecutor, nil)

	cmd := command.NewMarkTaskCompleteCommand(command.SourceMCPTool, "xorchestrator-abc1.2")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error on BD UpdateTaskStatus failure")
	require.Contains(t, err.Error(), "bd database locked")
	require.Contains(t, err.Error(), "failed to update BD task status")
}

func TestMarkTaskCompleteHandler_FailsOnAddCommentError(t *testing.T) {
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().UpdateStatus(mock.Anything, mock.Anything).Return(nil)
	bdExecutor.EXPECT().AddComment(mock.Anything, mock.Anything, mock.Anything).Return(errors.New("bd comment service unavailable"))

	handler := NewMarkTaskCompleteHandler(bdExecutor, nil)

	cmd := command.NewMarkTaskCompleteCommand(command.SourceMCPTool, "xorchestrator-abc1.2")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error on BD AddComment failure")
	require.Contains(t, err.Error(), "bd comment service unavailable")
	require.Contains(t, err.Error(), "failed to add BD comment")
}

func TestMarkTaskCompleteHandler_PanicsIfBDExecutorNil(t *testing.T) {
	require.Panics(t, func() {
		NewMarkTaskCompleteHandler(nil, nil)
	}, "expected panic when bdExecutor is nil")
}

func TestMarkTaskCompleteHandler_DeletesTaskFromRepository(t *testing.T) {
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().UpdateStatus("xorchestrator-abc1.2", beads.StatusClosed).Return(nil)
	bdExecutor.EXPECT().AddComment("xorchestrator-abc1.2", "coordinator", "Task completed").Return(nil)

	// Create task repo with a task
	taskRepo := repository.NewMemoryTaskRepository()
	task := &repository.TaskAssignment{
		TaskID:      "xorchestrator-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
	}
	require.NoError(t, taskRepo.Save(task))

	// Verify task exists before handler
	_, err := taskRepo.Get("xorchestrator-abc1.2")
	require.NoError(t, err, "task should exist before handle")

	handler := NewMarkTaskCompleteHandler(bdExecutor, taskRepo)

	cmd := command.NewMarkTaskCompleteCommand(command.SourceMCPTool, "xorchestrator-abc1.2")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, result.Success)

	// Verify task was deleted
	_, err = taskRepo.Get("xorchestrator-abc1.2")
	require.ErrorIs(t, err, repository.ErrTaskNotFound, "task should be deleted after handle")
}

func TestMarkTaskCompleteHandler_SucceedsWhenTaskNotInRepo(t *testing.T) {
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().UpdateStatus("xorchestrator-abc1.2", beads.StatusClosed).Return(nil)
	bdExecutor.EXPECT().AddComment("xorchestrator-abc1.2", "coordinator", "Task completed").Return(nil)

	// Create empty task repo (task doesn't exist in memory)
	taskRepo := repository.NewMemoryTaskRepository()

	handler := NewMarkTaskCompleteHandler(bdExecutor, taskRepo)

	cmd := command.NewMarkTaskCompleteCommand(command.SourceMCPTool, "xorchestrator-abc1.2")
	result, err := handler.Handle(context.Background(), cmd)

	// Should succeed even though task wasn't in memory
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestMarkTaskCompleteHandler_WorksWithNilTaskRepo(t *testing.T) {
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().UpdateStatus("xorchestrator-abc1.2", beads.StatusClosed).Return(nil)
	bdExecutor.EXPECT().AddComment("xorchestrator-abc1.2", "coordinator", "Task completed").Return(nil)

	// Construct handler with nil taskRepo (backward compatibility)
	handler := NewMarkTaskCompleteHandler(bdExecutor, nil)

	cmd := command.NewMarkTaskCompleteCommand(command.SourceMCPTool, "xorchestrator-abc1.2")
	result, err := handler.Handle(context.Background(), cmd)

	// Should succeed without panic even with nil taskRepo
	require.NoError(t, err)
	require.True(t, result.Success)

	completeResult, ok := result.Data.(*MarkTaskCompleteResult)
	require.True(t, ok, "expected MarkTaskCompleteResult, got: %T", result.Data)
	require.Equal(t, "xorchestrator-abc1.2", completeResult.TaskID)
}

// ===========================================================================
// MarkTaskFailedHandler Tests
// ===========================================================================

func TestMarkTaskFailedHandler_Success(t *testing.T) {
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	// MarkTaskFailed only calls AddComment, not UpdateStatus
	bdExecutor.EXPECT().AddComment("xorchestrator-abc1.2", "coordinator", "Task failed: Build failed due to missing dependency").Return(nil)

	handler := NewMarkTaskFailedHandler(bdExecutor)

	cmd := command.NewMarkTaskFailedCommand(command.SourceMCPTool, "xorchestrator-abc1.2", "Build failed due to missing dependency")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, result.Success, "expected success, got failure: %v", result.Error)
}

func TestMarkTaskFailedHandler_ReturnsResult(t *testing.T) {
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().AddComment("xorchestrator-abc1.2", "coordinator", "Task failed: Tests failing").Return(nil)

	handler := NewMarkTaskFailedHandler(bdExecutor)

	cmd := command.NewMarkTaskFailedCommand(command.SourceMCPTool, "xorchestrator-abc1.2", "Tests failing")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	failedResult, ok := result.Data.(*MarkTaskFailedResult)
	require.True(t, ok, "expected MarkTaskFailedResult, got: %T", result.Data)
	require.Equal(t, "xorchestrator-abc1.2", failedResult.TaskID)
	require.Equal(t, "Tests failing", failedResult.Reason)
}

func TestMarkTaskFailedHandler_FailsOnAddCommentError(t *testing.T) {
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().AddComment(mock.Anything, mock.Anything, mock.Anything).Return(errors.New("bd comment service unavailable"))

	handler := NewMarkTaskFailedHandler(bdExecutor)

	cmd := command.NewMarkTaskFailedCommand(command.SourceMCPTool, "xorchestrator-abc1.2", "Some reason")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error on BD AddComment failure")
	require.Contains(t, err.Error(), "bd comment service unavailable")
	require.Contains(t, err.Error(), "failed to add BD comment")
}

func TestMarkTaskFailedHandler_PanicsIfBDExecutorNil(t *testing.T) {
	require.Panics(t, func() {
		NewMarkTaskFailedHandler(nil)
	}, "expected panic when bdExecutor is nil")
}

func TestMarkTaskFailedHandler_DoesNotUpdateStatus(t *testing.T) {
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	// MarkTaskFailed only calls AddComment, not UpdateStatus - verify UpdateStatus is never called
	bdExecutor.EXPECT().AddComment("xorchestrator-abc1.2", "coordinator", "Task failed: Some reason").Return(nil)

	handler := NewMarkTaskFailedHandler(bdExecutor)

	cmd := command.NewMarkTaskFailedCommand(command.SourceMCPTool, "xorchestrator-abc1.2", "Some reason")
	_, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	// mockery will fail if UpdateStatus is unexpectedly called
}
