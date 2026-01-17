package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/xorchestrator/internal/orchestration/v2/command"
	"github.com/zjrosen/xorchestrator/internal/orchestration/v2/repository"
)

func TestGenerateAccountabilitySummaryHandler_Handle(t *testing.T) {
	t.Run("success - queues prompt to ready worker", func(t *testing.T) {
		processRepo := repository.NewMemoryProcessRepository()
		queueRepo := repository.NewMemoryQueueRepository(100)

		// Create a ready worker
		worker := &repository.Process{
			ID:     "worker-1",
			Role:   repository.RoleWorker,
			Status: repository.StatusReady,
		}
		err := processRepo.Save(worker)
		require.NoError(t, err)

		handler := NewGenerateAccountabilitySummaryHandler(processRepo, queueRepo)
		cmd := command.NewGenerateAccountabilitySummaryCommand(command.SourceMCPTool, "worker-1", "/tmp/session")

		result, err := handler.Handle(context.Background(), cmd)

		require.NoError(t, err)
		require.True(t, result.Success)
		require.NotNil(t, result.Data)

		// Verify follow-up command is created for delivery
		require.Len(t, result.FollowUp, 1)
		deliverCmd, ok := result.FollowUp[0].(*command.DeliverProcessQueuedCommand)
		require.True(t, ok)
		require.Equal(t, "worker-1", deliverCmd.ProcessID)

		// Verify message was queued
		queue := queueRepo.GetOrCreate("worker-1")
		require.Equal(t, 1, queue.Size())
	})

	t.Run("success - queues prompt to working worker", func(t *testing.T) {
		processRepo := repository.NewMemoryProcessRepository()
		queueRepo := repository.NewMemoryQueueRepository(100)

		// Create a working worker
		worker := &repository.Process{
			ID:     "worker-1",
			Role:   repository.RoleWorker,
			Status: repository.StatusWorking,
		}
		err := processRepo.Save(worker)
		require.NoError(t, err)

		handler := NewGenerateAccountabilitySummaryHandler(processRepo, queueRepo)
		cmd := command.NewGenerateAccountabilitySummaryCommand(command.SourceMCPTool, "worker-1", "/tmp/session")

		result, err := handler.Handle(context.Background(), cmd)

		require.NoError(t, err)
		require.True(t, result.Success)

		// Verify no follow-up (message is queued, delivery happens when turn completes)
		require.Empty(t, result.FollowUp)

		// Verify event was emitted
		require.Len(t, result.Events, 1)

		// Verify message was queued
		queue := queueRepo.GetOrCreate("worker-1")
		require.Equal(t, 1, queue.Size())
	})

	t.Run("error - worker not found", func(t *testing.T) {
		processRepo := repository.NewMemoryProcessRepository()
		queueRepo := repository.NewMemoryQueueRepository(100)

		handler := NewGenerateAccountabilitySummaryHandler(processRepo, queueRepo)
		cmd := command.NewGenerateAccountabilitySummaryCommand(command.SourceMCPTool, "worker-999", "/tmp/session")

		result, err := handler.Handle(context.Background(), cmd)

		require.Error(t, err)
		require.Nil(t, result)
		require.ErrorIs(t, err, ErrProcessNotFound)
	})

	t.Run("error - worker retired", func(t *testing.T) {
		processRepo := repository.NewMemoryProcessRepository()
		queueRepo := repository.NewMemoryQueueRepository(100)

		// Create a retired worker
		worker := &repository.Process{
			ID:     "worker-1",
			Role:   repository.RoleWorker,
			Status: repository.StatusRetired,
		}
		err := processRepo.Save(worker)
		require.NoError(t, err)

		handler := NewGenerateAccountabilitySummaryHandler(processRepo, queueRepo)
		cmd := command.NewGenerateAccountabilitySummaryCommand(command.SourceMCPTool, "worker-1", "/tmp/session")

		result, err := handler.Handle(context.Background(), cmd)

		require.Error(t, err)
		require.Nil(t, result)
		require.ErrorIs(t, err, ErrProcessRetired)
	})
}

func TestGenerateAccountabilitySummaryCommand_Validate(t *testing.T) {
	t.Run("valid command", func(t *testing.T) {
		cmd := command.NewGenerateAccountabilitySummaryCommand(command.SourceMCPTool, "worker-1", "/tmp/session")
		err := cmd.Validate()
		require.NoError(t, err)
	})

	t.Run("missing worker_id", func(t *testing.T) {
		cmd := command.NewGenerateAccountabilitySummaryCommand(command.SourceMCPTool, "", "/tmp/session")
		err := cmd.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "worker_id is required")
	})

	t.Run("missing session_dir", func(t *testing.T) {
		cmd := command.NewGenerateAccountabilitySummaryCommand(command.SourceMCPTool, "worker-1", "")
		err := cmd.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "session_dir is required")
	})
}
