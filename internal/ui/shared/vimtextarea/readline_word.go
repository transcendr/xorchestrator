package vimtextarea

import "unicode"

// readline_word.go provides word boundary detection for readline/bash/emacs semantics.
//
// Key difference from vim word motion:
// - Vim: Punctuation is its own word (e.g., "foo.bar" has 3 words: "foo", ".", "bar")
// - Readline: Punctuation is a delimiter (e.g., "foo.bar" has 2 words: "foo", "bar")
//
// Word characters: letters, numbers, underscore ([\p{L}\p{N}_])
// Delimiters: whitespace, punctuation, symbols

// isReadlineWordChar returns true for readline word characters.
// Only letters, numbers, and underscore count as word chars.
// Punctuation and whitespace are treated as delimiters.
func isReadlineWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_'
}

// readlineFindPrevWordStart finds the start of the previous word
// using readline/bash/Emacs semantics (punctuation as separator).
//
// Algorithm:
// - If at start of a word (current is word char, prev is delimiter), move to previous word
// - If in middle of word, move to start of current word
// - If on delimiter, skip delimiters backward, then move to start of previous word
//
// Returns the position at the start of the word (cursor should land here).
func readlineFindPrevWordStart(content []string, pos Position) Position {
	if len(content) == 0 {
		return Position{Row: 0, Col: 0}
	}

	// Edge case: if at start of document
	if pos.Row == 0 && pos.Col == 0 {
		return pos
	}

	// Helper: move to previous grapheme
	movePrev := func(p Position) Position {
		if p.Col > 0 {
			return Position{Row: p.Row, Col: p.Col - 1}
		}
		// Move to end of previous line
		if p.Row > 0 {
			return Position{Row: p.Row - 1, Col: GraphemeCount(content[p.Row-1])}
		}
		return p // Already at start
	}

	// Helper: get character at position
	charAt := func(p Position) rune {
		if p.Row >= len(content) || p.Row < 0 {
			return ' ' // Treat out of bounds as delimiter
		}
		line := content[p.Row]
		graphemeCount := GraphemeCount(line)
		if p.Col >= graphemeCount || p.Col < 0 {
			return ' ' // Treat out of bounds as delimiter
		}
		// Get the grapheme at this position
		grapheme := GraphemeAt(line, p.Col)
		// Get first rune of grapheme for word char detection
		if len(grapheme) > 0 {
			return []rune(grapheme)[0]
		}
		return ' '
	}

	// Start from current position
	currentPos := pos

	// Check if we're at a word boundary (on word char, prev is delimiter)
	// If so, step back once to move to previous word
	if isReadlineWordChar(charAt(currentPos)) {
		prevPos := movePrev(currentPos)
		if prevPos != currentPos && !isReadlineWordChar(charAt(prevPos)) {
			// At word boundary - step back to trigger previous word search
			currentPos = prevPos
		}
	}

	// Phase 1: Skip delimiters backward (if currently on delimiter)
	for !isReadlineWordChar(charAt(currentPos)) {
		prevPos := movePrev(currentPos)
		if prevPos == currentPos {
			// Reached start of document
			return currentPos
		}
		currentPos = prevPos
	}

	// Phase 2: Scan backward over word chars to find start
	for isReadlineWordChar(charAt(currentPos)) {
		prevPos := movePrev(currentPos)
		if prevPos == currentPos {
			// Reached start of document while in word
			return currentPos
		}
		// Check if previous position is still word char
		if !isReadlineWordChar(charAt(prevPos)) {
			// Found start of word
			return currentPos
		}
		currentPos = prevPos
	}

	return currentPos
}

// readlineFindNextWordEnd finds the end of current/next word
// using readline/bash/Emacs semantics.
//
// Algorithm:
// 1. If on word char → scan forward to end of current word
// 2. Else (on delimiter) → skip delimiters forward, then scan to end of next word
//
// Returns the position AFTER the last character of the word (cursor lands after word).
func readlineFindNextWordEnd(content []string, pos Position) Position {
	if len(content) == 0 {
		return Position{Row: 0, Col: 0}
	}

	// Helper: move to next grapheme
	moveNext := func(p Position) Position {
		if p.Row >= len(content) {
			return p // Beyond document
		}
		lineLen := GraphemeCount(content[p.Row])
		if p.Col < lineLen {
			return Position{Row: p.Row, Col: p.Col + 1}
		}
		// Move to start of next line
		if p.Row+1 < len(content) {
			return Position{Row: p.Row + 1, Col: 0}
		}
		return p // At end of document
	}

	// Helper: get character at position
	charAt := func(p Position) rune {
		if p.Row >= len(content) || p.Row < 0 {
			return ' ' // Treat out of bounds as delimiter
		}
		line := content[p.Row]
		graphemeCount := GraphemeCount(line)
		if p.Col >= graphemeCount || p.Col < 0 {
			return ' ' // Treat out of bounds as delimiter
		}
		// Get the grapheme at this position
		grapheme := GraphemeAt(line, p.Col)
		// Get first rune of grapheme for word char detection
		if len(grapheme) > 0 {
			return []rune(grapheme)[0]
		}
		return ' '
	}

	// Helper: check if at end of document
	atEnd := func(p Position) bool {
		return p.Row >= len(content)-1 && p.Col >= GraphemeCount(content[len(content)-1])
	}

	currentPos := pos

	// Phase 1: Skip delimiters forward (if currently on delimiter)
	for !isReadlineWordChar(charAt(currentPos)) {
		if atEnd(currentPos) {
			return currentPos
		}
		currentPos = moveNext(currentPos)
	}

	// Phase 2: Scan forward over word chars to find end
	for isReadlineWordChar(charAt(currentPos)) {
		nextPos := moveNext(currentPos)
		if atEnd(nextPos) {
			// Reached end of document while in word
			return nextPos
		}
		// Check if next position is still word char
		if !isReadlineWordChar(charAt(nextPos)) {
			// Found end of word - return position AFTER last word char
			return nextPos
		}
		currentPos = nextPos
	}

	return currentPos
}
