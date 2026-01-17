package gemini

import (
	"encoding/json"
	"log"

	"github.com/zjrosen/xorchestrator/internal/orchestration/client"
)

// geminiEvent represents the raw Gemini CLI stream-json event structure.
// This is used for parsing Gemini-specific fields before converting to client.OutputEvent.
//
// Gemini CLI uses a different event format than Claude/Amp/Codex:
//   - init: Session initialization with session_id and model
//   - message: Assistant or user messages with role-based discrimination
//   - tool_use: Tool invocation with name and parameters
//   - tool_result: Tool execution result with status and output
//   - result: Session completion with usage statistics
//   - error: Error events with message and optional code
type geminiEvent struct {
	// Type is the event type (e.g., "init", "message", "tool_use", "result")
	Type string `json:"type"`

	// SessionID is the session identifier (present in init events)
	SessionID string `json:"session_id,omitempty"`

	// Model is the model name (present in init events)
	Model string `json:"model,omitempty"`

	// Timestamp is the event timestamp
	Timestamp string `json:"timestamp,omitempty"`

	// Role is the message role: "assistant" or "user" (for message events)
	Role string `json:"role,omitempty"`

	// Content is the message content (for message events)
	Content string `json:"content,omitempty"`

	// Delta indicates this is a streaming chunk (should be accumulated with previous message)
	Delta bool `json:"delta,omitempty"`

	// Tool fields (Gemini format uses top-level fields)
	ToolName   string          `json:"tool_name,omitempty"`
	ToolID     string          `json:"tool_id,omitempty"`
	Parameters json.RawMessage `json:"parameters,omitempty"`
	Status     string          `json:"status,omitempty"`
	Output     string          `json:"output,omitempty"`

	// Stats contains token usage information (present in result events)
	Stats *geminiStats `json:"stats,omitempty"`

	// Error contains error details (for error events)
	Error *geminiError `json:"error,omitempty"`
}

// geminiStats represents token usage from Gemini's result events.
type geminiStats struct {
	// TokensPrompt is the number of input/prompt tokens
	TokensPrompt int `json:"tokens_prompt,omitempty"`

	// TokensCandidates is the number of output/candidate tokens
	TokensCandidates int `json:"tokens_candidates,omitempty"`

	// TokensCached is the number of cached tokens
	TokensCached int `json:"tokens_cached,omitempty"`

	// DurationMs is the total execution duration in milliseconds
	DurationMs int64 `json:"duration_ms,omitempty"`
}

// geminiError represents error information in Gemini events.
type geminiError struct {
	// Message is the error message
	Message string `json:"message,omitempty"`

	// Code is the optional error code
	Code string `json:"code,omitempty"`
}

// ParseEvent parses a JSON line from Gemini's stream-json output into a client.OutputEvent.
// Returns the parsed event or an error if parsing fails.
//
// Event mappings:
//   - init -> EventSystem (subtype: init) with SessionID extraction
//   - message (role: assistant) -> EventAssistant with text content
//   - message (role: user) -> ignored (returns empty event)
//   - tool_use -> EventToolUse with tool name and parameters
//   - tool_result -> EventToolResult with status and output
//   - result -> EventResult with usage mapping
//   - error -> EventError with message and code
//
// Unknown event types are logged as warnings but do not cause errors.
// The event type is passed through as-is for forward compatibility.
func ParseEvent(line []byte) (client.OutputEvent, error) {
	var raw geminiEvent
	if err := json.Unmarshal(line, &raw); err != nil {
		return client.OutputEvent{}, err
	}

	event := client.OutputEvent{
		Type: mapEventType(raw.Type, raw.Role),
	}

	// Copy raw data for debugging
	event.Raw = make([]byte, len(line))
	copy(event.Raw, line)

	// Handle init -> EventSystem (init subtype)
	if raw.Type == "init" {
		event.SubType = "init"
		event.SessionID = raw.SessionID
		return event, nil
	}

	// Handle message events with role-based discrimination
	if raw.Type == "message" {
		if raw.Role == "assistant" {
			event.Delta = raw.Delta // Pass through streaming chunk indicator
			event.Message = &client.MessageContent{
				Role:  raw.Role,
				Model: raw.Model,
				Content: []client.ContentBlock{
					{
						Type: "text",
						Text: raw.Content,
					},
				},
			}
		}
		// User messages (role: "user") are ignored - return event with empty message
		return event, nil
	}

	// Handle tool_use -> EventToolUse
	// Gemini format: {"tool_name": "...", "tool_id": "...", "parameters": {...}}
	if raw.Type == "tool_use" && raw.ToolName != "" {
		event.Tool = &client.ToolContent{
			ID:    raw.ToolID,
			Name:  raw.ToolName,
			Input: raw.Parameters,
		}
		// Populate Message.Content for process handler compatibility
		event.Message = &client.MessageContent{
			Role: "assistant",
			Content: []client.ContentBlock{
				{
					Type:  "tool_use",
					ID:    raw.ToolID,
					Name:  raw.ToolName,
					Input: raw.Parameters,
				},
			},
		}
		return event, nil
	}

	// Handle tool_result -> EventToolResult
	// Gemini format: {"tool_id": "...", "status": "...", "output": "..."}
	if raw.Type == "tool_result" && raw.ToolID != "" {
		event.Tool = &client.ToolContent{
			ID:     raw.ToolID,
			Output: raw.Output,
		}
		// Mark as error if status indicates failure
		if raw.Status == "error" || raw.Status == "failed" {
			event.IsErrorResult = true
		}
		return event, nil
	}

	// Handle result -> EventResult with usage
	if raw.Type == "result" {
		if raw.Stats != nil {
			// TokensUsed = prompt + cached tokens
			tokensUsed := raw.Stats.TokensPrompt + raw.Stats.TokensCached
			event.Usage = &client.UsageInfo{
				TokensUsed:   tokensUsed,
				TotalTokens:  1000000, // Gemini has 1M token context window
				OutputTokens: raw.Stats.TokensCandidates,
			}
			event.DurationMs = raw.Stats.DurationMs
		}
		return event, nil
	}

	// Handle error -> EventError
	if raw.Type == "error" && raw.Error != nil {
		event.Error = &client.ErrorInfo{
			Message: raw.Error.Message,
			Code:    raw.Error.Code,
		}
		return event, nil
	}

	return event, nil
}

// mapEventType maps Gemini event type and role strings to client.EventType.
// Role is used to discriminate message events (assistant vs user).
func mapEventType(geminiType, role string) client.EventType {
	switch geminiType {
	case "init":
		return client.EventSystem
	case "message":
		if role == "assistant" {
			return client.EventAssistant
		}
		// User messages don't have a direct mapping, treat as tool result
		// (typically used for injected prompts or tool results)
		return client.EventToolResult
	case "tool_use":
		return client.EventToolUse
	case "tool_result":
		return client.EventToolResult
	case "result":
		return client.EventResult
	case "error":
		return client.EventError
	default:
		// Log warning for unknown event types
		log.Printf("gemini: unknown event type: %q", geminiType)
		// Pass through as-is for forward compatibility
		return client.EventType(geminiType)
	}
}
