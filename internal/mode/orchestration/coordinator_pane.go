package orchestration

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/xorchestrator/internal/orchestration/events"
	"github.com/zjrosen/xorchestrator/internal/ui/shared/panes"
	"github.com/zjrosen/xorchestrator/internal/ui/styles"
)

// Coordinator pane styles
var (
	coordinatorMessageStyle = lipgloss.NewStyle().
		Foreground(CoordinatorColor)
)

// renderCoordinatorPane renders the left pane showing coordinator chat history.
// When fullscreen=true, renders in fullscreen mode with simplified title and no metrics.
func (m Model) renderCoordinatorPane(width, height int, fullscreen bool) string {
	// Get viewport from map (will be modified by helper via pointer)
	vp := m.coordinatorPane.viewports[viewportKey]

	// Build title and metrics based on fullscreen mode
	var leftTitle, metricsDisplay, bottomLeft string
	var hasNewContent bool
	var borderColor lipgloss.AdaptiveColor

	if fullscreen {
		// Fullscreen: simplified title, no metrics or new content indicator
		leftTitle = "● COORDINATOR"
		metricsDisplay = ""
		hasNewContent = false
		borderColor = CoordinatorColor
	} else {
		// Normal: dynamic status title with metrics
		leftTitle = m.buildCoordinatorTitle()
		if m.coordinatorMetrics != nil && m.coordinatorMetrics.TokensUsed > 0 {
			metricsDisplay = m.coordinatorMetrics.FormatContextDisplay()
		}
		hasNewContent = m.coordinatorPane.hasNewContent
		// Determine border color based on status
		switch m.coordinatorStatus {
		case events.ProcessStatusWorking:
			borderColor = workerWorkingBorderColor
		case events.ProcessStatusStopped, events.ProcessStatusRetired, events.ProcessStatusFailed:
			borderColor = workerStoppedBorderColor
		default:
			borderColor = styles.BorderDefaultColor
		}
	}

	// Add queue count if any messages are queued
	if m.coordinatorPane.queueCount > 0 {
		bottomLeft = QueuedCountStyle.Render(fmt.Sprintf("[%d queued]", m.coordinatorPane.queueCount))
	}

	// Use panes.ScrollablePane helper for viewport setup, padding, and auto-scroll
	result := panes.ScrollablePane(width, height, panes.ScrollableConfig{
		Viewport:       &vp,
		ContentDirty:   m.coordinatorPane.contentDirty,
		HasNewContent:  hasNewContent,
		MetricsDisplay: metricsDisplay,
		LeftTitle:      leftTitle,
		BottomLeft:     bottomLeft,
		TitleColor:     CoordinatorColor,
		BorderColor:    borderColor,
	}, m.renderCoordinatorContent)

	// Store updated viewport back to map (helper modified via pointer)
	m.coordinatorPane.viewports[viewportKey] = vp

	return result
}

// buildCoordinatorTitle builds the left title with status indicator for the coordinator pane.
// When port is available (> 0), it appends the port in muted style: "● COORDINATOR (8467)"
func (m Model) buildCoordinatorTitle() string {
	var indicator string
	var indicatorStyle lipgloss.Style

	// Use coordinatorStatus from v2 events instead of calling m.coord.Status()
	switch m.coordinatorStatus {
	case events.ProcessStatusReady, events.ProcessStatusWorking:
		// When ready or working, show indicator based on activity
		if m.coordinatorWorking {
			indicator = "●"
			indicatorStyle = workerWorkingStyle // Blue - actively working
		} else {
			indicator = "○"
			indicatorStyle = workerReadyStyle // Green - ready/waiting for input
		}
	case events.ProcessStatusPaused:
		indicator = "⏸"
		indicatorStyle = statusPausedStyle
	case events.ProcessStatusStopped:
		indicator = "⚠"
		indicatorStyle = workerStoppedStyle // Yellow/amber - stopped (can be resumed)
	case events.ProcessStatusFailed, events.ProcessStatusRetired:
		indicator = "✗"
		indicatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B"))
	default:
		// StatusPending, StatusStarting, or no status yet - show empty circle
		indicator = "○"
		indicatorStyle = lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
	}

	title := fmt.Sprintf("%s COORDINATOR", indicatorStyle.Render(indicator))

	// Append port in muted style if available
	if m.mcpPort > 0 {
		portDisplay := TitleContextStyle.Render(fmt.Sprintf("(%d)", m.mcpPort))
		title = fmt.Sprintf("%s %s", title, portDisplay)
	}

	return title
}

// renderCoordinatorContent builds the pre-wrapped content string for the viewport.
func (m Model) renderCoordinatorContent(wrapWidth int) string {
	return renderChatContent(m.coordinatorPane.messages, wrapWidth, ChatRenderConfig{
		AgentLabel: "Coordinator",
		AgentColor: coordinatorMessageStyle.GetForeground().(lipgloss.AdaptiveColor),
	})
}
