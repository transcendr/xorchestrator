package codex

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zjrosen/xorchestrator/internal/orchestration/client"

	"github.com/stretchr/testify/require"
)

// errTest is a sentinel error for testing
var errTest = errors.New("test error")

// =============================================================================
// Lifecycle Tests - Process struct behavior without actual subprocess spawning
// =============================================================================

// newTestProcess creates a Process struct for testing without spawning a real subprocess.
// This allows testing lifecycle methods, status transitions, and channel behavior.
func newTestProcess() *Process {
	ctx, cancel := context.WithCancel(context.Background())
	return &Process{
		sessionID:  "019b6dea-903b-7bd3-aef5-202a16205a9a",
		workDir:    "/test/project",
		status:     client.StatusRunning,
		events:     make(chan client.OutputEvent, 100),
		errors:     make(chan error, 10),
		cancelFunc: cancel,
		ctx:        ctx,
	}
}

func TestProcessLifecycle_StatusTransitions_PendingToRunning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &Process{
		status:     client.StatusPending,
		events:     make(chan client.OutputEvent, 100),
		errors:     make(chan error, 10),
		cancelFunc: cancel,
		ctx:        ctx,
	}

	require.Equal(t, client.StatusPending, p.Status())
	require.False(t, p.IsRunning())

	p.setStatus(client.StatusRunning)
	require.Equal(t, client.StatusRunning, p.Status())
	require.True(t, p.IsRunning())
}

func TestProcessLifecycle_StatusTransitions_RunningToCompleted(t *testing.T) {
	p := newTestProcess()

	require.Equal(t, client.StatusRunning, p.Status())
	require.True(t, p.IsRunning())

	p.setStatus(client.StatusCompleted)
	require.Equal(t, client.StatusCompleted, p.Status())
	require.False(t, p.IsRunning())
}

func TestProcessLifecycle_StatusTransitions_RunningToFailed(t *testing.T) {
	p := newTestProcess()

	require.Equal(t, client.StatusRunning, p.Status())
	require.True(t, p.IsRunning())

	p.setStatus(client.StatusFailed)
	require.Equal(t, client.StatusFailed, p.Status())
	require.False(t, p.IsRunning())
}

func TestProcessLifecycle_StatusTransitions_RunningToCancelled(t *testing.T) {
	p := newTestProcess()

	require.Equal(t, client.StatusRunning, p.Status())
	require.True(t, p.IsRunning())

	err := p.Cancel()
	require.NoError(t, err)
	require.Equal(t, client.StatusCancelled, p.Status())
	require.False(t, p.IsRunning())
}

func TestProcessLifecycle_Cancel_TerminatesAndSetsStatus(t *testing.T) {
	p := newTestProcess()

	// Verify initial state
	require.Equal(t, client.StatusRunning, p.Status())

	// Cancel should set status to Cancelled
	err := p.Cancel()
	require.NoError(t, err)
	require.Equal(t, client.StatusCancelled, p.Status())

	// Context should be cancelled
	select {
	case <-p.ctx.Done():
		// Expected - context was cancelled
	default:
		require.Fail(t, "Context should be cancelled after Cancel()")
	}
}

func TestProcessLifecycle_Cancel_RacePrevention(t *testing.T) {
	// This test verifies that Cancel() sets status BEFORE calling cancelFunc,
	// preventing race conditions with goroutines that check status.
	// Run multiple iterations to catch potential race conditions.

	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		p := &Process{
			status:     client.StatusRunning,
			events:     make(chan client.OutputEvent, 100),
			errors:     make(chan error, 10),
			cancelFunc: cancel,
			ctx:        ctx,
		}

		// Track status seen by a goroutine that races with Cancel
		var observedStatus client.ProcessStatus
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			// Wait for context cancellation
			<-p.ctx.Done()
			// Immediately check status - should already be StatusCancelled
			observedStatus = p.Status()
		}()

		// Small sleep to ensure goroutine is waiting
		time.Sleep(time.Microsecond)

		// Cancel the process
		p.Cancel()

		wg.Wait()

		// The goroutine should have seen StatusCancelled, not StatusRunning
		require.Equal(t, client.StatusCancelled, observedStatus,
			"Goroutine should see StatusCancelled after context cancel (iteration %d)", i)
	}
}

func TestProcessLifecycle_Cancel_DoesNotOverrideTerminalState(t *testing.T) {
	tests := []struct {
		name           string
		initialStatus  client.ProcessStatus
		expectedStatus client.ProcessStatus
	}{
		{
			name:           "does not override completed",
			initialStatus:  client.StatusCompleted,
			expectedStatus: client.StatusCompleted,
		},
		{
			name:           "does not override failed",
			initialStatus:  client.StatusFailed,
			expectedStatus: client.StatusFailed,
		},
		{
			name:           "does not override already cancelled",
			initialStatus:  client.StatusCancelled,
			expectedStatus: client.StatusCancelled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			p := &Process{
				status:     tt.initialStatus,
				events:     make(chan client.OutputEvent, 100),
				errors:     make(chan error, 10),
				cancelFunc: cancel,
				ctx:        ctx,
			}

			err := p.Cancel()
			require.NoError(t, err)
			require.Equal(t, tt.expectedStatus, p.Status())
		})
	}
}

func TestProcessLifecycle_SessionRef_ReturnsThreadID(t *testing.T) {
	p := newTestProcess()

	// SessionRef should return the thread_id (session ID for Codex)
	require.Equal(t, "019b6dea-903b-7bd3-aef5-202a16205a9a", p.SessionRef())
}

func TestProcessLifecycle_SessionRef_InitiallyEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &Process{
		sessionID:  "", // No session ID set initially
		workDir:    "/test/project",
		status:     client.StatusRunning,
		events:     make(chan client.OutputEvent, 100),
		errors:     make(chan error, 10),
		cancelFunc: cancel,
		ctx:        ctx,
	}

	// SessionRef should be empty until thread.started event is processed
	require.Equal(t, "", p.SessionRef())
}

func TestProcessLifecycle_WorkDir(t *testing.T) {
	p := newTestProcess()
	require.Equal(t, "/test/project", p.WorkDir())
}

func TestProcessLifecycle_PID_NilProcess(t *testing.T) {
	p := newTestProcess()
	// cmd is nil, so PID should return 0
	require.Equal(t, 0, p.PID())
}

func TestProcessLifecycle_Wait_BlocksUntilCompletion(t *testing.T) {
	p := newTestProcess()

	// Add a WaitGroup counter to simulate goroutines
	p.wg.Add(1)

	// Wait should block until wg is done
	done := make(chan bool)
	go func() {
		p.Wait()
		done <- true
	}()

	// Wait should be blocking
	select {
	case <-done:
		require.Fail(t, "Wait should be blocking")
	case <-time.After(10 * time.Millisecond):
		// Expected - still waiting
	}

	// Release the waitgroup
	p.wg.Done()

	// Wait should now complete
	select {
	case <-done:
		// Expected - Wait completed
	case <-time.After(time.Second):
		require.Fail(t, "Wait should have completed after wg.Done()")
	}
}

func TestProcessLifecycle_SendError_NonBlocking(t *testing.T) {
	// Create a process with a full error channel
	p := &Process{
		errors: make(chan error, 2), // Small capacity
	}

	// Fill the channel
	p.errors <- errTest
	p.errors <- errTest

	// Channel is now full - sendError should not block
	done := make(chan bool)
	go func() {
		p.sendError(ErrTimeout) // This should not block
		done <- true
	}()

	select {
	case <-done:
		// Expected - sendError returned without blocking
	case <-time.After(100 * time.Millisecond):
		require.Fail(t, "sendError blocked on full channel - should have dropped error")
	}

	// Original errors should still be in channel
	require.Len(t, p.errors, 2)
}

func TestProcessLifecycle_SendError_SuccessWhenSpaceAvailable(t *testing.T) {
	p := newTestProcess()

	// sendError should send to channel when space available
	p.sendError(ErrTimeout)

	select {
	case err := <-p.errors:
		require.Equal(t, ErrTimeout, err)
	default:
		require.Fail(t, "Error should have been sent to channel")
	}
}

func TestProcessLifecycle_EventsChannelCapacity(t *testing.T) {
	p := newTestProcess()

	// Events channel should have capacity 100
	require.Equal(t, 100, cap(p.events))
}

func TestProcessLifecycle_ErrorsChannelCapacity(t *testing.T) {
	p := newTestProcess()

	// Errors channel should have capacity 10
	require.Equal(t, 10, cap(p.errors))
}

func TestProcessLifecycle_EventsChannel(t *testing.T) {
	p := newTestProcess()

	// Events channel should be readable
	eventsCh := p.Events()
	require.NotNil(t, eventsCh)

	// Send an event
	go func() {
		p.events <- client.OutputEvent{Type: client.EventSystem, SubType: "init"}
	}()

	select {
	case event := <-eventsCh:
		require.Equal(t, client.EventSystem, event.Type)
		require.Equal(t, "init", event.SubType)
	case <-time.After(time.Second):
		require.Fail(t, "Timeout waiting for event")
	}
}

func TestProcessLifecycle_ErrorsChannel(t *testing.T) {
	p := newTestProcess()

	// Errors channel should be readable
	errorsCh := p.Errors()
	require.NotNil(t, errorsCh)

	// Send an error
	go func() {
		p.errors <- errTest
	}()

	select {
	case err := <-errorsCh:
		require.Equal(t, errTest, err)
	case <-time.After(time.Second):
		require.Fail(t, "Timeout waiting for error")
	}
}

// =============================================================================
// Interface Compliance Tests
// =============================================================================

func TestProcess_ImplementsHeadlessProcess(t *testing.T) {
	// This test verifies at runtime that Process implements HeadlessProcess.
	// The compile-time check in process.go handles this, but this provides
	// additional runtime verification.
	var p client.HeadlessProcess = newTestProcess()
	require.NotNil(t, p)

	// Verify all interface methods are callable
	_ = p.Events()
	_ = p.Errors()
	_ = p.SessionRef()
	_ = p.Status()
	_ = p.IsRunning()
	_ = p.WorkDir()
	_ = p.PID()
}

// =============================================================================
// ErrTimeout Tests
// =============================================================================

func TestErrTimeout(t *testing.T) {
	require.NotNil(t, ErrTimeout)
	require.Contains(t, ErrTimeout.Error(), "timed out")
}
