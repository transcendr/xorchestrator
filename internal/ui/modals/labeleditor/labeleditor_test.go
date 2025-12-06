package labeleditor

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLabels() []string {
	return []string{"bug", "feature", "ui"}
}

func TestLabelEditor_New(t *testing.T) {
	labels := testLabels()
	m := New("test-123", labels)

	assert.Equal(t, "test-123", m.issueID, "expected issueID to be set")

	// Verify all labels are visible in view
	view := m.View()
	for _, label := range labels {
		assert.Contains(t, view, label, "expected view to contain label %s", label)
	}
	// All labels should be checked (enabled by default)
	assert.Contains(t, view, "[x]", "expected enabled checkboxes in view")
}

func TestLabelEditor_New_EmptyLabels(t *testing.T) {
	m := New("test-123", []string{})

	assert.Equal(t, "test-123", m.issueID, "expected issueID to be set")
	view := m.View()
	assert.Contains(t, view, "no items", "expected empty state message")
}

func TestLabelEditor_AddLabel(t *testing.T) {
	m := New("test-123", []string{"existing"})

	// Tab to input field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Type a new label character by character
	for _, r := range "new-label" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press enter to add
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Tab to submit button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Press enter to save
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd, "expected command to be returned")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg")
	assert.Contains(t, saveMsg.Labels, "existing", "expected 'existing' in SaveMsg")
	assert.Contains(t, saveMsg.Labels, "new-label", "expected 'new-label' in SaveMsg")
}

func TestLabelEditor_ToggleLabel_Space(t *testing.T) {
	m := New("test-123", testLabels())

	// Navigate down to "feature" (second item)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// Press space to toggle off "feature"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	// Navigate to submit button (Tab to input, Tab to submit)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Press enter to save
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd, "expected command to be returned")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg")
	assert.Contains(t, saveMsg.Labels, "bug", "expected 'bug' in SaveMsg")
	assert.Contains(t, saveMsg.Labels, "ui", "expected 'ui' in SaveMsg")
	assert.NotContains(t, saveMsg.Labels, "feature", "expected 'feature' NOT in SaveMsg")
}

func TestLabelEditor_ToggleLabel_Enter(t *testing.T) {
	m := New("test-123", testLabels())

	// Press enter on first item (bug) to toggle it off
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Navigate to submit button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Press enter to save
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd, "expected command to be returned")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg")
	assert.NotContains(t, saveMsg.Labels, "bug", "expected 'bug' NOT in SaveMsg")
	assert.Contains(t, saveMsg.Labels, "feature", "expected 'feature' in SaveMsg")
	assert.Contains(t, saveMsg.Labels, "ui", "expected 'ui' in SaveMsg")
}

func TestLabelEditor_Navigation_JK(t *testing.T) {
	m := New("test-123", testLabels())

	// Initial state - should have cursor on first item
	view := m.View()
	// The ">" cursor should be on bug
	assert.Contains(t, view, ">[x] bug", "expected cursor on first label")

	// Navigate down with 'j'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	view = m.View()
	// The cursor should now be on feature
	assert.Contains(t, view, ">[x] feature", "expected cursor on second label")

	// Navigate down with 'j' again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	view = m.View()
	assert.Contains(t, view, ">[x] ui", "expected cursor on third label")
}

func TestLabelEditor_Navigation_Tab(t *testing.T) {
	m := New("test-123", testLabels())

	// View should start with focus indicator on list
	view := m.View()
	assert.Contains(t, view, ">[x]", "expected cursor indicator on labels")

	// Tab to input section
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	view = m.View()
	// Input section should now have focus indicator
	assert.Contains(t, view, ">Enter label", "expected cursor on input")

	// Tab to submit button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	view = m.View()
	// Check that Save button appears focused (different styling)
	assert.Contains(t, view, "Save", "expected Save button visible")
}

func TestLabelEditor_DuplicatePrevention(t *testing.T) {
	m := New("test-123", testLabels())

	// Tab to input field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Try to add existing label "bug"
	for _, r := range "bug" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press enter to try to add
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Navigate to submit and save
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd, "expected command to be returned")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg")

	// Count occurrences of "bug"
	count := 0
	for _, label := range saveMsg.Labels {
		if label == "bug" {
			count++
		}
	}
	assert.Equal(t, 1, count, "expected exactly one 'bug' label (duplicate rejected)")
}

func TestLabelEditor_EmptyInput(t *testing.T) {
	m := New("test-123", testLabels())

	// Tab to input field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Try to add empty label (just press enter)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Navigate to submit and save
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd, "expected command to be returned")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg")
	assert.Len(t, saveMsg.Labels, 3, "expected labels unchanged (empty rejected)")
}

func TestLabelEditor_Cancel_Esc(t *testing.T) {
	m := New("test-123", testLabels())

	// Press Esc
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	// Should emit CancelMsg
	require.NotNil(t, cmd, "expected command to be returned")
	msg := cmd()
	_, ok := msg.(CancelMsg)
	assert.True(t, ok, "expected CancelMsg to be returned")
}

func TestLabelEditor_Save_OnlyEnabledLabels(t *testing.T) {
	m := New("test-123", testLabels())

	// Navigate to "feature" (index 1) and toggle off
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	// Navigate to submit button and save
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should emit SaveMsg with only enabled labels
	require.NotNil(t, cmd, "expected command to be returned")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	assert.True(t, ok, "expected SaveMsg to be returned")
	assert.Equal(t, "test-123", saveMsg.IssueID, "expected correct issue ID")
	assert.Len(t, saveMsg.Labels, 2, "expected 2 enabled labels")
	assert.Contains(t, saveMsg.Labels, "bug", "expected 'bug' in SaveMsg")
	assert.Contains(t, saveMsg.Labels, "ui", "expected 'ui' in SaveMsg")
	assert.NotContains(t, saveMsg.Labels, "feature", "expected 'feature' NOT in SaveMsg")
}

func TestLabelEditor_Save_WithNewLabel(t *testing.T) {
	m := New("test-123", testLabels())

	// Tab to input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Add a new label
	for _, r := range "new-label" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Navigate to submit button and save
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should emit SaveMsg with all labels (including new one)
	require.NotNil(t, cmd, "expected command to be returned")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	assert.True(t, ok, "expected SaveMsg to be returned")
	assert.Len(t, saveMsg.Labels, 4, "expected 4 labels in SaveMsg")
	assert.Contains(t, saveMsg.Labels, "new-label", "expected new-label in SaveMsg")
}

func TestLabelEditor_Init(t *testing.T) {
	m := New("test-123", testLabels())
	cmd := m.Init()
	assert.Nil(t, cmd, "expected Init to return nil")
}

func TestLabelEditor_SetSize(t *testing.T) {
	m := New("test-123", testLabels())

	m = m.SetSize(120, 40)
	// Verify that SetSize returns a new model (immutability)
	m2 := m.SetSize(80, 24)
	_ = m2 // Just verify it doesn't panic
}

func TestLabelEditor_View(t *testing.T) {
	m := New("test-123", testLabels()).SetSize(80, 24)
	view := m.View()

	// Should contain title
	assert.Contains(t, view, "Edit Labels", "expected view to contain title")

	// Should contain labels
	assert.Contains(t, view, "bug", "expected view to contain 'bug'")
	assert.Contains(t, view, "feature", "expected view to contain 'feature'")
	assert.Contains(t, view, "ui", "expected view to contain 'ui'")

	// Should have checkboxes (all enabled by default)
	assert.Contains(t, view, "[x]", "expected view to contain enabled checkboxes")

	// Should have input hint and Save button
	assert.Contains(t, view, "Enter to add", "expected view to contain input hint")
	assert.Contains(t, view, "Save", "expected view to contain Save button")
}

func TestLabelEditor_View_WithDisabled(t *testing.T) {
	m := New("test-123", testLabels()).SetSize(80, 24)

	// Toggle first label off
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	view := m.View()

	// Should show unchecked checkbox
	assert.Contains(t, view, "[ ]", "expected view to contain disabled checkbox")
}

func TestLabelEditor_View_Empty(t *testing.T) {
	m := New("test-123", []string{}).SetSize(80, 24)
	view := m.View()

	assert.Contains(t, view, "no items", "expected empty state message")
}

func TestLabelEditor_SpaceInInput(t *testing.T) {
	m := New("test-123", []string{})

	// Tab to input field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Type "hello world" with space
	for _, r := range "hello world" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press enter to add
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Navigate to submit and save
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd, "expected command to be returned")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg")
	assert.Contains(t, saveMsg.Labels, "hello world", "expected 'hello world' label with space")
}

// TestLabelEditor_View_Golden uses teatest golden file comparison
// Run with -update flag to update golden files: go test -update ./internal/ui/modals/labeleditor/...
func TestLabelEditor_View_Golden(t *testing.T) {
	m := New("test-123", testLabels()).SetSize(80, 24)

	view := m.View()

	teatest.RequireEqualOutput(t, []byte(view))
}
