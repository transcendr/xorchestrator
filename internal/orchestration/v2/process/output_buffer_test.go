package process

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOutputBuffer_CreatesBufferWithCapacity(t *testing.T) {
	buf := NewOutputBuffer(10)
	assert.NotNil(t, buf)
	assert.Equal(t, 10, buf.Capacity())
	assert.Equal(t, 0, buf.Len())
}

func TestNewOutputBuffer_EnforcesMinimumCapacity(t *testing.T) {
	// Zero capacity should become 1
	buf := NewOutputBuffer(0)
	assert.Equal(t, 1, buf.Capacity())

	// Negative capacity should become 1
	buf = NewOutputBuffer(-5)
	assert.Equal(t, 1, buf.Capacity())
}

func TestOutputBuffer_Append_AddsLines(t *testing.T) {
	buf := NewOutputBuffer(10)

	buf.Append("line1")
	buf.Append("line2")
	buf.Append("line3")

	assert.Equal(t, 3, buf.Len())
	lines := buf.Lines()
	assert.Equal(t, []string{"line1", "line2", "line3"}, lines)
}

func TestOutputBuffer_Append_RespectsCapacityLimit(t *testing.T) {
	buf := NewOutputBuffer(3)

	buf.Append("line1")
	buf.Append("line2")
	buf.Append("line3")
	buf.Append("line4") // Should overwrite line1
	buf.Append("line5") // Should overwrite line2

	assert.Equal(t, 3, buf.Len())
	lines := buf.Lines()
	assert.Equal(t, []string{"line3", "line4", "line5"}, lines)
}

func TestOutputBuffer_Lines_ReturnsAllLines(t *testing.T) {
	buf := NewOutputBuffer(10)

	buf.Append("first")
	buf.Append("second")
	buf.Append("third")

	lines := buf.Lines()
	require.Len(t, lines, 3)
	assert.Equal(t, "first", lines[0])
	assert.Equal(t, "second", lines[1])
	assert.Equal(t, "third", lines[2])
}

func TestOutputBuffer_Lines_ReturnsEmptySliceWhenEmpty(t *testing.T) {
	buf := NewOutputBuffer(10)
	lines := buf.Lines()
	assert.Empty(t, lines)
}

func TestOutputBuffer_Lines_ReturnsCopy(t *testing.T) {
	buf := NewOutputBuffer(10)
	buf.Append("line1")

	lines1 := buf.Lines()
	lines2 := buf.Lines()

	// Modifying returned slice should not affect buffer
	lines1[0] = "modified"
	assert.Equal(t, "line1", lines2[0])
}

func TestOutputBuffer_Clear_EmptiesBuffer(t *testing.T) {
	buf := NewOutputBuffer(10)
	buf.Append("line1")
	buf.Append("line2")

	buf.Clear()

	assert.Equal(t, 0, buf.Len())
	assert.Empty(t, buf.Lines())
}

func TestOutputBuffer_Last_ReturnsLastNLines(t *testing.T) {
	buf := NewOutputBuffer(10)
	buf.Append("line1")
	buf.Append("line2")
	buf.Append("line3")
	buf.Append("line4")

	last2 := buf.Last(2)
	require.Len(t, last2, 2)
	assert.Equal(t, "line3", last2[0])
	assert.Equal(t, "line4", last2[1])
}

func TestOutputBuffer_Last_ReturnsAllLinesIfNExceedsSize(t *testing.T) {
	buf := NewOutputBuffer(10)
	buf.Append("line1")
	buf.Append("line2")

	last := buf.Last(10)
	require.Len(t, last, 2)
	assert.Equal(t, "line1", last[0])
	assert.Equal(t, "line2", last[1])
}

func TestOutputBuffer_Last_ReturnsNilForZeroOrNegative(t *testing.T) {
	buf := NewOutputBuffer(10)
	buf.Append("line1")

	assert.Nil(t, buf.Last(0))
	assert.Nil(t, buf.Last(-1))
}

func TestOutputBuffer_Last_WorksAfterWrapAround(t *testing.T) {
	buf := NewOutputBuffer(3)
	buf.Append("line1")
	buf.Append("line2")
	buf.Append("line3")
	buf.Append("line4") // Wraps around

	last2 := buf.Last(2)
	require.Len(t, last2, 2)
	assert.Equal(t, "line3", last2[0])
	assert.Equal(t, "line4", last2[1])
}

func TestOutputBuffer_String_JoinsLinesWithNewlines(t *testing.T) {
	buf := NewOutputBuffer(10)
	buf.Append("line1")
	buf.Append("line2")
	buf.Append("line3")

	assert.Equal(t, "line1\nline2\nline3", buf.String())
}

func TestOutputBuffer_String_ReturnsEmptyStringWhenEmpty(t *testing.T) {
	buf := NewOutputBuffer(10)
	assert.Equal(t, "", buf.String())
}

func TestOutputBuffer_ThreadSafety(t *testing.T) {
	buf := NewOutputBuffer(100)
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				buf.Append("line")
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = buf.Lines()
				_ = buf.Last(10)
				_ = buf.Len()
			}
		}()
	}

	wg.Wait()
	// Test passes if no race conditions detected
	assert.True(t, buf.Len() > 0)
}

func TestOutputBuffer_Capacity_ReturnsOriginalCapacity(t *testing.T) {
	buf := NewOutputBuffer(42)
	assert.Equal(t, 42, buf.Capacity())

	// Capacity doesn't change after operations
	buf.Append("line1")
	buf.Append("line2")
	assert.Equal(t, 42, buf.Capacity())
}

func TestOutputBuffer_Len_TracksLineCount(t *testing.T) {
	buf := NewOutputBuffer(10)
	assert.Equal(t, 0, buf.Len())

	buf.Append("line1")
	assert.Equal(t, 1, buf.Len())

	buf.Append("line2")
	assert.Equal(t, 2, buf.Len())

	buf.Clear()
	assert.Equal(t, 0, buf.Len())
}

func TestOutputBuffer_Len_CapsAtCapacity(t *testing.T) {
	buf := NewOutputBuffer(3)

	for i := 0; i < 10; i++ {
		buf.Append("line")
	}

	assert.Equal(t, 3, buf.Len())
}
