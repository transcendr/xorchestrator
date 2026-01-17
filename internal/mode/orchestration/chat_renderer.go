package orchestration

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/xorchestrator/internal/ui/shared/chatrender"
)

// ChatRenderConfig configures how chat messages are rendered.
// This is a convenience alias for the shared chatrender.RenderConfig.
type ChatRenderConfig struct {
	AgentLabel              string                 // Label for agent messages (e.g., "Coordinator" or "Worker")
	AgentColor              lipgloss.AdaptiveColor // Color for the agent role label
	ShowCoordinatorInWorker bool                   // Show "Coordinator" role in worker panes for coordinator messages
}

// renderChatContent renders a slice of ChatMessages with tool call grouping.
// Delegates to the shared chatrender package.
func renderChatContent(messages []ChatMessage, wrapWidth int, cfg ChatRenderConfig) string {
	return chatrender.RenderContent(messages, wrapWidth, chatrender.RenderConfig{
		AgentLabel:              cfg.AgentLabel,
		AgentColor:              cfg.AgentColor,
		UserLabel:               "User", // Orchestration mode uses "User" label
		ShowCoordinatorInWorker: cfg.ShowCoordinatorInWorker,
	})
}
