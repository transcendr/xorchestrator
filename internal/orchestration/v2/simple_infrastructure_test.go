package v2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
)

// ===========================================================================
// SimpleInfrastructure Tests
// ===========================================================================

func TestSimpleInfrastructureConfig_Validate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		mockClient := mocks.NewMockHeadlessClient(t)
		cfg := SimpleInfrastructureConfig{
			AIClient:     mockClient,
			WorkDir:      "/tmp/test",
			SystemPrompt: "You are a helpful assistant.",
		}
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("nil AIClient returns error", func(t *testing.T) {
		cfg := SimpleInfrastructureConfig{
			AIClient: nil,
			WorkDir:  "/tmp/test",
		}
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "AI client is required")
	})

	t.Run("empty WorkDir returns error", func(t *testing.T) {
		mockClient := mocks.NewMockHeadlessClient(t)
		cfg := SimpleInfrastructureConfig{
			AIClient: mockClient,
			WorkDir:  "",
		}
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "work directory is required")
	})
}

func TestNewSimpleInfrastructure(t *testing.T) {
	t.Run("creates infrastructure with valid config", func(t *testing.T) {
		mockClient := mocks.NewMockHeadlessClient(t)
		cfg := SimpleInfrastructureConfig{
			AIClient:     mockClient,
			WorkDir:      "/tmp/test",
			SystemPrompt: "You are a helpful assistant.",
		}

		infra, err := NewSimpleInfrastructure(cfg)
		require.NoError(t, err)
		require.NotNil(t, infra)

		assert.NotNil(t, infra.Processor)
		assert.NotNil(t, infra.EventBus)
		assert.NotNil(t, infra.ProcessRepo)
		assert.NotNil(t, infra.QueueRepo)
		assert.NotNil(t, infra.ProcessRegistry)
		assert.NotNil(t, infra.CmdSubmitter)
	})

	t.Run("returns error for nil AIClient", func(t *testing.T) {
		cfg := SimpleInfrastructureConfig{
			AIClient: nil,
			WorkDir:  "/tmp/test",
		}

		infra, err := NewSimpleInfrastructure(cfg)
		assert.Error(t, err)
		assert.Nil(t, infra)
		assert.Contains(t, err.Error(), "AI client is required")
	})

	t.Run("returns error for empty WorkDir", func(t *testing.T) {
		mockClient := mocks.NewMockHeadlessClient(t)
		cfg := SimpleInfrastructureConfig{
			AIClient: mockClient,
			WorkDir:  "",
		}

		infra, err := NewSimpleInfrastructure(cfg)
		assert.Error(t, err)
		assert.Nil(t, infra)
		assert.Contains(t, err.Error(), "work directory is required")
	})
}

func TestSimpleInfrastructure_Lifecycle(t *testing.T) {
	t.Run("start and shutdown", func(t *testing.T) {
		mockClient := mocks.NewMockHeadlessClient(t)
		cfg := SimpleInfrastructureConfig{
			AIClient:     mockClient,
			WorkDir:      "/tmp/test",
			SystemPrompt: "You are a helpful assistant.",
		}

		infra, err := NewSimpleInfrastructure(cfg)
		require.NoError(t, err)

		err = infra.Start()
		require.NoError(t, err)

		assert.True(t, infra.Processor.IsRunning())

		infra.Shutdown()
		assert.False(t, infra.Processor.IsRunning())
	})

	t.Run("shutdown handles unstarted infrastructure", func(t *testing.T) {
		mockClient := mocks.NewMockHeadlessClient(t)
		cfg := SimpleInfrastructureConfig{
			AIClient: mockClient,
			WorkDir:  "/tmp/test",
		}

		infra, err := NewSimpleInfrastructure(cfg)
		require.NoError(t, err)

		assert.NotPanics(t, func() {
			infra.Shutdown()
		})
	})
}

func TestSimpleInfrastructure_RegistersOnlyCoreHandlers(t *testing.T) {
	mockClient := mocks.NewMockHeadlessClient(t)
	cfg := SimpleInfrastructureConfig{
		AIClient:     mockClient,
		WorkDir:      "/tmp/test",
		SystemPrompt: "You are a helpful assistant.",
	}

	infra, err := NewSimpleInfrastructure(cfg)
	require.NoError(t, err)

	err = infra.Start()
	require.NoError(t, err)
	defer infra.Shutdown()

	assert.True(t, infra.Processor.IsRunning())
	assert.NotNil(t, infra.ProcessRepo)
	assert.NotNil(t, infra.QueueRepo)
	assert.NotNil(t, infra.ProcessRegistry)
}
