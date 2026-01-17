package picker

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"
)

func testOptions() []Option {
	return []Option{
		{Label: "Option 1", Value: "1"},
		{Label: "Option 2", Value: "2"},
		{Label: "Option 3", Value: "3"},
	}
}

func TestPicker_New(t *testing.T) {
	options := testOptions()
	m := New("Test Title", options)

	require.Equal(t, "Test Title", m.config.Title, "expected title to be set")
	require.Len(t, m.config.Options, 3, "expected 3 options")
	require.Equal(t, 0, m.selected, "expected default selection at 0")
}

func TestPicker_SetSelected(t *testing.T) {
	options := testOptions()
	m := New("Test", options)

	// Set valid index
	m = m.SetSelected(2)
	require.Equal(t, 2, m.selected, "expected selection at index 2")

	// Set invalid index (too high) - should not change
	m = m.SetSelected(10)
	require.Equal(t, 2, m.selected, "expected selection unchanged for invalid index")

	// Set invalid index (negative) - should not change
	m = m.SetSelected(-1)
	require.Equal(t, 2, m.selected, "expected selection unchanged for negative index")
}

func TestPicker_Selected(t *testing.T) {
	options := testOptions()
	m := New("Test", options)

	// Default selection
	selected := m.Selected()
	require.Equal(t, "Option 1", selected.Label, "expected first option selected")
	require.Equal(t, "1", selected.Value, "expected first option value")

	// After changing selection
	m = m.SetSelected(1)
	selected = m.Selected()
	require.Equal(t, "Option 2", selected.Label, "expected second option selected")
	require.Equal(t, "2", selected.Value, "expected second option value")
}

func TestPicker_Selected_Empty(t *testing.T) {
	m := New("Test", []Option{})
	selected := m.Selected()
	require.Equal(t, Option{}, selected, "expected empty option for empty picker")
}

func TestPicker_Update_NavigateDown(t *testing.T) {
	options := testOptions()
	m := New("Test", options)

	// Navigate down with 'j'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 1, m.selected, "expected selection at 1 after 'j'")

	// Navigate down with arrow key
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, 2, m.selected, "expected selection at 2 after down arrow")

	// At bottom boundary - should not go past
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 2, m.selected, "expected selection to stay at 2 (boundary)")
}

func TestPicker_Update_NavigateUp(t *testing.T) {
	options := testOptions()
	m := New("Test", options).SetSelected(2)

	// Navigate up with 'k'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 1, m.selected, "expected selection at 1 after 'k'")

	// Navigate up with arrow key
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	require.Equal(t, 0, m.selected, "expected selection at 0 after up arrow")

	// At top boundary - should not go past
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 0, m.selected, "expected selection to stay at 0 (boundary)")
}

func TestPicker_SetSize(t *testing.T) {
	m := New("Test", testOptions())

	m = m.SetSize(120, 40)
	require.Equal(t, 120, m.viewportWidth, "expected viewport width to be 120")
	require.Equal(t, 40, m.viewportHeight, "expected viewport height to be 40")

	// Verify immutability
	m2 := m.SetSize(80, 24)
	require.Equal(t, 80, m2.viewportWidth, "expected new model width to be 80")
	require.Equal(t, 24, m2.viewportHeight, "expected new model height to be 24")
	require.Equal(t, 120, m.viewportWidth, "expected original model width unchanged")
}

func TestPicker_SetBoxWidth(t *testing.T) {
	m := New("Test", testOptions())

	m = m.SetBoxWidth(50)
	require.Equal(t, 50, m.boxWidth, "expected box width to be 50")

	// Verify immutability
	m2 := m.SetBoxWidth(30)
	require.Equal(t, 30, m2.boxWidth, "expected new model box width to be 30")
	require.Equal(t, 50, m.boxWidth, "expected original model box width unchanged")
}

func TestPicker_FindIndexByValue(t *testing.T) {
	options := testOptions()

	// Find existing value
	index := FindIndexByValue(options, "2")
	require.Equal(t, 1, index, "expected index 1 for value '2'")

	// Find first value
	index = FindIndexByValue(options, "1")
	require.Equal(t, 0, index, "expected index 0 for value '1'")

	// Find last value
	index = FindIndexByValue(options, "3")
	require.Equal(t, 2, index, "expected index 2 for value '3'")

	// Not found - returns 0
	index = FindIndexByValue(options, "nonexistent")
	require.Equal(t, 0, index, "expected index 0 for non-existent value")
}

func TestPicker_View(t *testing.T) {
	options := testOptions()
	m := New("Select Option", options).SetSize(80, 24)
	view := m.View()

	// Should contain title
	require.Contains(t, view, "Select Option", "expected view to contain title")

	// Should contain options
	require.Contains(t, view, "Option 1", "expected view to contain Option 1")
	require.Contains(t, view, "Option 2", "expected view to contain Option 2")
	require.Contains(t, view, "Option 3", "expected view to contain Option 3")

	// Should have selection indicator on first option
	require.Contains(t, view, ">", "expected view to contain selection indicator")
}

func TestPicker_View_WithSelection(t *testing.T) {
	options := testOptions()
	m := New("Test", options).SetSelected(1).SetSize(80, 24)
	view := m.View()

	// View should render without error
	require.NotEmpty(t, view, "expected non-empty view")
	require.Contains(t, view, "Option 2", "expected view to contain selected option")
}

func TestPicker_View_Stability(t *testing.T) {
	options := testOptions()
	m := New("Test", options).SetSize(80, 24)

	view1 := m.View()
	view2 := m.View()

	// Same model should produce identical output
	require.Equal(t, view1, view2, "expected stable output from same model")
}

// TestPicker_View_Golden uses teatest golden file comparison
// Run with -update flag to update golden files: go test -update xorchestrator/internal/ui/shared/picker
func TestPicker_View_Golden(t *testing.T) {
	options := testOptions()
	m := New("Select Option", options).SetSize(80, 24)
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestPicker_View_Selected_Golden tests picker with non-default selection
func TestPicker_View_Selected_Golden(t *testing.T) {
	options := testOptions()
	m := New("Select Option", options).SetSelected(1).SetSize(80, 24)
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestPicker_OnSelect_CustomCallback tests that custom OnSelect callback fires on enter
func TestPicker_OnSelect_CustomCallback(t *testing.T) {
	type myMsg struct{ value string }

	m := NewWithConfig(Config{
		Title:   "Test",
		Options: []Option{{Label: "A", Value: "a"}},
		OnSelect: func(opt Option) tea.Msg {
			return myMsg{value: opt.Value}
		},
	})

	// Simulate enter key
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd, "expected command to be returned")
	msg := cmd()
	require.IsType(t, myMsg{}, msg, "expected custom message type")
	require.Equal(t, "a", msg.(myMsg).value, "expected value 'a'")
}

// TestPicker_OnCancel_CustomCallback tests that custom OnCancel callback fires on esc
func TestPicker_OnCancel_CustomCallback(t *testing.T) {
	type cancelledMsg struct{}

	m := NewWithConfig(Config{
		Title:    "Test",
		Options:  []Option{{Label: "A", Value: "a"}},
		OnCancel: func() tea.Msg { return cancelledMsg{} },
	})

	// Simulate esc key
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	require.NotNil(t, cmd, "expected command to be returned")
	msg := cmd()
	require.IsType(t, cancelledMsg{}, msg, "expected custom cancel message type")
}

// TestPicker_DefaultMessages_WhenNoCallbacks tests that SelectMsg/CancelMsg are produced when no callbacks
func TestPicker_DefaultMessages_WhenNoCallbacks(t *testing.T) {
	m := NewWithConfig(Config{
		Title:   "Test",
		Options: []Option{{Label: "A", Value: "a"}},
	})

	// Enter produces SelectMsg
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd, "expected command for enter")
	msg := cmd()
	require.IsType(t, SelectMsg{}, msg, "expected SelectMsg")
	selectMsg := msg.(SelectMsg)
	require.Equal(t, "a", selectMsg.Option.Value, "expected selected option value")

	// Esc produces CancelMsg
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd, "expected command for esc")
	msg = cmd()
	require.IsType(t, CancelMsg{}, msg, "expected CancelMsg")
}

// TestPicker_LegacyNew_StillWorks tests that legacy New() constructor maintains backward compatibility
func TestPicker_LegacyNew_StillWorks(t *testing.T) {
	options := testOptions()
	m := New("Test Title", options)

	// Verify legacy constructor works
	require.Equal(t, "Test Title", m.config.Title)
	require.Len(t, m.config.Options, 3)
	require.Equal(t, 0, m.selected)

	// Verify navigation still works
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 1, m.selected)

	// Verify enter produces default SelectMsg
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)
	msg := cmd()
	require.IsType(t, SelectMsg{}, msg)
	require.Equal(t, "2", msg.(SelectMsg).Option.Value)
}

// TestPicker_OnCancel_QKey tests that 'q' key also triggers cancel
func TestPicker_OnCancel_QKey(t *testing.T) {
	m := NewWithConfig(Config{
		Title:   "Test",
		Options: []Option{{Label: "A", Value: "a"}},
	})

	// Simulate 'q' key
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	require.NotNil(t, cmd, "expected command to be returned")
	msg := cmd()
	require.IsType(t, CancelMsg{}, msg, "expected CancelMsg from 'q' key")
}
