package formmodal

import (
	"perles/internal/ui/shared/colorpicker"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// SubmitMsg is sent when the form is submitted successfully.
//
// The Values map contains all field values keyed by FieldConfig.Key.
// Value types depend on field type:
//   - FieldTypeText: string
//   - FieldTypeColor: string (hex color, e.g., "#73F59F")
//   - FieldTypeList: []string (selected values)
//   - FieldTypeSelect: string (single selected value)
//
// Example:
//
//	switch msg := msg.(type) {
//	case formmodal.SubmitMsg:
//	    name := msg.Values["name"].(string)
//	    color := msg.Values["color"].(string)
//	    views := msg.Values["views"].([]string)
//	}
type SubmitMsg struct {
	Values map[string]any // Field values keyed by FieldConfig.Key
}

// CancelMsg is sent when the form is cancelled (via Esc key or Cancel button).
type CancelMsg struct{}

// Model is the form modal state.
//
// Create a new Model with New(cfg). The Model implements the Bubble Tea
// Model interface with Init(), Update(), and View() methods.
//
// Model is immutable - all methods return a new Model rather than
// modifying the receiver.
type Model struct {
	config        FormConfig
	fields        []fieldState
	focusedIndex  int // Index into fields (-1 = on buttons)
	focusedButton int // 0 = submit, 1 = cancel (when focusedIndex == -1)

	// Viewport for overlay positioning
	width, height int

	// Sub-overlay (e.g., colorpicker)
	colorPicker     colorpicker.Model
	showColorPicker bool

	// Validation error
	validationError string
}

// New creates a new form modal with the given configuration.
//
// The form starts with focus on the first field (or the submit button
// if there are no fields). Use SetSize to set viewport dimensions
// before rendering.
//
// Example:
//
//	cfg := FormConfig{Title: "Edit", Fields: []FieldConfig{...}}
//	m := New(cfg).SetSize(80, 24)
func New(cfg FormConfig) Model {
	m := Model{
		config:       cfg,
		fields:       make([]fieldState, len(cfg.Fields)),
		focusedIndex: 0,
		colorPicker:  colorpicker.New(),
	}

	// Initialize field states
	for i, fieldCfg := range cfg.Fields {
		m.fields[i] = newFieldState(fieldCfg)
	}

	// Focus the first text input if it exists
	if len(m.fields) > 0 && m.fields[0].config.Type == FieldTypeText {
		m.fields[0].textInput.Focus()
	}

	// If no fields, start on submit button
	if len(m.fields) == 0 {
		m.focusedIndex = -1
	}

	return m
}

// Init returns the initial command for the Bubble Tea model.
// Returns a cursor blink command if the first focused field is a text input.
func (m Model) Init() tea.Cmd {
	// Find the first focused text input and return its blink command
	if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
		if m.fields[m.focusedIndex].config.Type == FieldTypeText {
			return textinput.Blink
		}
	}
	return nil
}

// Update handles messages for the form modal.
//
// Returns SubmitMsg when form is submitted successfully, CancelMsg when
// cancelled. Returns nil commands for internal state changes.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	// Handle colorpicker result messages first
	switch msg := msg.(type) {
	case colorpicker.SelectMsg:
		m.showColorPicker = false
		// Update the focused color field with selected color
		if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
			fs := &m.fields[m.focusedIndex]
			if fs.config.Type == FieldTypeColor {
				fs.selectedColor = msg.Hex
			}
		}
		return m, nil

	case colorpicker.CancelMsg:
		m.showColorPicker = false
		return m, nil
	}

	// Forward all messages to colorpicker when it's open
	if m.showColorPicker {
		var cmd tea.Cmd
		m.colorPicker, cmd = m.colorPicker.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	// Forward to focused text input if applicable
	if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
		fs := &m.fields[m.focusedIndex]
		if fs.config.Type == FieldTypeText {
			var cmd tea.Cmd
			fs.textInput, cmd = fs.textInput.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// handleKeyMsg processes keyboard input.
func (m Model) handleKeyMsg(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, func() tea.Msg { return CancelMsg{} }

	case "tab", "ctrl+n":
		m = m.nextField()
		return m, m.blinkCmd()

	case "shift+tab", "ctrl+p":
		m = m.prevField()
		return m, m.blinkCmd()

	case "enter":
		return m.handleEnter()

	case "h", "left":
		// Navigate between buttons when focused on buttons
		if m.focusedIndex == -1 && m.focusedButton == 1 {
			m.focusedButton = 0
			return m, nil
		}

	case "l", "right":
		// Navigate between buttons when focused on buttons
		if m.focusedIndex == -1 && m.focusedButton == 0 {
			m.focusedButton = 1
			return m, nil
		}

	case "j", "down":
		// For list fields, navigate within the list
		if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
			fs := &m.fields[m.focusedIndex]
			if fs.config.Type == FieldTypeList || fs.config.Type == FieldTypeSelect {
				if fs.listCursor < len(fs.listItems)-1 {
					fs.listCursor++
				}
				return m, nil
			}
		}

	case "k", "up":
		// For list fields, navigate within the list
		if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
			fs := &m.fields[m.focusedIndex]
			if fs.config.Type == FieldTypeList || fs.config.Type == FieldTypeSelect {
				if fs.listCursor > 0 {
					fs.listCursor--
				}
				return m, nil
			}
		}

	case " ":
		// For list fields, toggle selection
		if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
			fs := &m.fields[m.focusedIndex]
			if fs.config.Type == FieldTypeList {
				if fs.listCursor >= 0 && fs.listCursor < len(fs.listItems) {
					if fs.config.MultiSelect {
						// Multi-select: toggle current item
						fs.listItems[fs.listCursor].selected = !fs.listItems[fs.listCursor].selected
					} else {
						// Single-select: select current, deselect others
						for i := range fs.listItems {
							fs.listItems[i].selected = (i == fs.listCursor)
						}
					}
				}
				return m, nil
			}
		}
	}

	// Forward to focused text input for character input
	if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
		fs := &m.fields[m.focusedIndex]
		if fs.config.Type == FieldTypeText {
			var cmd tea.Cmd
			fs.textInput, cmd = fs.textInput.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// handleEnter processes Enter key based on current focus.
func (m Model) handleEnter() (Model, tea.Cmd) {
	if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
		fs := &m.fields[m.focusedIndex]

		// Color field: open colorpicker overlay
		if fs.config.Type == FieldTypeColor {
			m.showColorPicker = true
			m.colorPicker = m.colorPicker.SetSelected(fs.selectedColor).SetSize(m.width, m.height)
			return m, nil
		}

		// Other fields: advance to next field
		m = m.nextField()
		return m, m.blinkCmd()
	}

	// On buttons
	switch m.focusedButton {
	case 0: // Submit
		return m.submit()
	case 1: // Cancel
		return m, func() tea.Msg { return CancelMsg{} }
	}

	return m, nil
}

// submit validates and submits the form.
func (m Model) submit() (Model, tea.Cmd) {
	// Clear previous error
	m.validationError = ""

	// Build values map
	values := make(map[string]any)
	for i := range m.fields {
		values[m.fields[i].config.Key] = m.fields[i].value()
	}

	// Run validation if provided
	if m.config.Validate != nil {
		if err := m.config.Validate(values); err != nil {
			m.validationError = err.Error()
			return m, nil
		}
	}

	return m, func() tea.Msg { return SubmitMsg{Values: values} }
}

// nextField moves focus to the next field or button.
func (m Model) nextField() Model {
	if m.focusedIndex >= 0 {
		// Blur current text input if applicable
		if m.fields[m.focusedIndex].config.Type == FieldTypeText {
			m.fields[m.focusedIndex].textInput.Blur()
		}

		if m.focusedIndex < len(m.fields)-1 {
			// Move to next field
			m.focusedIndex++
			if m.fields[m.focusedIndex].config.Type == FieldTypeText {
				m.fields[m.focusedIndex].textInput.Focus()
			}
		} else {
			// Move to submit button
			m.focusedIndex = -1
			m.focusedButton = 0
		}
	} else {
		// On buttons
		if m.focusedButton == 0 {
			m.focusedButton = 1
		} else {
			// Wrap to first field (or stay on buttons if no fields)
			if len(m.fields) > 0 {
				m.focusedIndex = 0
				if m.fields[0].config.Type == FieldTypeText {
					m.fields[0].textInput.Focus()
				}
			} else {
				m.focusedButton = 0
			}
		}
	}
	return m
}

// prevField moves focus to the previous field or button.
func (m Model) prevField() Model {
	if m.focusedIndex >= 0 {
		// Blur current text input if applicable
		if m.fields[m.focusedIndex].config.Type == FieldTypeText {
			m.fields[m.focusedIndex].textInput.Blur()
		}

		if m.focusedIndex > 0 {
			// Move to previous field
			m.focusedIndex--
			if m.fields[m.focusedIndex].config.Type == FieldTypeText {
				m.fields[m.focusedIndex].textInput.Focus()
			}
		} else {
			// Wrap to cancel button
			m.focusedIndex = -1
			m.focusedButton = 1
		}
	} else {
		// On buttons
		if m.focusedButton == 1 {
			m.focusedButton = 0
		} else {
			// Move to last field (or stay on buttons if no fields)
			if len(m.fields) > 0 {
				m.focusedIndex = len(m.fields) - 1
				if m.fields[m.focusedIndex].config.Type == FieldTypeText {
					m.fields[m.focusedIndex].textInput.Focus()
				}
			} else {
				m.focusedButton = 1
			}
		}
	}
	return m
}

// blinkCmd returns the blink command if the currently focused field is a text input.
func (m Model) blinkCmd() tea.Cmd {
	if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
		if m.fields[m.focusedIndex].config.Type == FieldTypeText {
			return textinput.Blink
		}
	}
	return nil
}

// SetSize sets the viewport dimensions for overlay rendering.
// Call this before View() or Overlay() to ensure proper centering.
func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	return m
}
