package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

// ===========================================================================
// HandlerFunc Tests
// ===========================================================================

func TestHandlerFunc_ImplementsCommandHandler(t *testing.T) {
	// Verify HandlerFunc can be used as CommandHandler
	var _ CommandHandler = HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return SuccessResult(nil), nil
	})
}

func TestHandlerFunc_Handle(t *testing.T) {
	called := false
	expectedData := "test-data"

	handler := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		called = true
		return SuccessResult(expectedData), nil
	})

	cmd := command.NewSpawnProcessCommand(command.SourceInternal, "coordinator")
	result, err := handler.Handle(context.Background(), cmd)

	require.True(t, called, "HandlerFunc was not called")
	require.NoError(t, err)
	require.Equal(t, expectedData, result.Data)
}

func TestHandlerFunc_HandleWithError(t *testing.T) {
	expectedErr := errors.New("handler error")

	handler := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return nil, expectedErr
	})

	cmd := command.NewSpawnProcessCommand(command.SourceInternal, "coordinator")
	result, err := handler.Handle(context.Background(), cmd)

	require.Equal(t, expectedErr, err)
	require.Nil(t, result)
}

// ===========================================================================
// HandlerRegistry Tests
// ===========================================================================

func TestNewHandlerRegistry(t *testing.T) {
	registry := NewHandlerRegistry()
	require.NotNil(t, registry, "NewHandlerRegistry returned nil")
	require.NotNil(t, registry.handlers, "handlers map was not initialized")
}

func TestHandlerRegistry_Register(t *testing.T) {
	registry := NewHandlerRegistry()
	handler := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return SuccessResult(nil), nil
	})

	registry.Register(command.CmdSpawnProcess, handler)

	// Verify the handler is stored
	types := registry.RegisteredTypes()
	require.Len(t, types, 1)
	require.Equal(t, command.CmdSpawnProcess, types[0])
}

func TestHandlerRegistry_Register_OverwritesPrevious(t *testing.T) {
	registry := NewHandlerRegistry()
	handler1Called := false
	handler2Called := false

	handler1 := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		handler1Called = true
		return SuccessResult("handler1"), nil
	})
	handler2 := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		handler2Called = true
		return SuccessResult("handler2"), nil
	})

	registry.Register(command.CmdSpawnProcess, handler1)
	registry.Register(command.CmdSpawnProcess, handler2)

	retrieved, err := registry.Get(command.CmdSpawnProcess)
	require.NoError(t, err)

	cmd := command.NewSpawnProcessCommand(command.SourceInternal, "coordinator")
	_, _ = retrieved.Handle(context.Background(), cmd)

	require.False(t, handler1Called, "handler1 should not have been called")
	require.True(t, handler2Called, "handler2 should have been called")
}

func TestHandlerRegistry_Get_ReturnsRegisteredHandler(t *testing.T) {
	registry := NewHandlerRegistry()
	expectedData := "handler-data"

	handler := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return SuccessResult(expectedData), nil
	})

	registry.Register(command.CmdSpawnProcess, handler)

	retrieved, err := registry.Get(command.CmdSpawnProcess)
	require.NoError(t, err)

	cmd := command.NewSpawnProcessCommand(command.SourceInternal, "coordinator")
	result, _ := retrieved.Handle(context.Background(), cmd)

	require.Equal(t, expectedData, result.Data)
}

func TestHandlerRegistry_Get_ErrorForUnregistered(t *testing.T) {
	registry := NewHandlerRegistry()

	_, err := registry.Get(command.CmdSpawnProcess)

	require.Error(t, err, "expected error for unregistered command type")
	require.ErrorIs(t, err, ErrHandlerNotFound)
}

func TestHandlerRegistry_MustGet_ReturnsRegisteredHandler(t *testing.T) {
	registry := NewHandlerRegistry()
	expectedData := "must-get-data"

	handler := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return SuccessResult(expectedData), nil
	})

	registry.Register(command.CmdRetireProcess, handler)

	retrieved := registry.MustGet(command.CmdRetireProcess)

	cmd := command.NewRetireProcessCommand(command.SourceInternal, "worker-1", "test reason")
	result, _ := retrieved.Handle(context.Background(), cmd)

	require.Equal(t, expectedData, result.Data)
}

func TestHandlerRegistry_MustGet_PanicsForUnregistered(t *testing.T) {
	registry := NewHandlerRegistry()

	require.Panics(t, func() {
		registry.MustGet(command.CmdSpawnProcess)
	}, "expected MustGet to panic for unregistered command type")
}

func TestHandlerRegistry_RegisteredTypes(t *testing.T) {
	registry := NewHandlerRegistry()
	handler := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return SuccessResult(nil), nil
	})

	registry.Register(command.CmdSpawnProcess, handler)
	registry.Register(command.CmdRetireProcess, handler)
	registry.Register(command.CmdAssignTask, handler)

	types := registry.RegisteredTypes()
	require.Len(t, types, 3)

	// Verify all expected types are present (order may vary)
	typeSet := make(map[command.CommandType]bool)
	for _, ct := range types {
		typeSet[ct] = true
	}

	expectedTypes := []command.CommandType{
		command.CmdSpawnProcess,
		command.CmdRetireProcess,
		command.CmdAssignTask,
	}
	for _, expected := range expectedTypes {
		require.True(t, typeSet[expected], "expected type %s not found in registered types", expected)
	}
}

// ===========================================================================
// Result Builder Tests
// ===========================================================================

func TestSuccessResult(t *testing.T) {
	data := "test-data"
	result := SuccessResult(data)

	require.True(t, result.Success, "expected Success to be true")
	require.Equal(t, data, result.Data)
	require.Nil(t, result.Error)
	require.Empty(t, result.Events)
	require.Empty(t, result.FollowUp)
}

func TestSuccessResult_NilData(t *testing.T) {
	result := SuccessResult(nil)

	require.True(t, result.Success, "expected Success to be true")
	require.Nil(t, result.Data)
}

func TestSuccessWithEvents(t *testing.T) {
	data := "event-data"
	event1 := "event1"
	event2 := struct{ Name string }{"event2"}

	result := SuccessWithEvents(data, event1, event2)

	require.True(t, result.Success, "expected Success to be true")
	require.Equal(t, data, result.Data)
	require.Len(t, result.Events, 2)
	require.Equal(t, event1, result.Events[0])
	require.Equal(t, event2, result.Events[1])
	require.Empty(t, result.FollowUp)
}

func TestSuccessWithEvents_NoEvents(t *testing.T) {
	result := SuccessWithEvents("data")

	require.True(t, result.Success, "expected Success to be true")
	require.Empty(t, result.Events)
}

func TestSuccessWithFollowUp(t *testing.T) {
	data := "followup-data"
	followUp1 := command.NewSpawnProcessCommand(command.SourceInternal, "worker")
	followUp2 := command.NewRetireProcessCommand(command.SourceInternal, "worker-1", "reason")

	result := SuccessWithFollowUp(data, followUp1, followUp2)

	require.True(t, result.Success, "expected Success to be true")
	require.Equal(t, data, result.Data)
	require.Len(t, result.FollowUp, 2)
	require.Equal(t, followUp1, result.FollowUp[0])
	require.Equal(t, followUp2, result.FollowUp[1])
	require.Empty(t, result.Events)
}

func TestSuccessWithFollowUp_NoFollowUp(t *testing.T) {
	result := SuccessWithFollowUp("data")

	require.True(t, result.Success, "expected Success to be true")
	require.Empty(t, result.FollowUp)
}

func TestSuccessWithEventsAndFollowUp(t *testing.T) {
	data := "combined-data"
	events := []any{"event1", "event2"}
	followUp := []command.Command{
		command.NewSpawnProcessCommand(command.SourceInternal, "worker"),
	}

	result := SuccessWithEventsAndFollowUp(data, events, followUp)

	require.True(t, result.Success, "expected Success to be true")
	require.Equal(t, data, result.Data)
	require.Len(t, result.Events, 2)
	require.Len(t, result.FollowUp, 1)
}

func TestErrorResult(t *testing.T) {
	testErr := errors.New("test error")
	result := ErrorResult(testErr)

	require.False(t, result.Success, "expected Success to be false")
	require.Equal(t, testErr, result.Error)
	require.Nil(t, result.Data)
	require.Empty(t, result.Events)
	require.Empty(t, result.FollowUp)
}

func TestErrorResult_NilError(t *testing.T) {
	result := ErrorResult(nil)

	require.False(t, result.Success, "expected Success to be false")
	require.Nil(t, result.Error)
}

// ===========================================================================
// Concurrency Tests
// ===========================================================================

func TestHandlerRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewHandlerRegistry()
	handler := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return SuccessResult(nil), nil
	})

	// Pre-register some handlers
	registry.Register(command.CmdSpawnProcess, handler)

	// Run concurrent reads and writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				if j%2 == 0 {
					registry.Register(command.CmdRetireProcess, handler)
				} else {
					_, _ = registry.Get(command.CmdSpawnProcess)
				}
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify registry is still functional
	_, err := registry.Get(command.CmdSpawnProcess)
	require.NoError(t, err, "registry corrupted after concurrent access")
}

// ===========================================================================
// HandlerRegistry Route Tests
// ===========================================================================

func TestHandlerRegistry_Route_StopProcessCommand(t *testing.T) {
	// Setup: create registry and register StopWorkerHandler
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	procRegistry := process.NewProcessRegistry()

	registry := NewHandlerRegistry()
	registry.Register(command.CmdStopProcess, NewStopWorkerHandler(processRepo, taskRepo, queueRepo, procRegistry))

	// Create a worker process in the repository
	workerProc := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
	}
	err := processRepo.Save(workerProc)
	require.NoError(t, err)

	// Create stop worker command
	cmd := command.NewStopProcessCommand(command.SourceUser, "worker-1", false, "test stop")

	// Route the command
	result, err := registry.Route(context.Background(), cmd)

	// Verify the command was routed to StopWorkerHandler
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	// Verify the result contains StopWorkerResult
	stopResult, ok := result.Data.(*StopWorkerResult)
	require.True(t, ok, "expected StopWorkerResult")
	require.Equal(t, "worker-1", stopResult.ProcessID)
}

func TestHandlerRegistry_Route_GenericHandlers(t *testing.T) {
	registry := NewHandlerRegistry()

	// Register a handler
	handlerCalled := false
	expectedData := "handler-data"
	handler := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		handlerCalled = true
		return SuccessResult(expectedData), nil
	})
	registry.Register(command.CmdSpawnProcess, handler)

	// Route a command that should use the handler
	cmd := command.NewSpawnProcessCommand(command.SourceInternal, "worker")
	result, err := registry.Route(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, handlerCalled, "handler was not called")
	require.NotNil(t, result)
	require.Equal(t, expectedData, result.Data)
}

func TestHandlerRegistry_Route_UnregisteredCommand(t *testing.T) {
	registry := NewHandlerRegistry()

	// Route a command for which no handler is registered
	cmd := command.NewSpawnProcessCommand(command.SourceInternal, "worker")
	_, err := registry.Route(context.Background(), cmd)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrHandlerNotFound)
}
