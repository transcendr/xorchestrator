// Package handler provides command handlers for the v2 orchestration architecture.
// This file contains the GenerateAccountabilitySummaryHandler for aggregating worker summaries.
package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/zjrosen/xorchestrator/internal/orchestration/events"
	"github.com/zjrosen/xorchestrator/internal/orchestration/v2/command"
	"github.com/zjrosen/xorchestrator/internal/orchestration/v2/prompt"
	"github.com/zjrosen/xorchestrator/internal/orchestration/v2/repository"
)

// ===========================================================================
// GenerateAccountabilitySummaryHandler
// ===========================================================================

// GenerateAccountabilitySummaryHandler handles CmdGenerateAccountabilitySummary commands.
// It sends the AggregationWorkerPrompt to an existing worker to aggregate accountability
// summaries from all workers into a unified session summary.
type GenerateAccountabilitySummaryHandler struct {
	processRepo repository.ProcessRepository
	queueRepo   repository.QueueRepository
}

// NewGenerateAccountabilitySummaryHandler creates a new GenerateAccountabilitySummaryHandler.
func NewGenerateAccountabilitySummaryHandler(
	processRepo repository.ProcessRepository,
	queueRepo repository.QueueRepository,
) *GenerateAccountabilitySummaryHandler {
	return &GenerateAccountabilitySummaryHandler{
		processRepo: processRepo,
		queueRepo:   queueRepo,
	}
}

// Handle processes a GenerateAccountabilitySummaryCommand.
// It queues the AggregationWorkerPrompt to the specified worker and creates a follow-up to deliver it.
func (h *GenerateAccountabilitySummaryHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	aggCmd := cmd.(*command.GenerateAccountabilitySummaryCommand)

	// Step 1: Get worker from repository
	proc, err := h.processRepo.Get(aggCmd.WorkerID)
	if err != nil {
		if errors.Is(err, repository.ErrProcessNotFound) {
			return nil, ErrProcessNotFound
		}
		return nil, fmt.Errorf("failed to get worker: %w", err)
	}

	// Step 2: Check if worker is retired
	if proc.Status == repository.StatusRetired {
		return nil, ErrProcessRetired
	}

	// Step 3: Queue the aggregation prompt to the worker
	aggPrompt := prompt.AggregationWorkerPrompt(aggCmd.SessionDir)
	queue := h.queueRepo.GetOrCreate(aggCmd.WorkerID)
	if err := queue.Enqueue(aggPrompt, repository.SenderCoordinator); err != nil {
		return nil, fmt.Errorf("failed to queue aggregation prompt: %w", err)
	}

	// Step 4: Build result
	result := &GenerateAccountabilitySummaryResult{
		WorkerID:   aggCmd.WorkerID,
		SessionDir: aggCmd.SessionDir,
		Queued:     proc.Status == repository.StatusWorking,
		QueueSize:  queue.Size(),
	}

	// Step 5: If worker is Working, just emit queue changed event
	if proc.Status == repository.StatusWorking {
		event := events.ProcessEvent{
			Type:       events.ProcessQueueChanged,
			ProcessID:  proc.ID,
			Role:       proc.Role,
			Status:     proc.Status,
			QueueCount: queue.Size(),
		}
		return SuccessWithEvents(result, event), nil
	}

	// Step 6: Worker is Ready - create follow-up to deliver from queue
	deliverCmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, aggCmd.WorkerID)

	return SuccessWithFollowUp(result, deliverCmd), nil
}

// GenerateAccountabilitySummaryResult contains the result of assigning an aggregation task.
type GenerateAccountabilitySummaryResult struct {
	WorkerID   string
	SessionDir string
	Queued     bool
	QueueSize  int
}
