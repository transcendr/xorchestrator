package mcp

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// P0 Tests - Required Before Merge

func TestIsDuplicate_Basic(t *testing.T) {
	dedup := NewMessageDeduplicator(50 * time.Millisecond)

	// First call should not be duplicate
	assert.False(t, dedup.IsDuplicate("worker-1", "hello"))

	// Immediate second call should be duplicate
	assert.True(t, dedup.IsDuplicate("worker-1", "hello"))

	// Verify suppressed count
	assert.Equal(t, int64(1), dedup.SuppressedCount())

	// Third call should also be duplicate
	assert.True(t, dedup.IsDuplicate("worker-1", "hello"))
	assert.Equal(t, int64(2), dedup.SuppressedCount())
}

func TestIsDuplicate_AfterExpiry(t *testing.T) {
	dedup := NewMessageDeduplicator(50 * time.Millisecond)

	// First call should not be duplicate
	assert.False(t, dedup.IsDuplicate("worker-1", "hello"))
	assert.Equal(t, 1, dedup.Len())

	// Wait past window
	time.Sleep(60 * time.Millisecond)

	// Should no longer be duplicate after window expires
	assert.False(t, dedup.IsDuplicate("worker-1", "hello"))

	// Entry count should still be 1 (new entry replaced expired)
	assert.Equal(t, 1, dedup.Len())

	// No duplicates were suppressed (the second call was allowed)
	assert.Equal(t, int64(0), dedup.SuppressedCount())
}

func TestIsDuplicate_DifferentWorkers(t *testing.T) {
	dedup := NewMessageDeduplicator(50 * time.Millisecond)

	// Same message to different workers should all be allowed
	assert.False(t, dedup.IsDuplicate("worker-1", "hello"))
	assert.False(t, dedup.IsDuplicate("worker-2", "hello"))
	assert.False(t, dedup.IsDuplicate("worker-3", "hello"))

	// All three entries should be tracked
	assert.Equal(t, 3, dedup.Len())

	// No duplicates were suppressed
	assert.Equal(t, int64(0), dedup.SuppressedCount())

	// But same message to same worker should be duplicate
	assert.True(t, dedup.IsDuplicate("worker-1", "hello"))
	assert.Equal(t, int64(1), dedup.SuppressedCount())
}

func TestIsDuplicate_DifferentMessages(t *testing.T) {
	dedup := NewMessageDeduplicator(50 * time.Millisecond)

	// Different messages to same worker should all be allowed
	assert.False(t, dedup.IsDuplicate("worker-1", "hello"))
	assert.False(t, dedup.IsDuplicate("worker-1", "world"))
	assert.False(t, dedup.IsDuplicate("worker-1", "foo"))

	// All three entries should be tracked
	assert.Equal(t, 3, dedup.Len())

	// No duplicates were suppressed
	assert.Equal(t, int64(0), dedup.SuppressedCount())

	// But same message should be duplicate
	assert.True(t, dedup.IsDuplicate("worker-1", "hello"))
	assert.Equal(t, int64(1), dedup.SuppressedCount())
}

func TestIsDuplicate_ConcurrentAccess(t *testing.T) {
	dedup := NewMessageDeduplicator(100 * time.Millisecond)

	// Concurrently send different messages
	var wg sync.WaitGroup
	numGoroutines := 50
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			dedup.IsDuplicate("worker-1", fmt.Sprintf("msg-%d", id))
		}(i)
	}
	wg.Wait()

	// All 50 unique messages should be tracked (no panic, no data corruption)
	assert.Equal(t, numGoroutines, dedup.Len())

	// No duplicates since all messages were unique
	assert.Equal(t, int64(0), dedup.SuppressedCount())
}

func TestIsDuplicate_ConcurrentSameMessage(t *testing.T) {
	// This test verifies that when multiple goroutines send the same message
	// simultaneously, only one passes through and the rest are detected as duplicates.
	// This tests the race condition prevention via sync.Mutex.

	dedup := NewMessageDeduplicator(100 * time.Millisecond)

	var wg sync.WaitGroup
	numGoroutines := 20
	var passedCount int64
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			isDup := dedup.IsDuplicate("worker-1", "same-message")
			if !isDup {
				mu.Lock()
				passedCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// Exactly one message should have passed through
	assert.Equal(t, int64(1), passedCount, "only one message should pass through")

	// All others should have been suppressed
	assert.Equal(t, int64(numGoroutines-1), dedup.SuppressedCount())

	// Only one entry should be in the map
	assert.Equal(t, 1, dedup.Len())
}

// P1 Tests - Follow-up

func TestIsDuplicate_CleanupRemovesExpired(t *testing.T) {
	dedup := NewMessageDeduplicator(50 * time.Millisecond)

	// Add multiple entries
	assert.False(t, dedup.IsDuplicate("worker-1", "msg-1"))
	assert.False(t, dedup.IsDuplicate("worker-2", "msg-2"))
	assert.False(t, dedup.IsDuplicate("worker-3", "msg-3"))
	assert.Equal(t, 3, dedup.Len())

	// Wait for entries to expire
	time.Sleep(60 * time.Millisecond)

	// Trigger lazy cleanup by calling IsDuplicate with a new message
	assert.False(t, dedup.IsDuplicate("worker-4", "msg-4"))

	// Only the new entry should remain (expired entries cleaned up)
	assert.Equal(t, 1, dedup.Len())
}

func TestIsDuplicate_BoundaryCondition(t *testing.T) {
	// Test behavior at exactly the window boundary
	// The implementation uses >= for expiry check, so at exactly window duration
	// the entry should be considered expired.

	dedup := NewMessageDeduplicator(50 * time.Millisecond)

	// First call
	assert.False(t, dedup.IsDuplicate("worker-1", "hello"))

	// Wait exactly the window duration (or slightly more to ensure we're past it)
	time.Sleep(50 * time.Millisecond)

	// At exactly the boundary (or just past), should allow the message
	// Note: Due to timing precision, we add a small buffer
	time.Sleep(5 * time.Millisecond)

	assert.False(t, dedup.IsDuplicate("worker-1", "hello"),
		"message should be allowed after window expires")
}

// Additional edge case tests

func TestNewMessageDeduplicator_DefaultWindow(t *testing.T) {
	// Test that zero or negative window defaults to DefaultDeduplicationWindow
	dedup1 := NewMessageDeduplicator(0)
	dedup2 := NewMessageDeduplicator(-1 * time.Second)

	// Both should work correctly (not panic or have zero window)
	assert.False(t, dedup1.IsDuplicate("worker-1", "hello"))
	assert.True(t, dedup1.IsDuplicate("worker-1", "hello"))

	assert.False(t, dedup2.IsDuplicate("worker-1", "hello"))
	assert.True(t, dedup2.IsDuplicate("worker-1", "hello"))
}

func TestMessageDeduplicator_Clear(t *testing.T) {
	dedup := NewMessageDeduplicator(50 * time.Millisecond)

	// Add entries and some suppressions
	assert.False(t, dedup.IsDuplicate("worker-1", "hello"))
	assert.True(t, dedup.IsDuplicate("worker-1", "hello"))
	assert.Equal(t, 1, dedup.Len())
	assert.Equal(t, int64(1), dedup.SuppressedCount())

	// Clear
	dedup.Clear()

	// Should be empty
	assert.Equal(t, 0, dedup.Len())
	assert.Equal(t, int64(0), dedup.SuppressedCount())

	// Same message should now be allowed again
	assert.False(t, dedup.IsDuplicate("worker-1", "hello"))
}

func TestIsDuplicate_EmptyStrings(t *testing.T) {
	dedup := NewMessageDeduplicator(50 * time.Millisecond)

	// Empty message should work
	assert.False(t, dedup.IsDuplicate("worker-1", ""))
	assert.True(t, dedup.IsDuplicate("worker-1", ""))

	// Empty worker ID should work
	assert.False(t, dedup.IsDuplicate("", "hello"))
	assert.True(t, dedup.IsDuplicate("", "hello"))

	// Both empty should work
	assert.False(t, dedup.IsDuplicate("", ""))
	assert.True(t, dedup.IsDuplicate("", ""))
}

func TestIsDuplicate_LargeMessage(t *testing.T) {
	dedup := NewMessageDeduplicator(50 * time.Millisecond)

	// Create a large message (10KB)
	largeMessage := make([]byte, 10*1024)
	for i := range largeMessage {
		largeMessage[i] = byte('a' + (i % 26))
	}
	msg := string(largeMessage)

	// Should handle large messages correctly
	assert.False(t, dedup.IsDuplicate("worker-1", msg))
	assert.True(t, dedup.IsDuplicate("worker-1", msg))

	// Only one entry
	assert.Equal(t, 1, dedup.Len())
}

func TestComputeHash_Deterministic(t *testing.T) {
	// Verify hash is deterministic
	hash1 := computeHash("worker-1", "hello")
	hash2 := computeHash("worker-1", "hello")
	assert.Equal(t, hash1, hash2)

	// Different inputs should produce different hashes
	hash3 := computeHash("worker-2", "hello")
	assert.NotEqual(t, hash1, hash3)

	hash4 := computeHash("worker-1", "world")
	assert.NotEqual(t, hash1, hash4)
}

func TestComputeHash_OrderMatters(t *testing.T) {
	// Verify that worker_id:message produces different hash than message:worker_id
	// This ensures the separator is properly included
	hash1 := computeHash("hello", "worker-1")
	hash2 := computeHash("worker-1", "hello")
	assert.NotEqual(t, hash1, hash2)
}
