package texthistory

import (
	"strings"
	"sync"
	"testing"
)

// Test History Navigation

func TestNewHistoryManager(t *testing.T) {
	tests := []struct {
		name        string
		maxSize     int
		wantMaxSize int
	}{
		{"default size when 0", 0, DefaultMaxHistory},
		{"default size when negative", -1, DefaultMaxHistory},
		{"custom size", 50, 50},
		{"clamped to max", MaxHistorySize + 100, MaxHistorySize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHistoryManager(tt.maxSize)
			if h == nil {
				t.Fatal("expected non-nil HistoryManager")
			}
			if h.maxSize != tt.wantMaxSize {
				t.Errorf("maxSize = %d, want %d", h.maxSize, tt.wantMaxSize)
			}
			if h.pos != -1 {
				t.Errorf("initial pos = %d, want -1", h.pos)
			}
		})
	}
}

func TestAddSingleEntry(t *testing.T) {
	h := NewHistoryManager(10)
	h.Add("first entry")

	if h.Len() != 1 {
		t.Errorf("Len() = %d, want 1", h.Len())
	}

	entries := h.Entries()
	if len(entries) != 1 || entries[0] != "first entry" {
		t.Errorf("Entries() = %v, want [first entry]", entries)
	}
}

func TestAddEmpty(t *testing.T) {
	h := NewHistoryManager(10)
	h.Add("")
	h.Add("   ")

	// Empty strings and whitespace-only strings should not be added
	if h.Len() != 0 {
		t.Errorf("Len() = %d, want 0 (empty and whitespace strings should not be added)", h.Len())
	}
}

func TestAddDuplicateConsecutive(t *testing.T) {
	h := NewHistoryManager(10)
	h.Add("duplicate")
	h.Add("duplicate")
	h.Add("duplicate")

	// Consecutive duplicates should not be added
	if h.Len() != 1 {
		t.Errorf("Len() = %d, want 1", h.Len())
	}
}

func TestAddMultipleEntries(t *testing.T) {
	h := NewHistoryManager(10)
	entries := []string{"first", "second", "third", "fourth"}

	for _, e := range entries {
		h.Add(e)
	}

	if h.Len() != 4 {
		t.Errorf("Len() = %d, want 4", h.Len())
	}

	got := h.Entries()
	for i, want := range entries {
		if got[i] != want {
			t.Errorf("Entries()[%d] = %s, want %s", i, got[i], want)
		}
	}
}

func TestRingBufferWrapAround(t *testing.T) {
	h := NewHistoryManager(3) // Small buffer for testing
	h.Add("1")
	h.Add("2")
	h.Add("3")
	h.Add("4") // Should evict "1"

	if h.Len() != 3 {
		t.Errorf("Len() = %d, want 3", h.Len())
	}

	entries := h.Entries()
	want := []string{"2", "3", "4"}
	for i, w := range want {
		if entries[i] != w {
			t.Errorf("Entries()[%d] = %s, want %s", i, entries[i], w)
		}
	}
}

func TestPreviousOnEmpty(t *testing.T) {
	h := NewHistoryManager(10)
	got := h.Previous()
	if got != "" {
		t.Errorf("Previous() on empty history = %q, want %q", got, "")
	}
}

func TestPreviousOnSingleEntry(t *testing.T) {
	h := NewHistoryManager(10)
	h.Add("only")

	got := h.Previous()
	if got != "only" {
		t.Errorf("Previous() = %q, want %q", got, "only")
	}

	// Calling again should return same
	got = h.Previous()
	if got != "only" {
		t.Errorf("Previous() again = %q, want %q", got, "only")
	}
}

func TestPreviousNavigation(t *testing.T) {
	h := NewHistoryManager(10)
	h.Add("first")
	h.Add("second")
	h.Add("third")

	// Navigate backward through history
	if got := h.Previous(); got != "third" {
		t.Errorf("Previous() = %q, want %q", got, "third")
	}
	if got := h.Previous(); got != "second" {
		t.Errorf("Previous() = %q, want %q", got, "second")
	}
	if got := h.Previous(); got != "first" {
		t.Errorf("Previous() = %q, want %q", got, "first")
	}
	// At oldest - should stay there
	if got := h.Previous(); got != "first" {
		t.Errorf("Previous() at oldest = %q, want %q", got, "first")
	}
}

func TestNextOnEmpty(t *testing.T) {
	h := NewHistoryManager(10)
	got := h.Next()
	if got != "" {
		t.Errorf("Next() on empty = %q, want %q", got, "")
	}
}

func TestNextWithoutNavigating(t *testing.T) {
	h := NewHistoryManager(10)
	h.Add("entry")

	// Next without Previous should return empty
	got := h.Next()
	if got != "" {
		t.Errorf("Next() without navigating = %q, want %q", got, "")
	}
}

func TestNextNavigation(t *testing.T) {
	h := NewHistoryManager(10)
	h.Add("first")
	h.Add("second")
	h.Add("third")

	// Go back
	h.Previous() // third
	h.Previous() // second
	h.Previous() // first

	// Now go forward
	if got := h.Next(); got != "second" {
		t.Errorf("Next() = %q, want %q", got, "second")
	}
	if got := h.Next(); got != "third" {
		t.Errorf("Next() = %q, want %q", got, "third")
	}
	// At newest - should exit navigation
	if got := h.Next(); got != "" {
		t.Errorf("Next() at newest = %q, want %q", got, "")
	}
}

func TestCurrent(t *testing.T) {
	h := NewHistoryManager(10)
	h.Add("first")
	h.Add("second")

	// Not navigating - should return empty
	if got := h.Current(); got != "" {
		t.Errorf("Current() not navigating = %q, want %q", got, "")
	}

	// Start navigating
	h.Previous()
	if got := h.Current(); got != "second" {
		t.Errorf("Current() = %q, want %q", got, "second")
	}

	h.Previous()
	if got := h.Current(); got != "first" {
		t.Errorf("Current() = %q, want %q", got, "first")
	}
}

func TestIsNavigating(t *testing.T) {
	h := NewHistoryManager(10)
	h.Add("entry")

	if h.IsNavigating() {
		t.Error("IsNavigating() = true, want false initially")
	}

	h.Previous()
	if !h.IsNavigating() {
		t.Error("IsNavigating() = false, want true after Previous()")
	}

	h.Next()
	if h.IsNavigating() {
		t.Error("IsNavigating() = true, want false after exiting via Next()")
	}
}

func TestReset(t *testing.T) {
	h := NewHistoryManager(10)
	h.Add("entry")
	h.Previous()

	if !h.IsNavigating() {
		t.Fatal("should be navigating before reset")
	}

	h.Reset()
	if h.IsNavigating() {
		t.Error("IsNavigating() = true after Reset(), want false")
	}
	if got := h.Current(); got != "" {
		t.Errorf("Current() after Reset() = %q, want %q", got, "")
	}
}

func TestHistoryClear(t *testing.T) {
	h := NewHistoryManager(10)
	h.Add("first")
	h.Add("second")
	h.Previous()

	h.Clear()

	if h.Len() != 0 {
		t.Errorf("Len() after Clear() = %d, want 0", h.Len())
	}
	if h.IsNavigating() {
		t.Error("IsNavigating() = true after Clear(), want false")
	}
}

// Test Draft Preservation

func TestAddResetsPosition(t *testing.T) {
	h := NewHistoryManager(10)
	h.Add("first")
	h.Add("second")
	h.Previous()

	if !h.IsNavigating() {
		t.Fatal("should be navigating")
	}

	h.Add("third")

	if h.IsNavigating() {
		t.Error("IsNavigating() = true after Add(), want false")
	}
}

// Test Edge Cases

func TestUnicodeEmoji(t *testing.T) {
	h := NewHistoryManager(10)
	emoji := "Hello 👋 World 🌍"
	h.Add(emoji)

	got := h.Previous()
	if got != emoji {
		t.Errorf("Previous() = %q, want %q", got, emoji)
	}
}

func TestUnicodeCJK(t *testing.T) {
	h := NewHistoryManager(10)
	cjk := "こんにちは世界"
	h.Add(cjk)

	got := h.Previous()
	if got != cjk {
		t.Errorf("Previous() = %q, want %q", got, cjk)
	}
}

func TestLargeString(t *testing.T) {
	h := NewHistoryManager(10)
	large := strings.Repeat("a", 100000) // 100k chars
	h.Add(large)

	got := h.Previous()
	if got != large {
		t.Errorf("Previous() length = %d, want %d", len(got), len(large))
	}
}

func TestRapidNavigation(t *testing.T) {
	h := NewHistoryManager(100)
	// Start from 1 to avoid empty string at i=0
	for i := 1; i <= 100; i++ {
		h.Add(strings.Repeat("a", i))
	}

	// Rapidly navigate backward
	for i := 0; i < 1000; i++ {
		h.Previous()
	}

	// Should be at oldest entry (strings.Repeat("a", 1) = "a")
	got := h.Current()
	want := "a"
	if got != want {
		t.Errorf("Current() after rapid navigation = %q, want %q (oldest)", got, want)
	}
}

// Test Concurrency

func TestConcurrentAdd(t *testing.T) {
	h := NewHistoryManager(1000)
	var wg sync.WaitGroup

	// Spawn 10 goroutines adding entries
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				h.Add(strings.Repeat("a", id*100+j))
			}
		}(i)
	}

	wg.Wait()

	// Should have entries (exact count may vary due to duplicates)
	if h.Len() == 0 {
		t.Error("Len() = 0 after concurrent adds, want > 0")
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	h := NewHistoryManager(100)
	for i := 0; i < 50; i++ {
		h.Add(strings.Repeat("a", i))
	}

	var wg sync.WaitGroup

	// Writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				h.Add(strings.Repeat("b", id*20+j))
			}
		}(i)
	}

	// Readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				h.Previous()
				h.Next()
				h.Current()
				h.Len()
			}
		}()
	}

	wg.Wait()
	// Test passes if no race condition detected
}

// Benchmark tests

func BenchmarkAdd(b *testing.B) {
	h := NewHistoryManager(1000)
	entry := "benchmark entry"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Add(entry)
	}
}

func BenchmarkPrevious(b *testing.B) {
	h := NewHistoryManager(1000)
	for i := 0; i < 1000; i++ {
		h.Add(strings.Repeat("a", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Previous()
		if i%1000 == 999 {
			h.Reset() // Reset periodically
		}
	}
}

func BenchmarkNext(b *testing.B) {
	h := NewHistoryManager(1000)
	for i := 0; i < 1000; i++ {
		h.Add(strings.Repeat("a", i))
	}

	// Navigate to start
	for i := 0; i < 1000; i++ {
		h.Previous()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Next()
		if i%1000 == 999 {
			// Navigate back to start
			for j := 0; j < 1000; j++ {
				h.Previous()
			}
		}
	}
}
