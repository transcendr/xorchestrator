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

// ArrowUpReadlineCommand handles Up arrow in readline mode with special first-line behavior.
type ArrowUpReadlineCommand struct {
	MotionBase
}

func (c *ArrowUpReadlineCommand) Execute(m *Model) ExecuteResult {
	// Special readline behavior: if on first line, jump to column 0
	if m.cursorRow == 0 {
		m.cursorCol = 0
		m.preferredCol = 0
		return Executed
	}

	// Otherwise, move to previous line preserving preferredCol
	m.cursorRow--

	// Use preferredCol if set, otherwise use current column
	targetCol := m.preferredCol
	if targetCol == 0 {
		targetCol = m.cursorCol
		m.preferredCol = targetCol // remember for next vertical move
	}

	// Clamp to line length
	lineLen := GraphemeCount(m.content[m.cursorRow])
	if targetCol > lineLen {
		m.cursorCol = lineLen
	} else {
		m.cursorCol = targetCol
	}

	// Clear visual selection
	m.visualAnchor = Position{}

	return Executed
}

func (c *ArrowUpReadlineCommand) Keys() []string {
	return []string{"<up-readline>"}
}

func (c *ArrowUpReadlineCommand) Mode() Mode {
	return ModeInsert
}

func (c *ArrowUpReadlineCommand) ID() string {
	return "motion.arrow_up_readline"
}

// ArrowDownReadlineCommand handles Down arrow in readline mode with special last-line behavior.
type ArrowDownReadlineCommand struct {
	MotionBase
}

func (c *ArrowDownReadlineCommand) Execute(m *Model) ExecuteResult {
	// Special readline behavior: if on last line, jump to end
	if m.cursorRow == len(m.content)-1 {
		m.cursorCol = GraphemeCount(m.content[m.cursorRow])
		m.preferredCol = m.cursorCol
		return Executed
	}

	// Otherwise, move to next line preserving preferredCol
	m.cursorRow++

	targetCol := m.preferredCol
	if targetCol == 0 {
		targetCol = m.cursorCol
		m.preferredCol = targetCol
	}

	lineLen := GraphemeCount(m.content[m.cursorRow])
	if targetCol > lineLen {
		m.cursorCol = lineLen
	} else {
		m.cursorCol = targetCol
	}

	// Clear visual selection
	m.visualAnchor = Position{}

	return Executed
}

func (c *ArrowDownReadlineCommand) Keys() []string {
	return []string{"<down-readline>"}
}

func (c *ArrowDownReadlineCommand) Mode() Mode {
	return ModeInsert
}

func (c *ArrowDownReadlineCommand) ID() string {
	return "motion.arrow_down_readline"
}

// DeleteWordBackwardInsertCommand deletes from cursor to start of previous word.
type DeleteWordBackwardInsertCommand struct {
	DeleteBase
	row         int    // Row where deletion started (original cursor)
	col         int    // Column where deletion started (original cursor)
	startRow    int    // Row where deletion begins (word start)
	startCol    int    // Column where deletion begins (word start)
	deletedText string // Text that was deleted (for undo)
}

func (c *DeleteWordBackwardInsertCommand) Execute(m *Model) ExecuteResult {
	// Capture starting position
	c.row = m.cursorRow
	c.col = m.cursorCol

	// Find previous word start
	startPos := readlineFindPrevWordStart(m.content, Position{Row: m.cursorRow, Col: m.cursorCol})

	// No-op if already at word start
	if startPos.Row == c.row && startPos.Col == c.col {
		return Skipped
	}

	// Store deletion start position
	c.startRow = startPos.Row
	c.startCol = startPos.Col

	// Extract and delete range
	if startPos.Row == c.row {
		// Same line deletion
		line := m.content[c.row]
		c.deletedText = SliceByGraphemes(line, startPos.Col, c.col)
		m.content[c.row] = SliceByGraphemes(line, 0, startPos.Col) + SliceByGraphemes(line, c.col, GraphemeCount(line))
	} else {
		// Multi-line deletion - extract deleted text
		var parts []string
		parts = append(parts, SliceByGraphemes(m.content[startPos.Row], startPos.Col, GraphemeCount(m.content[startPos.Row])))
		for i := startPos.Row + 1; i < c.row; i++ {
			parts = append(parts, m.content[i])
		}
		parts = append(parts, SliceByGraphemes(m.content[c.row], 0, c.col))

		// Join with newlines
		c.deletedText = parts[0]
		for i := 1; i < len(parts); i++ {
			c.deletedText += "\n" + parts[i]
		}

		// Join lines
		newLine := SliceByGraphemes(m.content[startPos.Row], 0, startPos.Col) + SliceByGraphemes(m.content[c.row], c.col, GraphemeCount(m.content[c.row]))
		newContent := make([]string, len(m.content)-(c.row-startPos.Row))
		copy(newContent[:startPos.Row], m.content[:startPos.Row])
		newContent[startPos.Row] = newLine
		copy(newContent[startPos.Row+1:], m.content[c.row+1:])
		m.content = newContent
	}

	// Move cursor to deletion point
	m.cursorRow = startPos.Row
	m.cursorCol = startPos.Col
	m.preferredCol = startPos.Col
	m.visualAnchor = Position{}

	return Executed
}

func (c *DeleteWordBackwardInsertCommand) Undo(m *Model) error {
	// Re-insert deleted text at startRow/startCol (not current cursor)
	if c.deletedText == "" {
		return nil
	}

	// Split deleted text by newlines
	var lines []string
	currentLine := ""
	for _, r := range c.deletedText {
		if r == '\n' {
			lines = append(lines, currentLine)
			currentLine = ""
		} else {
			currentLine += string(r)
		}
	}
	lines = append(lines, currentLine)

	if len(lines) == 1 {
		// Single line - insert at startRow/startCol
		line := m.content[c.startRow]
		m.content[c.startRow] = InsertAtGrapheme(line, c.startCol, c.deletedText)
	} else {
		// Multi-line - split and reconstruct at startRow/startCol
		line := m.content[c.startRow]
		before := SliceByGraphemes(line, 0, c.startCol)
		after := SliceByGraphemes(line, c.startCol, GraphemeCount(line))

		newContent := make([]string, len(m.content)+len(lines)-1)
		copy(newContent[:c.startRow], m.content[:c.startRow])
		newContent[c.startRow] = before + lines[0]
		for i := 1; i < len(lines)-1; i++ {
			newContent[c.startRow+i] = lines[i]
		}
		newContent[c.startRow+len(lines)-1] = lines[len(lines)-1] + after
		copy(newContent[c.startRow+len(lines):], m.content[c.startRow+1:])
		m.content = newContent
	}

	// Restore cursor to original position
	m.cursorRow = c.row
	m.cursorCol = c.col

	return nil
}

func (c *DeleteWordBackwardInsertCommand) Keys() []string {
	return []string{"<word-backspace>"}
}

func (c *DeleteWordBackwardInsertCommand) Mode() Mode {
	return ModeInsert
}

func (c *DeleteWordBackwardInsertCommand) ID() string {
	return "delete.word_backward_insert"
}

// DeleteWordForwardInsertCommand deletes from cursor to end of next word.
type DeleteWordForwardInsertCommand struct {
	DeleteBase
	row         int    // Row where deletion started (original cursor)
	col         int    // Column where deletion started (original cursor)
	deletedText string // Text that was deleted (for undo)
}

func (c *DeleteWordForwardInsertCommand) Execute(m *Model) ExecuteResult {
	// Capture starting position
	c.row = m.cursorRow
	c.col = m.cursorCol

	// Find next word end
	endPos := readlineFindNextWordEnd(m.content, Position{Row: m.cursorRow, Col: m.cursorCol})

	// No-op if already at word end
	if endPos.Row == c.row && endPos.Col == c.col {
		return Skipped
	}

	// Extract and delete range
	if endPos.Row == c.row {
		// Same line deletion
		line := m.content[c.row]
		c.deletedText = SliceByGraphemes(line, c.col, endPos.Col)
		m.content[c.row] = SliceByGraphemes(line, 0, c.col) + SliceByGraphemes(line, endPos.Col, GraphemeCount(line))
	} else {
		// Multi-line deletion - extract deleted text
		var parts []string
		parts = append(parts, SliceByGraphemes(m.content[c.row], c.col, GraphemeCount(m.content[c.row])))
		for i := c.row + 1; i < endPos.Row; i++ {
			parts = append(parts, m.content[i])
		}
		parts = append(parts, SliceByGraphemes(m.content[endPos.Row], 0, endPos.Col))

		// Join with newlines
		c.deletedText = parts[0]
		for i := 1; i < len(parts); i++ {
			c.deletedText += "\n" + parts[i]
		}

		// Join lines
		newLine := SliceByGraphemes(m.content[c.row], 0, c.col) + SliceByGraphemes(m.content[endPos.Row], endPos.Col, GraphemeCount(m.content[endPos.Row]))
		newContent := make([]string, len(m.content)-(endPos.Row-c.row))
		copy(newContent[:c.row], m.content[:c.row])
		newContent[c.row] = newLine
		copy(newContent[c.row+1:], m.content[endPos.Row+1:])
		m.content = newContent
	}

	// Cursor stays at deletion point
	m.cursorRow = c.row
	m.cursorCol = c.col
	m.preferredCol = c.col
	m.visualAnchor = Position{}

	return Executed
}

func (c *DeleteWordForwardInsertCommand) Undo(m *Model) error {
	// Re-insert deleted text at original cursor position (row/col)
	if c.deletedText == "" {
		return nil
	}

	// Split deleted text by newlines
	var lines []string
	currentLine := ""
	for _, r := range c.deletedText {
		if r == '\n' {
			lines = append(lines, currentLine)
			currentLine = ""
		} else {
			currentLine += string(r)
		}
	}
	lines = append(lines, currentLine)

	if len(lines) == 1 {
		// Single line - insert at original position
		line := m.content[c.row]
		m.content[c.row] = InsertAtGrapheme(line, c.col, c.deletedText)
	} else {
		// Multi-line - split and reconstruct at original position
		line := m.content[c.row]
		before := SliceByGraphemes(line, 0, c.col)
		after := SliceByGraphemes(line, c.col, GraphemeCount(line))

		newContent := make([]string, len(m.content)+len(lines)-1)
		copy(newContent[:c.row], m.content[:c.row])
		newContent[c.row] = before + lines[0]
		for i := 1; i < len(lines)-1; i++ {
			newContent[c.row+i] = lines[i]
		}
		newContent[c.row+len(lines)-1] = lines[len(lines)-1] + after
		copy(newContent[c.row+len(lines):], m.content[c.row+1:])
		m.content = newContent
	}

	// Restore cursor to original position
	m.cursorRow = c.row
	m.cursorCol = c.col

	return nil
}

func (c *DeleteWordForwardInsertCommand) Keys() []string {
	return []string{"<word-delete>"}
}

func (c *DeleteWordForwardInsertCommand) Mode() Mode {
	return ModeInsert
}

func (c *DeleteWordForwardInsertCommand) ID() string {
	return "delete.word_forward_insert"
}
