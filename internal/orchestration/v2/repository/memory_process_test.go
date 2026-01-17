package repository

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zjrosen/xorchestrator/internal/orchestration/events"
)

// ===========================================================================
// ProcessRole and ProcessStatus Type Tests
// ===========================================================================

func TestProcessRole_Constants(t *testing.T) {
	// Verify ProcessRole constants have correct values
	assert.Equal(t, ProcessRole("coordinator"), RoleCoordinator)
	assert.Equal(t, ProcessRole("worker"), RoleWorker)
}

func TestProcessStatus_Constants(t *testing.T) {
	// Verify ProcessStatus constants have correct values
	assert.Equal(t, ProcessStatus("pending"), StatusPending)
	assert.Equal(t, ProcessStatus("starting"), StatusStarting)
	assert.Equal(t, ProcessStatus("ready"), StatusReady)
	assert.Equal(t, ProcessStatus("working"), StatusWorking)
	assert.Equal(t, ProcessStatus("retired"), StatusRetired)
	assert.Equal(t, ProcessStatus("failed"), StatusFailed)
}

func TestCoordinatorID_Constant(t *testing.T) {
	// Verify CoordinatorID has the expected value
	assert.Equal(t, "coordinator", CoordinatorID)
}

// ===========================================================================
// Process Entity Tests
// ===========================================================================

func TestProcess_IsCoordinator_ReturnsTrue(t *testing.T) {
	p := &Process{
		ID:   CoordinatorID,
		Role: RoleCoordinator,
	}
	assert.True(t, p.IsCoordinator())
}

func TestProcess_IsCoordinator_ReturnsFalse(t *testing.T) {
	p := &Process{
		ID:   "worker-1",
		Role: RoleWorker,
	}
	assert.False(t, p.IsCoordinator())
}

func TestProcess_IsWorker_ReturnsTrue(t *testing.T) {
	p := &Process{
		ID:   "worker-1",
		Role: RoleWorker,
	}
	assert.True(t, p.IsWorker())
}

func TestProcess_IsWorker_ReturnsFalse(t *testing.T) {
	p := &Process{
		ID:   CoordinatorID,
		Role: RoleCoordinator,
	}
	assert.False(t, p.IsWorker())
}

func TestProcess_IsActive_ReturnsTrueForReady(t *testing.T) {
	p := &Process{
		ID:     "worker-1",
		Role:   RoleWorker,
		Status: StatusReady,
	}
	assert.True(t, p.IsActive())
}

func TestProcess_IsActive_ReturnsTrueForWorking(t *testing.T) {
	p := &Process{
		ID:     "worker-1",
		Role:   RoleWorker,
		Status: StatusWorking,
	}
	assert.True(t, p.IsActive())
}

func TestProcess_IsActive_ReturnsFalseForPending(t *testing.T) {
	p := &Process{
		ID:     "worker-1",
		Role:   RoleWorker,
		Status: StatusPending,
	}
	assert.False(t, p.IsActive())
}

func TestProcess_IsActive_ReturnsFalseForStarting(t *testing.T) {
	p := &Process{
		ID:     "worker-1",
		Role:   RoleWorker,
		Status: StatusStarting,
	}
	assert.False(t, p.IsActive())
}

func TestProcess_IsActive_ReturnsFalseForRetired(t *testing.T) {
	p := &Process{
		ID:     "worker-1",
		Role:   RoleWorker,
		Status: StatusRetired,
	}
	assert.False(t, p.IsActive())
}

func TestProcess_IsActive_ReturnsFalseForFailed(t *testing.T) {
	p := &Process{
		ID:     "worker-1",
		Role:   RoleWorker,
		Status: StatusFailed,
	}
	assert.False(t, p.IsActive())
}

// ===========================================================================
// MemoryProcessRepository Tests
// ===========================================================================

func TestMemoryProcessRepository_Save_CreatesNewProcess(t *testing.T) {
	repo := NewMemoryProcessRepository()

	process := &Process{
		ID:        "worker-1",
		Role:      RoleWorker,
		Status:    StatusReady,
		CreatedAt: time.Now(),
	}
	err := repo.Save(process)
	require.NoError(t, err)

	got, err := repo.Get("worker-1")
	require.NoError(t, err)
	assert.Equal(t, "worker-1", got.ID)
	assert.Equal(t, RoleWorker, got.Role)
	assert.Equal(t, StatusReady, got.Status)
}

func TestMemoryProcessRepository_Save_UpdatesExistingProcess(t *testing.T) {
	repo := NewMemoryProcessRepository()

	process := &Process{
		ID:     "worker-1",
		Role:   RoleWorker,
		Status: StatusReady,
	}
	require.NoError(t, repo.Save(process))

	// Update the process
	process.Status = StatusWorking
	process.TaskID = "test-task-1"
	require.NoError(t, repo.Save(process))

	got, err := repo.Get("worker-1")
	require.NoError(t, err)
	assert.Equal(t, StatusWorking, got.Status)
	assert.Equal(t, "test-task-1", got.TaskID)
}

func TestMemoryProcessRepository_Get_ReturnsProcess(t *testing.T) {
	repo := NewMemoryProcessRepository()

	process := &Process{
		ID:     "worker-1",
		Role:   RoleWorker,
		Status: StatusReady,
	}
	require.NoError(t, repo.Save(process))

	got, err := repo.Get("worker-1")
	require.NoError(t, err)
	assert.Equal(t, process, got)
}

func TestMemoryProcessRepository_Get_ReturnsErrProcessNotFound(t *testing.T) {
	repo := NewMemoryProcessRepository()

	got, err := repo.Get("nonexistent")
	assert.Nil(t, got)
	assert.ErrorIs(t, err, ErrProcessNotFound)
}

func TestMemoryProcessRepository_List_ReturnsAllProcesses(t *testing.T) {
	repo := NewMemoryProcessRepository()

	coord := &Process{ID: CoordinatorID, Role: RoleCoordinator, Status: StatusReady}
	worker1 := &Process{ID: "worker-1", Role: RoleWorker, Status: StatusReady}
	worker2 := &Process{ID: "worker-2", Role: RoleWorker, Status: StatusWorking}

	require.NoError(t, repo.Save(coord))
	require.NoError(t, repo.Save(worker1))
	require.NoError(t, repo.Save(worker2))

	processes := repo.List()
	assert.Len(t, processes, 3)

	// Verify all processes are present (order not guaranteed)
	ids := make(map[string]bool)
	for _, p := range processes {
		ids[p.ID] = true
	}
	assert.True(t, ids[CoordinatorID])
	assert.True(t, ids["worker-1"])
	assert.True(t, ids["worker-2"])
}

func TestMemoryProcessRepository_List_ReturnsEmptySlice(t *testing.T) {
	repo := NewMemoryProcessRepository()

	processes := repo.List()
	assert.Len(t, processes, 0)
	assert.NotNil(t, processes)
}

func TestMemoryProcessRepository_GetCoordinator_ReturnsCoordinator(t *testing.T) {
	repo := NewMemoryProcessRepository()

	coord := &Process{
		ID:     CoordinatorID,
		Role:   RoleCoordinator,
		Status: StatusReady,
	}
	require.NoError(t, repo.Save(coord))

	got, err := repo.GetCoordinator()
	require.NoError(t, err)
	assert.Equal(t, coord, got)
}

func TestMemoryProcessRepository_GetCoordinator_ReturnsErrProcessNotFound(t *testing.T) {
	repo := NewMemoryProcessRepository()

	got, err := repo.GetCoordinator()
	assert.Nil(t, got)
	assert.ErrorIs(t, err, ErrProcessNotFound)
}

func TestMemoryProcessRepository_GetCoordinator_IgnoresWorkers(t *testing.T) {
	repo := NewMemoryProcessRepository()

	// Add only workers, no coordinator
	worker1 := &Process{ID: "worker-1", Role: RoleWorker, Status: StatusReady}
	worker2 := &Process{ID: "worker-2", Role: RoleWorker, Status: StatusWorking}

	require.NoError(t, repo.Save(worker1))
	require.NoError(t, repo.Save(worker2))

	got, err := repo.GetCoordinator()
	assert.Nil(t, got)
	assert.ErrorIs(t, err, ErrProcessNotFound)
}

func TestMemoryProcessRepository_Workers_ReturnsOnlyWorkers(t *testing.T) {
	repo := NewMemoryProcessRepository()

	coord := &Process{ID: CoordinatorID, Role: RoleCoordinator, Status: StatusReady}
	worker1 := &Process{ID: "worker-1", Role: RoleWorker, Status: StatusReady}
	worker2 := &Process{ID: "worker-2", Role: RoleWorker, Status: StatusWorking}

	require.NoError(t, repo.Save(coord))
	require.NoError(t, repo.Save(worker1))
	require.NoError(t, repo.Save(worker2))

	workers := repo.Workers()
	assert.Len(t, workers, 2)

	ids := make(map[string]bool)
	for _, w := range workers {
		ids[w.ID] = true
	}
	assert.True(t, ids["worker-1"])
	assert.True(t, ids["worker-2"])
	assert.False(t, ids[CoordinatorID], "coordinator should not be in workers list")
}

func TestMemoryProcessRepository_Workers_ExcludesCoordinator(t *testing.T) {
	repo := NewMemoryProcessRepository()

	coord := &Process{ID: CoordinatorID, Role: RoleCoordinator, Status: StatusReady}
	require.NoError(t, repo.Save(coord))

	workers := repo.Workers()
	assert.Len(t, workers, 0)
}

func TestMemoryProcessRepository_Workers_ReturnsEmptySlice(t *testing.T) {
	repo := NewMemoryProcessRepository()

	workers := repo.Workers()
	assert.Len(t, workers, 0)
	assert.NotNil(t, workers)
}

func TestMemoryProcessRepository_ActiveWorkers_ExcludesRetired(t *testing.T) {
	repo := NewMemoryProcessRepository()

	worker1 := &Process{ID: "worker-1", Role: RoleWorker, Status: StatusReady}
	worker2 := &Process{ID: "worker-2", Role: RoleWorker, Status: StatusWorking}
	worker3 := &Process{ID: "worker-3", Role: RoleWorker, Status: StatusRetired}

	require.NoError(t, repo.Save(worker1))
	require.NoError(t, repo.Save(worker2))
	require.NoError(t, repo.Save(worker3))

	active := repo.ActiveWorkers()
	assert.Len(t, active, 2)

	ids := make(map[string]bool)
	for _, w := range active {
		ids[w.ID] = true
	}
	assert.True(t, ids["worker-1"])
	assert.True(t, ids["worker-2"])
	assert.False(t, ids["worker-3"], "retired worker should not be in active list")
}

func TestMemoryProcessRepository_ActiveWorkers_ExcludesFailed(t *testing.T) {
	repo := NewMemoryProcessRepository()

	worker1 := &Process{ID: "worker-1", Role: RoleWorker, Status: StatusReady}
	worker2 := &Process{ID: "worker-2", Role: RoleWorker, Status: StatusWorking}
	worker3 := &Process{ID: "worker-3", Role: RoleWorker, Status: StatusFailed}

	require.NoError(t, repo.Save(worker1))
	require.NoError(t, repo.Save(worker2))
	require.NoError(t, repo.Save(worker3))

	active := repo.ActiveWorkers()
	assert.Len(t, active, 2)

	ids := make(map[string]bool)
	for _, w := range active {
		ids[w.ID] = true
	}
	assert.True(t, ids["worker-1"])
	assert.True(t, ids["worker-2"])
	assert.False(t, ids["worker-3"], "failed worker should not be in active list")
}

func TestMemoryProcessRepository_ActiveWorkers_IncludesReadyAndWorking(t *testing.T) {
	repo := NewMemoryProcessRepository()

	worker1 := &Process{ID: "worker-1", Role: RoleWorker, Status: StatusReady}
	worker2 := &Process{ID: "worker-2", Role: RoleWorker, Status: StatusWorking}
	worker3 := &Process{ID: "worker-3", Role: RoleWorker, Status: StatusStarting}
	worker4 := &Process{ID: "worker-4", Role: RoleWorker, Status: StatusPending}

	require.NoError(t, repo.Save(worker1))
	require.NoError(t, repo.Save(worker2))
	require.NoError(t, repo.Save(worker3))
	require.NoError(t, repo.Save(worker4))

	active := repo.ActiveWorkers()
	// All non-terminal statuses should be included
	assert.Len(t, active, 4)
}

func TestMemoryProcessRepository_ActiveWorkers_ExcludesCoordinator(t *testing.T) {
	repo := NewMemoryProcessRepository()

	coord := &Process{ID: CoordinatorID, Role: RoleCoordinator, Status: StatusReady}
	worker1 := &Process{ID: "worker-1", Role: RoleWorker, Status: StatusReady}

	require.NoError(t, repo.Save(coord))
	require.NoError(t, repo.Save(worker1))

	active := repo.ActiveWorkers()
	assert.Len(t, active, 1)
	assert.Equal(t, "worker-1", active[0].ID)
}

func TestMemoryProcessRepository_ReadyWorkers_FiltersByStatusReady(t *testing.T) {
	repo := NewMemoryProcessRepository()

	idle := events.ProcessPhaseIdle

	worker1 := &Process{ID: "worker-1", Role: RoleWorker, Status: StatusReady, Phase: &idle}
	worker2 := &Process{ID: "worker-2", Role: RoleWorker, Status: StatusWorking, Phase: &idle}
	worker3 := &Process{ID: "worker-3", Role: RoleWorker, Status: StatusRetired, Phase: &idle}

	require.NoError(t, repo.Save(worker1))
	require.NoError(t, repo.Save(worker2))
	require.NoError(t, repo.Save(worker3))

	ready := repo.ReadyWorkers()
	assert.Len(t, ready, 1)
	assert.Equal(t, "worker-1", ready[0].ID)
}

func TestMemoryProcessRepository_ReadyWorkers_FiltersByIdlePhase(t *testing.T) {
	repo := NewMemoryProcessRepository()

	idle := events.ProcessPhaseIdle
	implementing := events.ProcessPhaseImplementing

	worker1 := &Process{ID: "worker-1", Role: RoleWorker, Status: StatusReady, Phase: &idle}
	worker2 := &Process{ID: "worker-2", Role: RoleWorker, Status: StatusReady, Phase: &implementing}
	worker3 := &Process{ID: "worker-3", Role: RoleWorker, Status: StatusReady, Phase: nil} // nil phase should be treated as idle

	require.NoError(t, repo.Save(worker1))
	require.NoError(t, repo.Save(worker2))
	require.NoError(t, repo.Save(worker3))

	ready := repo.ReadyWorkers()
	assert.Len(t, ready, 2)

	ids := make(map[string]bool)
	for _, w := range ready {
		ids[w.ID] = true
	}
	assert.True(t, ids["worker-1"])
	assert.True(t, ids["worker-3"], "nil phase should be treated as idle")
	assert.False(t, ids["worker-2"], "implementing phase should not be in ready list")
}

func TestMemoryProcessRepository_ReadyWorkers_ExcludesCoordinator(t *testing.T) {
	repo := NewMemoryProcessRepository()

	coord := &Process{ID: CoordinatorID, Role: RoleCoordinator, Status: StatusReady}
	require.NoError(t, repo.Save(coord))

	ready := repo.ReadyWorkers()
	assert.Len(t, ready, 0)
}

func TestMemoryProcessRepository_Reset(t *testing.T) {
	repo := NewMemoryProcessRepository()

	process := &Process{ID: "worker-1", Role: RoleWorker, Status: StatusReady}
	require.NoError(t, repo.Save(process))
	assert.Len(t, repo.List(), 1)

	repo.Reset()

	assert.Len(t, repo.List(), 0)
	_, err := repo.Get("worker-1")
	assert.ErrorIs(t, err, ErrProcessNotFound)
}

func TestMemoryProcessRepository_AddProcess(t *testing.T) {
	repo := NewMemoryProcessRepository()

	process := &Process{
		ID:     "worker-1",
		Role:   RoleWorker,
		Status: StatusReady,
	}
	repo.AddProcess(process)

	got, err := repo.Get("worker-1")
	require.NoError(t, err)
	assert.Equal(t, process, got)
}

// ===========================================================================
// Thread Safety Tests
// ===========================================================================

func TestMemoryProcessRepository_ThreadSafety(t *testing.T) {
	repo := NewMemoryProcessRepository()
	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			process := &Process{
				ID:     processID(id),
				Role:   RoleWorker,
				Status: StatusReady,
			}
			repo.Save(process)
		}(i)
	}

	wg.Wait()

	// Verify all processes were saved
	processes := repo.List()
	assert.Len(t, processes, numGoroutines)
}

func TestMemoryProcessRepository_ConcurrentReadWrite(t *testing.T) {
	repo := NewMemoryProcessRepository()
	var wg sync.WaitGroup

	// Pre-populate with some processes
	for i := 0; i < 10; i++ {
		repo.Save(&Process{
			ID:     processID(i),
			Role:   RoleWorker,
			Status: StatusReady,
		})
	}
	repo.Save(&Process{
		ID:     CoordinatorID,
		Role:   RoleCoordinator,
		Status: StatusReady,
	})

	// Concurrent reads and writes
	for i := 0; i < 50; i++ {
		wg.Add(5)

		// List reader
		go func() {
			defer wg.Done()
			_ = repo.List()
		}()

		// Workers reader
		go func() {
			defer wg.Done()
			_ = repo.Workers()
			_ = repo.ActiveWorkers()
			_ = repo.ReadyWorkers()
		}()

		// GetCoordinator reader
		go func() {
			defer wg.Done()
			_, _ = repo.GetCoordinator()
		}()

		// Writer
		go func(id int) {
			defer wg.Done()
			repo.Save(&Process{
				ID:     processID(100 + id),
				Role:   RoleWorker,
				Status: StatusWorking,
			})
		}(i)

		// Getter
		go func(id int) {
			defer wg.Done()
			repo.Get(processID(id % 10))
		}(i)
	}

	wg.Wait()
}

// Helper function to generate process IDs for testing
func processID(n int) string {
	return workerID(n) // Reuse the existing helper
}
