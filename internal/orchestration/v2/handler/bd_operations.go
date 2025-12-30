// Package handler provides command handlers for the v2 orchestration architecture.
// This file contains handlers for BD task status commands: MarkTaskComplete and MarkTaskFailed.
// These handlers interact with the BD executor to update task status in the beads database.
package handler

import (
	"context"
	"fmt"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
)

// ===========================================================================
// MarkTaskCompleteHandler
// ===========================================================================

// MarkTaskCompleteHandler handles CmdMarkTaskComplete commands.
// It marks a BD task as completed by updating its status to "closed" and adding a completion comment.
type MarkTaskCompleteHandler struct {
	bdExecutor beads.BeadsExecutor
}

// NewMarkTaskCompleteHandler creates a new MarkTaskCompleteHandler.
// Panics if bdExecutor is nil.
func NewMarkTaskCompleteHandler(bdExecutor beads.BeadsExecutor) *MarkTaskCompleteHandler {
	if bdExecutor == nil {
		panic("bdExecutor is required for MarkTaskCompleteHandler")
	}
	return &MarkTaskCompleteHandler{
		bdExecutor: bdExecutor,
	}
}

// Handle processes a MarkTaskCompleteCommand.
// It updates the BD task status to "closed" and adds a completion comment.
func (h *MarkTaskCompleteHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	markCmd := cmd.(*command.MarkTaskCompleteCommand)

	// 1. Update task status to closed
	if err := h.bdExecutor.UpdateStatus(markCmd.TaskID, beads.StatusClosed); err != nil {
		return nil, fmt.Errorf("failed to update BD task status: %w", err)
	}

	// 2. Add completion comment
	comment := "Task completed"
	if err := h.bdExecutor.AddComment(markCmd.TaskID, "coordinator", comment); err != nil {
		return nil, fmt.Errorf("failed to add BD comment: %w", err)
	}

	// 3. Return success result
	result := &MarkTaskCompleteResult{
		TaskID: markCmd.TaskID,
	}

	return SuccessResult(result), nil
}

// MarkTaskCompleteResult contains the result of marking a task as complete.
type MarkTaskCompleteResult struct {
	TaskID string
}

// ===========================================================================
// MarkTaskFailedHandler
// ===========================================================================

// MarkTaskFailedHandler handles CmdMarkTaskFailed commands.
// It adds a failure comment to the BD task with the provided reason.
type MarkTaskFailedHandler struct {
	bdExecutor beads.BeadsExecutor
}

// NewMarkTaskFailedHandler creates a new MarkTaskFailedHandler.
// Panics if bdExecutor is nil.
func NewMarkTaskFailedHandler(bdExecutor beads.BeadsExecutor) *MarkTaskFailedHandler {
	if bdExecutor == nil {
		panic("bdExecutor is required for MarkTaskFailedHandler")
	}
	return &MarkTaskFailedHandler{
		bdExecutor: bdExecutor,
	}
}

// Handle processes a MarkTaskFailedCommand.
// It adds a failure comment to the BD task with the provided reason.
func (h *MarkTaskFailedHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	markCmd := cmd.(*command.MarkTaskFailedCommand)

	// 1. Add failure comment with reason
	comment := fmt.Sprintf("Task failed: %s", markCmd.Reason)
	if err := h.bdExecutor.AddComment(markCmd.TaskID, "coordinator", comment); err != nil {
		return nil, fmt.Errorf("failed to add BD comment: %w", err)
	}

	// 2. Return success result
	result := &MarkTaskFailedResult{
		TaskID: markCmd.TaskID,
		Reason: markCmd.Reason,
	}

	return SuccessResult(result), nil
}

// MarkTaskFailedResult contains the result of marking a task as failed.
type MarkTaskFailedResult struct {
	TaskID string
	Reason string
}
