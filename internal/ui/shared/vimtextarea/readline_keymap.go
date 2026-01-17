package vimtextarea

import tea "github.com/charmbracelet/bubbletea"

// ReadlineKeymap implements the readline/bash/emacs editing paradigm.
// It only operates in Insert mode, providing standard terminal line editing keybindings.
type ReadlineKeymap struct{}

func (ReadlineKeymap) Name() string {
	return "readline"
}

func (ReadlineKeymap) SupportedModes() []Mode {
	return []Mode{ModeInsert}
}

func (ReadlineKeymap) Resolve(msg tea.KeyMsg, mode Mode, pending *PendingCommandBuilder) string {
	// Defensive: readline only operates in Insert mode
	if mode != ModeInsert {
		return ""
	}

	// Map physical keys to canonical command keys
	switch {
	// Submit
	case msg.Type == tea.KeyCtrlD:
		return "<submit>"

	// Newline (Enter inserts newline, not submit)
	case msg.Type == tea.KeyEnter:
		return "<newline>"

	// Arrow keys with special boundary behavior
	case msg.Type == tea.KeyUp:
		return "<up-readline>"
	case msg.Type == tea.KeyDown:
		return "<down-readline>"

	// Word navigation
	case isAltLeft(msg):
		return "<word-left>"
	case isAltRight(msg):
		return "<word-right>"

	// Word deletion
	case isAltBackspace(msg):
		return "<word-backspace>"
	case isAltDelete(msg):
		return "<word-delete>"
	case msg.Type == tea.KeyCtrlW:
		return "<word-backspace>"

	// Emacs-style line navigation
	case msg.String() == "ctrl+a":
		return "<home>"
	case msg.String() == "ctrl+e":
		return "<end>"

	// All other keys fall through to default vim key translation
	// This includes: Home, End, arrow keys, backspace, delete, typing, etc.
	default:
		return keyToString(msg)
	}
}

// isAltLeft detects Alt+Left arrow or Alt+b (macOS "Option as Meta" mode).
func isAltLeft(msg tea.KeyMsg) bool {
	// Primary: Alt+Left arrow
	if msg.Alt && msg.Type == tea.KeyLeft {
		return true
	}
	// macOS "Option as Meta": Alt+b for backward-word
	// Only check if not pasted (avoid false positives from pasted text)
	if !msg.Paste && msg.Alt && msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'b' {
		return true
	}
	return false
}

// isAltRight detects Alt+Right arrow or Alt+f (macOS "Option as Meta" mode).
func isAltRight(msg tea.KeyMsg) bool {
	// Primary: Alt+Right arrow
	if msg.Alt && msg.Type == tea.KeyRight {
		return true
	}
	// macOS "Option as Meta": Alt+f for forward-word
	// Only check if not pasted (avoid false positives from pasted text)
	if !msg.Paste && msg.Alt && msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'f' {
		return true
	}
	return false
}

// isAltBackspace detects Alt+Backspace (macOS Option+Delete).
func isAltBackspace(msg tea.KeyMsg) bool {
	return msg.Alt && msg.Type == tea.KeyBackspace
}

// isAltDelete detects Alt+Delete (macOS Fn+Option+Delete for forward delete).
func isAltDelete(msg tea.KeyMsg) bool {
	return msg.Alt && msg.Type == tea.KeyDelete
}
