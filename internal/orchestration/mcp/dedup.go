package mcp

import (
	"hash/fnv"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultDeduplicationWindow is the default time window for detecting duplicates.
// Messages with identical content sent to the same worker within this window
// are considered duplicates and silently dropped.
const DefaultDeduplicationWindow = 5 * time.Second

// MessageDeduplicator tracks recent messages to detect and prevent duplicates.
// It uses hash-based tracking with time-windowed expiration.
// Thread-safe for concurrent use.
type MessageDeduplicator struct {
	seen            map[uint64]time.Time // hash -> first seen timestamp
	window          time.Duration        // deduplication time window
	mu              sync.Mutex           // protects seen map
	suppressedCount atomic.Int64         // observability counter
}

// NewMessageDeduplicator creates a new deduplicator with the given time window.
// If window is zero or negative, DefaultDeduplicationWindow is used.
func NewMessageDeduplicator(window time.Duration) *MessageDeduplicator {
	if window <= 0 {
		window = DefaultDeduplicationWindow
	}
	return &MessageDeduplicator{
		seen:   make(map[uint64]time.Time),
		window: window,
	}
}

// IsDuplicate checks if a message is a duplicate within the time window.
// Returns true if this exact workerID+message combination was seen recently.
// Thread-safe.
func (d *MessageDeduplicator) IsDuplicate(workerID, message string) bool {
	hash := computeHash(workerID, message)
	now := time.Now()

	d.mu.Lock()
	defer d.mu.Unlock()

	// Lazy cleanup of expired entries
	for h, ts := range d.seen {
		if now.Sub(ts) >= d.window {
			delete(d.seen, h)
		}
	}

	// Check for existing entry within window
	if ts, exists := d.seen[hash]; exists {
		if now.Sub(ts) < d.window {
			d.suppressedCount.Add(1)
			return true
		}
	}

	// Record new entry
	d.seen[hash] = now
	return false
}

// computeHash generates a 64-bit FNV-1a hash of workerID and message.
func computeHash(workerID, message string) uint64 {
	h := fnv.New64a()
	// hash.Hash.Write never returns an error for FNV
	_, _ = h.Write([]byte(workerID))
	_, _ = h.Write([]byte(":"))
	_, _ = h.Write([]byte(message))
	return h.Sum64()
}

// Len returns the number of tracked messages (for testing/debugging).
func (d *MessageDeduplicator) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.seen)
}

// Clear removes all tracked messages (for testing).
func (d *MessageDeduplicator) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen = make(map[uint64]time.Time)
	d.suppressedCount.Store(0)
}

// SuppressedCount returns the number of duplicate messages that were suppressed.
func (d *MessageDeduplicator) SuppressedCount() int64 {
	return d.suppressedCount.Load()
}
