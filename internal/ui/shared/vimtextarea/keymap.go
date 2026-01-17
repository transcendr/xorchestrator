package vimtextarea

import tea "github.com/charmbracelet/bubbletea"

// Keymap defines an interface for mapping physical keys to canonical key strings.
// This allows different editing paradigms (vim, readline/emacs, etc.) to coexist
// by translating terminal input into canonical command keys.
type Keymap interface {
	// Name returns the keymap identifier (e.g., "vim", "readline").
	Name() string

	// Resolve translates a physical key message into a canonical key string
	// that the CommandRegistry can dispatch. Returns empty string to ignore the key.
	//
	// Parameters:
	//   msg: The physical key event from the terminal
	//   mode: Current editing mode (Insert, Normal, Visual, etc.)
	//   pending: Pending multi-key command builder (for vim sequences like "d" then "w")
	//
	// Returns:
	//   Canonical key string (e.g., "<submit>", "<word-left>", "a", etc.)
	//   Empty string means "ignore this key" (no command execution)
	Resolve(msg tea.KeyMsg, mode Mode, pending *PendingCommandBuilder) string

	// SupportedModes returns the modes this keymap handles.
	// Keys in unsupported modes may be ignored or delegated to default behavior.
	SupportedModes() []Mode
}

// VimKeymap implements the vim editing paradigm.
// It delegates to the existing keyToString function to preserve 100% vim compatibility.
type VimKeymap struct{}

func (VimKeymap) Name() string {
	return "vim"
}

func (VimKeymap) SupportedModes() []Mode {
	return []Mode{ModeNormal, ModeInsert, ModeVisual, ModeVisualLine, ModeReplace}
}

func (VimKeymap) Resolve(msg tea.KeyMsg, mode Mode, pending *PendingCommandBuilder) string {
	// Delegate to existing vim key translation logic
	// This ensures 100% backward compatibility with existing vim behavior
	return keyToString(msg)
}
