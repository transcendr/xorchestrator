package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/mock"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/pubsub"
)

// mockCommandSubmitter implements process.CommandSubmitter for testing.
type mockCommandSubmitter struct {
	commands []command.Command
}

func (m *mockCommandSubmitter) Submit(cmd command.Command) {
	m.commands = append(m.commands, cmd)
}

func TestUnifiedProcessSpawner_SpawnProcess_Worker(t *testing.T) {
	mockClient := mock.NewClient()
	eventBus := pubsub.NewBroker[any]()
	submitter := &mockCommandSubmitter{}

	spawner := NewUnifiedProcessSpawner(UnifiedSpawnerConfig{
		Client:     mockClient,
		WorkDir:    "/test/workdir",
		Port:       8080,
		Extensions: nil,
		Submitter:  submitter,
		EventBus:   eventBus,
	})

	proc, err := spawner.SpawnProcess(context.Background(), "worker-1", repository.RoleWorker)
	require.NoError(t, err)
	require.NotNil(t, proc)
	assert.Equal(t, "worker-1", proc.ID)
	assert.Equal(t, repository.RoleWorker, proc.Role)

	// Cleanup
	proc.Stop()
}

func TestUnifiedProcessSpawner_SpawnProcess_Coordinator(t *testing.T) {
	mockClient := mock.NewClient()
	eventBus := pubsub.NewBroker[any]()
	submitter := &mockCommandSubmitter{}

	spawner := NewUnifiedProcessSpawner(UnifiedSpawnerConfig{
		Client:     mockClient,
		WorkDir:    "/test/workdir",
		Port:       8080,
		Extensions: nil,
		Submitter:  submitter,
		EventBus:   eventBus,
	})

	proc, err := spawner.SpawnProcess(context.Background(), repository.CoordinatorID, repository.RoleCoordinator)
	require.NoError(t, err)
	require.NotNil(t, proc)
	assert.Equal(t, repository.CoordinatorID, proc.ID)
	assert.Equal(t, repository.RoleCoordinator, proc.Role)

	// Cleanup
	proc.Stop()
}

func TestUnifiedProcessSpawner_SpawnProcess_NilClient(t *testing.T) {
	eventBus := pubsub.NewBroker[any]()
	submitter := &mockCommandSubmitter{}

	spawner := NewUnifiedProcessSpawner(UnifiedSpawnerConfig{
		Client:     nil,
		WorkDir:    "/test/workdir",
		Port:       8080,
		Extensions: nil,
		Submitter:  submitter,
		EventBus:   eventBus,
	})

	proc, err := spawner.SpawnProcess(context.Background(), "worker-1", repository.RoleWorker)
	require.Error(t, err)
	require.Nil(t, proc)
	assert.Contains(t, err.Error(), "client is nil")
}

func TestUnifiedProcessSpawner_GenerateMCPConfig_HTTP(t *testing.T) {
	mockClient := mock.NewClient()
	spawner := &UnifiedProcessSpawnerImpl{
		client:  mockClient,
		port:    9999,
		workDir: "/test",
	}

	config, err := spawner.generateMCPConfig("worker-1")
	require.NoError(t, err)
	assert.Contains(t, config, "9999")
	assert.Contains(t, config, "worker-1")
}
