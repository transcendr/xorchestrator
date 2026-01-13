package amp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/client"
)

func TestBuildArgs(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		isResume bool
		expected []string
	}{
		{
			name: "minimal config",
			cfg: Config{
				WorkDir: "/project",
			},
			isResume: false,
			expected: []string{
				"-x", "--stream-json",
				"--no-notifications",
			},
		},
		{
			name: "with skip permissions",
			cfg: Config{
				WorkDir:         "/project",
				SkipPermissions: true,
			},
			isResume: false,
			expected: []string{
				"-x", "--stream-json",
				"--dangerously-allow-all",
				"--no-notifications",
			},
		},
		{
			name: "with disable IDE",
			cfg: Config{
				WorkDir:    "/project",
				DisableIDE: true,
			},
			isResume: false,
			expected: []string{
				"-x", "--stream-json",
				"--no-notifications",
				"--no-ide",
			},
		},
		{
			name: "with sonnet model",
			cfg: Config{
				WorkDir: "/project",
				Model:   "sonnet",
			},
			isResume: false,
			expected: []string{
				"-x", "--stream-json",
				"--no-notifications",
				"--use-sonnet",
			},
		},
		{
			name: "with mode",
			cfg: Config{
				WorkDir: "/project",
				Mode:    "rush",
			},
			isResume: false,
			expected: []string{
				"-x", "--stream-json",
				"--no-notifications",
				"-m", "rush",
			},
		},
		{
			name: "with MCP config",
			cfg: Config{
				WorkDir:   "/project",
				MCPConfig: `{"servers":{}}`,
			},
			isResume: false,
			expected: []string{
				"-x", "--stream-json",
				"--no-notifications",
				"--mcp-config", `{"servers":{}}`,
			},
		},
		{
			name: "resume with thread ID",
			cfg: Config{
				WorkDir:  "/project",
				ThreadID: "T-abc123",
			},
			isResume: true,
			expected: []string{
				"threads", "continue", "T-abc123",
				"-x", "--stream-json",
				"--no-notifications",
			},
		},
		{
			name: "full config",
			cfg: Config{
				WorkDir:         "/project",
				ThreadID:        "T-xyz789",
				Model:           "sonnet",
				Mode:            "smart",
				SkipPermissions: true,
				DisableIDE:      true,
				MCPConfig:       `{"mcpServers":{}}`,
			},
			isResume: true,
			expected: []string{
				"threads", "continue", "T-xyz789",
				"-x", "--stream-json",
				"--dangerously-allow-all",
				"--no-notifications",
				"--no-ide",
				"--use-sonnet",
				"-m", "smart",
				"--mcp-config", `{"mcpServers":{}}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := buildArgs(tt.cfg, tt.isResume)
			require.Equal(t, tt.expected, args)
		})
	}
}

func TestParseEvent(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		validate func(t *testing.T, e client.OutputEvent)
	}{
		{
			name: "system init event",
			json: `{"type":"system","subtype":"init","session_id":"T-abc123","cwd":"/project","tools":["Bash","Read"]}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventSystem, e.Type)
				require.Equal(t, "init", e.SubType)
				require.Equal(t, "T-abc123", e.SessionID)
				require.Equal(t, "/project", e.WorkDir)
				require.True(t, e.IsInit())
				require.False(t, e.IsAssistant())
			},
		},
		{
			name: "assistant message with text",
			json: `{"type":"assistant","message":{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"Hello from Amp!"}],"model":"claude-sonnet-4"}}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventAssistant, e.Type)
				require.True(t, e.IsAssistant())
				require.NotNil(t, e.Message)
				require.Equal(t, "assistant", e.Message.Role)
				require.Equal(t, "claude-sonnet-4", e.Message.Model)
				require.Equal(t, "Hello from Amp!", e.Message.GetText())
			},
		},
		{
			name: "assistant message with tool use",
			json: `{"type":"assistant","message":{"id":"msg_2","content":[{"type":"tool_use","id":"toolu_123","name":"Bash","input":{"cmd":"ls -la"}}]}}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventAssistant, e.Type)
				require.True(t, e.IsAssistant())
				require.NotNil(t, e.Message)
				require.True(t, e.Message.HasToolUses())
				tools := e.Message.GetToolUses()
				require.Len(t, tools, 1)
				require.Equal(t, "Bash", tools[0].Name)
				require.Equal(t, "toolu_123", tools[0].ID)
			},
		},
		{
			name: "user/tool_result event",
			json: `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_123","content":"success"}]}}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventToolResult, e.Type)
				require.True(t, e.IsToolResult())
			},
		},
		{
			name: "result success event",
			json: `{"type":"result","subtype":"success","duration_ms":5000,"is_error":false,"num_turns":3,"result":"Task completed","session_id":"T-abc123"}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventResult, e.Type)
				require.Equal(t, "success", e.SubType)
				require.True(t, e.IsResult())
				require.False(t, e.IsErrorResult)
				require.Equal(t, "Task completed", e.Result)
				require.Equal(t, int64(5000), e.DurationMs)
				require.Equal(t, "T-abc123", e.SessionID)
			},
		},
		{
			name: "result error event",
			json: `{"type":"result","subtype":"error","is_error":true,"result":"Context window exceeded"}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventResult, e.Type)
				require.True(t, e.IsResult())
				require.True(t, e.IsErrorResult)
				require.Equal(t, "Context window exceeded", e.Result)
			},
		},
		{
			name: "error event",
			json: `{"type":"error","error":{"message":"Something went wrong","code":"INTERNAL"}}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventError, e.Type)
				require.NotNil(t, e.Error)
				require.Equal(t, "Something went wrong", e.Error.Message)
				require.Equal(t, "INTERNAL", e.Error.Code)
			},
		},
		{
			name: "assistant with usage info",
			json: `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done"}],"usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":1000,"cache_creation_input_tokens":500,"max_tokens":168000}}}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventAssistant, e.Type)
				require.NotNil(t, e.Usage)
				// TokensUsed = input + cache_read + cache_creation = 100 + 1000 + 500 = 1600
				require.Equal(t, 1600, e.Usage.TokensUsed)
				require.Equal(t, 50, e.Usage.OutputTokens)
				require.Equal(t, 200000, e.Usage.TotalTokens) // Default context window
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := parseEvent([]byte(tt.json))
			require.NoError(t, err)
			tt.validate(t, event)
		})
	}
}

func TestParseEvent_InvalidJSON(t *testing.T) {
	_, err := parseEvent([]byte("not json"))
	require.Error(t, err)
}

func TestMapEventType(t *testing.T) {
	tests := []struct {
		input    string
		expected client.EventType
	}{
		{"system", client.EventSystem},
		{"assistant", client.EventAssistant},
		{"user", client.EventToolResult}, // Amp uses "user" for tool results
		{"result", client.EventResult},
		{"error", client.EventError},
		{"unknown", client.EventType("unknown")},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := mapEventType(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestConfigFromClient(t *testing.T) {
	tests := []struct {
		name     string
		cfg      client.Config
		validate func(t *testing.T, c Config)
	}{
		{
			name: "basic config",
			cfg: client.Config{
				WorkDir: "/project",
				Prompt:  "Hello",
			},
			validate: func(t *testing.T, c Config) {
				require.Equal(t, "/project", c.WorkDir)
				require.Equal(t, "Hello", c.Prompt)
				require.True(t, c.DisableIDE) // Always disabled in headless mode
			},
		},
		{
			name: "with system prompt prepended",
			cfg: client.Config{
				WorkDir:      "/project",
				SystemPrompt: "You are helpful",
				Prompt:       "Hello",
			},
			validate: func(t *testing.T, c Config) {
				require.Equal(t, "You are helpful\n\nHello", c.Prompt)
			},
		},
		{
			name: "session ID maps to thread ID",
			cfg: client.Config{
				WorkDir:   "/project",
				SessionID: "session-123",
			},
			validate: func(t *testing.T, c Config) {
				require.Equal(t, "session-123", c.ThreadID)
			},
		},
		{
			name: "skip permissions",
			cfg: client.Config{
				WorkDir:         "/project",
				SkipPermissions: true,
			},
			validate: func(t *testing.T, c Config) {
				require.True(t, c.SkipPermissions)
			},
		},
		{
			name: "MCP config",
			cfg: client.Config{
				WorkDir:   "/project",
				MCPConfig: `{"servers":{}}`,
			},
			validate: func(t *testing.T, c Config) {
				require.Equal(t, `{"servers":{}}`, c.MCPConfig)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ampCfg := configFromClient(tt.cfg)
			tt.validate(t, ampCfg)
		})
	}
}

func TestToolExtraction(t *testing.T) {
	// Test that tool_use content blocks are properly extracted
	jsonStr := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_abc","name":"Read","input":{"path":"/file.txt"}}]}}`

	event, err := parseEvent([]byte(jsonStr))
	require.NoError(t, err)
	require.NotNil(t, event.Tool)
	require.Equal(t, "toolu_abc", event.Tool.ID)
	require.Equal(t, "Read", event.Tool.Name)

	// Verify input is preserved as raw JSON
	var input struct {
		Path string `json:"path"`
	}
	err = json.Unmarshal(event.Tool.Input, &input)
	require.NoError(t, err)
	require.Equal(t, "/file.txt", input.Path)
}

func TestMultipleToolUses(t *testing.T) {
	// Test that multiple tool_use blocks are detected
	jsonStr := `{"type":"assistant","message":{"content":[
		{"type":"text","text":"Let me check both files."},
		{"type":"tool_use","id":"toolu_1","name":"Read","input":{"path":"a.go"}},
		{"type":"tool_use","id":"toolu_2","name":"Read","input":{"path":"b.go"}}
	]}}`

	event, err := parseEvent([]byte(jsonStr))
	require.NoError(t, err)
	require.NotNil(t, event.Message)
	require.True(t, event.Message.HasToolUses())

	tools := event.Message.GetToolUses()
	require.Len(t, tools, 2)
	require.Equal(t, "toolu_1", tools[0].ID)
	require.Equal(t, "toolu_2", tools[1].ID)
}
