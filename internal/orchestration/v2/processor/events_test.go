package processor

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zjrosen/xorchestrator/internal/orchestration/v2/command"
)

// TestCommandLogEvent_FieldsExported verifies that all CommandLogEvent struct fields
// are properly exported (capital letters) and accessible.
func TestCommandLogEvent_FieldsExported(t *testing.T) {
	// This test verifies that all fields are exported by attempting to access them.
	// If any field were unexported, this would fail to compile.
	event := CommandLogEvent{
		CommandID:   "test-id",
		CommandType: command.CmdSpawnProcess,
		Source:      command.SourceUser,
		Success:     true,
		Error:       nil,
		Duration:    100 * time.Millisecond,
		Timestamp:   time.Now(),
	}

	// Verify each field is accessible and holds the expected value
	assert.Equal(t, "test-id", event.CommandID)
	assert.Equal(t, command.CmdSpawnProcess, event.CommandType)
	assert.Equal(t, command.SourceUser, event.Source)
	assert.True(t, event.Success)
	assert.Nil(t, event.Error)
	assert.Equal(t, 100*time.Millisecond, event.Duration)
	assert.False(t, event.Timestamp.IsZero())
}

// TestCommandLogEvent_AllFieldTypes verifies that CommandLogEvent can be created
// with all field types including error values.
func TestCommandLogEvent_AllFieldTypes(t *testing.T) {
	tests := []struct {
		name        string
		commandID   string
		commandType command.CommandType
		source      command.CommandSource
		success     bool
		err         error
		duration    time.Duration
	}{
		{
			name:        "success with mcp_tool source",
			commandID:   "cmd-123",
			commandType: command.CmdAssignTask,
			source:      command.SourceMCPTool,
			success:     true,
			err:         nil,
			duration:    50 * time.Millisecond,
		},
		{
			name:        "failure with internal source",
			commandID:   "cmd-456",
			commandType: command.CmdRetireProcess,
			source:      command.SourceInternal,
			success:     false,
			err:         errors.New("process not found"),
			duration:    10 * time.Millisecond,
		},
		{
			name:        "success with callback source",
			commandID:   "cmd-789",
			commandType: command.CmdReportComplete,
			source:      command.SourceCallback,
			success:     true,
			err:         nil,
			duration:    200 * time.Millisecond,
		},
		{
			name:        "failure with user source",
			commandID:   "cmd-abc",
			commandType: command.CmdSendToProcess,
			source:      command.SourceUser,
			success:     false,
			err:         errors.New("invalid message"),
			duration:    1 * time.Second,
		},
		{
			name:        "zero duration",
			commandID:   "cmd-zero",
			commandType: command.CmdBroadcast,
			source:      command.SourceInternal,
			success:     true,
			err:         nil,
			duration:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timestamp := time.Now()
			event := CommandLogEvent{
				CommandID:   tt.commandID,
				CommandType: tt.commandType,
				Source:      tt.source,
				Success:     tt.success,
				Error:       tt.err,
				Duration:    tt.duration,
				Timestamp:   timestamp,
			}

			require.Equal(t, tt.commandID, event.CommandID)
			require.Equal(t, tt.commandType, event.CommandType)
			require.Equal(t, tt.source, event.Source)
			require.Equal(t, tt.success, event.Success)
			require.Equal(t, tt.err, event.Error)
			require.Equal(t, tt.duration, event.Duration)
			require.Equal(t, timestamp, event.Timestamp)
		})
	}
}
