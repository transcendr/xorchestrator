package command

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// ===========================================================================
// WorkflowStatus Tests
// ===========================================================================

func TestWorkflowStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status WorkflowStatus
		want   bool
	}{
		{"success is valid", WorkflowStatusSuccess, true},
		{"partial is valid", WorkflowStatusPartial, true},
		{"aborted is valid", WorkflowStatusAborted, true},
		{"empty is invalid", "", false},
		{"uppercase SUCCESS is invalid", "SUCCESS", false},
		{"random string is invalid", "failed", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.status.IsValid())
		})
	}
}

func TestWorkflowStatus_String(t *testing.T) {
	tests := []struct {
		status WorkflowStatus
		want   string
	}{
		{WorkflowStatusSuccess, "success"},
		{WorkflowStatusPartial, "partial"},
		{WorkflowStatusAborted, "aborted"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			require.Equal(t, tt.want, tt.status.String())
		})
	}
}

// ===========================================================================
// SignalWorkflowCompleteCommand Tests
// ===========================================================================

func TestSignalWorkflowCompleteCommand_Validate(t *testing.T) {
	tests := []struct {
		name        string
		status      WorkflowStatus
		summary     string
		epicID      string
		tasksClosed int
		wantErr     bool
		errSubstr   string
	}{
		{
			name:        "valid success with all fields",
			status:      WorkflowStatusSuccess,
			summary:     "All tasks completed successfully",
			epicID:      "xorchestrator-abc1",
			tasksClosed: 5,
			wantErr:     false,
		},
		{
			name:        "valid partial without optional fields",
			status:      WorkflowStatusPartial,
			summary:     "Some tasks completed",
			epicID:      "",
			tasksClosed: 0,
			wantErr:     false,
		},
		{
			name:        "valid aborted",
			status:      WorkflowStatusAborted,
			summary:     "Workflow aborted by user",
			epicID:      "",
			tasksClosed: 0,
			wantErr:     false,
		},
		{
			name:        "invalid status - empty",
			status:      "",
			summary:     "Some summary",
			epicID:      "",
			tasksClosed: 0,
			wantErr:     true,
			errSubstr:   "status must be success, partial, or aborted",
		},
		{
			name:        "invalid status - random string",
			status:      "failed",
			summary:     "Some summary",
			epicID:      "",
			tasksClosed: 0,
			wantErr:     true,
			errSubstr:   "status must be success, partial, or aborted",
		},
		{
			name:        "invalid status - uppercase",
			status:      "SUCCESS",
			summary:     "Some summary",
			epicID:      "",
			tasksClosed: 0,
			wantErr:     true,
			errSubstr:   "status must be success, partial, or aborted",
		},
		{
			name:        "empty summary",
			status:      WorkflowStatusSuccess,
			summary:     "",
			epicID:      "",
			tasksClosed: 0,
			wantErr:     true,
			errSubstr:   "summary is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewSignalWorkflowCompleteCommand(SourceMCPTool, tt.status, tt.summary, tt.epicID, tt.tasksClosed)
			err := cmd.Validate()
			if tt.wantErr {
				require.Error(t, err)
				if tt.errSubstr != "" {
					require.Contains(t, err.Error(), tt.errSubstr)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSignalWorkflowCompleteCommand_Type(t *testing.T) {
	cmd := NewSignalWorkflowCompleteCommand(SourceMCPTool, WorkflowStatusSuccess, "summary", "", 0)
	require.Equal(t, CmdSignalWorkflowComplete, cmd.Type())
}

func TestSignalWorkflowCompleteCommand_String(t *testing.T) {
	tests := []struct {
		name        string
		status      WorkflowStatus
		epicID      string
		tasksClosed int
		wantContain string
	}{
		{
			name:        "with epic ID",
			status:      WorkflowStatusSuccess,
			epicID:      "xorchestrator-abc1",
			tasksClosed: 3,
			wantContain: "epic=xorchestrator-abc1",
		},
		{
			name:        "without epic ID",
			status:      WorkflowStatusPartial,
			epicID:      "",
			tasksClosed: 2,
			wantContain: "status=partial",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewSignalWorkflowCompleteCommand(SourceMCPTool, tt.status, "summary", tt.epicID, tt.tasksClosed)
			str := cmd.String()
			require.Contains(t, str, tt.wantContain)
		})
	}
}

func TestSignalWorkflowCompleteCommand_ImplementsCommand(t *testing.T) {
	var _ Command = &SignalWorkflowCompleteCommand{}
}

func TestNewSignalWorkflowCompleteCommand(t *testing.T) {
	tests := []struct {
		name           string
		source         CommandSource
		status         WorkflowStatus
		summary        string
		epicID         string
		tasksClosed    int
		expectedType   CommandType
		expectedSource CommandSource
	}{
		{
			name:           "from MCP tool",
			source:         SourceMCPTool,
			status:         WorkflowStatusSuccess,
			summary:        "All done",
			epicID:         "xorchestrator-abc1",
			tasksClosed:    5,
			expectedType:   CmdSignalWorkflowComplete,
			expectedSource: SourceMCPTool,
		},
		{
			name:           "from internal",
			source:         SourceInternal,
			status:         WorkflowStatusAborted,
			summary:        "User cancelled",
			epicID:         "",
			tasksClosed:    0,
			expectedType:   CmdSignalWorkflowComplete,
			expectedSource: SourceInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewSignalWorkflowCompleteCommand(tt.source, tt.status, tt.summary, tt.epicID, tt.tasksClosed)

			// Verify fields are set correctly
			require.Equal(t, tt.status, cmd.Status)
			require.Equal(t, tt.summary, cmd.Summary)
			require.Equal(t, tt.epicID, cmd.EpicID)
			require.Equal(t, tt.tasksClosed, cmd.TasksClosed)

			// Verify BaseCommand fields
			require.Equal(t, tt.expectedType, cmd.Type())
			require.Equal(t, tt.expectedSource, cmd.Source())
			require.NotEmpty(t, cmd.ID())
			require.False(t, cmd.CreatedAt().IsZero())
		})
	}
}

func TestCmdSignalWorkflowComplete_ConstantValue(t *testing.T) {
	require.Equal(t, CommandType("signal_workflow_complete"), CmdSignalWorkflowComplete)
}
