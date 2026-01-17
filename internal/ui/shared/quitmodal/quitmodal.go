// Package quitmodal provides a reusable quit confirmation modal component.
// It wraps the existing modal.Model with purpose-built API for quit confirmations,
// including visibility management and a Result enum that lets callers decide
// their own exit behavior (tea.Quit vs custom quit messages).
package quitmodal

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zjrosen/xorchestrator/internal/ui/shared/modal"
)

// Result indicates the outcome of modal interaction.
type Result int

const (
	ResultNone   Result = iota // No action needed (modal still visible or not visible)
	ResultQuit                 // User confirmed quit
	ResultCancel               // User cancelled/dismissed
)

// Config controls quit modal appearance.
type Config struct {
	Title   string // e.g., "Exit Application?" or "Exit Orchestration Mode?"
	Message string // e.g., "Are you sure you want to quit?"
}

// Model represents the quit confirmation modal state.
type Model struct {
	modal   modal.Model
	visible bool
	width   int
	height  int
}

// New creates a new quit modal with the given configuration.
// The modal starts hidden; call Show() to display it.
func New(cfg Config) Model {
	return Model{
		modal: modal.New(modal.Config{
			Title:          cfg.Title,
			Message:        cfg.Message,
			ConfirmVariant: modal.ButtonDanger,
		}),
		visible: false,
	}
}

// Show makes the modal visible and applies cached dimensions.
func (m *Model) Show() {
	m.visible = true
	// Apply cached dimensions to inner modal for correct overlay centering
	m.modal.SetSize(m.width, m.height)
}

// Hide dismisses the modal.
func (m *Model) Hide() {
	m.visible = false
}

// IsVisible returns whether the modal is currently displayed.
func (m Model) IsVisible() bool {
	return m.visible
}

// SetSize updates viewport dimensions for overlay centering.
// Dimensions are cached and applied to the inner modal when Show() is called.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	// Also update inner modal if currently visible
	if m.visible {
		m.modal.SetSize(width, height)
	}
}

// Update processes messages and returns the result.
// Returns ResultQuit when user confirms, ResultCancel when dismissed.
// Returns ResultNone when not visible or for messages that don't resolve the modal.
//
// The caller should handle the Result:
//
//	switch result {
//	case quitmodal.ResultQuit:
//	    return m, tea.Quit  // or custom quit message
//	case quitmodal.ResultCancel:
//	    return m, nil
//	}
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Result) {
	if !m.visible {
		return m, nil, ResultNone
	}

	// Handle modal result messages (from inner modal's commands)
	switch msg.(type) {
	case modal.SubmitMsg:
		m.visible = false
		return m, nil, ResultQuit
	case modal.CancelMsg:
		m.visible = false
		return m, nil, ResultCancel
	}

	// Handle key messages directly for immediate response
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyCtrlC:
			// Force-quit (Ctrl+C) when modal visible
			m.visible = false
			return m, nil, ResultQuit
		case tea.KeyEscape:
			// Cancel/dismiss modal
			m.visible = false
			return m, nil, ResultCancel
		}
		// Enter is delegated to inner modal to respect button focus
	}

	// Delegate to inner modal for other messages (navigation, etc.)
	var cmd tea.Cmd
	m.modal, cmd = m.modal.Update(msg)

	return m, cmd, ResultNone
}

// Overlay renders the modal on top of the given background.
func (m Model) Overlay(bg string) string {
	return m.modal.Overlay(bg)
}

// Init returns the initial command (nil, since quit modals have no inputs).
func (m Model) Init() tea.Cmd {
	return nil
}
