package chatpanel

// Role constants for chat messages.
const (
	// RoleUser indicates a message from the user.
	RoleUser = "user"
	// RoleAssistant indicates a message from the AI assistant.
	RoleAssistant = "assistant"
)

// AssistantResponseMsg is sent when the assistant produces output.
type AssistantResponseMsg struct {
	Content string
}

// AssistantDoneMsg is sent when the assistant completes a response.
type AssistantDoneMsg struct{}

// AssistantErrorMsg is sent when an error occurs during assistant interaction.
type AssistantErrorMsg struct {
	Error error
}

// SendMessageMsg is sent when the user submits a message.
type SendMessageMsg struct {
	Content string
}

// RequestQuitMsg is sent when the user presses Ctrl+C in normal mode.
// The parent should handle this to show a quit confirmation or exit.
type RequestQuitMsg struct{}

// NewSessionRequestMsg is sent when the user presses Ctrl+N to create a new session.
// The parent (app.go) should handle this by:
// 1. Generating a new session ID
// 2. Calling chatPanel.CreateSession(sessionID)
// 3. Spawning a process for the new session
// 4. Returning NewSessionCreatedMsg on success
type NewSessionRequestMsg struct{}

// NewSessionCreatedMsg is sent by the parent after successfully creating a new session.
// The chat panel should switch to the new session and focus the input.
type NewSessionCreatedMsg struct {
	SessionID string
}

// SessionSwitchedMsg is sent when the active session changes (e.g., via Sessions tab).
// This is used for cross-component communication if other parts of the UI need to know.
type SessionSwitchedMsg struct {
	SessionID string
}
