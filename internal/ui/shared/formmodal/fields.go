package formmodal

import "github.com/charmbracelet/bubbles/textinput"

// fieldState holds runtime state for a field.
type fieldState struct {
	config FieldConfig // Original configuration

	// Text field state
	textInput textinput.Model

	// Color field state (Phase 3)
	selectedColor string // Current hex color

	// List field state (Phase 4)
	listCursor int        // Cursor position within list
	listItems  []listItem // Items with selection state
}

// listItem tracks selection state for list items.
type listItem struct {
	label    string
	value    string
	selected bool
}

// newFieldState creates a fieldState from a FieldConfig.
func newFieldState(cfg FieldConfig) fieldState {
	fs := fieldState{
		config: cfg,
	}

	switch cfg.Type {
	case FieldTypeText:
		ti := textinput.New()
		ti.Placeholder = cfg.Placeholder
		ti.Prompt = ""
		if cfg.MaxLength > 0 {
			ti.CharLimit = cfg.MaxLength
		}
		if cfg.InitialValue != "" {
			ti.SetValue(cfg.InitialValue)
		}
		ti.Width = 36 // Default width, fits in 50-wide modal
		fs.textInput = ti

	case FieldTypeColor:
		fs.selectedColor = cfg.InitialColor
		if fs.selectedColor == "" {
			fs.selectedColor = "#73F59F" // Default green
		}

	case FieldTypeList, FieldTypeSelect:
		fs.listItems = make([]listItem, len(cfg.Options))
		for i, opt := range cfg.Options {
			fs.listItems[i] = listItem{
				label:    opt.Label,
				value:    opt.Value,
				selected: opt.Selected,
			}
		}
	}

	return fs
}

// value extracts the current value from the field state.
func (fs *fieldState) value() any {
	switch fs.config.Type {
	case FieldTypeText:
		return fs.textInput.Value()

	case FieldTypeColor:
		return fs.selectedColor

	case FieldTypeList:
		// Return slice of selected values
		var selected []string
		for _, item := range fs.listItems {
			if item.selected {
				selected = append(selected, item.value)
			}
		}
		return selected

	case FieldTypeSelect:
		// Return single selected value
		if fs.listCursor >= 0 && fs.listCursor < len(fs.listItems) {
			return fs.listItems[fs.listCursor].value
		}
		return ""
	}
	return nil
}
