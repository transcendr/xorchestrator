package command

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCommandType_String(t *testing.T) {
	tests := []struct {
		name     string
		cmdType  CommandType
		expected string
	}{
		// Task Assignment Commands
		{"assign_task", CmdAssignTask, "assign_task"},
		{"assign_review", CmdAssignReview, "assign_review"},
		{"approve_commit", CmdApproveCommit, "approve_commit"},
		// Message Routing Commands
		{"broadcast", CmdBroadcast, "broadcast"},
		// State Transition Commands
		{"report_complete", CmdReportComplete, "report_complete"},
		{"report_verdict", CmdReportVerdict, "report_verdict"},
		{"transition_phase", CmdTransitionPhase, "transition_phase"},
		// Unified Process Commands
		{"spawn_process", CmdSpawnProcess, "spawn_process"},
		{"retire_process", CmdRetireProcess, "retire_process"},
		{"replace_process", CmdReplaceProcess, "replace_process"},
		{"send_to_process", CmdSendToProcess, "send_to_process"},
		{"deliver_process_queued", CmdDeliverProcessQueued, "deliver_process_queued"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.cmdType.String())
		})
	}
}

func TestCommandSource_String(t *testing.T) {
	tests := []struct {
		name     string
		source   CommandSource
		expected string
	}{
		{"mcp_tool", SourceMCPTool, "mcp_tool"},
		{"internal", SourceInternal, "internal"},
		{"callback", SourceCallback, "callback"},
		{"user", SourceUser, "user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.source.String())
		})
	}
}

func TestNewBaseCommand_GeneratesUniqueIDs(t *testing.T) {
	cmd1 := NewBaseCommand(CmdSpawnProcess, SourceMCPTool)
	cmd2 := NewBaseCommand(CmdSpawnProcess, SourceMCPTool)

	require.NotEmpty(t, cmd1.ID())
	require.NotEmpty(t, cmd2.ID())
	require.NotEqual(t, cmd1.ID(), cmd2.ID())
}

func TestNewBaseCommand_SetsTimestampCorrectly(t *testing.T) {
	before := time.Now()
	cmd := NewBaseCommand(CmdAssignTask, SourceInternal)
	after := time.Now()

	require.False(t, cmd.CreatedAt().Before(before), "CreatedAt should not be before test start")
	require.False(t, cmd.CreatedAt().After(after), "CreatedAt should not be after test end")
}

func TestBaseCommand_ID(t *testing.T) {
	cmd := NewBaseCommand(CmdSpawnProcess, SourceMCPTool)
	id := cmd.ID()

	// UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx (36 chars)
	require.Len(t, id, 36, "ID should be 36 characters (UUID format)")

	// Verify it returns the same ID on multiple calls
	require.Equal(t, id, cmd.ID(), "ID() should return the same value on multiple calls")
}

func TestBaseCommand_Type(t *testing.T) {
	tests := []struct {
		cmdType CommandType
	}{
		{CmdSpawnProcess},
		{CmdRetireProcess},
		{CmdAssignTask},
		{CmdBroadcast},
	}

	for _, tt := range tests {
		t.Run(string(tt.cmdType), func(t *testing.T) {
			cmd := NewBaseCommand(tt.cmdType, SourceMCPTool)
			require.Equal(t, tt.cmdType, cmd.Type())
		})
	}
}

func TestBaseCommand_CreatedAt(t *testing.T) {
	before := time.Now()
	cmd := NewBaseCommand(CmdSpawnProcess, SourceMCPTool)
	after := time.Now()

	createdAt := cmd.CreatedAt()

	// Should be between before and after
	require.False(t, createdAt.Before(before) || createdAt.After(after),
		"CreatedAt() = %v, should be between %v and %v", createdAt, before, after)

	// Should return the same value on multiple calls
	require.Equal(t, createdAt, cmd.CreatedAt(), "CreatedAt() should return the same value on multiple calls")
}

func TestBaseCommand_Priority(t *testing.T) {
	cmd := NewBaseCommand(CmdSpawnProcess, SourceMCPTool)

	// Default priority should be 0
	require.Equal(t, 0, cmd.Priority(), "Default priority should be 0")

	// Test SetPriority
	cmd.SetPriority(1)
	require.Equal(t, 1, cmd.Priority(), "Priority after SetPriority(1) should be 1")
}

func TestBaseCommand_Source(t *testing.T) {
	tests := []struct {
		source CommandSource
	}{
		{SourceMCPTool},
		{SourceInternal},
		{SourceCallback},
		{SourceUser},
	}

	for _, tt := range tests {
		t.Run(string(tt.source), func(t *testing.T) {
			cmd := NewBaseCommand(CmdSpawnProcess, tt.source)
			require.Equal(t, tt.source, cmd.Source())
		})
	}
}

func TestBaseCommand_TraceID(t *testing.T) {
	cmd := NewBaseCommand(CmdSpawnProcess, SourceMCPTool)

	// Default traceID should be empty
	require.Empty(t, cmd.TraceID(), "Default traceID should be empty")

	// Test SetTraceID
	traceID := "trace-123"
	cmd.SetTraceID(traceID)
	require.Equal(t, traceID, cmd.TraceID())
}

func TestBaseCommand_Validate(t *testing.T) {
	cmd := NewBaseCommand(CmdSpawnProcess, SourceMCPTool)

	// BaseCommand.Validate() should return nil (no-op)
	require.NoError(t, cmd.Validate())
}

func TestBaseCommand_ImplementsCommandInterface(t *testing.T) {
	// Verify that BaseCommand implements the Command interface
	var _ Command = &BaseCommand{}
}

func TestCommandResult_Fields(t *testing.T) {
	// Test that CommandResult can hold all its field types
	result := CommandResult{
		Success:  true,
		Events:   []any{"event1", "event2"},
		FollowUp: []Command{},
		Error:    nil,
		Data:     "some data",
	}

	require.True(t, result.Success)
	require.Len(t, result.Events, 2)
	require.Nil(t, result.Error)
	require.Equal(t, "some data", result.Data)
}

func TestErrQueueFull(t *testing.T) {
	// Verify ErrQueueFull is defined and has expected message
	require.NotNil(t, ErrQueueFull)
	require.Equal(t, "command queue is full", ErrQueueFull.Error())
}

func TestAllCommandTypesAreDefined(t *testing.T) {
	// Verify all command types are distinct and defined
	allTypes := []CommandType{
		// Task Assignment Commands
		CmdAssignTask,
		CmdAssignReview,
		CmdApproveCommit,
		// Message Routing Commands
		CmdBroadcast,
		// State Transition Commands
		CmdReportComplete,
		CmdReportVerdict,
		CmdTransitionPhase,
		// BD Task Commands
		CmdMarkTaskComplete,
		CmdMarkTaskFailed,
		// Unified Process Commands
		CmdSpawnProcess,
		CmdRetireProcess,
		CmdReplaceProcess,
		CmdSendToProcess,
		CmdDeliverProcessQueued,
		CmdProcessTurnComplete,
	}

	require.Len(t, allTypes, 15)

	// Check all are unique
	seen := make(map[CommandType]bool)
	for _, ct := range allTypes {
		require.NotEmpty(t, ct, "CommandType should not be empty")
		require.False(t, seen[ct], "Duplicate CommandType: %s", ct)
		seen[ct] = true
	}
}

func TestAllCommandSourcesAreDefined(t *testing.T) {
	// Verify all 4 command sources are distinct and defined
	allSources := []CommandSource{
		SourceMCPTool,
		SourceInternal,
		SourceCallback,
		SourceUser,
	}

	require.Len(t, allSources, 4)

	// Check all are unique
	seen := make(map[CommandSource]bool)
	for _, cs := range allSources {
		require.NotEmpty(t, cs, "CommandSource should not be empty")
		require.False(t, seen[cs], "Duplicate CommandSource: %s", cs)
		seen[cs] = true
	}
}
