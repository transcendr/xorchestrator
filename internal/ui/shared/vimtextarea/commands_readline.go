package vimtextarea

// commands_readline.go contains readline/bash/emacs style editing commands
// that operate in Insert mode only.

// MoveWordBackwardInsertCommand moves the cursor to the start of the previous word
// using readline/bash semantics (punctuation as separator).
type MoveWordBackwardInsertCommand struct {
	MotionBase
}

func (c *MoveWordBackwardInsertCommand) Execute(m *Model) ExecuteResult {
	// Find previous word start
	newPos := readlineFindPrevWordStart(m.content, Position{Row: m.cursorRow, Col: m.cursorCol})

	// Update cursor position
	m.cursorRow = newPos.Row
	m.cursorCol = newPos.Col

	// Update preferredCol for vertical movement
	m.preferredCol = newPos.Col

	// Clear any visual selection
	m.visualAnchor = Position{}

	return Executed
}

func (c *MoveWordBackwardInsertCommand) Keys() []string {
	return []string{"<word-left>"}
}

func (c *MoveWordBackwardInsertCommand) Mode() Mode {
	return ModeInsert
}

func (c *MoveWordBackwardInsertCommand) ID() string {
	return "move.word_backward_insert"
}

// MoveWordForwardInsertCommand moves the cursor to the end of the current/next word
// using readline/bash semantics (punctuation as separator).
type MoveWordForwardInsertCommand struct {
	MotionBase
}

func (c *MoveWordForwardInsertCommand) Execute(m *Model) ExecuteResult {
	// Find next word end
	newPos := readlineFindNextWordEnd(m.content, Position{Row: m.cursorRow, Col: m.cursorCol})

	// Update cursor position
	m.cursorRow = newPos.Row
	m.cursorCol = newPos.Col

	// Update preferredCol for vertical movement
	m.preferredCol = newPos.Col

	// Clear any visual selection
	m.visualAnchor = Position{}

	return Executed
}

func (c *MoveWordForwardInsertCommand) Keys() []string {
	return []string{"<word-right>"}
}

func (c *MoveWordForwardInsertCommand) Mode() Mode {
	return ModeInsert
}

func (c *MoveWordForwardInsertCommand) ID() string {
	return "move.word_forward_insert"
}
