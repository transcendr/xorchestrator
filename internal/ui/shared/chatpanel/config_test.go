package chatpanel

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/xorchestrator/internal/orchestration/workflow"
)

func TestConfig_WithWorkflowRegistry(t *testing.T) {
	registry := workflow.NewRegistry()
	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}

	require.Equal(t, "claude", cfg.ClientType)
	require.Equal(t, "/test/dir", cfg.WorkDir)
	require.Equal(t, 30*time.Minute, cfg.SessionTimeout)
	require.NotNil(t, cfg.WorkflowRegistry)
	require.Same(t, registry, cfg.WorkflowRegistry)
}

func TestConfig_NilWorkflowRegistry_IsValid(t *testing.T) {
	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: nil,
	}

	require.Equal(t, "claude", cfg.ClientType)
	require.Equal(t, "/test/dir", cfg.WorkDir)
	require.Equal(t, 30*time.Minute, cfg.SessionTimeout)
	require.Nil(t, cfg.WorkflowRegistry, "nil WorkflowRegistry should be valid (feature disabled)")
}

func TestDefaultConfig_HasNilWorkflowRegistry(t *testing.T) {
	cfg := DefaultConfig()

	require.Nil(t, cfg.WorkflowRegistry, "default config should have nil WorkflowRegistry")
}
