package quitmodal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/xorchestrator/internal/ui/shared/modal"
)

func TestNew_CreatesModalWithCorrectConfig(t *testing.T) {
	cfg := Config{
		Title:   "Exit Application?",
		Message: "Are you sure you want to quit?",
	}

	m := New(cfg)

	// Should start hidden
	require.False(t, m.IsVisible(), "expected modal to start hidden")

	// Verify inner modal has correct config by rendering and checking content
	view := m.modal.View()
	require.Contains(t, view, "Exit Application?", "expected view to contain title")
	require.Contains(t, view, "Are you sure you want to quit?", "expected view to contain message")
	require.Contains(t, view, "Confirm", "expected view to contain Confirm button (ButtonDanger)")
}

func TestShow_SetsVisibleTrue(t *testing.T) {
	m := New(Config{
		Title:   "Test",
		Message: "Test message",
	})

	require.False(t, m.IsVisible(), "expected modal to start hidden")

	m.Show()

	require.True(t, m.IsVisible(), "expected modal to be visible after Show()")
}

func TestHide_SetsVisibleFalse(t *testing.T) {
	m := New(Config{
		Title:   "Test",
		Message: "Test message",
	})

	m.Show()
	require.True(t, m.IsVisible(), "expected modal to be visible after Show()")

	m.Hide()

	require.False(t, m.IsVisible(), "expected modal to be hidden after Hide()")
}

func TestIsVisible_ReflectsState(t *testing.T) {
	m := New(Config{
		Title:   "Test",
		Message: "Test message",
	})

	// Initial state
	require.False(t, m.IsVisible())

	// After Show()
	m.Show()
	require.True(t, m.IsVisible())

	// After Hide()
	m.Hide()
	require.False(t, m.IsVisible())

	// Multiple Show() calls
	m.Show()
	m.Show()
	require.True(t, m.IsVisible())
}

func TestUpdate_ReturnsResultNone_WhenNotVisible(t *testing.T) {
	m := New(Config{
		Title:   "Test",
		Message: "Test message",
	})

	// Modal is not visible, any message should return ResultNone
	newM, cmd, result := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.Equal(t, ResultNone, result, "expected ResultNone when modal not visible")
	require.Nil(t, cmd, "expected nil command when modal not visible")
	require.False(t, newM.IsVisible(), "expected modal to remain not visible")
}

func TestUpdate_ReturnsResultQuit_OnSubmitMsg(t *testing.T) {
	m := New(Config{
		Title:   "Exit?",
		Message: "Are you sure?",
	})
	m.Show()

	newM, _, result := m.Update(modal.SubmitMsg{})

	require.Equal(t, ResultQuit, result, "expected ResultQuit on SubmitMsg")
	require.False(t, newM.IsVisible(), "expected modal to be hidden after submit")
}

func TestUpdate_ReturnsResultCancel_OnCancelMsg(t *testing.T) {
	m := New(Config{
		Title:   "Exit?",
		Message: "Are you sure?",
	})
	m.Show()

	newM, _, result := m.Update(modal.CancelMsg{})

	require.Equal(t, ResultCancel, result, "expected ResultCancel on CancelMsg")
	require.False(t, newM.IsVisible(), "expected modal to be hidden after cancel")
}

func TestUpdate_ReturnsResultQuit_OnCtrlC_ForceQuit(t *testing.T) {
	m := New(Config{
		Title:   "Exit?",
		Message: "Are you sure?",
	})
	m.Show()

	// Ctrl+C while modal is visible = force quit
	newM, _, result := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	require.Equal(t, ResultQuit, result, "expected ResultQuit on Ctrl+C (force quit)")
	require.False(t, newM.IsVisible(), "expected modal to be hidden after force quit")
}

func TestUpdate_DelegatesToInnerModal(t *testing.T) {
	m := New(Config{
		Title:   "Exit?",
		Message: "Are you sure?",
	})
	m.Show()

	// Tab should navigate within the modal (from Save to Cancel button)
	// The inner modal starts with Save button focused
	newM, _, result := m.Update(tea.KeyMsg{Type: tea.KeyTab})

	require.Equal(t, ResultNone, result, "expected ResultNone for navigation keys")
	require.True(t, newM.IsVisible(), "expected modal to remain visible during navigation")
}

func TestSetSize_CachesDimensionsAndPropagates(t *testing.T) {
	m := New(Config{
		Title:   "Test",
		Message: "Test message",
	})

	// Set size before showing
	m.SetSize(100, 50)

	require.Equal(t, 100, m.width, "expected width to be cached")
	require.Equal(t, 50, m.height, "expected height to be cached")

	// Show modal - should apply cached dimensions to inner modal
	m.Show()

	// Verify dimensions were applied by checking overlay works correctly
	bg := "background"
	overlay := m.Overlay(bg)
	require.NotEmpty(t, overlay, "expected overlay to render")
}

func TestSetSize_UpdatesInnerModalWhenVisible(t *testing.T) {
	m := New(Config{
		Title:   "Test",
		Message: "Test message",
	})

	m.Show()

	// SetSize while visible should update inner modal immediately
	m.SetSize(200, 100)

	require.Equal(t, 200, m.width, "expected width to be cached")
	require.Equal(t, 100, m.height, "expected height to be cached")
}

func TestOverlay_DelegatesToInnerModal(t *testing.T) {
	m := New(Config{
		Title:   "Test Modal",
		Message: "Test message",
	})
	m.SetSize(80, 24)
	m.Show()

	bg := "background content"
	overlay := m.Overlay(bg)

	require.Contains(t, overlay, "Test Modal", "expected overlay to contain modal title")
}

func TestInit_ReturnsNil(t *testing.T) {
	m := New(Config{
		Title:   "Test",
		Message: "Test message",
	})

	cmd := m.Init()

	require.Nil(t, cmd, "expected Init() to return nil for quit modals (no inputs)")
}

func TestShow_AppliesCachedDimensions(t *testing.T) {
	m := New(Config{
		Title:   "Test",
		Message: "Test message",
	})

	// Set dimensions before showing
	m.SetSize(120, 40)

	// Hide and show multiple times should still apply dimensions
	m.Show()
	require.True(t, m.IsVisible())

	m.Hide()
	require.False(t, m.IsVisible())

	// Show again - dimensions should still be cached and applied
	m.Show()
	require.True(t, m.IsVisible())
	require.Equal(t, 120, m.width, "expected cached width to persist")
	require.Equal(t, 40, m.height, "expected cached height to persist")
}

func TestUpdate_NavigationWithinModal(t *testing.T) {
	m := New(Config{
		Title:   "Exit Application?",
		Message: "Are you sure you want to quit?",
	})
	m.Show()

	// Navigate with arrow keys (should not trigger quit/cancel)
	m, _, result := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	require.Equal(t, ResultNone, result, "expected ResultNone for right arrow")
	require.True(t, m.IsVisible())

	m, _, result = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	require.Equal(t, ResultNone, result, "expected ResultNone for left arrow")
	require.True(t, m.IsVisible())

	m, _, result = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, ResultNone, result, "expected ResultNone for down arrow")
	require.True(t, m.IsVisible())

	m, _, result = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	require.Equal(t, ResultNone, result, "expected ResultNone for up arrow")
	require.True(t, m.IsVisible())
}

func TestUpdate_EscapeReturnsResultCancel(t *testing.T) {
	m := New(Config{
		Title:   "Exit?",
		Message: "Are you sure?",
	})
	m.Show()

	// Escape key should return ResultCancel immediately
	m, cmd, result := m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	require.Equal(t, ResultCancel, result, "expected ResultCancel on Escape")
	require.Nil(t, cmd, "expected no command")
	require.False(t, m.IsVisible(), "expected modal to be hidden after cancel")
}

func TestUpdate_EnterOnConfirmButton_ReturnsResultQuit(t *testing.T) {
	m := New(Config{
		Title:   "Exit?",
		Message: "Are you sure?",
	})
	m.Show()

	// Enter key delegates to inner modal which returns SubmitMsg command
	// Modal starts with focus on Confirm button
	m, cmd, result := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.Equal(t, ResultNone, result, "first update returns command, not result")
	require.NotNil(t, cmd, "expected command from inner modal")

	// Execute the command to get the SubmitMsg
	msg := cmd()

	// Process the SubmitMsg
	m, _, result = m.Update(msg)
	require.Equal(t, ResultQuit, result, "expected ResultQuit on SubmitMsg")
	require.False(t, m.IsVisible(), "expected modal to be hidden after confirm")
}

func TestUpdate_EnterOnCancelButton_ReturnsResultCancel(t *testing.T) {
	m := New(Config{
		Title:   "Exit?",
		Message: "Are you sure?",
	})
	m.Show()

	// Navigate to Cancel button (right arrow)
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})

	// Enter on Cancel button should return CancelMsg command
	m, cmd, result := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.Equal(t, ResultNone, result, "first update returns command, not result")
	require.NotNil(t, cmd, "expected command from inner modal")

	// Execute the command to get the CancelMsg
	msg := cmd()

	// Process the CancelMsg
	m, _, result = m.Update(msg)
	require.Equal(t, ResultCancel, result, "expected ResultCancel on CancelMsg")
	require.False(t, m.IsVisible(), "expected modal to be hidden after cancel")
}
