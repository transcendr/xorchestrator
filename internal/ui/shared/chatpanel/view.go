package chatpanel

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/ui/shared/chatrender"
	"github.com/zjrosen/perles/internal/ui/shared/panes"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// Border colors for assistant status (matches orchestration mode)
var (
	assistantWorkingBorderColor = lipgloss.AdaptiveColor{Light: "#54A0FF", Dark: "#54A0FF"} // Blue - working
	inputFocusedBorderColor     = lipgloss.AdaptiveColor{Light: "#AAAAAA", Dark: "#888888"} // Slightly brighter when focused
)

// QueuedCountStyle is used for the queue count indicator.
// Uses orange color to draw attention to pending queued messages (matches orchestration mode).
var QueuedCountStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#FFA500", Dark: "#FFB347"})

// View renders the chat panel with separate message pane and input area.
func (m Model) View() string {
	if !m.visible {
		return ""
	}

	// Early return for zero dimensions
	if m.width < 1 || m.height < 1 {
		return ""
	}

	// Input pane height: 6 lines (4 content lines + 2 borders)
	inputPaneHeight := 6

	// Message pane height: remaining space
	messagePaneHeight := max(m.height-inputPaneHeight, 3)

	// Calculate content height for tabs (subtract 2 for borders)
	contentHeight := max(messagePaneHeight-2, 1)

	// Render content based on active tab
	var tabContent string
	switch m.activeTab {
	case TabChat:
		// Get viewport from active session for per-session scroll state
		session := m.ActiveSession()
		if session == nil {
			tabContent = ""
			break
		}

		// Render chat messages using viewport
		vpWidth := max(m.width-2, 1)
		content := m.renderMessages(vpWidth)

		// Pad content to push to bottom (chat-like behavior)
		contentLines := len(splitLines(content))
		if contentLines < contentHeight {
			padding := make([]string, contentHeight-contentLines)
			content = joinLines(append(padding, splitLines(content)...))
		}

		// Update viewport from session
		wasAtBottom := session.Viewport.AtBottom()
		session.Viewport.Width = vpWidth
		session.Viewport.Height = contentHeight
		session.Viewport.SetContent(content)
		if wasAtBottom {
			session.Viewport.GotoBottom()
		}

		tabContent = session.Viewport.View()

	case TabSessions:
		tabContent = m.renderSessionsTab(contentHeight)
	}

	// Build tabs for the top pane
	tabs := []panes.Tab{
		{Label: "Chat", Content: tabContent},
		{Label: "Sessions", Content: tabContent},
	}

	// Determine border color based on assistant status
	borderColor := styles.BorderDefaultColor
	if m.assistantWorking {
		borderColor = assistantWorkingBorderColor
	}

	// Build bottom-left queue indicator for message pane
	var bottomLeft string
	if m.queueCount > 0 {
		bottomLeft = QueuedCountStyle.Render(fmt.Sprintf("[%d queued]", m.queueCount))
	}

	// Build bottom-right metrics display
	var bottomRight string
	if m.metrics != nil && m.metrics.TokensUsed > 0 {
		bottomRight = m.metrics.FormatContextDisplay()
	}

	// Render the tabbed pane
	// BorderedPane handles hidden tabs internally, so we pass the raw activeTab index
	tabbedPane := panes.BorderedPane(panes.BorderConfig{
		Content:     tabContent,
		Width:       m.width,
		Height:      messagePaneHeight,
		Tabs:        tabs,
		ActiveTab:   m.activeTab,
		BorderColor: borderColor,
		BottomLeft:  bottomLeft,
		BottomRight: bottomRight,
	})

	// Render the input pane
	inputWidth := m.width - 2 - 2 // borders and padding
	inputContent := lipgloss.JoinHorizontal(lipgloss.Left,
		" ",
		lipgloss.NewStyle().Width(inputWidth).Render(m.input.View()),
		" ",
	)

	// Use slightly brighter border when focused
	inputBorderColor := styles.BorderDefaultColor
	if m.focused {
		inputBorderColor = inputFocusedBorderColor
	}

	// Show session ID in input pane top-left when multiple sessions exist
	var inputTopLeft string
	if len(m.sessions) > 1 {
		inputTopLeft = m.activeSessionID
	}

	inputPane := panes.BorderedPane(panes.BorderConfig{
		Content:     inputContent,
		Width:       m.width,
		Height:      inputPaneHeight,
		TopLeft:     inputTopLeft,
		BottomLeft:  m.input.ModeIndicator(),
		BorderColor: inputBorderColor,
		// PreWrapped true because vimtextarea handles its own soft-wrapping
		PreWrapped: true,
	})

	// Stack message pane and input pane vertically
	return lipgloss.JoinVertical(lipgloss.Left,
		tabbedPane,
		inputPane,
	)
}

// splitLines splits a string into lines.
func splitLines(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, "\n")
}

// joinLines joins lines with newlines.
func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}

// renderMessages renders the chat messages using the shared chatrender package.
// Uses the active session's messages for multi-session support.
func (m Model) renderMessages(wrapWidth int) string {
	session := m.ActiveSession()
	if session == nil || len(session.Messages) == 0 {
		return ""
	}

	// Use shared renderer with "You" label (default) and "Assistant" agent label
	return chatrender.RenderContent(session.Messages, wrapWidth, chatrender.RenderConfig{
		AgentLabel: "Assistant",
		AgentColor: chatrender.AssistantColor,
	})
}

// renderSessionsTab renders the Sessions tab content with a selectable session list.
// Shows each session with: ● session-id (N msgs) - Status
// ● indicates unread content, ○ indicates viewed session.
// The first item is always "Create new session" option.
func (m Model) renderSessionsTab(contentHeight int) string {
	// Build session list
	var lines []string

	// Calculate inner width for padding (width minus borders)
	innerWidth := max(m.width-2, 1)

	// Background color for selection
	bgColor := styles.SelectionBackgroundColor

	// First item: "Create new session" option (cursor index 0)
	createNewSelected := m.sessionListCursor == 0 && m.activeTab == TabSessions
	createNewText := "+ Create new session"
	createNewPadding := max(innerWidth-lipgloss.Width(createNewText), 0)
	if createNewSelected {
		createNewStyle := lipgloss.NewStyle().Foreground(styles.StatusSuccessColor).Background(bgColor).Bold(true)
		spaceStyle := lipgloss.NewStyle().Background(bgColor)
		lines = append(lines, createNewStyle.Render(createNewText)+spaceStyle.Render(strings.Repeat(" ", createNewPadding)))
	} else {
		createNewStyle := lipgloss.NewStyle().Foreground(styles.StatusSuccessColor)
		lines = append(lines, createNewStyle.Render(createNewText))
	}

	// Session items start at cursor index 1 (index 0 is "Create new session")
	for i, sessionID := range m.sessionOrder {
		session := m.sessions[sessionID]
		if session == nil {
			continue
		}

		// Cursor index is i+1 because index 0 is "Create new session"
		isSelected := (i+1) == m.sessionListCursor && m.activeTab == TabSessions
		isPendingRetire := sessionID == m.pendingRetireSessionID

		// Build activity indicator: ● for new content, ○ for viewed
		var indicatorStyle lipgloss.Style
		var indicatorChar string
		if session.HasNewContent {
			indicatorChar = "●"
			indicatorStyle = lipgloss.NewStyle().Foreground(styles.StatusSuccessColor)
		} else {
			indicatorChar = "○"
			indicatorStyle = lipgloss.NewStyle().Foreground(styles.TextMutedColor)
		}

		// Build status display
		var statusText string
		var statusStyle lipgloss.Style
		if isPendingRetire {
			// Show confirmation prompt instead of status
			statusText = "Press d to confirm, esc to cancel"
			statusStyle = lipgloss.NewStyle().Foreground(styles.StatusWarningColor)
		} else if session.Status.IsTerminal() {
			statusText = "Session ended"
			statusStyle = lipgloss.NewStyle().Foreground(styles.StatusErrorColor)
		} else {
			switch session.Status {
			case events.ProcessStatusWorking:
				statusText = "Working"
				statusStyle = lipgloss.NewStyle().Foreground(styles.StatusInProgressColor)
			case events.ProcessStatusReady:
				statusText = "Ready"
				statusStyle = lipgloss.NewStyle().Foreground(styles.StatusSuccessColor)
			case events.ProcessStatusPending:
				statusText = "Pending"
				statusStyle = lipgloss.NewStyle().Foreground(styles.TextMutedColor)
			case events.ProcessStatusStarting:
				statusText = "Starting"
				statusStyle = lipgloss.NewStyle().Foreground(styles.TextMutedColor)
			case events.ProcessStatusPaused:
				statusText = "Paused"
				statusStyle = lipgloss.NewStyle().Foreground(styles.TextMutedColor)
			case events.ProcessStatusStopped:
				statusText = "Stopped"
				statusStyle = lipgloss.NewStyle().Foreground(styles.StatusErrorColor)
			default:
				statusText = string(session.Status)
				statusStyle = lipgloss.NewStyle().Foreground(styles.TextMutedColor)
			}
		}

		// Mark active session with asterisk
		activeMarker := " "
		if sessionID == m.activeSessionID {
			activeMarker = "*"
		}

		// Message count
		msgCountStr := fmt.Sprintf("(%d msgs)", len(session.Messages))

		// Build the line content (without styling) to calculate padding
		// Format: ● session-1* (5 msgs) - Ready (or confirmation prompt)
		plainContent := fmt.Sprintf("%s %s%s %s - %s", indicatorChar, session.ID, activeMarker, msgCountStr, statusText)
		padding := max(innerWidth-lipgloss.Width(plainContent), 0)

		// Build the styled line
		var result strings.Builder
		if isSelected {
			// Apply background to all segments
			spaceStyle := lipgloss.NewStyle().Background(bgColor)
			nameStyle := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor).Background(bgColor).Bold(true)

			result.WriteString(indicatorStyle.Background(bgColor).Render(indicatorChar))
			result.WriteString(spaceStyle.Render(" "))
			result.WriteString(nameStyle.Render(session.ID + activeMarker))
			result.WriteString(spaceStyle.Render(" "))
			result.WriteString(spaceStyle.Render(msgCountStr))
			result.WriteString(spaceStyle.Render(" - "))
			result.WriteString(statusStyle.Background(bgColor).Render(statusText))
			result.WriteString(spaceStyle.Render(strings.Repeat(" ", padding)))
		} else {
			// Normal rendering without background
			nameStyle := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor)

			result.WriteString(indicatorStyle.Render(indicatorChar))
			result.WriteString(" ")
			result.WriteString(nameStyle.Render(session.ID + activeMarker))
			result.WriteString(" ")
			result.WriteString(msgCountStr)
			result.WriteString(" - ")
			result.WriteString(statusStyle.Render(statusText))
		}

		lines = append(lines, result.String())
	}

	// Join lines and pad to content height if needed
	content := strings.Join(lines, "\n")
	contentLines := len(lines)
	if contentLines < contentHeight {
		// Pad with empty lines at bottom using strings.Builder for efficiency
		var sb strings.Builder
		sb.WriteString(content)
		for i := 0; i < contentHeight-contentLines; i++ {
			sb.WriteString("\n")
		}
		content = sb.String()
	}

	return content
}
