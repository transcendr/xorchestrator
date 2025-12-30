package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/v2/adapter"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/orchestration/validation"
)

// Validation constants for post_reflections tool.
const (
	// MinSummaryLength is the minimum length for a reflection summary (at least a sentence).
	MinSummaryLength = 20
)

// MessageStore defines the interface for message storage operations.
// This allows for dependency injection and easier testing.
// Note: This interface is a subset of repository.MessageRepository.
// MemoryMessageRepository satisfies this interface, verified by compile-time assertion below.
type MessageStore interface {
	UnreadFor(agentID string) []message.Entry
	MarkRead(agentID string)
	Append(from, to, content string, msgType message.MessageType) (*message.Entry, error)
}

// Compile-time interface assertion: MemoryMessageRepository satisfies MessageStore.
// This ensures compatibility with the v2 MessageRepository migration.
var _ MessageStore = (*repository.MemoryMessageRepository)(nil)

// ReflectionWriter defines the interface for writing worker reflections.
// This allows the session service to handle storage without tight coupling.
type ReflectionWriter interface {
	// WriteWorkerReflection saves a worker's reflection markdown to their session directory.
	// Returns the file path where the reflection was saved.
	WriteWorkerReflection(workerID, taskID string, content []byte) (string, error)
}

// WorkerServer is an MCP server that exposes communication tools to worker agents.
// Each worker gets its own MCP server instance with a unique worker ID.
type WorkerServer struct {
	*Server
	workerID         string
	msgStore         MessageStore
	reflectionWriter ReflectionWriter
	// dedup tracks recent messages to prevent duplicate sends to coordinator
	dedup *MessageDeduplicator

	// V2 adapter for command-based processing
	// See docs/proposals/orchestration-v2-architecture.md for architecture details
	v2Adapter *adapter.V2Adapter
}

// NewWorkerServer creates a new worker MCP server.
// Note: Instructions are generated dynamically via WorkerSystemPrompt.
// The full instructions are provided via AppendSystemPrompt when spawning the worker.
func NewWorkerServer(workerID string, msgStore MessageStore) *WorkerServer {
	// Generate instructions for this worker
	instructions := WorkerSystemPrompt(workerID)

	ws := &WorkerServer{
		Server:   NewServer("perles-worker", "1.0.0", WithInstructions(instructions)),
		workerID: workerID,
		msgStore: msgStore,
		dedup:    NewMessageDeduplicator(DefaultDeduplicationWindow),
	}

	ws.registerTools()
	return ws
}

// SetReflectionWriter sets the reflection writer for saving worker reflections.
// This must be called before the post_reflections tool can be used.
func (ws *WorkerServer) SetReflectionWriter(writer ReflectionWriter) {
	ws.reflectionWriter = writer
}

// SetV2Adapter allows setting the v2 adapter after construction.
func (ws *WorkerServer) SetV2Adapter(adapter *adapter.V2Adapter) {
	ws.v2Adapter = adapter
}

// registerTools registers all worker tools with the MCP server.
func (ws *WorkerServer) registerTools() {
	// check_messages - Pull-based message retrieval
	ws.RegisterTool(Tool{
		Name:        "check_messages",
		Description: "Check for new messages addressed to this worker. Returns structured JSON with unread messages from the coordinator or other workers.",
		InputSchema: &InputSchema{
			Type:       "object",
			Properties: map[string]*PropertySchema{},
			Required:   []string{},
		},
		OutputSchema: &OutputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"unread_count": {Type: "number", Description: "Number of unread messages"},
				"messages": {
					Type:        "array",
					Description: "List of unread messages",
					Items: &PropertySchema{
						Type: "object",
						Properties: map[string]*PropertySchema{
							"timestamp": {Type: "string", Description: "Message timestamp (HH:MM:SS format)"},
							"from":      {Type: "string", Description: "Sender ID (COORDINATOR, WORKER.N, etc.)"},
							"to":        {Type: "string", Description: "Recipient ID (ALL, WORKER.N, etc.)"},
							"content":   {Type: "string", Description: "Message content"},
						},
						Required: []string{"timestamp", "from", "to", "content"},
					},
				},
			},
			Required: []string{"unread_count", "messages"},
		},
	}, ws.handleCheckMessages)

	// post_message - Post a message to the message log
	ws.RegisterTool(Tool{
		Name:        "post_message",
		Description: "Send a message to the coordinator, other workers, or ALL agents.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"to":      {Type: "string", Description: "Recipient: 'COORDINATOR', 'ALL', or a worker ID (e.g., 'WORKER.2')"},
				"content": {Type: "string", Description: "Message content"},
			},
			Required: []string{"to", "content"},
		},
	}, ws.handlePostMessage)

	// signal_ready - Worker ready notification
	ws.RegisterTool(Tool{
		Name:        "signal_ready",
		Description: "Signal that you are ready for task assignment. Call this once when you first boot up.",
		InputSchema: &InputSchema{
			Type:       "object",
			Properties: map[string]*PropertySchema{},
			Required:   []string{},
		},
	}, ws.handleSignalReady)

	// report_implementation_complete - Signal implementation is done
	ws.RegisterTool(Tool{
		Name:        "report_implementation_complete",
		Description: "Signal that implementation is complete and ready for review. Call this when you have finished implementing the assigned task.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"summary": {Type: "string", Description: "Brief summary of what was implemented"},
			},
			Required: []string{"summary"},
		},
	}, ws.handleReportImplementationComplete)

	// report_review_verdict - Report code review verdict
	ws.RegisterTool(Tool{
		Name:        "report_review_verdict",
		Description: "Report your code review verdict. Use APPROVED if the implementation meets all criteria, DENIED if changes are required.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"verdict":  {Type: "string", Description: "Review verdict: 'APPROVED' or 'DENIED'"},
				"comments": {Type: "string", Description: "Review comments explaining the verdict"},
			},
			Required: []string{"verdict", "comments"},
		},
	}, ws.handleReportReviewVerdict)

	// post_reflections - Save worker reflection to session directory
	ws.RegisterTool(Tool{
		Name:        "post_reflections",
		Description: "Save your reflection and learnings from the completed task. Call this after committing to document insights, mistakes, and learnings for future sessions.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"task_id":   {Type: "string", Description: "The task ID this reflection is for"},
				"summary":   {Type: "string", Description: "Brief summary of work completed (1-2 sentences)"},
				"insights":  {Type: "string", Description: "What approaches worked well? Patterns worth remembering? (optional)"},
				"mistakes":  {Type: "string", Description: "Errors made and lessons learned (optional)"},
				"learnings": {Type: "string", Description: "General learnings for future workers in this codebase (optional)"},
			},
			Required: []string{"task_id", "summary"},
		},
		OutputSchema: &OutputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"status":    {Type: "string", Description: "Success or error status"},
				"file_path": {Type: "string", Description: "Path where reflection was saved"},
				"message":   {Type: "string", Description: "Human-readable result message"},
			},
			Required: []string{"status", "message"},
		},
	}, ws.handlePostReflections)
}

// Tool argument structs for JSON parsing.
type sendMessageArgs struct {
	To      string `json:"to"`
	Content string `json:"content"`
}

// postReflectionsArgs defines the arguments for the post_reflections tool.
type postReflectionsArgs struct {
	TaskID    string `json:"task_id"`
	Summary   string `json:"summary"`
	Insights  string `json:"insights,omitempty"`
	Mistakes  string `json:"mistakes,omitempty"`
	Learnings string `json:"learnings,omitempty"`
}

// checkMessagesResponse is the structured response for check_messages.
type checkMessagesResponse struct {
	UnreadCount int                 `json:"unread_count"`
	Messages    []checkMessageEntry `json:"messages"`
}

// checkMessageEntry is a single unread message.
type checkMessageEntry struct {
	Timestamp string `json:"timestamp"` // HH:MM:SS format
	From      string `json:"from"`
	To        string `json:"to"`
	Content   string `json:"content"`
}

// handleCheckMessages returns unread messages for this worker.
func (ws *WorkerServer) handleCheckMessages(_ context.Context, _ json.RawMessage) (*ToolCallResult, error) {
	if ws.msgStore == nil {
		return nil, fmt.Errorf("message store not available")
	}

	// Get unread messages for this worker
	unread := ws.msgStore.UnreadFor(ws.workerID)

	// Mark as read after retrieving
	ws.msgStore.MarkRead(ws.workerID)

	// Build structured response
	messages := make([]checkMessageEntry, len(unread))
	for i, entry := range unread {
		messages[i] = checkMessageEntry{
			Timestamp: entry.Timestamp.Format("15:04:05"),
			From:      entry.From,
			To:        entry.To,
			Content:   entry.Content,
		}
	}

	response := checkMessagesResponse{
		UnreadCount: len(messages),
		Messages:    messages,
	}

	data, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling response: %w", err)
	}

	log.Debug(log.CatMCP, "Returned unread messages", "workerID", ws.workerID, "count", len(unread))
	return SuccessResult(string(data)), nil
}

// handlePostMessage posts a message to the message log from this worker.
func (ws *WorkerServer) handlePostMessage(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args sendMessageArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.To == "" {
		return nil, fmt.Errorf("to is required")
	}
	if args.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	if ws.msgStore == nil {
		return nil, fmt.Errorf("message store not available")
	}

	// Check for duplicate message within deduplication window
	if ws.dedup.IsDuplicate(ws.workerID, args.Content) {
		log.Debug(log.CatMCP, "Duplicate message suppressed", "workerID", ws.workerID)
		return SuccessResult(fmt.Sprintf("Message sent to %s", args.To)), nil
	}

	_, err := ws.msgStore.Append(ws.workerID, args.To, args.Content, message.MessageInfo)
	if err != nil {
		log.Debug(log.CatMCP, "Failed to send message", "workerID", ws.workerID, "to", args.To, "error", err)
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	log.Debug(log.CatMCP, "Message sent", "workerID", ws.workerID, "to", args.To)

	return SuccessResult(fmt.Sprintf("Message sent to %s", args.To)), nil
}

// handleSignalReady signals the coordinator that this worker is ready for task assignment.
func (ws *WorkerServer) handleSignalReady(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	return ws.v2Adapter.HandleSignalReady(ctx, args, ws.workerID)
}

// handleReportImplementationComplete signals that implementation is complete and ready for review.
func (ws *WorkerServer) handleReportImplementationComplete(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	return ws.v2Adapter.HandleReportImplementationComplete(ctx, rawArgs, ws.workerID)
}

// handleReportReviewVerdict reports the code review verdict (APPROVED or DENIED).
func (ws *WorkerServer) handleReportReviewVerdict(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	return ws.v2Adapter.HandleReportReviewVerdict(ctx, rawArgs, ws.workerID)
}

// validateReflectionArgs validates the arguments for the post_reflections tool.
// It checks task_id format (to prevent path traversal), summary length bounds,
// and total content length.
func validateReflectionArgs(args postReflectionsArgs) error {
	// Validate task_id is not empty
	if args.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}

	// Validate task_id format to prevent path traversal attacks
	// Reject patterns containing ".." or "/" which could escape the session directory
	if strings.Contains(args.TaskID, "..") || strings.Contains(args.TaskID, "/") {
		return fmt.Errorf("invalid task_id format: contains path traversal characters")
	}

	// Validate task_id matches expected format
	if !validation.IsValidTaskID(args.TaskID) {
		return fmt.Errorf("invalid task_id format: %s", args.TaskID)
	}

	// Validate summary is not empty
	if args.Summary == "" {
		return fmt.Errorf("summary is required")
	}

	// Validate summary length bounds
	if len(args.Summary) < MinSummaryLength {
		return fmt.Errorf("summary too short (min %d chars, got %d)", MinSummaryLength, len(args.Summary))
	}

	return nil
}

// buildReflectionMarkdown generates the markdown content for a worker reflection.
// It includes a metadata header and conditionally includes optional sections
// (Insights, Mistakes & Lessons, Learnings) only if they are non-empty.
func buildReflectionMarkdown(workerID string, args postReflectionsArgs) string {
	var b strings.Builder

	// Header with metadata
	b.WriteString("# Worker Reflection\n\n")
	b.WriteString(fmt.Sprintf("**Worker:** %s\n", workerID))
	b.WriteString(fmt.Sprintf("**Task:** %s\n", args.TaskID))
	b.WriteString(fmt.Sprintf("**Date:** %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

	// Summary section (always included)
	b.WriteString("## Summary\n\n")
	b.WriteString(args.Summary)
	b.WriteString("\n\n")

	// Insights section (optional)
	if args.Insights != "" {
		b.WriteString("## Insights\n\n")
		b.WriteString(args.Insights)
		b.WriteString("\n\n")
	}

	// Mistakes & Lessons section (optional)
	if args.Mistakes != "" {
		b.WriteString("## Mistakes & Lessons\n\n")
		b.WriteString(args.Mistakes)
		b.WriteString("\n\n")
	}

	// Learnings section (optional)
	if args.Learnings != "" {
		b.WriteString("## Learnings\n\n")
		b.WriteString(args.Learnings)
		b.WriteString("\n\n")
	}

	return b.String()
}

// handlePostReflections saves a worker's reflection to their session directory.
func (ws *WorkerServer) handlePostReflections(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args postReflectionsArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	// Validate input using the dedicated validation function
	if err := validateReflectionArgs(args); err != nil {
		return nil, err
	}

	// Check that reflectionWriter is configured (graceful error, not panic)
	if ws.reflectionWriter == nil {
		return nil, fmt.Errorf("reflection writer not configured")
	}

	// Build markdown content
	content := buildReflectionMarkdown(ws.workerID, args)

	// Write to session directory
	filePath, err := ws.reflectionWriter.WriteWorkerReflection(ws.workerID, args.TaskID, []byte(content))
	if err != nil {
		log.Debug(log.CatMCP, "Failed to write reflection", "workerID", ws.workerID, "error", err)
		return nil, fmt.Errorf("failed to save reflection: %w", err)
	}

	log.Debug(log.CatMCP, "Worker posted reflection", "workerID", ws.workerID, "taskID", args.TaskID, "path", filePath)

	// Return structured response with status, file_path, message
	response := map[string]any{
		"status":    "success",
		"file_path": filePath,
		"message":   fmt.Sprintf("Reflection saved to %s", filePath),
	}
	data, _ := json.MarshalIndent(response, "", "  ")
	return StructuredResult(string(data), response), nil
}
