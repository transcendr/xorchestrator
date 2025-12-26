// Package chainart provides the shared chain ASCII art used by nobeads and outdated views.
package chainart

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Chain link color definitions (internal to package).
var (
	link1Color      = lipgloss.Color("#54A0FF") // Blue
	link2Color      = lipgloss.Color("#73F59F") // Green
	brokenColor     = lipgloss.Color("#696969") // Grey - broken/inactive
	link4Color      = lipgloss.Color("#FECA57") // Yellow
	link5Color      = lipgloss.Color("#7D56F4") // Purple
	connectorColor  = lipgloss.Color("#CCCCCC") // Light - consistent connectors
	dimmedColor     = lipgloss.Color("#4A4A4A") // Dimmed grey for pending phases
	completedColor  = lipgloss.Color("#73F59F") // Green - for completed phases in progress mode
	inProgressColor = lipgloss.Color("#FECA57") // Yellow - for current phase in progress mode
	failedColor     = lipgloss.Color("#FF6B6B") // Red - for failed phase broken link
)

// linkColors returns the ordered slice of colors for each chain link (1-5).
// Used for the standard chain art (BuildChainArt, BuildIntactChainArt).
func linkColors() []lipgloss.Color {
	return []lipgloss.Color{link1Color, link2Color, brokenColor, link4Color, link5Color}
}

// Chain link pieces (rendered separately for coloring).
// Each link is rendered independently then joined horizontally.
var (
	// First link (left side open for chain start)
	link1Lines = []string{
		"╔═══════╗",
		"║       ╠",
		"║       ╠",
		"╚═══════╝",
	}

	// Middle links (open on both sides)
	link2Lines = []string{
		"╔═══════╗",
		"╣       ╠",
		"╣       ╠",
		"╚═══════╝",
	}

	// Broken link with radiating crack lines
	brokenLines = []string{
		"    \\│/    ",
		"╔════╲   │   ╱════╗",
		"╣     ╲  │  ╱     ╠",
		"╣     ╱  │  ╲     ╠",
		"╚════╱   │   ╲════╝",
		"    /│\\    ",
	}

	link4Lines = []string{
		"╔═══════╗",
		"╣       ╠",
		"╣       ╠",
		"╚═══════╝",
	}

	// Last link (right side closed for chain end)
	link5Lines = []string{
		"╔═══════╗",
		"╣       ║",
		"╣       ║",
		"╚═══════╝",
	}

	// Connectors between links
	connectorLines = []string{
		"",
		"═══",
		"═══",
		"",
	}
)

// BuildChainArt constructs the colored chain ASCII art.
// The broken link is taller than the regular links, so we need to
// align them properly by adding padding to the shorter links.
func BuildChainArt() string {
	// Create styles from the color definitions
	link1Style := lipgloss.NewStyle().Foreground(link1Color)
	link2Style := lipgloss.NewStyle().Foreground(link2Color)
	brokenStyle := lipgloss.NewStyle().Foreground(brokenColor)
	link4Style := lipgloss.NewStyle().Foreground(link4Color)
	link5Style := lipgloss.NewStyle().Foreground(link5Color)
	connectorStyle := lipgloss.NewStyle().Foreground(connectorColor)
	// Broken link is 6 lines, regular links are 4 lines
	// Add 1 empty line at top and 1 at bottom of regular links for alignment
	paddedLink1 := padLines(link1Lines)
	paddedLink2 := padLines(link2Lines)
	paddedLink4 := padLines(link4Lines)
	paddedLink5 := padLines(link5Lines)
	paddedConnector := padLines(connectorLines)

	// Render each piece with its style
	link1Rendered := renderLines(paddedLink1, link1Style)
	conn1Rendered := renderLines(paddedConnector, connectorStyle)
	link2Rendered := renderLines(paddedLink2, link2Style)
	conn2Rendered := renderLines(paddedConnector, connectorStyle)
	brokenRendered := renderLines(brokenLines, brokenStyle)
	conn3Rendered := renderLines(paddedConnector, connectorStyle)
	link4Rendered := renderLines(paddedLink4, link4Style)
	conn4Rendered := renderLines(paddedConnector, connectorStyle)
	link5Rendered := renderLines(paddedLink5, link5Style)

	// Join horizontally
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		link1Rendered,
		conn1Rendered,
		link2Rendered,
		conn2Rendered,
		brokenRendered,
		conn3Rendered,
		link4Rendered,
		conn4Rendered,
		link5Rendered,
	)
}

// padLines adds empty lines to center the content vertically within 6 lines.
func padLines(lines []string) []string {
	const targetHeight = 6
	if len(lines) >= targetHeight {
		return lines
	}

	// Find the width of the widest line for padding
	maxWidth := 0
	for _, line := range lines {
		if w := lipgloss.Width(line); w > maxWidth {
			maxWidth = w
		}
	}

	// Calculate padding
	diff := targetHeight - len(lines)
	topPad := diff / 2
	bottomPad := diff - topPad

	emptyLine := strings.Repeat(" ", maxWidth)
	result := make([]string, 0, targetHeight)

	// Add top padding
	for range topPad {
		result = append(result, emptyLine)
	}

	// Add original lines
	result = append(result, lines...)

	// Add bottom padding
	for range bottomPad {
		result = append(result, emptyLine)
	}

	return result
}

// renderLines joins lines with newlines and applies a style.
func renderLines(lines []string, style lipgloss.Style) string {
	rendered := strings.Join(lines, "\n")
	return style.Render(rendered)
}

// BuildIntactChainArt constructs the colored chain ASCII art with all links intact.
// Used for loading states where we want to show a complete, unbroken chain.
// Unlike BuildChainArt(), the middle link uses a normal shape instead of the cracked one.
func BuildIntactChainArt() string {
	// Create styles from the color definitions
	link1Style := lipgloss.NewStyle().Foreground(link1Color)
	link2Style := lipgloss.NewStyle().Foreground(link2Color)
	link3Style := lipgloss.NewStyle().Foreground(brokenColor) // Grey - same color as broken
	link4Style := lipgloss.NewStyle().Foreground(link4Color)
	link5Style := lipgloss.NewStyle().Foreground(link5Color)
	connectorStyle := lipgloss.NewStyle().Foreground(connectorColor)

	// All links are 4 lines - no padding needed for intact chain
	// Use the standard middle link shape for the 3rd link
	link3Lines := link2Lines // Same shape as link2 (open on both sides)

	// Connectors without vertical padding (all links same height)
	intactConnector := []string{
		"   ",
		"═══",
		"═══",
		"   ",
	}

	// Render each piece with its style
	link1Rendered := renderLines(link1Lines, link1Style)
	conn1Rendered := renderLines(intactConnector, connectorStyle)
	link2Rendered := renderLines(link2Lines, link2Style)
	conn2Rendered := renderLines(intactConnector, connectorStyle)
	link3Rendered := renderLines(link3Lines, link3Style)
	conn3Rendered := renderLines(intactConnector, connectorStyle)
	link4Rendered := renderLines(link4Lines, link4Style)
	conn4Rendered := renderLines(intactConnector, connectorStyle)
	link5Rendered := renderLines(link5Lines, link5Style)

	// Join horizontally
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		link1Rendered,
		conn1Rendered,
		link2Rendered,
		conn2Rendered,
		link3Rendered,
		conn3Rendered,
		link4Rendered,
		conn4Rendered,
		link5Rendered,
	)
}

// BuildProgressChainArt constructs the colored chain ASCII art based on phase progress.
// Parameters:
//   - completedPhases: number of phases completed (0-4), these links show their full color
//   - failedPhase: 0-4 for which phase failed, -1 if no failure (shows broken link at position)
//
// The 5 chain links map to 4 phases + final state:
//   - Link 0: Phase 0 (InitCreatingWorkspace)
//   - Link 1: Phase 1 (InitSpawningCoordinator)
//   - Link 2: Phase 2 (InitSpawningWorkers)
//   - Link 3: Phase 3 (InitWorkersReady)
//   - Link 4: Final link (always shown when all complete)
//
// During loading: Completed phases are colored, pending phases are dimmed.
// On failure: The broken link appears at the failed phase position.
func BuildProgressChainArt(completedPhases int, failedPhase int) string {
	colors := linkColors()
	dimmedStyle := lipgloss.NewStyle().Foreground(dimmedColor)
	connectorStyle := lipgloss.NewStyle().Foreground(connectorColor)

	// If there's a failed phase, show broken chain at that position
	if failedPhase >= 0 && failedPhase < 5 {
		return buildBrokenChainAtPosition(failedPhase, completedPhases, colors, dimmedStyle, connectorStyle)
	}

	// Normal progress: show intact chain with progressive coloring
	return buildProgressIntactChain(completedPhases, colors, dimmedStyle, connectorStyle)
}

// buildProgressIntactChain builds an intact chain with progressive coloring.
// Completed phases show green, current phase shows yellow, pending phases are dimmed.
func buildProgressIntactChain(completedPhases int, _ []lipgloss.Color, dimmedStyle, connectorStyle lipgloss.Style) string {
	// Link templates (indices 0-4)
	linkTemplates := [][]string{link1Lines, link2Lines, link2Lines, link4Lines, link5Lines}

	// Progress coloring styles
	completedStyle := lipgloss.NewStyle().Foreground(completedColor)
	inProgressStyle := lipgloss.NewStyle().Foreground(inProgressColor)

	// Connectors without vertical padding (all links same height)
	intactConnector := []string{
		"   ",
		"═══",
		"═══",
		"   ",
	}

	var parts []string

	for i := 0; i < 5; i++ {
		// Determine color for this link
		var style lipgloss.Style
		if i < completedPhases {
			// Completed: green
			style = completedStyle
		} else if i == completedPhases {
			// Current phase: yellow (in progress)
			style = inProgressStyle
		} else {
			// Pending: dimmed
			style = dimmedStyle
		}

		parts = append(parts, renderLines(linkTemplates[i], style))

		// Add connector after each link except the last
		if i < 4 {
			parts = append(parts, renderLines(intactConnector, connectorStyle))
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// buildBrokenChainAtPosition builds a chain with a broken link at the specified position.
// Completed phases show green, the broken phase shows red, pending phases are dimmed.
func buildBrokenChainAtPosition(brokenPos, completedPhases int, _ []lipgloss.Color, dimmedStyle, connectorStyle lipgloss.Style) string {
	// Broken link is 6 lines tall, need to pad regular links (4 lines)
	completedStyle := lipgloss.NewStyle().Foreground(completedColor)

	var parts []string

	for i := 0; i < 5; i++ {
		var style lipgloss.Style
		if i < completedPhases {
			// Completed: green
			style = completedStyle
		} else if i == brokenPos {
			// Failed phase: red (broken)
			style = lipgloss.NewStyle().Foreground(failedColor)
		} else {
			// Pending: dimmed
			style = dimmedStyle
		}

		if i == brokenPos {
			// Render broken link (already 6 lines tall)
			parts = append(parts, renderLines(brokenLines, style))
		} else {
			// Regular link (needs padding to 6 lines)
			var linkLines []string
			switch i {
			case 0:
				linkLines = link1Lines
			case 4:
				linkLines = link5Lines
			default:
				linkLines = link2Lines
			}
			paddedLines := padLines(linkLines)
			parts = append(parts, renderLines(paddedLines, style))
		}

		// Add connector after each link except the last
		if i < 4 {
			paddedConnector := padLines([]string{"", "═══", "═══", ""})
			parts = append(parts, renderLines(paddedConnector, connectorStyle))
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}
