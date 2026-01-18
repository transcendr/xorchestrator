package vimtextarea

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestReadlineKeymap_Resolve_SubmitAndNewline(t *testing.T) {
	km := ReadlineKeymap{}

	// Ctrl+D -> submit
	assert.Equal(t, "<submit>", km.Resolve(tea.KeyMsg{Type: tea.KeyCtrlD}, ModeInsert, nil))

	// Enter -> newline
	assert.Equal(t, "<newline>", km.Resolve(tea.KeyMsg{Type: tea.KeyEnter}, ModeInsert, nil))
}

func TestReadlineKeymap_Resolve_WordNavigation(t *testing.T) {
	km := ReadlineKeymap{}

	// Alt+Left
	assert.Equal(t, "<word-left>", km.Resolve(tea.KeyMsg{Type: tea.KeyLeft, Alt: true}, ModeInsert, nil))

	// Alt+b (macOS Option as Meta)
	assert.Equal(t, "<word-left>", km.Resolve(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}, Alt: true}, ModeInsert, nil))

	// Alt+Right
	assert.Equal(t, "<word-right>", km.Resolve(tea.KeyMsg{Type: tea.KeyRight, Alt: true}, ModeInsert, nil))

	// Alt+f (macOS Option as Meta)
	assert.Equal(t, "<word-right>", km.Resolve(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}, Alt: true}, ModeInsert, nil))
}

func TestReadlineKeymap_Resolve_WordDeletion(t *testing.T) {
	km := ReadlineKeymap{}

	// Alt+Backspace
	assert.Equal(t, "<word-backspace>", km.Resolve(tea.KeyMsg{Type: tea.KeyBackspace, Alt: true}, ModeInsert, nil))

	// Alt+Delete
	assert.Equal(t, "<word-delete>", km.Resolve(tea.KeyMsg{Type: tea.KeyDelete, Alt: true}, ModeInsert, nil))

	// Ctrl+W
	assert.Equal(t, "<word-backspace>", km.Resolve(tea.KeyMsg{Type: tea.KeyCtrlW}, ModeInsert, nil))
}

func TestReadlineKeymap_Resolve_LineNavigation(t *testing.T) {
	km := ReadlineKeymap{}

	// Test that Ctrl+A/E map to home/end via keyToString fallback
	// These are handled by the keymap's default case
	assert.Equal(t, "<home>", km.Resolve(tea.KeyMsg{Type: tea.KeyCtrlA}, ModeInsert, nil))
	assert.Equal(t, "<end>", km.Resolve(tea.KeyMsg{Type: tea.KeyCtrlE}, ModeInsert, nil))
}

func TestReadlineKeymap_Resolve_NonInsertMode(t *testing.T) {
	km := ReadlineKeymap{}

	// All keys should return empty string in non-Insert modes
	assert.Equal(t, "", km.Resolve(tea.KeyMsg{Type: tea.KeyCtrlD}, ModeNormal, nil))
	assert.Equal(t, "", km.Resolve(tea.KeyMsg{Type: tea.KeyEnter}, ModeVisual, nil))
	assert.Equal(t, "", km.Resolve(tea.KeyMsg{Type: tea.KeyLeft, Alt: true}, ModeReplace, nil))
}

func TestReadlineKeymap_Resolve_PasteFlag(t *testing.T) {
	km := ReadlineKeymap{}

	// Alt+b with Paste flag should NOT trigger word navigation (avoid false positives)
	assert.NotEqual(t, "<word-left>", km.Resolve(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}, Alt: true, Paste: true}, ModeInsert, nil))
}
