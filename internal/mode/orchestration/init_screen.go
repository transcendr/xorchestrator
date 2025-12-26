package orchestration

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/ui/shared/chainart"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// spinnerFrames defines the braille spinner animation sequence.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// phaseLabels maps each InitPhase to its display text.
var phaseLabels = map[InitPhase]string{
	InitCreatingWorkspace:    "Creating Workspace",
	InitSpawningCoordinator:  "Spawning Coordinator",
	InitAwaitingFirstMessage: "Coordinator Loaded",
	InitSpawningWorkers:      "Spawning Workers",
	InitWorkersReady:         "Workers Loaded",
}

// phaseOrder defines the order in which phases are displayed.
var phaseOrder = []InitPhase{
	InitCreatingWorkspace,
	InitSpawningCoordinator,
	InitAwaitingFirstMessage,
	InitSpawningWorkers,
	InitWorkersReady,
}

// Phase status indicator styles.
var (
	phaseCompletedStyle = lipgloss.NewStyle().
				Foreground(styles.StatusSuccessColor)

	phaseInProgressStyle = lipgloss.NewStyle().
				Foreground(styles.StatusWarningColor)

	phaseFailedStyle = lipgloss.NewStyle().
				Foreground(styles.StatusErrorColor)

	phasePendingStyle = lipgloss.NewStyle().
				Foreground(styles.TextMutedColor)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(styles.TextPrimaryColor)

	errorTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(styles.StatusErrorColor)

	errorMessageStyle = lipgloss.NewStyle().
				Foreground(styles.StatusErrorColor)

	hintStyle = lipgloss.NewStyle().
			Foreground(styles.TextMutedColor)
)

// phaseToLinkIndex maps InitPhase to chain link index (0-4).
// The 5 chain links map to the 5 loading phases:
//   - Link 0: InitCreatingWorkspace
//   - Link 1: InitSpawningCoordinator
//   - Link 2: InitAwaitingFirstMessage
//   - Link 3: InitSpawningWorkers
//   - Link 4: InitWorkersReady
func phaseToLinkIndex(phase InitPhase) int {
	switch phase {
	case InitCreatingWorkspace:
		return 0
	case InitSpawningCoordinator:
		return 1
	case InitAwaitingFirstMessage:
		return 2
	case InitSpawningWorkers:
		return 3
	case InitWorkersReady:
		return 4
	case InitReady:
		return 5 // All phases complete, show all links colored
	default:
		return 0
	}
}

// renderInitScreen renders the initialization loading screen.
// This displays the chain art header, initialization phases with status indicators,
// and action hints at the bottom.
func (m Model) renderInitScreen() string {
	var lines []string

	// Get current phase from initializer
	currentPhase := m.getInitPhase()

	// Determine if we're in an error/timeout state
	isError := currentPhase == InitFailed || currentPhase == InitTimedOut

	// Calculate completed phases and failed phase for chain art
	completedPhases := phaseToLinkIndex(currentPhase)
	failedPhase := -1 // No failure by default

	if currentPhase == InitFailed || currentPhase == InitTimedOut {
		// Use getFailedPhase to determine which phase failed/timed out
		failedPhaseValue := m.getFailedPhase()
		failedPhase = phaseToLinkIndex(failedPhaseValue)
		// Completed phases are those before the failed phase
		completedPhases = failedPhase
	}

	// Chain art header - progressive coloring based on phase progress
	chainArt := chainart.BuildProgressChainArt(completedPhases, failedPhase)

	// Get chain art width for centering text elements
	chainArtWidth := lipgloss.Width(chainArt)

	// Style for centering text within the chain art width
	centerTextStyle := lipgloss.NewStyle().
		Width(chainArtWidth).
		Align(lipgloss.Center)

	lines = append(lines, chainArt)
	lines = append(lines, "") // Spacing

	// Title
	var title string
	switch currentPhase {
	case InitFailed:
		title = errorTitleStyle.Render("Initialization Failed")
	case InitTimedOut:
		title = errorTitleStyle.Render("Initialization Timed Out")
	default:
		title = titleStyle.Render("Initializing Orchestration")
	}
	lines = append(lines, centerTextStyle.Render(title))
	lines = append(lines, "") // Spacing

	// Build phase lines
	var phaseLines []string
	for _, phase := range phaseOrder {
		label := phaseLabels[phase]
		indicator, labelStyle := m.getPhaseIndicatorAndStyle(phase, currentPhase)

		// Append "(timed out)" suffix for the timeout case
		if currentPhase == InitTimedOut && phase == InitWorkersReady {
			label = label + " (timed out)"
		}

		// Show worker loading progress for WorkersReady phase
		if phase == InitWorkersReady && currentPhase >= InitSpawningWorkers {
			_, _, expected, confirmed := m.getSpinnerData()
			confirmedCount := len(confirmed)
			if expected > 0 && confirmedCount > 0 && confirmedCount < expected {
				label = fmt.Sprintf("%s (%d/%d)", label, confirmedCount, expected)
			}
		}

		line := indicator + " " + labelStyle.Render(label)
		phaseLines = append(phaseLines, line)
	}
	// Join phase lines and center the block
	phaseBlock := strings.Join(phaseLines, "\n")
	lines = append(lines, centerTextStyle.Render(phaseBlock))

	// Error message (if failed)
	if currentPhase == InitFailed {
		if initErr := m.getInitError(); initErr != nil {
			lines = append(lines, "") // Spacing
			errMsg := errorMessageStyle.Render("Error: " + initErr.Error())
			lines = append(lines, centerTextStyle.Render(errMsg))
		}
	}

	// Timeout message
	if currentPhase == InitTimedOut {
		lines = append(lines, "") // Spacing
		timeoutMsg := errorMessageStyle.Render("The workers did not load in time.")
		lines = append(lines, centerTextStyle.Render(timeoutMsg))
		lines = append(lines, centerTextStyle.Render(errorMessageStyle.Render("This may indicate an API or network issue.")))
	}

	lines = append(lines, "") // Spacing
	lines = append(lines, "") // Extra spacing before hints

	// Action hints
	if isError {
		hints := hintStyle.Render("[R] Retry     [ESC] Exit to Kanban")
		lines = append(lines, centerTextStyle.Render(hints))
	} else {
		hints := hintStyle.Render("[ESC] Cancel")
		lines = append(lines, centerTextStyle.Render(hints))
	}

	// Join all lines
	content := strings.Join(lines, "\n")

	// Center the content horizontally and vertically
	contentWidth := lipgloss.Width(content)
	contentHeight := lipgloss.Height(content)

	// Calculate horizontal padding
	leftPad := 0
	if m.width > contentWidth {
		leftPad = (m.width - contentWidth) / 2
	}

	// Calculate vertical padding
	topPad := 0
	if m.height > contentHeight {
		topPad = (m.height - contentHeight) / 2
	}

	// Apply centering
	centeredStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		PaddingLeft(leftPad).
		PaddingTop(topPad)

	return centeredStyle.Render(content)
}

// getPhaseIndicatorAndStyle returns the styled status indicator and label style for a phase.
func (m Model) getPhaseIndicatorAndStyle(phase InitPhase, currentPhase InitPhase) (string, lipgloss.Style) {
	// Handle special states
	if currentPhase == InitReady {
		// All phases complete
		return phaseCompletedStyle.Render("✓"), phaseCompletedStyle
	}

	if currentPhase == InitFailed {
		// Find which phase failed
		failedPhase := m.getFailedPhase()
		if phase < failedPhase {
			return phaseCompletedStyle.Render("✓"), phaseCompletedStyle
		} else if phase == failedPhase {
			return phaseFailedStyle.Render("✗"), phaseFailedStyle
		}
		return phasePendingStyle.Render(" "), phasePendingStyle
	}

	if currentPhase == InitTimedOut {
		// Find which phase timed out
		failedPhase := m.getFailedPhase()
		if phase < failedPhase {
			return phaseCompletedStyle.Render("✓"), phaseCompletedStyle
		} else if phase == failedPhase {
			return phaseFailedStyle.Render("✗"), phaseFailedStyle
		}
		return phasePendingStyle.Render(" "), phasePendingStyle
	}

	// Normal loading state
	if phase < currentPhase {
		return phaseCompletedStyle.Render("✓"), phaseCompletedStyle
	} else if phase == currentPhase {
		// Spinner for current phase
		frame := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
		return phaseInProgressStyle.Render(frame), phaseInProgressStyle
	}

	// Pending phase
	return phasePendingStyle.Render(" "), phasePendingStyle
}

// getInitPhase returns the current initialization phase from the Initializer.
// Returns InitNotStarted if initializer is nil.
func (m Model) getInitPhase() InitPhase {
	if m.initializer != nil {
		return m.initializer.Phase()
	}
	return InitNotStarted
}

// getInitError returns the initialization error from the Initializer.
// Returns nil if initializer is nil.
func (m Model) getInitError() error {
	if m.initializer != nil {
		return m.initializer.Error()
	}
	return nil
}

// getSpinnerData returns spinner data from the Initializer.
// Returns defaults if initializer is nil.
func (m Model) getSpinnerData() (phase InitPhase, workersSpawned, expectedWorkers int, confirmedWorkers map[string]bool) {
	if m.initializer != nil {
		return m.initializer.SpinnerData()
	}
	return InitNotStarted, 0, 4, make(map[string]bool)
}

// getFailedPhase determines which phase failed based on the current state.
// This is called when the phase is InitFailed or InitTimedOut.
func (m Model) getFailedPhase() InitPhase {
	if m.initializer != nil {
		return m.initializer.FailedAtPhase()
	}
	// Initializer should always be present; default to first phase if somehow nil
	return InitCreatingWorkspace
}
