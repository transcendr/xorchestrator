package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
)

// mockMessageStore implements MessageStore for testing.
type mockMessageStore struct {
	entries   []message.Entry
	readState map[string]int
	mu        sync.RWMutex

	// Track method calls for verification
	appendCalls    []appendCall
	unreadForCalls []string
	markReadCalls  []string
}

type appendCall struct {
	From    string
	To      string
	Content string
	Type    message.MessageType
}

func newMockMessageStore() *mockMessageStore {
	return &mockMessageStore{
		entries:        make([]message.Entry, 0),
		readState:      make(map[string]int),
		appendCalls:    make([]appendCall, 0),
		unreadForCalls: make([]string, 0),
		markReadCalls:  make([]string, 0),
	}
}

// addEntry adds a message directly for test setup.
func (m *mockMessageStore) addEntry(from, to, content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, message.Entry{
		ID:        "test-" + from + "-" + to,
		Timestamp: time.Now(),
		From:      from,
		To:        to,
		Content:   content,
		Type:      message.MessageInfo,
	})
}

// UnreadFor returns all unread messages for the given agent (no recipient filtering).
func (m *mockMessageStore) UnreadFor(agentID string) []message.Entry {
	m.mu.Lock()
	m.unreadForCalls = append(m.unreadForCalls, agentID)
	m.mu.Unlock()

	m.mu.RLock()
	defer m.mu.RUnlock()

	lastRead := m.readState[agentID]
	if lastRead >= len(m.entries) {
		return nil
	}

	// Return all unread entries (no recipient filtering)
	unread := make([]message.Entry, len(m.entries)-lastRead)
	copy(unread, m.entries[lastRead:])
	return unread
}

// MarkRead marks all messages up to now as read by the given agent.
func (m *mockMessageStore) MarkRead(agentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markReadCalls = append(m.markReadCalls, agentID)
	m.readState[agentID] = len(m.entries)
}

// Append adds a new message to the log.
func (m *mockMessageStore) Append(from, to, content string, msgType message.MessageType) (*message.Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.appendCalls = append(m.appendCalls, appendCall{
		From:    from,
		To:      to,
		Content: content,
		Type:    msgType,
	})

	entry := message.Entry{
		ID:        "test-" + from + "-" + to,
		Timestamp: time.Now(),
		From:      from,
		To:        to,
		Content:   content,
		Type:      msgType,
	}

	m.entries = append(m.entries, entry)
	return &entry, nil
}

// TestWorkerServer_RegistersAllTools verifies all 6 worker tools are registered.
func TestWorkerServer_RegistersAllTools(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	expectedTools := []string{
		"check_messages",
		"post_message",
		"signal_ready",
		"report_implementation_complete",
		"report_review_verdict",
		"post_reflections",
	}

	for _, toolName := range expectedTools {
		_, ok := ws.tools[toolName]
		require.True(t, ok, "Tool %q not registered", toolName)
		_, ok = ws.handlers[toolName]
		require.True(t, ok, "Handler for %q not registered", toolName)
	}

	require.Equal(t, len(expectedTools), len(ws.tools), "Tool count mismatch")
}

// TestWorkerServer_ToolSchemas verifies tool schemas are valid.
func TestWorkerServer_ToolSchemas(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	for name, tool := range ws.tools {
		t.Run(name, func(t *testing.T) {
			require.NotEmpty(t, tool.Name, "Tool name is empty")
			require.NotEmpty(t, tool.Description, "Tool description is empty")
			require.NotNil(t, tool.InputSchema, "Tool inputSchema is nil")
			if tool.InputSchema != nil {
				require.Equal(t, "object", tool.InputSchema.Type, "InputSchema.Type mismatch")
			}
		})
	}
}

// TestWorkerServer_Instructions tests that instructions are set correctly.
func TestWorkerServer_Instructions(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	require.NotEmpty(t, ws.instructions, "Instructions should be set")
	require.Equal(t, "perles-worker", ws.info.Name, "Server name mismatch")
	require.Equal(t, "1.0.0", ws.info.Version, "Server version mismatch")
}

// TestWorkerServer_DifferentWorkerIDs verifies different workers get separate identities.
func TestWorkerServer_DifferentWorkerIDs(t *testing.T) {
	store := newMockMessageStore()
	ws1 := NewWorkerServer("WORKER.1", store)
	ws2 := NewWorkerServer("WORKER.2", store)

	// Test through behavior - send message from each worker
	handler1 := ws1.handlers["post_message"]
	handler2 := ws2.handlers["post_message"]

	_, _ = handler1(context.Background(), json.RawMessage(`{"to": "ALL", "content": "from worker 1"}`))
	_, _ = handler2(context.Background(), json.RawMessage(`{"to": "ALL", "content": "from worker 2"}`))

	// Verify messages were sent with correct worker IDs
	require.Len(t, store.appendCalls, 2, "Expected 2 append calls")
	require.Equal(t, "WORKER.1", store.appendCalls[0].From, "First message from mismatch")
	require.Equal(t, "WORKER.2", store.appendCalls[1].From, "Second message from mismatch")
}

// TestWorkerServer_CheckMessagesNoStore tests check_messages when no store is available.
func TestWorkerServer_CheckMessagesNoStore(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)
	handler := ws.handlers["check_messages"]

	_, err := handler(context.Background(), json.RawMessage(`{}`))
	require.Error(t, err, "Expected error when message store is nil")
	require.Contains(t, err.Error(), "message store not available", "Error should mention 'message store not available'")
}

// TestWorkerServer_CheckMessagesHappyPath tests successful message retrieval.
func TestWorkerServer_CheckMessagesHappyPath(t *testing.T) {
	store := newMockMessageStore()
	store.addEntry(message.ActorCoordinator, "WORKER.1", "Hello worker!")
	store.addEntry(message.ActorCoordinator, "WORKER.1", "Please start task")

	ws := NewWorkerServer("WORKER.1", store)
	handler := ws.handlers["check_messages"]

	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err, "Unexpected error")

	// Verify UnreadFor was called with correct worker ID
	require.Len(t, store.unreadForCalls, 1, "UnreadFor should be called once")
	require.Equal(t, "WORKER.1", store.unreadForCalls[0], "UnreadFor called with wrong worker ID")

	// Verify MarkRead was called
	require.Len(t, store.markReadCalls, 1, "MarkRead should be called once")
	require.Equal(t, "WORKER.1", store.markReadCalls[0], "MarkRead called with wrong worker ID")

	// Verify result contains message count
	require.NotNil(t, result, "Expected result with content")
	require.NotEmpty(t, result.Content, "Expected result with content")
	text := result.Content[0].Text

	// Parse JSON response
	var response checkMessagesResponse
	require.NoError(t, json.Unmarshal([]byte(text), &response), "Failed to parse JSON response")

	require.Equal(t, 2, response.UnreadCount, "Expected unread_count=2")
	require.Len(t, response.Messages, 2, "Expected 2 messages")
	require.Equal(t, "Hello worker!", response.Messages[0].Content, "First message content mismatch")
}

// TestWorkerServer_CheckMessagesNoMessages tests when there are no unread messages.
func TestWorkerServer_CheckMessagesNoMessages(t *testing.T) {
	store := newMockMessageStore()
	ws := NewWorkerServer("WORKER.1", store)
	handler := ws.handlers["check_messages"]

	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err, "Unexpected error")

	require.NotNil(t, result, "Expected result with content")
	require.NotEmpty(t, result.Content, "Expected result with content")
	text := result.Content[0].Text

	// Parse JSON response
	var response checkMessagesResponse
	require.NoError(t, json.Unmarshal([]byte(text), &response), "Failed to parse JSON response")

	require.Equal(t, 0, response.UnreadCount, "Expected unread_count=0")
	require.Empty(t, response.Messages, "Expected 0 messages")
}

// TestWorkerServer_CheckMessagesSeesAllMessages tests that workers see all messages.
func TestWorkerServer_CheckMessagesSeesAllMessages(t *testing.T) {
	store := newMockMessageStore()
	// Messages for different workers
	store.addEntry(message.ActorCoordinator, "WORKER.1", "For worker 1")
	store.addEntry(message.ActorCoordinator, "WORKER.2", "For worker 2")
	store.addEntry(message.ActorCoordinator, message.ActorAll, "For everyone")

	ws := NewWorkerServer("WORKER.1", store)
	handler := ws.handlers["check_messages"]

	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err, "Unexpected error")

	text := result.Content[0].Text

	// Parse JSON response
	var response checkMessagesResponse
	require.NoError(t, json.Unmarshal([]byte(text), &response), "Failed to parse JSON response")

	// Workers see ALL messages (no filtering by recipient)
	require.Equal(t, 3, response.UnreadCount, "Expected 3 messages")

	contents := make(map[string]bool)
	for _, msg := range response.Messages {
		contents[msg.Content] = true
	}

	require.True(t, contents["For worker 1"], "Should contain message addressed to WORKER.1")
	require.True(t, contents["For everyone"], "Should contain message addressed to ALL")
	require.True(t, contents["For worker 2"], "Should contain message addressed to WORKER.2 (workers see all messages)")
}

// TestWorkerServer_CheckMessagesReadTracking tests that messages are marked as read.
func TestWorkerServer_CheckMessagesReadTracking(t *testing.T) {
	store := newMockMessageStore()
	store.addEntry(message.ActorCoordinator, "WORKER.1", "First message")

	ws := NewWorkerServer("WORKER.1", store)
	handler := ws.handlers["check_messages"]

	// First call should return the message
	result1, _ := handler(context.Background(), json.RawMessage(`{}`))
	var response1 checkMessagesResponse
	require.NoError(t, json.Unmarshal([]byte(result1.Content[0].Text), &response1), "Failed to parse JSON response")
	require.Equal(t, 1, response1.UnreadCount, "First call should return 1 message")
	require.Equal(t, "First message", response1.Messages[0].Content, "First call should return the message")

	// Second call should return no new messages
	result2, _ := handler(context.Background(), json.RawMessage(`{}`))
	var response2 checkMessagesResponse
	require.NoError(t, json.Unmarshal([]byte(result2.Content[0].Text), &response2), "Failed to parse JSON response")
	require.Equal(t, 0, response2.UnreadCount, "Second call should return 0 unread messages")

	// Add a new message
	store.addEntry(message.ActorCoordinator, "WORKER.1", "Second message")

	// Third call should return only the new message
	result3, _ := handler(context.Background(), json.RawMessage(`{}`))
	var response3 checkMessagesResponse
	require.NoError(t, json.Unmarshal([]byte(result3.Content[0].Text), &response3), "Failed to parse JSON response")
	require.Equal(t, 1, response3.UnreadCount, "Third call should return 1 new message")
	require.Equal(t, "Second message", response3.Messages[0].Content, "Third call should return the new message")
}

// TestWorkerServer_SendMessageValidation tests input validation for post_message.
func TestWorkerServer_SendMessageValidation(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)
	handler := ws.handlers["post_message"]

	tests := []struct {
		name    string
		args    string
		wantErr string
	}{
		{
			name:    "missing to",
			args:    `{"content": "hello"}`,
			wantErr: "to is required",
		},
		{
			name:    "missing content",
			args:    `{"to": "COORDINATOR"}`,
			wantErr: "content is required",
		},
		{
			name:    "empty to",
			args:    `{"to": "", "content": "hello"}`,
			wantErr: "to is required",
		},
		{
			name:    "empty content",
			args:    `{"to": "COORDINATOR", "content": ""}`,
			wantErr: "content is required",
		},
		{
			name:    "message store not available",
			args:    `{"to": "COORDINATOR", "content": "hello"}`,
			wantErr: "message store not available",
		},
		{
			name:    "invalid json",
			args:    `not json`,
			wantErr: "invalid arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler(context.Background(), json.RawMessage(tt.args))
			require.Error(t, err, "Expected error but got none")
			require.Contains(t, err.Error(), tt.wantErr, "Error should contain expected message")
		})
	}
}

// TestWorkerServer_SendMessageHappyPath tests successful message sending.
func TestWorkerServer_SendMessageHappyPath(t *testing.T) {
	store := newMockMessageStore()
	ws := NewWorkerServer("WORKER.1", store)
	handler := ws.handlers["post_message"]

	result, err := handler(context.Background(), json.RawMessage(`{"to": "COORDINATOR", "content": "Task complete"}`))
	require.NoError(t, err, "Unexpected error")

	// Verify Append was called with correct parameters
	require.Len(t, store.appendCalls, 1, "Expected 1 append call")
	call := store.appendCalls[0]
	require.Equal(t, "WORKER.1", call.From, "From mismatch")
	require.Equal(t, "COORDINATOR", call.To, "To mismatch")
	require.Equal(t, "Task complete", call.Content, "Content mismatch")
	require.Equal(t, message.MessageInfo, call.Type, "Type mismatch")

	// Verify success result
	require.Contains(t, result.Content[0].Text, "Message sent to COORDINATOR", "Result should confirm sending")
}

// TestWorkerServer_SignalReadyValidation tests signal_ready with v2 adapter.
// In v2 architecture, signal_ready posts to the v2 adapter's message log (if configured).
func TestWorkerServer_SignalReadyValidation(t *testing.T) {
	tws := NewTestWorkerServer(t, "WORKER.1", nil)
	defer tws.Close()
	handler := tws.handlers["signal_ready"]

	// signal_ready always returns success
	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err, "signal_ready should not error")
	require.Contains(t, result.Content[0].Text, "ready signal acknowledged", "Result should confirm signal")
}

// TestWorkerServer_SignalReadyHappyPath tests successful ready signaling with v2.
// In v2 architecture, signal_ready posts to the v2 adapter's message log, not the worker's message store.
func TestWorkerServer_SignalReadyHappyPath(t *testing.T) {
	store := newMockMessageStore()
	tws := NewTestWorkerServer(t, "WORKER.1", store)
	defer tws.Close()
	handler := tws.handlers["signal_ready"]

	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err, "Unexpected error")

	// signal_ready posts to v2 adapter's message log, not to worker's message store
	require.Len(t, store.appendCalls, 0, "signal_ready posts to v2 message log, not worker store")

	// Verify success result
	require.Contains(t, result.Content[0].Text, "ready signal acknowledged", "Result should confirm signal")
}

// TestWorkerServer_ToolDescriptionsAreHelpful verifies tool descriptions are informative.
func TestWorkerServer_ToolDescriptionsAreHelpful(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	tests := []struct {
		toolName      string
		mustContain   []string
		descMinLength int
	}{
		{
			toolName:      "check_messages",
			mustContain:   []string{"message", "unread"},
			descMinLength: 30,
		},
		{
			toolName:      "post_message",
			mustContain:   []string{"message", "coordinator"},
			descMinLength: 30,
		},
		{
			toolName:      "signal_ready",
			mustContain:   []string{"ready", "task", "assignment"},
			descMinLength: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			tool := ws.tools[tt.toolName]
			desc := strings.ToLower(tool.Description)

			require.GreaterOrEqual(t, len(tool.Description), tt.descMinLength, "Description too short: want at least %d chars", tt.descMinLength)

			for _, keyword := range tt.mustContain {
				require.Contains(t, desc, keyword, "Description should contain %q", keyword)
			}
		})
	}
}

// TestWorkerServer_InstructionsContainToolNames verifies instructions mention all tools.
func TestWorkerServer_InstructionsContainToolNames(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)
	instructions := strings.ToLower(ws.instructions)

	toolNames := []string{"check_messages", "post_message", "signal_ready"}
	for _, name := range toolNames {
		require.Contains(t, instructions, name, "Instructions should mention %q", name)
	}
}

// TestWorkerServer_CheckMessagesSchema verifies check_messages tool schema.
func TestWorkerServer_CheckMessagesSchema(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	tool, ok := ws.tools["check_messages"]
	require.True(t, ok, "check_messages tool not registered")

	require.Empty(t, tool.InputSchema.Required, "check_messages should not have required parameters")
}

// TestWorkerServer_SendMessageSchema verifies post_message tool schema.
func TestWorkerServer_SendMessageSchema(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	tool, ok := ws.tools["post_message"]
	require.True(t, ok, "post_message tool not registered")

	require.Len(t, tool.InputSchema.Required, 2, "post_message should have 2 required parameters")

	requiredSet := make(map[string]bool)
	for _, r := range tool.InputSchema.Required {
		requiredSet[r] = true
	}
	require.True(t, requiredSet["to"], "'to' should be required")
	require.True(t, requiredSet["content"], "'content' should be required")
}

// TestWorkerServer_SignalReadySchema verifies signal_ready tool schema.
func TestWorkerServer_SignalReadySchema(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	tool, ok := ws.tools["signal_ready"]
	require.True(t, ok, "signal_ready tool not registered")

	require.Empty(t, tool.InputSchema.Required, "signal_ready should have 0 required parameters")
	require.Empty(t, tool.InputSchema.Properties, "signal_ready should have 0 properties")
}

// TestWorkerServer_ReportImplementationComplete_SubmitsCommand tests command submission in v2.
// In v2 architecture, report_implementation_complete submits a command to the processor,
// not through the callback mechanism.
func TestWorkerServer_ReportImplementationComplete_SubmitsCommand(t *testing.T) {
	store := newMockMessageStore()
	tws := NewTestWorkerServer(t, "WORKER.1", store)
	defer tws.Close()
	handler := tws.handlers["report_implementation_complete"]

	// Configure mock to return success
	tws.V2Handler.SetResult(&command.CommandResult{
		Success: true,
		Data:    "Implementation complete",
	})

	result, err := handler(context.Background(), json.RawMessage(`{"summary": "completed feature X"}`))
	require.NoError(t, err, "Expected no error with v2 adapter")
	require.NotNil(t, result)
	require.False(t, result.IsError, "Expected success result")

	// Verify command was submitted
	commands := tws.V2Handler.GetCommands()
	require.Len(t, commands, 1, "Expected 1 command")
	require.Equal(t, command.CmdReportComplete, commands[0].Type(), "Expected ReportComplete command")
}

// TestWorkerServer_ReportImplementationComplete_EmptySummary tests that empty summary is accepted in v2.
// In v2 architecture, summary is optional (empty string is valid).
func TestWorkerServer_ReportImplementationComplete_EmptySummary(t *testing.T) {
	store := newMockMessageStore()
	tws := NewTestWorkerServer(t, "WORKER.1", store)
	defer tws.Close()
	handler := tws.handlers["report_implementation_complete"]

	// Configure mock to return success
	tws.V2Handler.SetResult(&command.CommandResult{
		Success: true,
		Data:    "Implementation complete",
	})

	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err, "Empty summary should not error in v2")
	require.NotNil(t, result)
	require.False(t, result.IsError, "Expected success result")
}

// TestWorkerServer_ReportImplementationComplete_ProcessorRejectsWrongPhase tests that processor validates phase.
// In v2 architecture, phase validation happens in the processor, not the MCP handler.
func TestWorkerServer_ReportImplementationComplete_ProcessorRejectsWrongPhase(t *testing.T) {
	store := newMockMessageStore()
	tws := NewTestWorkerServer(t, "WORKER.1", store)
	defer tws.Close()
	handler := tws.handlers["report_implementation_complete"]

	// Configure mock to return error (simulating processor rejecting wrong phase)
	tws.V2Handler.SetResult(&command.CommandResult{
		Success: false,
		Error:   fmt.Errorf("worker not in implementing or addressing_feedback phase"),
	})

	result, err := handler(context.Background(), json.RawMessage(`{"summary": "done"}`))
	require.NoError(t, err, "Handler returns nil error")
	require.NotNil(t, result)
	require.True(t, result.IsError, "Expected error result from processor")
	require.Contains(t, result.Content[0].Text, "not in implementing or addressing_feedback phase", "Expected phase error")
}

// TestWorkerServer_ReportImplementationComplete_HappyPath tests successful completion in v2.
func TestWorkerServer_ReportImplementationComplete_HappyPath(t *testing.T) {
	store := newMockMessageStore()
	tws := NewTestWorkerServer(t, "WORKER.1", store)
	defer tws.Close()
	handler := tws.handlers["report_implementation_complete"]

	// Configure mock to return success
	tws.V2Handler.SetResult(&command.CommandResult{
		Success: true,
		Data:    "Implementation complete",
	})

	result, err := handler(context.Background(), json.RawMessage(`{"summary": "Added feature X with tests"}`))
	require.NoError(t, err, "Unexpected error")

	// Verify command was submitted
	commands := tws.V2Handler.GetCommands()
	require.Len(t, commands, 1, "Expected 1 command")
	require.Equal(t, command.CmdReportComplete, commands[0].Type(), "Expected ReportComplete command")

	// Verify success result
	require.NotNil(t, result, "Expected result with content")
	require.NotEmpty(t, result.Content, "Expected result with content")
	require.False(t, result.IsError, "Expected success result")
}

// TestWorkerServer_ReportImplementationComplete_AddressingFeedback tests completion from addressing_feedback phase in v2.
func TestWorkerServer_ReportImplementationComplete_AddressingFeedback(t *testing.T) {
	store := newMockMessageStore()
	tws := NewTestWorkerServer(t, "WORKER.1", store)
	defer tws.Close()
	handler := tws.handlers["report_implementation_complete"]

	// Configure mock to return success
	tws.V2Handler.SetResult(&command.CommandResult{
		Success: true,
		Data:    "Implementation complete",
	})

	result, err := handler(context.Background(), json.RawMessage(`{"summary": "Fixed review feedback"}`))
	require.NoError(t, err, "Should succeed in v2")
	require.NotNil(t, result)
	require.False(t, result.IsError, "Expected success result")
}

// TestWorkerServer_ReportReviewVerdict_SubmitsCommand tests command submission in v2.
func TestWorkerServer_ReportReviewVerdict_SubmitsCommand(t *testing.T) {
	store := newMockMessageStore()
	tws := NewTestWorkerServer(t, "WORKER.1", store)
	defer tws.Close()
	handler := tws.handlers["report_review_verdict"]

	// Configure mock to return success
	tws.V2Handler.SetResult(&command.CommandResult{
		Success: true,
		Data:    "Verdict submitted",
	})

	result, err := handler(context.Background(), json.RawMessage(`{"verdict": "APPROVED", "comments": "LGTM"}`))
	require.NoError(t, err, "Expected no error with v2 adapter")
	require.NotNil(t, result)
	require.False(t, result.IsError, "Expected success result")

	// Verify command was submitted
	commands := tws.V2Handler.GetCommands()
	require.Len(t, commands, 1, "Expected 1 command")
	require.Equal(t, command.CmdReportVerdict, commands[0].Type(), "Expected ReportVerdict command")
}

// TestWorkerServer_ReportReviewVerdict_MissingVerdict tests validation in v2.
func TestWorkerServer_ReportReviewVerdict_MissingVerdict(t *testing.T) {
	store := newMockMessageStore()
	tws := NewTestWorkerServer(t, "WORKER.1", store)
	defer tws.Close()
	handler := tws.handlers["report_review_verdict"]

	// v2 adapter validates verdict is required before submitting to processor
	_, err := handler(context.Background(), json.RawMessage(`{"comments": "LGTM"}`))
	require.Error(t, err, "Expected error for missing verdict")
	require.Contains(t, err.Error(), "verdict is required", "Expected 'verdict is required' error")
}

// TestWorkerServer_ReportReviewVerdict_EmptyComments tests that empty comments are valid in v2.
// In v2 architecture, comments are optional.
func TestWorkerServer_ReportReviewVerdict_EmptyComments(t *testing.T) {
	store := newMockMessageStore()
	tws := NewTestWorkerServer(t, "WORKER.1", store)
	defer tws.Close()
	handler := tws.handlers["report_review_verdict"]

	// Configure mock to return success
	tws.V2Handler.SetResult(&command.CommandResult{
		Success: true,
		Data:    "Verdict submitted",
	})

	result, err := handler(context.Background(), json.RawMessage(`{"verdict": "APPROVED"}`))
	require.NoError(t, err, "Empty comments should not error in v2")
	require.NotNil(t, result)
	require.False(t, result.IsError, "Expected success result")
}

// TestWorkerServer_ReportReviewVerdict_InvalidVerdict tests invalid verdict value in v2.
func TestWorkerServer_ReportReviewVerdict_InvalidVerdict(t *testing.T) {
	store := newMockMessageStore()
	tws := NewTestWorkerServer(t, "WORKER.1", store)
	defer tws.Close()
	handler := tws.handlers["report_review_verdict"]

	// v2 adapter validates verdict value
	_, err := handler(context.Background(), json.RawMessage(`{"verdict": "MAYBE", "comments": "Not sure"}`))
	require.Error(t, err, "Expected error for invalid verdict")
	require.Contains(t, err.Error(), "must be APPROVED or DENIED", "Expected verdict validation error")
}

// TestWorkerServer_ReportReviewVerdict_ProcessorRejectsWrongPhase tests that processor validates phase.
// In v2 architecture, phase validation happens in the processor, not the MCP handler.
func TestWorkerServer_ReportReviewVerdict_ProcessorRejectsWrongPhase(t *testing.T) {
	store := newMockMessageStore()
	tws := NewTestWorkerServer(t, "WORKER.1", store)
	defer tws.Close()
	handler := tws.handlers["report_review_verdict"]

	// Configure mock to return error (simulating processor rejecting wrong phase)
	tws.V2Handler.SetResult(&command.CommandResult{
		Success: false,
		Error:   fmt.Errorf("worker not in reviewing phase"),
	})

	result, err := handler(context.Background(), json.RawMessage(`{"verdict": "APPROVED", "comments": "LGTM"}`))
	require.NoError(t, err, "Handler returns nil error")
	require.NotNil(t, result)
	require.True(t, result.IsError, "Expected error result from processor")
	require.Contains(t, result.Content[0].Text, "not in reviewing phase", "Expected phase error")
}

// TestWorkerServer_ReportReviewVerdict_Approved tests successful approval in v2.
func TestWorkerServer_ReportReviewVerdict_Approved(t *testing.T) {
	store := newMockMessageStore()
	tws := NewTestWorkerServer(t, "WORKER.1", store)
	defer tws.Close()
	handler := tws.handlers["report_review_verdict"]

	// Configure mock to return success
	tws.V2Handler.SetResult(&command.CommandResult{
		Success: true,
		Data:    "Review verdict APPROVED submitted",
	})

	result, err := handler(context.Background(), json.RawMessage(`{"verdict": "APPROVED", "comments": "Code looks great, tests pass"}`))
	require.NoError(t, err, "Unexpected error")

	// Verify command was submitted
	commands := tws.V2Handler.GetCommands()
	require.Len(t, commands, 1, "Expected 1 command")
	require.Equal(t, command.CmdReportVerdict, commands[0].Type(), "Expected ReportVerdict command")

	// Verify success result
	require.NotNil(t, result, "Expected result with content")
	require.NotEmpty(t, result.Content, "Expected result with content")
	require.False(t, result.IsError, "Expected success result")
	require.Contains(t, result.Content[0].Text, "APPROVED", "Response should contain 'APPROVED'")
}

// TestWorkerServer_ReportReviewVerdict_Denied tests successful denial in v2.
func TestWorkerServer_ReportReviewVerdict_Denied(t *testing.T) {
	store := newMockMessageStore()
	tws := NewTestWorkerServer(t, "WORKER.1", store)
	defer tws.Close()
	handler := tws.handlers["report_review_verdict"]

	// Configure mock to return success
	tws.V2Handler.SetResult(&command.CommandResult{
		Success: true,
		Data:    "Review verdict DENIED submitted",
	})

	result, err := handler(context.Background(), json.RawMessage(`{"verdict": "DENIED", "comments": "Missing error handling in line 50"}`))
	require.NoError(t, err, "Unexpected error")

	// Verify command was submitted
	commands := tws.V2Handler.GetCommands()
	require.Len(t, commands, 1, "Expected 1 command")
	require.Equal(t, command.CmdReportVerdict, commands[0].Type(), "Expected ReportVerdict command")

	// Verify success result contains DENIED
	require.NotNil(t, result)
	require.False(t, result.IsError, "Expected success result")
	require.Contains(t, result.Content[0].Text, "DENIED", "Response should contain 'DENIED'")
}

// TestWorkerServer_ReportImplementationCompleteSchema verifies tool schema.
func TestWorkerServer_ReportImplementationCompleteSchema(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	tool, ok := ws.tools["report_implementation_complete"]
	require.True(t, ok, "report_implementation_complete tool not registered")

	require.Len(t, tool.InputSchema.Required, 1, "report_implementation_complete should have 1 required parameter")
	require.Equal(t, "summary", tool.InputSchema.Required[0], "Required parameter should be 'summary'")

	_, ok = tool.InputSchema.Properties["summary"]
	require.True(t, ok, "'summary' property should be defined")
}

// TestWorkerServer_ReportReviewVerdictSchema verifies tool schema.
func TestWorkerServer_ReportReviewVerdictSchema(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	tool, ok := ws.tools["report_review_verdict"]
	require.True(t, ok, "report_review_verdict tool not registered")

	require.Len(t, tool.InputSchema.Required, 2, "report_review_verdict should have 2 required parameters")

	requiredSet := make(map[string]bool)
	for _, r := range tool.InputSchema.Required {
		requiredSet[r] = true
	}
	require.True(t, requiredSet["verdict"], "'verdict' should be required")
	require.True(t, requiredSet["comments"], "'comments' should be required")

	_, ok = tool.InputSchema.Properties["verdict"]
	require.True(t, ok, "'verdict' property should be defined")
	_, ok = tool.InputSchema.Properties["comments"]
	require.True(t, ok, "'comments' property should be defined")
}

// ============================================================================
// Tests for validateReflectionArgs
// ============================================================================

// TestValidateReflectionArgs_ValidInput tests that valid args pass validation.
func TestValidateReflectionArgs_ValidInput(t *testing.T) {
	args := postReflectionsArgs{
		TaskID:    "perles-abc123",
		Summary:   "Implemented feature X with comprehensive tests and documentation.",
		Insights:  "Found that approach Y works well for this use case.",
		Mistakes:  "Initially forgot to handle edge case Z.",
		Learnings: "The foo module requires careful handling of bar.",
	}

	err := validateReflectionArgs(args)
	require.NoError(t, err, "Valid input should pass validation")
}

// TestValidateReflectionArgs_EmptyTaskID tests that empty task_id is rejected.
func TestValidateReflectionArgs_EmptyTaskID(t *testing.T) {
	args := postReflectionsArgs{
		TaskID:  "",
		Summary: "A valid summary that is at least twenty chars.",
	}

	err := validateReflectionArgs(args)
	require.Error(t, err, "Empty task_id should be rejected")
	require.Contains(t, err.Error(), "task_id is required")
}

// TestValidateReflectionArgs_InvalidTaskIDFormat tests that path traversal is rejected.
func TestValidateReflectionArgs_InvalidTaskIDFormat(t *testing.T) {
	tests := []struct {
		name    string
		taskID  string
		wantErr string
	}{
		{
			name:    "path traversal with ..",
			taskID:  "../../etc/passwd",
			wantErr: "path traversal characters",
		},
		{
			name:    "path with forward slash",
			taskID:  "task/id",
			wantErr: "path traversal characters",
		},
		{
			name:    "double dots in middle",
			taskID:  "task..id",
			wantErr: "path traversal characters",
		},
		{
			name:    "invalid format - no hyphen",
			taskID:  "invalidtaskid",
			wantErr: "invalid task_id format",
		},
		{
			name:    "invalid format - special chars",
			taskID:  "task-@#$%",
			wantErr: "invalid task_id format",
		},
		{
			name:    "too short suffix",
			taskID:  "t-a",
			wantErr: "invalid task_id format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := postReflectionsArgs{
				TaskID:  tt.taskID,
				Summary: "A valid summary that is at least twenty chars.",
			}
			err := validateReflectionArgs(args)
			require.Error(t, err, "Invalid task_id %q should be rejected", tt.taskID)
			require.Contains(t, err.Error(), tt.wantErr, "Error should mention expected issue")
		})
	}
}

// TestValidateReflectionArgs_ValidTaskIDFormats tests various valid task ID formats.
func TestValidateReflectionArgs_ValidTaskIDFormats(t *testing.T) {
	validTaskIDs := []string{
		"perles-abc123",
		"ms-e52",
		"task-abc",
		"bd-12345",
		"perles-s157",
		"perles-s157.1",
		"ms-abc123.42",
	}

	for _, taskID := range validTaskIDs {
		t.Run(taskID, func(t *testing.T) {
			args := postReflectionsArgs{
				TaskID:  taskID,
				Summary: "A valid summary that is at least twenty chars.",
			}
			err := validateReflectionArgs(args)
			require.NoError(t, err, "Valid task_id %q should pass validation", taskID)
		})
	}
}

// TestValidateReflectionArgs_SummaryTooShort tests that summary < 20 chars is rejected.
func TestValidateReflectionArgs_SummaryTooShort(t *testing.T) {
	args := postReflectionsArgs{
		TaskID:  "perles-abc123",
		Summary: "Too short",
	}

	err := validateReflectionArgs(args)
	require.Error(t, err, "Summary too short should be rejected")
	require.Contains(t, err.Error(), "summary too short")
	require.Contains(t, err.Error(), "min 20 chars")
}

// TestValidateReflectionArgs_ExactlyMinSummaryLength tests boundary at exactly 20 chars.
func TestValidateReflectionArgs_ExactlyMinSummaryLength(t *testing.T) {
	args := postReflectionsArgs{
		TaskID:  "perles-abc123",
		Summary: strings.Repeat("x", MinSummaryLength), // Exactly 20 chars
	}

	err := validateReflectionArgs(args)
	require.NoError(t, err, "Summary with exactly min length should pass")
}

// TestValidateReflectionArgs_EmptySummary tests that empty summary is rejected.
func TestValidateReflectionArgs_EmptySummary(t *testing.T) {
	args := postReflectionsArgs{
		TaskID:  "perles-abc123",
		Summary: "",
	}

	err := validateReflectionArgs(args)
	require.Error(t, err, "Empty summary should be rejected")
	require.Contains(t, err.Error(), "summary is required")
}

// ============================================================================
// Tests for buildReflectionMarkdown
// ============================================================================

// TestBuildReflectionMarkdown_AllFields tests markdown generation with all fields provided.
func TestBuildReflectionMarkdown_AllFields(t *testing.T) {
	args := postReflectionsArgs{
		TaskID:    "perles-abc123",
		Summary:   "Implemented user validation with regex patterns.",
		Insights:  "Pre-compiled regex is 10x faster.",
		Mistakes:  "Initially forgot to handle empty strings.",
		Learnings: "Always validate input at boundaries.",
	}

	md := buildReflectionMarkdown("WORKER.1", args)

	// Verify header
	assert.Contains(t, md, "# Worker Reflection")
	assert.Contains(t, md, "**Worker:** WORKER.1")
	assert.Contains(t, md, "**Task:** perles-abc123")
	assert.Contains(t, md, "**Date:**")

	// Verify all sections are present
	assert.Contains(t, md, "## Summary")
	assert.Contains(t, md, "Implemented user validation with regex patterns.")

	assert.Contains(t, md, "## Insights")
	assert.Contains(t, md, "Pre-compiled regex is 10x faster.")

	assert.Contains(t, md, "## Mistakes & Lessons")
	assert.Contains(t, md, "Initially forgot to handle empty strings.")

	assert.Contains(t, md, "## Learnings")
	assert.Contains(t, md, "Always validate input at boundaries.")
}

// TestBuildReflectionMarkdown_OnlySummary tests markdown generation with only required fields.
func TestBuildReflectionMarkdown_OnlySummary(t *testing.T) {
	args := postReflectionsArgs{
		TaskID:  "ms-e52",
		Summary: "Fixed a critical bug in authentication flow.",
	}

	md := buildReflectionMarkdown("WORKER.2", args)

	// Verify header
	assert.Contains(t, md, "# Worker Reflection")
	assert.Contains(t, md, "**Worker:** WORKER.2")
	assert.Contains(t, md, "**Task:** ms-e52")

	// Verify summary is present
	assert.Contains(t, md, "## Summary")
	assert.Contains(t, md, "Fixed a critical bug in authentication flow.")

	// Verify optional sections are NOT present
	assert.NotContains(t, md, "## Insights")
	assert.NotContains(t, md, "## Mistakes & Lessons")
	assert.NotContains(t, md, "## Learnings")
}

// TestBuildReflectionMarkdown_PartialOptionalFields tests with some optional fields.
func TestBuildReflectionMarkdown_PartialOptionalFields(t *testing.T) {
	tests := []struct {
		name       string
		args       postReflectionsArgs
		shouldHave []string
		shouldNot  []string
	}{
		{
			name: "only insights",
			args: postReflectionsArgs{
				TaskID:   "task-abc",
				Summary:  "Completed the refactoring.",
				Insights: "The new pattern is cleaner.",
			},
			shouldHave: []string{"## Summary", "## Insights"},
			shouldNot:  []string{"## Mistakes & Lessons", "## Learnings"},
		},
		{
			name: "only mistakes",
			args: postReflectionsArgs{
				TaskID:   "task-abc",
				Summary:  "Completed the refactoring.",
				Mistakes: "Broke tests initially.",
			},
			shouldHave: []string{"## Summary", "## Mistakes & Lessons"},
			shouldNot:  []string{"## Insights", "## Learnings"},
		},
		{
			name: "only learnings",
			args: postReflectionsArgs{
				TaskID:    "task-abc",
				Summary:   "Completed the refactoring.",
				Learnings: "Read the docs first.",
			},
			shouldHave: []string{"## Summary", "## Learnings"},
			shouldNot:  []string{"## Insights", "## Mistakes & Lessons"},
		},
		{
			name: "insights and learnings",
			args: postReflectionsArgs{
				TaskID:    "task-abc",
				Summary:   "Completed the refactoring.",
				Insights:  "Works well.",
				Learnings: "Good to know.",
			},
			shouldHave: []string{"## Summary", "## Insights", "## Learnings"},
			shouldNot:  []string{"## Mistakes & Lessons"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := buildReflectionMarkdown("WORKER.1", tt.args)

			for _, s := range tt.shouldHave {
				assert.Contains(t, md, s, "Should contain %q", s)
			}
			for _, s := range tt.shouldNot {
				assert.NotContains(t, md, s, "Should NOT contain %q", s)
			}
		})
	}
}

// TestBuildReflectionMarkdown_DateFormat tests that date is in expected format.
func TestBuildReflectionMarkdown_DateFormat(t *testing.T) {
	args := postReflectionsArgs{
		TaskID:  "perles-abc",
		Summary: "Test summary for date format.",
	}

	md := buildReflectionMarkdown("WORKER.1", args)

	// Date format should be YYYY-MM-DD HH:MM:SS (e.g., 2025-12-30 01:23:45)
	// We can't check exact time, but we can verify the format pattern exists
	assert.Regexp(t, `\*\*Date:\*\* \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}`, md, "Date should be in expected format")
}

// TestBuildReflectionMarkdown_PreservesNewlines tests that content with newlines is preserved.
func TestBuildReflectionMarkdown_PreservesNewlines(t *testing.T) {
	args := postReflectionsArgs{
		TaskID:  "task-abc",
		Summary: "Line 1\nLine 2\nLine 3",
	}

	md := buildReflectionMarkdown("WORKER.1", args)

	assert.Contains(t, md, "Line 1\nLine 2\nLine 3", "Newlines in content should be preserved")
}

// ============================================================================
// Tests for handlePostReflections
// ============================================================================

// mockReflectionWriter implements ReflectionWriter for testing.
type mockReflectionWriter struct {
	mu         sync.Mutex
	calls      []reflectionWriterCall
	returnPath string
	returnErr  error
}

type reflectionWriterCall struct {
	WorkerID string
	TaskID   string
	Content  []byte
}

func newMockReflectionWriter() *mockReflectionWriter {
	return &mockReflectionWriter{
		returnPath: "/mock/path/reflection.md",
	}
}

func (m *mockReflectionWriter) WriteWorkerReflection(workerID, taskID string, content []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, reflectionWriterCall{
		WorkerID: workerID,
		TaskID:   taskID,
		Content:  content,
	})
	return m.returnPath, m.returnErr
}

// TestHandlePostReflections_Success tests valid reflection saves and returns path.
func TestHandlePostReflections_Success(t *testing.T) {
	store := newMockMessageStore()
	writer := newMockReflectionWriter()
	writer.returnPath = "/sessions/abc/workers/WORKER.1/reflection.md"

	ws := NewWorkerServer("WORKER.1", store)
	ws.SetReflectionWriter(writer)
	handler := ws.handlers["post_reflections"]

	args := `{
		"task_id": "perles-abc123",
		"summary": "Implemented feature X with comprehensive tests.",
		"insights": "Found that pattern Y works well.",
		"mistakes": "Initially forgot edge case Z.",
		"learnings": "Always check for nil before dereferencing."
	}`

	result, err := handler(context.Background(), json.RawMessage(args))
	require.NoError(t, err, "Unexpected error")

	// Verify writer was called
	require.Len(t, writer.calls, 1, "Expected 1 write call")
	require.Equal(t, "WORKER.1", writer.calls[0].WorkerID, "WorkerID mismatch")
	require.Equal(t, "perles-abc123", writer.calls[0].TaskID, "TaskID mismatch")
	require.Contains(t, string(writer.calls[0].Content), "# Worker Reflection", "Content should be markdown")
	require.Contains(t, string(writer.calls[0].Content), "Implemented feature X", "Content should contain summary")

	// Verify structured response
	require.NotNil(t, result, "Expected result with content")
	require.NotEmpty(t, result.Content, "Expected result with content")
	text := result.Content[0].Text
	require.Contains(t, text, `"status"`, "Response should contain status")
	require.Contains(t, text, `"file_path"`, "Response should contain file_path")
	require.Contains(t, text, `"success"`, "Status should be success")
	require.Contains(t, text, writer.returnPath, "Response should contain file path")
}

// TestHandlePostReflections_EmptyTaskID tests that missing task_id returns error.
func TestHandlePostReflections_EmptyTaskID(t *testing.T) {
	store := newMockMessageStore()
	writer := newMockReflectionWriter()

	ws := NewWorkerServer("WORKER.1", store)
	ws.SetReflectionWriter(writer)
	handler := ws.handlers["post_reflections"]

	args := `{
		"task_id": "",
		"summary": "A valid summary that is at least twenty chars."
	}`

	_, err := handler(context.Background(), json.RawMessage(args))
	require.Error(t, err, "Expected error for empty task_id")
	require.Contains(t, err.Error(), "task_id is required", "Error should mention task_id")
}

// TestHandlePostReflections_EmptySummary tests that missing summary returns error.
func TestHandlePostReflections_EmptySummary(t *testing.T) {
	store := newMockMessageStore()
	writer := newMockReflectionWriter()

	ws := NewWorkerServer("WORKER.1", store)
	ws.SetReflectionWriter(writer)
	handler := ws.handlers["post_reflections"]

	args := `{
		"task_id": "perles-abc123",
		"summary": ""
	}`

	_, err := handler(context.Background(), json.RawMessage(args))
	require.Error(t, err, "Expected error for empty summary")
	require.Contains(t, err.Error(), "summary is required", "Error should mention summary")
}

// TestHandlePostReflections_InvalidTaskID tests that path traversal is rejected.
func TestHandlePostReflections_InvalidTaskID(t *testing.T) {
	store := newMockMessageStore()
	writer := newMockReflectionWriter()

	ws := NewWorkerServer("WORKER.1", store)
	ws.SetReflectionWriter(writer)
	handler := ws.handlers["post_reflections"]

	args := `{
		"task_id": "../../etc/passwd",
		"summary": "A valid summary that is at least twenty chars."
	}`

	_, err := handler(context.Background(), json.RawMessage(args))
	require.Error(t, err, "Expected error for path traversal")
	require.Contains(t, err.Error(), "path traversal", "Error should mention path traversal")
}

// TestHandlePostReflections_SummaryTooShort tests validation for summary length.
func TestHandlePostReflections_SummaryTooShort(t *testing.T) {
	store := newMockMessageStore()
	writer := newMockReflectionWriter()

	ws := NewWorkerServer("WORKER.1", store)
	ws.SetReflectionWriter(writer)
	handler := ws.handlers["post_reflections"]

	args := `{
		"task_id": "perles-abc123",
		"summary": "Too short"
	}`

	_, err := handler(context.Background(), json.RawMessage(args))
	require.Error(t, err, "Expected error for summary too short")
	require.Contains(t, err.Error(), "summary too short", "Error should mention summary too short")
}

// TestHandlePostReflections_NilReflectionWriter tests graceful error when writer not configured.
func TestHandlePostReflections_NilReflectionWriter(t *testing.T) {
	store := newMockMessageStore()

	ws := NewWorkerServer("WORKER.1", store)
	// Don't set reflection writer - leave it nil
	handler := ws.handlers["post_reflections"]

	args := `{
		"task_id": "perles-abc123",
		"summary": "A valid summary that is at least twenty chars."
	}`

	_, err := handler(context.Background(), json.RawMessage(args))
	require.Error(t, err, "Expected error for nil reflection writer")
	require.Contains(t, err.Error(), "reflection writer not configured", "Error should mention reflection writer")
}

// TestHandlePostReflections_WriterError tests that writer errors are propagated.
func TestHandlePostReflections_WriterError(t *testing.T) {
	store := newMockMessageStore()
	writer := newMockReflectionWriter()
	writer.returnErr = fmt.Errorf("disk full")

	ws := NewWorkerServer("WORKER.1", store)
	ws.SetReflectionWriter(writer)
	handler := ws.handlers["post_reflections"]

	args := `{
		"task_id": "perles-abc123",
		"summary": "A valid summary that is at least twenty chars."
	}`

	_, err := handler(context.Background(), json.RawMessage(args))
	require.Error(t, err, "Expected error when writer fails")
	require.Contains(t, err.Error(), "failed to save reflection", "Error should mention save failure")
	require.Contains(t, err.Error(), "disk full", "Error should contain underlying error")
}

// TestHandlePostReflections_InvalidJSON tests that invalid JSON returns error.
func TestHandlePostReflections_InvalidJSON(t *testing.T) {
	store := newMockMessageStore()
	writer := newMockReflectionWriter()

	ws := NewWorkerServer("WORKER.1", store)
	ws.SetReflectionWriter(writer)
	handler := ws.handlers["post_reflections"]

	_, err := handler(context.Background(), json.RawMessage(`not json`))
	require.Error(t, err, "Expected error for invalid JSON")
	require.Contains(t, err.Error(), "invalid arguments", "Error should mention invalid arguments")
}

// TestHandlePostReflections_OnlyRequiredFields tests success with only required fields.
func TestHandlePostReflections_OnlyRequiredFields(t *testing.T) {
	store := newMockMessageStore()
	writer := newMockReflectionWriter()

	ws := NewWorkerServer("WORKER.1", store)
	ws.SetReflectionWriter(writer)
	handler := ws.handlers["post_reflections"]

	args := `{
		"task_id": "perles-abc123",
		"summary": "A valid summary that is at least twenty chars."
	}`

	result, err := handler(context.Background(), json.RawMessage(args))
	require.NoError(t, err, "Should succeed with only required fields")

	// Verify content doesn't have optional sections
	content := string(writer.calls[0].Content)
	require.Contains(t, content, "## Summary", "Should have summary section")
	require.NotContains(t, content, "## Insights", "Should not have insights section")
	require.NotContains(t, content, "## Mistakes", "Should not have mistakes section")
	require.NotContains(t, content, "## Learnings", "Should not have learnings section")

	// Verify success response
	require.Contains(t, result.Content[0].Text, "success", "Response should indicate success")
}

// TestHandlePostReflections_NoMessageStore tests success even without message store.
func TestHandlePostReflections_NoMessageStore(t *testing.T) {
	writer := newMockReflectionWriter()

	ws := NewWorkerServer("WORKER.1", nil) // nil message store
	ws.SetReflectionWriter(writer)
	handler := ws.handlers["post_reflections"]

	args := `{
		"task_id": "perles-abc123",
		"summary": "A valid summary that is at least twenty chars."
	}`

	result, err := handler(context.Background(), json.RawMessage(args))
	require.NoError(t, err, "Should succeed even without message store")
	require.Contains(t, result.Content[0].Text, "success", "Response should indicate success")

	// Verify writer was still called
	require.Len(t, writer.calls, 1, "Writer should still be called")
}

// ============================================================================
// Tests for post_reflections tool registration
// ============================================================================

// TestPostReflectionsToolRegistered tests that post_reflections tool appears in registered tools.
func TestPostReflectionsToolRegistered(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	tool, ok := ws.tools["post_reflections"]
	require.True(t, ok, "post_reflections tool should be registered")

	// Verify tool metadata
	require.Equal(t, "post_reflections", tool.Name, "Tool name should be post_reflections")
	require.NotEmpty(t, tool.Description, "Tool should have description")
	require.Contains(t, strings.ToLower(tool.Description), "reflection", "Description should mention reflection")

	// Verify input schema
	require.NotNil(t, tool.InputSchema, "Tool should have input schema")
	require.Equal(t, "object", tool.InputSchema.Type, "InputSchema type should be object")

	// Verify required fields
	requiredSet := make(map[string]bool)
	for _, r := range tool.InputSchema.Required {
		requiredSet[r] = true
	}
	require.True(t, requiredSet["task_id"], "task_id should be required")
	require.True(t, requiredSet["summary"], "summary should be required")

	// Verify all properties exist
	_, hasTaskID := tool.InputSchema.Properties["task_id"]
	require.True(t, hasTaskID, "task_id property should be defined")
	_, hasSummary := tool.InputSchema.Properties["summary"]
	require.True(t, hasSummary, "summary property should be defined")
	_, hasInsights := tool.InputSchema.Properties["insights"]
	require.True(t, hasInsights, "insights property should be defined")
	_, hasMistakes := tool.InputSchema.Properties["mistakes"]
	require.True(t, hasMistakes, "mistakes property should be defined")
	_, hasLearnings := tool.InputSchema.Properties["learnings"]
	require.True(t, hasLearnings, "learnings property should be defined")

	// Verify output schema
	require.NotNil(t, tool.OutputSchema, "Tool should have output schema")
	_, hasStatus := tool.OutputSchema.Properties["status"]
	require.True(t, hasStatus, "status output property should be defined")
	_, hasFilePath := tool.OutputSchema.Properties["file_path"]
	require.True(t, hasFilePath, "file_path output property should be defined")
	_, hasMessage := tool.OutputSchema.Properties["message"]
	require.True(t, hasMessage, "message output property should be defined")
}

// TestPostReflectionsToolHandlerRegistered tests that handler is registered.
func TestPostReflectionsToolHandlerRegistered(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	_, ok := ws.handlers["post_reflections"]
	require.True(t, ok, "post_reflections handler should be registered")
}

// TestWorkerServer_RegistersAllToolsIncludingPostReflections verifies all 6 worker tools are registered.
func TestWorkerServer_RegistersAllToolsIncludingPostReflections(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	expectedTools := []string{
		"check_messages",
		"post_message",
		"signal_ready",
		"report_implementation_complete",
		"report_review_verdict",
		"post_reflections",
	}

	for _, toolName := range expectedTools {
		_, ok := ws.tools[toolName]
		require.True(t, ok, "Tool %q not registered", toolName)
		_, ok = ws.handlers[toolName]
		require.True(t, ok, "Handler for %q not registered", toolName)
	}

	require.Equal(t, len(expectedTools), len(ws.tools), "Tool count mismatch")
}
