package vimtextarea

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadlineFindPrevWordStart_Basic(t *testing.T) {
	content := []string{"hello world"}

	// From end of "world" (pos 11) -> start of "world" (pos 6)
	result := readlineFindPrevWordStart(content, Position{Row: 0, Col: 11})
	assert.Equal(t, Position{Row: 0, Col: 6}, result)

	// From "w" in "world" (pos 6) -> start of "hello" (pos 0)
	result = readlineFindPrevWordStart(content, Position{Row: 0, Col: 6})
	assert.Equal(t, Position{Row: 0, Col: 0}, result)
}

func TestReadlineFindPrevWordStart_Punctuation(t *testing.T) {
	// Punctuation treated as delimiter
	content := []string{"foo.bar"}

	// From end of "bar" -> start of "bar"
	result := readlineFindPrevWordStart(content, Position{Row: 0, Col: 7})
	assert.Equal(t, Position{Row: 0, Col: 4}, result)

	// From "." -> start of "foo"
	result = readlineFindPrevWordStart(content, Position{Row: 0, Col: 3})
	assert.Equal(t, Position{Row: 0, Col: 0}, result)
}

func TestReadlineFindPrevWordStart_Underscore(t *testing.T) {
	// Underscore is word char
	content := []string{"snake_case"}

	// From end -> should treat as single word
	result := readlineFindPrevWordStart(content, Position{Row: 0, Col: 10})
	assert.Equal(t, Position{Row: 0, Col: 0}, result)
}

func TestReadlineFindNextWordEnd_Basic(t *testing.T) {
	content := []string{"hello world"}

	// From start of "hello" -> end of "hello" (after 'o')
	result := readlineFindNextWordEnd(content, Position{Row: 0, Col: 0})
	assert.Equal(t, Position{Row: 0, Col: 5}, result)

	// From space -> end of "world"
	result = readlineFindNextWordEnd(content, Position{Row: 0, Col: 5})
	assert.Equal(t, Position{Row: 0, Col: 11}, result)
}

func TestReadlineFindNextWordEnd_Punctuation(t *testing.T) {
	content := []string{"foo.bar"}

	// From start of "foo" -> end of "foo" (before '.')
	result := readlineFindNextWordEnd(content, Position{Row: 0, Col: 0})
	assert.Equal(t, Position{Row: 0, Col: 3}, result)

	// From "." -> end of "bar"
	result = readlineFindNextWordEnd(content, Position{Row: 0, Col: 3})
	assert.Equal(t, Position{Row: 0, Col: 7}, result)
}

func TestReadlineFindPrevWordStart_Unicode(t *testing.T) {
	content := []string{"café bar"}

	// From end of "bar" -> start of "bar"
	result := readlineFindPrevWordStart(content, Position{Row: 0, Col: 8})
	assert.Equal(t, Position{Row: 0, Col: 5}, result)

	// From space -> start of "café"
	result = readlineFindPrevWordStart(content, Position{Row: 0, Col: 5})
	assert.Equal(t, Position{Row: 0, Col: 0}, result)
}

func TestReadlineWordBoundary_EdgeCases(t *testing.T) {
	// Start of document
	content := []string{"hello"}
	result := readlineFindPrevWordStart(content, Position{Row: 0, Col: 0})
	assert.Equal(t, Position{Row: 0, Col: 0}, result)

	// End of document
	result = readlineFindNextWordEnd(content, Position{Row: 0, Col: 5})
	assert.Equal(t, Position{Row: 0, Col: 5}, result)

	// Empty line
	content = []string{""}
	result = readlineFindPrevWordStart(content, Position{Row: 0, Col: 0})
	assert.Equal(t, Position{Row: 0, Col: 0}, result)
}

func TestReadlineWordBoundary_MultipleDelimiters(t *testing.T) {
	content := []string{"hello....world"}

	// From end of "world" -> start of "world" (skips delimiters)
	result := readlineFindPrevWordStart(content, Position{Row: 0, Col: 14})
	assert.Equal(t, Position{Row: 0, Col: 9}, result)

	// From delimiters -> start of "hello"
	result = readlineFindPrevWordStart(content, Position{Row: 0, Col: 7})
	assert.Equal(t, Position{Row: 0, Col: 0}, result)
}
