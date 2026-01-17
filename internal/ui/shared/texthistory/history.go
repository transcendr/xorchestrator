package texthistory

import (
	"strings"
	"sync"
)

const (
	// DefaultMaxHistory is the default maximum number of history entries
	DefaultMaxHistory = 100
	// MaxHistorySize is the absolute maximum number of entries allowed
	MaxHistorySize = 10000
)

// HistoryManager manages a ring buffer of command history entries.
// It provides navigation through previous entries (Previous/Next) and
// tracks the current position within the history.
//
// The implementation uses immutable-style methods that return new state,
// aligning with Go's value semantics while maintaining thread safety.
type HistoryManager struct {
	entries []string // Ring buffer of history entries
	maxSize int      // Maximum number of entries to keep
	pos     int      // Current position in history (-1 = not navigating)
	mu      sync.RWMutex
}

// NewHistoryManager creates a new history manager with the specified maximum size.
// If maxSize is 0 or negative, DefaultMaxHistory is used.
// If maxSize exceeds MaxHistorySize, it is clamped to MaxHistorySize.
func NewHistoryManager(maxSize int) *HistoryManager {
	if maxSize <= 0 {
		maxSize = DefaultMaxHistory
	}
	if maxSize > MaxHistorySize {
		maxSize = MaxHistorySize
	}

	return &HistoryManager{
		entries: make([]string, 0, maxSize),
		maxSize: maxSize,
		pos:     -1, // -1 indicates not currently navigating history
	}
}

// Add adds a new entry to the history buffer.
// If the entry is empty or identical to the most recent entry, it is not added.
// When the buffer is full, the oldest entry is removed.
// Adding an entry resets the navigation position.
func (h *HistoryManager) Add(text string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Don't add empty or whitespace-only entries
	if strings.TrimSpace(text) == "" {
		return
	}

	// Don't add duplicate consecutive entries
	if len(h.entries) > 0 && h.entries[len(h.entries)-1] == text {
		return
	}

	// Add to buffer, removing oldest if at capacity
	if len(h.entries) >= h.maxSize {
		// Shift all entries down by one
		copy(h.entries, h.entries[1:])
		h.entries[len(h.entries)-1] = text
	} else {
		h.entries = append(h.entries, text)
	}

	// Reset navigation position
	h.pos = -1
}

// Previous returns the previous entry in history.
// If already at the oldest entry, it returns the oldest entry.
// If history is empty, returns an empty string.
// This moves the navigation position backward.
func (h *HistoryManager) Previous() string {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.entries) == 0 {
		return ""
	}

	// Initialize position to most recent if not navigating
	if h.pos == -1 {
		h.pos = len(h.entries) - 1
		return h.entries[h.pos]
	}

	// Move backward if not at oldest
	if h.pos > 0 {
		h.pos--
	}

	return h.entries[h.pos]
}

// Next returns the next entry in history (toward more recent).
// If already at the newest entry or not navigating, returns empty string.
// This moves the navigation position forward.
func (h *HistoryManager) Next() string {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.entries) == 0 || h.pos == -1 {
		return ""
	}

	// Move forward toward more recent entries
	if h.pos < len(h.entries)-1 {
		h.pos++
		return h.entries[h.pos]
	}

	// At newest entry - exit history navigation
	h.pos = -1
	return ""
}

// Current returns the entry at the current navigation position.
// If not currently navigating history, returns empty string.
func (h *HistoryManager) Current() string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.pos == -1 || len(h.entries) == 0 {
		return ""
	}

	return h.entries[h.pos]
}

// IsNavigating returns true if currently navigating through history.
func (h *HistoryManager) IsNavigating() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.pos != -1
}

// Reset resets the navigation position without clearing history.
// This exits history navigation mode.
func (h *HistoryManager) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pos = -1
}

// Clear removes all entries from history and resets navigation.
func (h *HistoryManager) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = make([]string, 0, h.maxSize)
	h.pos = -1
}

// Len returns the number of entries currently in history.
func (h *HistoryManager) Len() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.entries)
}

// Entries returns a copy of all history entries, oldest to newest.
// This is safe for concurrent use as it returns a copy.
func (h *HistoryManager) Entries() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]string, len(h.entries))
	copy(result, h.entries)
	return result
}
