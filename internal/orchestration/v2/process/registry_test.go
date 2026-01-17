package process

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zjrosen/xorchestrator/internal/orchestration/v2/repository"
)

func TestNewProcessRegistry_CreatesEmptyRegistry(t *testing.T) {
	reg := NewProcessRegistry()
	assert.NotNil(t, reg)
	assert.Empty(t, reg.All())
	assert.Empty(t, reg.IDs())
}

func TestProcessRegistry_Register_AddsProcess(t *testing.T) {
	reg := NewProcessRegistry()
	proc := &Process{
		ID:   "worker-1",
		Role: repository.RoleWorker,
	}

	reg.Register(proc)

	got := reg.Get("worker-1")
	assert.Equal(t, proc, got)
}

func TestProcessRegistry_Register_ReplacesExistingProcess(t *testing.T) {
	reg := NewProcessRegistry()
	proc1 := &Process{ID: "worker-1", Role: repository.RoleWorker, taskID: "task-1"}
	proc2 := &Process{ID: "worker-1", Role: repository.RoleWorker, taskID: "task-2"}

	reg.Register(proc1)
	reg.Register(proc2)

	got := reg.Get("worker-1")
	// Verify it's the second process (same pointer)
	assert.Same(t, proc2, got)
	// Verify it's not the first process (different pointer)
	assert.NotSame(t, proc1, got)
}

func TestProcessRegistry_Unregister_RemovesProcess(t *testing.T) {
	reg := NewProcessRegistry()
	proc := &Process{ID: "worker-1", Role: repository.RoleWorker}
	reg.Register(proc)

	removed := reg.Unregister("worker-1")
	assert.True(t, removed)
	assert.Nil(t, reg.Get("worker-1"))
}

func TestProcessRegistry_Unregister_ReturnsFalseForUnknownID(t *testing.T) {
	reg := NewProcessRegistry()

	removed := reg.Unregister("nonexistent")
	assert.False(t, removed)
}

func TestProcessRegistry_Get_ReturnsRegisteredProcess(t *testing.T) {
	reg := NewProcessRegistry()
	proc := &Process{ID: "worker-1", Role: repository.RoleWorker}
	reg.Register(proc)

	got := reg.Get("worker-1")
	assert.Equal(t, proc, got)
}

func TestProcessRegistry_Get_ReturnsNilForUnknownID(t *testing.T) {
	reg := NewProcessRegistry()

	got := reg.Get("nonexistent")
	assert.Nil(t, got)
}

func TestProcessRegistry_GetCoordinator_ReturnsCoordinatorProcess(t *testing.T) {
	reg := NewProcessRegistry()
	coord := &Process{ID: repository.CoordinatorID, Role: repository.RoleCoordinator}
	worker := &Process{ID: "worker-1", Role: repository.RoleWorker}

	reg.Register(coord)
	reg.Register(worker)

	got := reg.GetCoordinator()
	assert.Equal(t, coord, got)
}

func TestProcessRegistry_GetCoordinator_ReturnsNilWhenNoCoordinator(t *testing.T) {
	reg := NewProcessRegistry()
	worker := &Process{ID: "worker-1", Role: repository.RoleWorker}
	reg.Register(worker)

	got := reg.GetCoordinator()
	assert.Nil(t, got)
}

func TestProcessRegistry_Workers_ReturnsOnlyWorkerProcesses(t *testing.T) {
	reg := NewProcessRegistry()
	coord := &Process{ID: repository.CoordinatorID, Role: repository.RoleCoordinator}
	worker1 := &Process{ID: "worker-1", Role: repository.RoleWorker}
	worker2 := &Process{ID: "worker-2", Role: repository.RoleWorker}

	reg.Register(coord)
	reg.Register(worker1)
	reg.Register(worker2)

	workers := reg.Workers()
	require.Len(t, workers, 2)

	// Check that coordinator is excluded
	for _, w := range workers {
		assert.Equal(t, repository.RoleWorker, w.Role)
	}
}

func TestProcessRegistry_Workers_ExcludesCoordinator(t *testing.T) {
	reg := NewProcessRegistry()
	coord := &Process{ID: repository.CoordinatorID, Role: repository.RoleCoordinator}
	reg.Register(coord)

	workers := reg.Workers()
	assert.Empty(t, workers)
}

func TestProcessRegistry_Workers_ReturnsEmptySliceWhenNoWorkers(t *testing.T) {
	reg := NewProcessRegistry()
	workers := reg.Workers()
	assert.Empty(t, workers)
}

func TestProcessRegistry_ActiveCount_CountsNonRetiredWorkers(t *testing.T) {
	reg := NewProcessRegistry()
	worker1 := &Process{ID: "worker-1", Role: repository.RoleWorker, isRetired: false}
	worker2 := &Process{ID: "worker-2", Role: repository.RoleWorker, isRetired: false}
	retiredWorker := &Process{ID: "worker-3", Role: repository.RoleWorker, isRetired: true}

	reg.Register(worker1)
	reg.Register(worker2)
	reg.Register(retiredWorker)

	assert.Equal(t, 2, reg.ActiveCount())
}

func TestProcessRegistry_ActiveCount_ExcludesCoordinator(t *testing.T) {
	reg := NewProcessRegistry()
	coord := &Process{ID: repository.CoordinatorID, Role: repository.RoleCoordinator}
	worker := &Process{ID: "worker-1", Role: repository.RoleWorker}

	reg.Register(coord)
	reg.Register(worker)

	assert.Equal(t, 1, reg.ActiveCount())
}

func TestProcessRegistry_ActiveCount_ReturnsZeroForEmptyRegistry(t *testing.T) {
	reg := NewProcessRegistry()
	assert.Equal(t, 0, reg.ActiveCount())
}

func TestProcessRegistry_All_ReturnsAllProcesses(t *testing.T) {
	reg := NewProcessRegistry()
	coord := &Process{ID: repository.CoordinatorID, Role: repository.RoleCoordinator}
	worker1 := &Process{ID: "worker-1", Role: repository.RoleWorker}
	worker2 := &Process{ID: "worker-2", Role: repository.RoleWorker}

	reg.Register(coord)
	reg.Register(worker1)
	reg.Register(worker2)

	all := reg.All()
	assert.Len(t, all, 3)
}

func TestProcessRegistry_IDs_ReturnsAllIDs(t *testing.T) {
	reg := NewProcessRegistry()
	reg.Register(&Process{ID: "coordinator", Role: repository.RoleCoordinator})
	reg.Register(&Process{ID: "worker-1", Role: repository.RoleWorker})
	reg.Register(&Process{ID: "worker-2", Role: repository.RoleWorker})

	ids := reg.IDs()
	require.Len(t, ids, 3)
	assert.Contains(t, ids, "coordinator")
	assert.Contains(t, ids, "worker-1")
	assert.Contains(t, ids, "worker-2")
}

func TestProcessRegistry_ConcurrentRegister(t *testing.T) {
	reg := NewProcessRegistry()
	var wg sync.WaitGroup

	// Concurrent registrations
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			proc := &Process{ID: "worker", Role: repository.RoleWorker}
			reg.Register(proc)
		}(i)
	}

	wg.Wait()
	// Should not panic and should have exactly one process
	assert.Equal(t, 1, len(reg.All()))
}

func TestProcessRegistry_ConcurrentGet(t *testing.T) {
	reg := NewProcessRegistry()
	proc := &Process{ID: "worker-1", Role: repository.RoleWorker}
	reg.Register(proc)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := reg.Get("worker-1")
			assert.Equal(t, proc, got)
		}()
	}

	wg.Wait()
}

func TestProcessRegistry_ConcurrentRegisterUnregister(t *testing.T) {
	reg := NewProcessRegistry()
	var wg sync.WaitGroup

	// Register processes
	for i := 0; i < 50; i++ {
		proc := &Process{ID: "persistent", Role: repository.RoleWorker}
		reg.Register(proc)
	}

	// Concurrent register and unregister
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			proc := &Process{ID: "temp", Role: repository.RoleWorker}
			reg.Register(proc)
		}()
		go func() {
			defer wg.Done()
			reg.Unregister("temp")
		}()
	}

	wg.Wait()
	// Should not panic - final state is non-deterministic but valid
}

func TestProcessRegistry_StopAll_StopsAllProcesses(t *testing.T) {
	reg := NewProcessRegistry()

	// Create processes with properly initialized context/cancel/eventDone
	createStoppableProcess := func(id string, role repository.ProcessRole) *Process {
		ctx, cancel := context.WithCancel(context.Background())
		eventDone := make(chan struct{})
		close(eventDone) // Pre-close so Stop() doesn't block

		return &Process{
			ID:        id,
			Role:      role,
			ctx:       ctx,
			cancel:    cancel,
			eventDone: eventDone,
		}
	}

	reg.Register(createStoppableProcess("coordinator", repository.RoleCoordinator))
	reg.Register(createStoppableProcess("worker-1", repository.RoleWorker))
	reg.Register(createStoppableProcess("worker-2", repository.RoleWorker))

	// StopAll should not panic and should complete
	assert.NotPanics(t, func() {
		reg.StopAll()
	})

	// Verify all 3 processes are still in registry (StopAll doesn't unregister)
	assert.Len(t, reg.All(), 3)
}

func TestProcessRegistry_StopAll_EmptyRegistry(t *testing.T) {
	reg := NewProcessRegistry()

	// Should not panic on empty registry
	assert.NotPanics(t, func() {
		reg.StopAll()
	})
}
