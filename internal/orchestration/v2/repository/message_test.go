package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/xorchestrator/internal/orchestration/message"
)

// ===========================================================================
// Constructor Tests
// ===========================================================================

func TestNewMemoryMessageRepository(t *testing.T) {
	repo := NewMemoryMessageRepository()

	require.NotNil(t, repo, "constructor should return non-nil repository")
	require.NotNil(t, repo.Broker(), "broker should be non-nil")
	require.Equal(t, 0, repo.Count(), "new repository should be empty")
	require.Empty(t, repo.Entries(), "new repository should have no entries")
}

// ===========================================================================
// Append Tests
// ===========================================================================

func TestMemoryMessageRepository_Append(t *testing.T) {
	t.Run("creates message with UUID and timestamp", func(t *testing.T) {
		repo := NewMemoryMessageRepository()
		beforeTime := time.Now()

		msg, err := repo.Append("WORKER.1", "COORDINATOR", "Hello", message.MessageInfo)

		require.NoError(t, err)
		require.NotNil(t, msg)

		// Verify UUID is valid
		_, err = uuid.Parse(msg.ID)
		require.NoError(t, err, "ID should be a valid UUID")

		// Verify timestamp is reasonable
		require.False(t, msg.Timestamp.IsZero(), "timestamp should be set")
		require.True(t, msg.Timestamp.After(beforeTime) || msg.Timestamp.Equal(beforeTime),
			"timestamp should be at or after test start")
		require.True(t, msg.Timestamp.Before(time.Now().Add(time.Second)),
			"timestamp should be recent")

		// Verify other fields
		require.Equal(t, "WORKER.1", msg.From)
		require.Equal(t, "COORDINATOR", msg.To)
		require.Equal(t, "Hello", msg.Content)
		require.Equal(t, message.MessageInfo, msg.Type)
	})

	t.Run("sender automatically marked as read", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		msg, err := repo.Append("WORKER.1", "COORDINATOR", "Hello", message.MessageInfo)

		require.NoError(t, err)
		require.Contains(t, msg.ReadBy, "WORKER.1", "sender should be in ReadBy")
	})

	t.Run("multiple messages get unique IDs", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		msg1, _ := repo.Append("A", "B", "First", message.MessageInfo)
		msg2, _ := repo.Append("A", "B", "Second", message.MessageInfo)
		msg3, _ := repo.Append("A", "B", "Third", message.MessageInfo)

		require.NotEqual(t, msg1.ID, msg2.ID, "messages should have unique IDs")
		require.NotEqual(t, msg2.ID, msg3.ID, "messages should have unique IDs")
		require.NotEqual(t, msg1.ID, msg3.ID, "messages should have unique IDs")
	})

	t.Run("supports all message types", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		types := []message.MessageType{
			message.MessageInfo,
			message.MessageRequest,
			message.MessageResponse,
			message.MessageCompletion,
			message.MessageError,
			message.MessageHandoff,
			message.MessageWorkerReady,
		}

		for _, msgType := range types {
			msg, err := repo.Append("A", "B", "Content", msgType)
			require.NoError(t, err)
			require.Equal(t, msgType, msg.Type)
		}

		require.Equal(t, len(types), repo.Count())
	})
}

// ===========================================================================
// Entries Tests
// ===========================================================================

func TestMemoryMessageRepository_Entries(t *testing.T) {
	t.Run("returns empty slice for empty repository", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		entries := repo.Entries()

		require.NotNil(t, entries, "should return non-nil slice")
		require.Empty(t, entries)
	})

	t.Run("returns all messages in append order", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		_, _ = repo.Append("A", "B", "First", message.MessageInfo)
		_, _ = repo.Append("C", "D", "Second", message.MessageRequest)
		_, _ = repo.Append("E", "F", "Third", message.MessageResponse)

		entries := repo.Entries()

		require.Len(t, entries, 3)
		require.Equal(t, "First", entries[0].Content)
		require.Equal(t, "Second", entries[1].Content)
		require.Equal(t, "Third", entries[2].Content)
	})

	t.Run("returns copy not original slice", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		_, _ = repo.Append("A", "B", "Original", message.MessageInfo)

		entries := repo.Entries()
		entries[0].Content = "Modified"

		// Original should be unchanged
		originalEntries := repo.Entries()
		require.Equal(t, "Original", originalEntries[0].Content,
			"modifying returned slice should not affect repository")
	})
}

// ===========================================================================
// UnreadFor Tests
// ===========================================================================

func TestMemoryMessageRepository_UnreadFor(t *testing.T) {
	t.Run("returns all messages for new agent", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		_, _ = repo.Append("COORDINATOR", "ALL", "Message 1", message.MessageInfo)
		_, _ = repo.Append("COORDINATOR", "ALL", "Message 2", message.MessageInfo)

		unread := repo.UnreadFor("WORKER.1")

		require.Len(t, unread, 2)
		require.Equal(t, "Message 1", unread[0].Content)
		require.Equal(t, "Message 2", unread[1].Content)
	})

	t.Run("returns empty for agent who read all", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		_, _ = repo.Append("COORDINATOR", "ALL", "Message 1", message.MessageInfo)
		repo.MarkRead("WORKER.1")

		unread := repo.UnreadFor("WORKER.1")

		require.Empty(t, unread)
	})

	t.Run("returns only new messages after MarkRead", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		_, _ = repo.Append("COORDINATOR", "ALL", "Old message", message.MessageInfo)
		repo.MarkRead("WORKER.1")
		_, _ = repo.Append("COORDINATOR", "ALL", "New message", message.MessageInfo)

		unread := repo.UnreadFor("WORKER.1")

		require.Len(t, unread, 1)
		require.Equal(t, "New message", unread[0].Content)
	})

	t.Run("different agents have independent read states", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		_, _ = repo.Append("COORDINATOR", "ALL", "Message 1", message.MessageInfo)
		_, _ = repo.Append("COORDINATOR", "ALL", "Message 2", message.MessageInfo)

		repo.MarkRead("WORKER.1") // Worker 1 has read all

		_, _ = repo.Append("COORDINATOR", "ALL", "Message 3", message.MessageInfo)

		// Worker 1 sees only message 3
		unread1 := repo.UnreadFor("WORKER.1")
		require.Len(t, unread1, 1)
		require.Equal(t, "Message 3", unread1[0].Content)

		// Worker 2 sees all 3 messages
		unread2 := repo.UnreadFor("WORKER.2")
		require.Len(t, unread2, 3)
	})

	t.Run("returns nil for empty repository", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		unread := repo.UnreadFor("WORKER.1")

		require.Nil(t, unread)
	})
}

// ===========================================================================
// MarkRead Tests
// ===========================================================================

func TestMemoryMessageRepository_MarkRead(t *testing.T) {
	t.Run("updates read state to current count", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		_, _ = repo.Append("COORDINATOR", "ALL", "Message 1", message.MessageInfo)
		_, _ = repo.Append("COORDINATOR", "ALL", "Message 2", message.MessageInfo)

		repo.MarkRead("WORKER.1")

		unread := repo.UnreadFor("WORKER.1")
		require.Empty(t, unread)
	})

	t.Run("adds agent to ReadBy on all entries", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		_, _ = repo.Append("COORDINATOR", "ALL", "Message 1", message.MessageInfo)
		_, _ = repo.Append("COORDINATOR", "ALL", "Message 2", message.MessageInfo)

		repo.MarkRead("WORKER.1")

		entries := repo.Entries()
		for i, entry := range entries {
			require.Contains(t, entry.ReadBy, "WORKER.1",
				"entry %d should have WORKER.1 in ReadBy", i)
		}
	})

	t.Run("idempotent - calling twice does not duplicate ReadBy", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		_, _ = repo.Append("COORDINATOR", "ALL", "Message 1", message.MessageInfo)

		repo.MarkRead("WORKER.1")
		repo.MarkRead("WORKER.1")

		entries := repo.Entries()
		count := 0
		for _, reader := range entries[0].ReadBy {
			if reader == "WORKER.1" {
				count++
			}
		}
		require.Equal(t, 1, count, "WORKER.1 should appear only once in ReadBy")
	})

	t.Run("works for previously unknown agent", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		_, _ = repo.Append("COORDINATOR", "ALL", "Message 1", message.MessageInfo)

		// Marking read for an agent that never called UnreadFor before
		repo.MarkRead("NEW_AGENT")

		unread := repo.UnreadFor("NEW_AGENT")
		require.Empty(t, unread)
	})
}

// ===========================================================================
// Count Tests
// ===========================================================================

func TestMemoryMessageRepository_Count(t *testing.T) {
	t.Run("returns 0 for empty repository", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		require.Equal(t, 0, repo.Count())
	})

	t.Run("returns accurate count after appends", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		require.Equal(t, 0, repo.Count())

		_, _ = repo.Append("A", "B", "1", message.MessageInfo)
		require.Equal(t, 1, repo.Count())

		_, _ = repo.Append("A", "B", "2", message.MessageInfo)
		require.Equal(t, 2, repo.Count())

		_, _ = repo.Append("A", "B", "3", message.MessageInfo)
		require.Equal(t, 3, repo.Count())
	})

	t.Run("count matches Entries length", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		for i := 0; i < 10; i++ {
			_, _ = repo.Append("A", "B", "msg", message.MessageInfo)
		}

		require.Equal(t, repo.Count(), len(repo.Entries()))
	})
}

// ===========================================================================
// Broker Tests
// ===========================================================================

func TestMemoryMessageRepository_Broker(t *testing.T) {
	t.Run("returns non-nil broker", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		broker := repo.Broker()

		require.NotNil(t, broker)
	})

	t.Run("returns same broker instance on multiple calls", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		broker1 := repo.Broker()
		broker2 := repo.Broker()

		require.Same(t, broker1, broker2, "should return same broker instance")
	})

	t.Run("publishes event on Append", func(t *testing.T) {
		repo := NewMemoryMessageRepository()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := repo.Broker().Subscribe(ctx)

		// Append a message
		_, err := repo.Append("WORKER.1", "COORDINATOR", "Hello", message.MessageInfo)
		require.NoError(t, err)

		// Should receive event
		select {
		case event := <-ch:
			require.Equal(t, message.EventPosted, event.Payload.Type)
			require.Equal(t, "WORKER.1", event.Payload.Entry.From)
			require.Equal(t, "COORDINATOR", event.Payload.Entry.To)
			require.Equal(t, "Hello", event.Payload.Entry.Content)
			require.Equal(t, message.MessageInfo, event.Payload.Entry.Type)
		case <-time.After(time.Second):
			require.FailNow(t, "timeout waiting for event")
		}
	})

	t.Run("multiple subscribers receive events", func(t *testing.T) {
		repo := NewMemoryMessageRepository()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch1 := repo.Broker().Subscribe(ctx)
		ch2 := repo.Broker().Subscribe(ctx)

		_, err := repo.Append("WORKER.1", "ALL", "Broadcast", message.MessageInfo)
		require.NoError(t, err)

		// Both subscribers should receive the event
		select {
		case e1 := <-ch1:
			select {
			case e2 := <-ch2:
				require.Equal(t, e1.Payload.Entry.ID, e2.Payload.Entry.ID)
				require.Equal(t, "Broadcast", e1.Payload.Entry.Content)
			case <-time.After(time.Second):
				require.FailNow(t, "timeout waiting for event on ch2")
			}
		case <-time.After(time.Second):
			require.FailNow(t, "timeout waiting for event on ch1")
		}
	})

	t.Run("events contain complete message data", func(t *testing.T) {
		repo := NewMemoryMessageRepository()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := repo.Broker().Subscribe(ctx)

		appended, _ := repo.Append("SENDER", "RECIPIENT", "Content", message.MessageCompletion)

		select {
		case event := <-ch:
			require.Equal(t, appended.ID, event.Payload.Entry.ID)
			require.Equal(t, appended.From, event.Payload.Entry.From)
			require.Equal(t, appended.To, event.Payload.Entry.To)
			require.Equal(t, appended.Content, event.Payload.Entry.Content)
			require.Equal(t, appended.Type, event.Payload.Entry.Type)
			require.Equal(t, appended.Timestamp, event.Payload.Entry.Timestamp)
		case <-time.After(time.Second):
			require.FailNow(t, "timeout waiting for event")
		}
	})
}

// ===========================================================================
// Broadcast Semantics Tests
// ===========================================================================

func TestMemoryMessageRepository_BroadcastSemantics(t *testing.T) {
	t.Run("all agents see all messages regardless of To field", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		// Messages with different recipients
		_, _ = repo.Append("COORDINATOR", "ALL", "Broadcast message", message.MessageInfo)
		_, _ = repo.Append("COORDINATOR", "WORKER.1", "Direct to Worker 1", message.MessageRequest)
		_, _ = repo.Append("COORDINATOR", "WORKER.2", "Direct to Worker 2", message.MessageRequest)

		// All workers see all messages (no recipient filtering)
		unread1 := repo.UnreadFor("WORKER.1")
		require.Len(t, unread1, 3, "Worker 1 should see all 3 messages")
		require.Equal(t, "Broadcast message", unread1[0].Content)
		require.Equal(t, "Direct to Worker 1", unread1[1].Content)
		require.Equal(t, "Direct to Worker 2", unread1[2].Content)

		// Worker 2 also sees all messages
		unread2 := repo.UnreadFor("WORKER.2")
		require.Len(t, unread2, 3, "Worker 2 should see all 3 messages")

		// Coordinator sees all messages
		unreadCoord := repo.UnreadFor("COORDINATOR")
		require.Len(t, unreadCoord, 3, "Coordinator should see all 3 messages")
	})

	t.Run("To field is metadata not access control", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		// Message intended only for WORKER.1
		_, _ = repo.Append("COORDINATOR", "WORKER.1", "Private message", message.MessageInfo)

		// Even WORKER.99 can read it
		unread := repo.UnreadFor("WORKER.99")
		require.Len(t, unread, 1)
		require.Equal(t, "Private message", unread[0].Content)
	})
}

// ===========================================================================
// Concurrent Access Tests
// ===========================================================================

func TestMemoryMessageRepository_ConcurrentAccess(t *testing.T) {
	t.Run("10 readers and 5 writers", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		// Pre-populate with some messages
		for i := 0; i < 100; i++ {
			_, _ = repo.Append("COORDINATOR", "ALL", "Initial message", message.MessageInfo)
		}

		var wg sync.WaitGroup

		// 10 concurrent readers
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				workerName := message.WorkerID(workerID)
				for j := 0; j < 100; j++ {
					_ = repo.Entries()
					_ = repo.UnreadFor(workerName)
					_ = repo.Count()
				}
			}(i)
		}

		// 5 concurrent writers (MarkRead operations)
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				workerName := message.WorkerID(workerID)
				for j := 0; j < 50; j++ {
					repo.MarkRead(workerName)
					time.Sleep(time.Microsecond)
				}
			}(i)
		}

		// Wait for all goroutines
		wg.Wait()

		// Should have no race conditions (verified by running with -race flag)
	})

	t.Run("concurrent appends and reads", func(t *testing.T) {
		repo := NewMemoryMessageRepository()

		var wg sync.WaitGroup

		// 5 concurrent appenders
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				workerName := message.WorkerID(workerID)
				for j := 0; j < 20; j++ {
					_, _ = repo.Append(workerName, "COORDINATOR", "Message", message.MessageInfo)
				}
			}(i)
		}

		// 5 concurrent readers
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				workerName := message.WorkerID(workerID)
				for j := 0; j < 50; j++ {
					_ = repo.Entries()
					_ = repo.UnreadFor(workerName)
				}
			}(i + 10)
		}

		wg.Wait()

		// Verify final count
		require.Equal(t, 100, repo.Count(), "should have 5 writers * 20 messages = 100")
	})
}

// ===========================================================================
// Test Helper Tests
// ===========================================================================

func TestMemoryMessageRepository_Reset(t *testing.T) {
	repo := NewMemoryMessageRepository()

	_, _ = repo.Append("A", "B", "Message", message.MessageInfo)
	repo.MarkRead("A")

	require.Equal(t, 1, repo.Count())

	repo.Reset()

	require.Equal(t, 0, repo.Count())
	require.Empty(t, repo.Entries())
	require.NotNil(t, repo.Broker(), "broker should be available after reset")
}

func TestMemoryMessageRepository_AddMessage(t *testing.T) {
	repo := NewMemoryMessageRepository()

	msg, err := repo.AddMessage("FROM", "TO", "Content", message.MessageRequest)

	require.NoError(t, err)
	require.NotNil(t, msg)
	require.Equal(t, "FROM", msg.From)
	require.Equal(t, "TO", msg.To)
	require.Equal(t, "Content", msg.Content)
	require.Equal(t, message.MessageRequest, msg.Type)
	require.Equal(t, 1, repo.Count())
}
