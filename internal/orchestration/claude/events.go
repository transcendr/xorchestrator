// Package claude provides a Go interface to headless Claude Code sessions.
package claude

import (
	"encoding/json"

	"github.com/zjrosen/xorchestrator/internal/log"
	"github.com/zjrosen/xorchestrator/internal/orchestration/client"
)

// rawUsage holds raw token usage from Claude CLI JSON output.
type rawUsage struct {
	InputTokens              int `json:"input_tokens,omitempty"`
	OutputTokens             int `json:"output_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}
type contentBlock struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
	// Tool use fields (when Type == "tool_use")
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type messageContent struct {
	ID      string         `json:"id,omitempty"`
	Role    string         `json:"role,omitempty"`
	Content []contentBlock `json:"content,omitempty"`
	Model   string         `json:"model,omitempty"`
	Usage   *rawUsage      `json:"usage,omitempty"`
}

// rawEvent is used for parsing raw Claude CLI JSON output.
// It mirrors client.OutputEvent but with rawUsage for the usage field.
type rawEvent struct {
	Type            client.EventType             `json:"type"`
	SubType         string                       `json:"subtype,omitempty"`
	SessionID       string                       `json:"session_id,omitempty"`
	WorkDir         string                       `json:"cwd,omitempty"`
	Message         *messageContent              `json:"message,omitempty"`
	Tool            *client.ToolContent          `json:"tool,omitempty"`
	Usage           *rawUsage                    `json:"usage,omitempty"`
	ModelUsage      map[string]client.ModelUsage `json:"modelUsage,omitempty"` //nolint:tagliatelle // Claude CLI uses camelCase
	Error           *client.ErrorInfo            `json:"error,omitempty"`
	TotalCostUSD    float64                      `json:"total_cost_usd,omitempty"`
	DurationMs      int64                        `json:"duration_ms,omitempty"`
	IsErrorResult   bool                         `json:"is_error,omitempty"`
	Result          string                       `json:"result,omitempty"`
	NumTurns        int                          `json:"num_turns,omitempty"`
	ParentToolUseId string                       `json:"parent_tool_use_id,omitempty"`
}

// parseEvent parses raw Claude CLI JSON and returns a client.OutputEvent.
func parseEvent(data []byte) (client.OutputEvent, error) {
	var raw rawEvent
	if err := json.Unmarshal(data, &raw); err != nil {
		return client.OutputEvent{}, err
	}

	event := client.OutputEvent{
		Type:          raw.Type,
		SubType:       raw.SubType,
		SessionID:     raw.SessionID,
		WorkDir:       raw.WorkDir,
		Tool:          raw.Tool,
		ModelUsage:    raw.ModelUsage,
		Error:         raw.Error,
		TotalCostUSD:  raw.TotalCostUSD,
		DurationMs:    raw.DurationMs,
		IsErrorResult: raw.IsErrorResult,
		Result:        raw.Result,
	}

	if raw.Message != nil {
		event.Message = &client.MessageContent{
			ID:    raw.Message.ID,
			Role:  raw.Message.Role,
			Model: raw.Message.Model,
		}
		for _, block := range raw.Message.Content {
			event.Message.Content = append(event.Message.Content, client.ContentBlock{
				Type:  block.Type,
				Text:  block.Text,
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			})
		}
	}

	// TODO this could be wrong but currently the EventResult doesn't feel like its the correct token usage.
	// will need to revisit this to understand if this is a Claude bug or if we should be using the assistant event
	if raw.Type == client.EventAssistant && raw.Message != nil && raw.Message.Usage != nil {
		tokensUsed := raw.Message.Usage.InputTokens + raw.Message.Usage.CacheReadInputTokens + raw.Message.Usage.CacheCreationInputTokens
		log.Debug(log.CatOrch, "TOKENS USED", "tokens", tokensUsed)
		event.Usage = &client.UsageInfo{
			TokensUsed: tokensUsed,
			// TODO this has to be dynamic based on model eventually for now we are just using opus 4.5
			TotalTokens:  200000,
			OutputTokens: raw.Message.Usage.OutputTokens,
		}
	}

	// Copy raw data for debugging
	event.Raw = make([]byte, len(data))
	copy(event.Raw, data)

	return event, nil
}
