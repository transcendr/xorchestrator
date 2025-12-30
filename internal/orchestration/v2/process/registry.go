package process

import (
	"fmt"
	"sync"

	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

// ProcessRegistry tracks active Process instances for runtime operations.
// This replaces both the WorkerRegistry (for workers) and the coordinator singleton,
// providing a unified registry for all process types.
//
// The ProcessRepository is the source of truth for process state (persisted data).
// The ProcessRegistry holds the live Process objects with their event loops.
type ProcessRegistry struct {
	processes map[string]*Process
	mu        sync.RWMutex
}

// NewProcessRegistry creates a new ProcessRegistry.
func NewProcessRegistry() *ProcessRegistry {
	return &ProcessRegistry{
		processes: make(map[string]*Process),
	}
}

// Register adds a Process to the registry.
// If a process with the same ID already exists, it is replaced.
func (r *ProcessRegistry) Register(p *Process) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.processes[p.ID] = p
}

// Unregister removes a Process from the registry.
// Returns true if the process was found and removed.
func (r *ProcessRegistry) Unregister(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.processes[id]; exists {
		delete(r.processes, id)
		return true
	}
	return false
}

// Get returns the Process for the given ID, or nil if not found.
func (r *ProcessRegistry) Get(id string) *Process {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.processes[id]
}

// GetCoordinator returns the coordinator process, or nil if not registered.
func (r *ProcessRegistry) GetCoordinator() *Process {
	return r.Get(repository.CoordinatorID)
}

// Workers returns all registered worker processes (excluding coordinator).
// Returns a slice copy to prevent races.
func (r *ProcessRegistry) Workers() []*Process {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var workers []*Process
	for _, p := range r.processes {
		if p.Role == repository.RoleWorker {
			workers = append(workers, p)
		}
	}
	return workers
}

// ActiveCount returns the number of non-retired worker processes.
// This excludes the coordinator and any retired workers.
func (r *ProcessRegistry) ActiveCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, p := range r.processes {
		if p.Role == repository.RoleWorker && !p.isRetired {
			count++
		}
	}
	return count
}

// All returns all registered Process instances.
// Returns a slice copy to prevent races.
func (r *ProcessRegistry) All() []*Process {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Process, 0, len(r.processes))
	for _, p := range r.processes {
		result = append(result, p)
	}
	return result
}

// IDs returns all registered process IDs.
func (r *ProcessRegistry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.processes))
	for id := range r.processes {
		ids = append(ids, id)
	}
	return ids
}

// ResumeProcess implements the integration.ProcessResumer interface.
// It attaches a new AI process to an existing Process and restarts the event loop.
// This is called by ProcessSessionDeliverer when delivering a queued message.
// Works for both coordinator and worker processes.
func (r *ProcessRegistry) ResumeProcess(processID string, proc client.HeadlessProcess) error {
	p := r.Get(processID)
	if p == nil {
		return fmt.Errorf("process not found: %s", processID)
	}

	p.Resume(proc)
	return nil
}
