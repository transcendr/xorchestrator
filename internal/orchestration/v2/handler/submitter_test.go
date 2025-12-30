package handler

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
)

// mockProcessor implements ProcessorLike for testing.
type mockProcessor struct {
	submitted []command.Command
	err       error
}

func (m *mockProcessor) Submit(cmd command.Command) error {
	if m.err != nil {
		return m.err
	}
	m.submitted = append(m.submitted, cmd)
	return nil
}

func TestProcessorSubmitterAdapter_New(t *testing.T) {
	proc := &mockProcessor{}
	adapter := NewProcessorSubmitterAdapter(proc)
	require.NotNil(t, adapter)
}

func TestProcessorSubmitterAdapter_Submit_DelegatesToProcessor(t *testing.T) {
	proc := &mockProcessor{}
	adapter := NewProcessorSubmitterAdapter(proc)

	cmd := command.NewSpawnProcessCommand(command.SourceMCPTool, "coordinator")
	adapter.Submit(cmd)

	require.Len(t, proc.submitted, 1)
	require.Equal(t, cmd, proc.submitted[0])
}

func TestProcessorSubmitterAdapter_Submit_IgnoresError(t *testing.T) {
	proc := &mockProcessor{err: command.ErrQueueFull}
	adapter := NewProcessorSubmitterAdapter(proc)

	// Should not panic even when processor returns error
	cmd := command.NewSpawnProcessCommand(command.SourceMCPTool, "coordinator")
	adapter.Submit(cmd)

	// The command should not have been added (error occurred)
	require.Empty(t, proc.submitted)
}

func TestProcessorSubmitterAdapter_Submit_NilProcessor(t *testing.T) {
	adapter := NewProcessorSubmitterAdapter(nil)

	// Should not panic with nil processor
	cmd := command.NewSpawnProcessCommand(command.SourceMCPTool, "coordinator")
	adapter.Submit(cmd) // No panic
}

func TestProcessorSubmitterAdapter_ImplementsCommandSubmitter(t *testing.T) {
	proc := &mockProcessor{}
	adapter := NewProcessorSubmitterAdapter(proc)

	// Verify it implements process.CommandSubmitter interface
	var _ process.CommandSubmitter = adapter
}

func TestProcessorSubmitterAdapter_MultipleSubmits(t *testing.T) {
	proc := &mockProcessor{}
	adapter := NewProcessorSubmitterAdapter(proc)

	cmd1 := command.NewSpawnProcessCommand(command.SourceMCPTool, "coordinator")
	cmd2 := command.NewSpawnProcessCommand(command.SourceMCPTool, "coordinator")
	cmd3 := command.NewSpawnProcessCommand(command.SourceMCPTool, "coordinator")

	adapter.Submit(cmd1)
	adapter.Submit(cmd2)
	adapter.Submit(cmd3)

	require.Len(t, proc.submitted, 3)
}
