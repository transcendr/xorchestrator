package chatrender

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChatMessage_JSONRoundTrip(t *testing.T) {
	ts := time.Date(2026, 1, 11, 15, 30, 0, 0, time.UTC)
	original := Message{
		Role:       "assistant",
		Content:    "Hello, world!",
		IsToolCall: true,
		Timestamp:  &ts,
	}

	// Marshal to JSON
	data, err := json.Marshal(original)
	require.NoError(t, err)

	// Verify JSON structure uses expected field names
	var jsonMap map[string]any
	err = json.Unmarshal(data, &jsonMap)
	require.NoError(t, err)
	require.Contains(t, jsonMap, "role")
	require.Contains(t, jsonMap, "content")
	require.Contains(t, jsonMap, "is_tool_call")
	require.Contains(t, jsonMap, "ts")

	// Unmarshal back to Message
	var decoded Message
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Verify round-trip preserves all fields
	require.Equal(t, original.Role, decoded.Role)
	require.Equal(t, original.Content, decoded.Content)
	require.Equal(t, original.IsToolCall, decoded.IsToolCall)
	require.NotNil(t, decoded.Timestamp)
	require.True(t, original.Timestamp.Equal(*decoded.Timestamp))
}

func TestChatMessage_OmitEmpty(t *testing.T) {
	t.Run("nil timestamp omitted", func(t *testing.T) {
		msg := Message{
			Role:    "user",
			Content: "Test message",
		}

		data, err := json.Marshal(msg)
		require.NoError(t, err)

		// Verify ts field is NOT present in JSON
		var jsonMap map[string]any
		err = json.Unmarshal(data, &jsonMap)
		require.NoError(t, err)
		require.NotContains(t, jsonMap, "ts", "nil timestamp should be omitted")
	})

	t.Run("false IsToolCall omitted", func(t *testing.T) {
		msg := Message{
			Role:       "assistant",
			Content:    "Regular message",
			IsToolCall: false,
		}

		data, err := json.Marshal(msg)
		require.NoError(t, err)

		// Verify is_tool_call field is NOT present in JSON
		var jsonMap map[string]any
		err = json.Unmarshal(data, &jsonMap)
		require.NoError(t, err)
		require.NotContains(t, jsonMap, "is_tool_call", "false IsToolCall should be omitted")
	})

	t.Run("true IsToolCall included", func(t *testing.T) {
		msg := Message{
			Role:       "assistant",
			Content:    "🔧 Tool call",
			IsToolCall: true,
		}

		data, err := json.Marshal(msg)
		require.NoError(t, err)

		// Verify is_tool_call field IS present in JSON
		var jsonMap map[string]any
		err = json.Unmarshal(data, &jsonMap)
		require.NoError(t, err)
		require.Contains(t, jsonMap, "is_tool_call", "true IsToolCall should be present")
		require.Equal(t, true, jsonMap["is_tool_call"])
	})

	t.Run("non-nil timestamp included", func(t *testing.T) {
		ts := time.Date(2026, 1, 11, 15, 30, 0, 0, time.UTC)
		msg := Message{
			Role:      "assistant",
			Content:   "Message with timestamp",
			Timestamp: &ts,
		}

		data, err := json.Marshal(msg)
		require.NoError(t, err)

		// Verify ts field IS present in JSON
		var jsonMap map[string]any
		err = json.Unmarshal(data, &jsonMap)
		require.NoError(t, err)
		require.Contains(t, jsonMap, "ts", "non-nil timestamp should be present")
	})
}

func TestChatMessage_BackwardCompatibility(t *testing.T) {
	t.Run("existing construction pattern works", func(t *testing.T) {
		// This is how existing code constructs Messages
		msg := Message{
			Role:    "user",
			Content: "Test message",
		}

		// Should be usable without setting Timestamp or IsToolCall
		require.Equal(t, "user", msg.Role)
		require.Equal(t, "Test message", msg.Content)
		require.False(t, msg.IsToolCall)
		require.Nil(t, msg.Timestamp)
	})

	t.Run("unmarshal legacy JSON without ts field", func(t *testing.T) {
		// Simulate JSON from before Timestamp field existed
		legacyJSON := `{"role":"assistant","content":"Hello"}`

		var msg Message
		err := json.Unmarshal([]byte(legacyJSON), &msg)
		require.NoError(t, err)

		require.Equal(t, "assistant", msg.Role)
		require.Equal(t, "Hello", msg.Content)
		require.False(t, msg.IsToolCall)
		require.Nil(t, msg.Timestamp)
	})

	t.Run("unmarshal legacy JSON without is_tool_call field", func(t *testing.T) {
		// Simulate JSON from before or with omitted IsToolCall
		legacyJSON := `{"role":"assistant","content":"Hello"}`

		var msg Message
		err := json.Unmarshal([]byte(legacyJSON), &msg)
		require.NoError(t, err)

		require.False(t, msg.IsToolCall)
	})
}
