package processor

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zjrosen/xorchestrator/internal/orchestration/v2/command"
	"github.com/zjrosen/xorchestrator/internal/pubsub"
	"pgregory.net/rapid"
)

// ===========================================================================
// Test Helpers
// ===========================================================================

// testCommand is a simple command for testing.
type testCommand struct {
	*command.BaseCommand
	value        int
	shouldFail   bool
	validateErr  error
	processingMs int // Artificial delay in milliseconds
}

func newTestCommand(value int) *testCommand {
	base := command.NewBaseCommand("test_command", command.SourceInternal)
	return &testCommand{
		BaseCommand: &base,
		value:       value,
	}
}

func (c *testCommand) Validate() error {
	if c.validateErr != nil {
		return c.validateErr
	}
	return nil
}

// testHandler records processed commands for verification.
type testHandler struct {
	mu        sync.Mutex
	processed []int
	errors    []error
}

func newTestHandler() *testHandler {
	return &testHandler{
		processed: make([]int, 0),
	}
}

func (h *testHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	tc, ok := cmd.(*testCommand)
	if !ok {
		return nil, errors.New("unexpected command type")
	}

	// Artificial delay for testing
	if tc.processingMs > 0 {
		time.Sleep(time.Duration(tc.processingMs) * time.Millisecond)
	}

	if tc.shouldFail {
		h.mu.Lock()
		h.errors = append(h.errors, errors.New("handler failure"))
		h.mu.Unlock()
		return nil, errors.New("handler failure")
	}

	h.mu.Lock()
	h.processed = append(h.processed, tc.value)
	h.mu.Unlock()

	return &command.CommandResult{
		Success: true,
		Data:    tc.value,
	}, nil
}

func (h *testHandler) getProcessed() []int {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]int, len(h.processed))
	copy(result, h.processed)
	return result
}

// startProcessor creates and starts a processor, returning it and a cleanup function.
func startProcessor(t *testing.T, opts ...Option) (*CommandProcessor, *testHandler, func()) {
	handler := newTestHandler()
	p := NewCommandProcessor(opts...)
	p.RegisterHandler("test_command", handler)

	ctx, cancel := context.WithCancel(context.Background())
	go p.Run(ctx)

	// Wait for processor to start
	require.Eventually(t, func() bool {
		return p.IsRunning()
	}, time.Second, 10*time.Millisecond)

	cleanup := func() {
		cancel()
		p.Stop()
	}

	return p, handler, cleanup
}

// ===========================================================================
// Basic Flow Tests
// ===========================================================================

func TestProcessor_SubmitAndProcess(t *testing.T) {
	p, handler, cleanup := startProcessor(t)
	defer cleanup()

	cmd := newTestCommand(42)
	err := p.Submit(cmd)
	require.NoError(t, err)

	// Wait for processing
	require.Eventually(t, func() bool {
		return len(handler.getProcessed()) == 1
	}, time.Second, 10*time.Millisecond)

	processed := handler.getProcessed()
	assert.Equal(t, []int{42}, processed)
	assert.Equal(t, int64(1), p.ProcessedCount())
}

func TestProcessor_Submit_QueueFull(t *testing.T) {
	// Create processor with tiny queue
	p, _, cleanup := startProcessor(t, WithQueueCapacity(1))
	defer cleanup()

	// Fill the queue by pausing the handler
	handler := &blockingHandler{
		ready:   make(chan struct{}),
		proceed: make(chan struct{}),
	}
	p.handlers["blocking_command"] = handler

	// Submit a blocking command to occupy the processor
	cmd1 := &blockingCommand{BaseCommand: baseCmd("blocking_command")}
	err := p.Submit(cmd1)
	require.NoError(t, err)

	// Wait until handler starts processing
	<-handler.ready

	// Now the processor is blocked, fill the queue
	cmd2 := newTestCommand(1)
	err = p.Submit(cmd2)
	require.NoError(t, err)

	// Queue is now full (capacity 1), next should fail
	cmd3 := newTestCommand(2)
	err = p.Submit(cmd3)
	assert.ErrorIs(t, err, command.ErrQueueFull)

	// Let the blocked command proceed
	close(handler.proceed)
}

// ===========================================================================
// WaitForReady Tests
// ===========================================================================

func TestProcessor_WaitForReady(t *testing.T) {
	// Create processor but don't start it yet
	handler := newTestHandler()
	p := NewCommandProcessor()
	p.RegisterHandler("test_command", handler)

	ctx := context.Background()

	// Start the processor in a goroutine
	go p.Run(ctx)

	// WaitForReady should return quickly once processor is running
	err := p.WaitForReady(ctx)
	require.NoError(t, err)

	// Verify processor is actually ready by submitting a command
	cmd := newTestCommand(42)
	err = p.Submit(cmd)
	require.NoError(t, err)

	// Cleanup
	p.Stop()
}

func TestProcessor_WaitForReady_AlreadyRunning(t *testing.T) {
	p, _, cleanup := startProcessor(t)
	defer cleanup()

	// Should return immediately if already running
	err := p.WaitForReady(context.Background())
	require.NoError(t, err)
}

func TestProcessor_WaitForReady_ContextCancelled(t *testing.T) {
	// Create processor but never start it
	p := NewCommandProcessor()

	// Cancel context before waiting
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.WaitForReady(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

// ===========================================================================
// SubmitAndWait Tests
// ===========================================================================

func TestProcessor_SubmitAndWait(t *testing.T) {
	p, _, cleanup := startProcessor(t)
	defer cleanup()

	cmd := newTestCommand(123)
	result, err := p.SubmitAndWait(context.Background(), cmd)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, 123, result.Data)
}

func TestProcessor_SubmitAndWait_Timeout(t *testing.T) {
	p, _, cleanup := startProcessor(t)
	defer cleanup()

	// Register a slow handler
	slowHandler := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		time.Sleep(500 * time.Millisecond)
		return &command.CommandResult{Success: true}, nil
	})
	p.handlers["slow_command"] = slowHandler

	cmd := &simpleCommand{BaseCommand: baseCmd("slow_command")}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result, err := p.SubmitAndWait(ctx, cmd)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestProcessor_SubmitAndWait_ContextCancelled(t *testing.T) {
	p, _, cleanup := startProcessor(t)
	defer cleanup()

	// Register a slow handler
	slowHandler := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		time.Sleep(500 * time.Millisecond)
		return &command.CommandResult{Success: true}, nil
	})
	p.handlers["slow_command"] = slowHandler

	cmd := &simpleCommand{BaseCommand: baseCmd("slow_command")}
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result, err := p.SubmitAndWait(ctx, cmd)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.Canceled)
}

// ===========================================================================
// Validation and Error Tests
// ===========================================================================

func TestProcessor_ValidationError(t *testing.T) {
	eventBus := pubsub.NewBroker[any]()
	defer eventBus.Close()

	p, _, cleanup := startProcessor(t, WithEventBus(eventBus))
	defer cleanup()

	// Subscribe to events
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := eventBus.Subscribe(ctx)

	cmd := newTestCommand(1)
	cmd.validateErr = errors.New("validation failed")

	result, err := p.SubmitAndWait(context.Background(), cmd)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.EqualError(t, result.Error, "validation failed")
	assert.Equal(t, int64(1), p.ErrorCount())

	// Check error event was emitted
	select {
	case event := <-events:
		errorEvent, ok := event.Payload.(CommandErrorEvent)
		require.True(t, ok)
		assert.Equal(t, cmd.ID(), errorEvent.CommandID)
		assert.EqualError(t, errorEvent.Error, "validation failed")
	case <-time.After(time.Second):
		require.FailNow(t, "timeout waiting for error event")
	}
}

func TestProcessor_HandlerError(t *testing.T) {
	eventBus := pubsub.NewBroker[any]()
	defer eventBus.Close()

	p, _, cleanup := startProcessor(t, WithEventBus(eventBus))
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := eventBus.Subscribe(ctx)

	cmd := newTestCommand(1)
	cmd.shouldFail = true

	result, err := p.SubmitAndWait(context.Background(), cmd)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.EqualError(t, result.Error, "handler failure")
	assert.Equal(t, int64(1), p.ErrorCount())

	// Check error event was emitted
	select {
	case event := <-events:
		errorEvent, ok := event.Payload.(CommandErrorEvent)
		require.True(t, ok)
		assert.EqualError(t, errorEvent.Error, "handler failure")
	case <-time.After(time.Second):
		require.FailNow(t, "timeout waiting for error event")
	}
}

func TestProcessor_UnknownCommandType(t *testing.T) {
	eventBus := pubsub.NewBroker[any]()
	defer eventBus.Close()

	p, _, cleanup := startProcessor(t, WithEventBus(eventBus))
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := eventBus.Subscribe(ctx)

	// Submit a command with no registered handler
	cmd := &simpleCommand{BaseCommand: baseCmd("unknown_type")}

	result, err := p.SubmitAndWait(context.Background(), cmd)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.ErrorIs(t, result.Error, ErrUnknownCommandType)

	// Check error event
	select {
	case event := <-events:
		errorEvent, ok := event.Payload.(CommandErrorEvent)
		require.True(t, ok)
		assert.ErrorIs(t, errorEvent.Error, ErrUnknownCommandType)
	case <-time.After(time.Second):
		require.FailNow(t, "timeout waiting for error event")
	}
}

// ===========================================================================
// Follow-up Commands Tests
// ===========================================================================

func TestProcessor_FollowUpCommands(t *testing.T) {
	p, handler, cleanup := startProcessor(t)
	defer cleanup()

	// Register handler that generates follow-up commands
	followUpHandler := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		tc := cmd.(*testCommand)
		if tc.value == 1 {
			// Generate follow-up commands
			return &command.CommandResult{
				Success: true,
				Data:    1,
				FollowUp: []command.Command{
					newTestCommand(2),
					newTestCommand(3),
				},
			}, nil
		}
		// Default: just record the value
		handler.mu.Lock()
		handler.processed = append(handler.processed, tc.value)
		handler.mu.Unlock()
		return &command.CommandResult{Success: true, Data: tc.value}, nil
	})
	p.handlers["test_command"] = followUpHandler

	cmd := newTestCommand(1)
	result, err := p.SubmitAndWait(context.Background(), cmd)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	// Wait for follow-up commands to be processed
	require.Eventually(t, func() bool {
		return p.ProcessedCount() >= 3
	}, time.Second, 10*time.Millisecond)

	// Check that follow-ups were processed
	processed := handler.getProcessed()
	assert.Contains(t, processed, 2)
	assert.Contains(t, processed, 3)
}

// ===========================================================================
// Events Tests
// ===========================================================================

func TestProcessor_Events(t *testing.T) {
	eventBus := pubsub.NewBroker[any]()
	defer eventBus.Close()

	p, _, cleanup := startProcessor(t, WithEventBus(eventBus))
	defer cleanup()

	// Subscribe to events
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := eventBus.Subscribe(ctx)

	// Register handler that emits events
	eventHandler := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return &command.CommandResult{
			Success: true,
			Events:  []any{"event1", "event2"},
		}, nil
	})
	p.handlers["event_command"] = eventHandler

	cmd := &simpleCommand{BaseCommand: baseCmd("event_command")}
	_, err := p.SubmitAndWait(context.Background(), cmd)
	require.NoError(t, err)

	// Check events were emitted
	received := make([]any, 0)
	timeout := time.After(time.Second)
	for i := 0; i < 2; i++ {
		select {
		case event := <-events:
			received = append(received, event.Payload)
		case <-timeout:
			require.FailNow(t, "timeout waiting for events")
		}
	}

	assert.Contains(t, received, "event1")
	assert.Contains(t, received, "event2")
}

// ===========================================================================
// Lifecycle Tests
// ===========================================================================

func TestProcessor_Stop(t *testing.T) {
	p := NewCommandProcessor()
	handler := newTestHandler()
	p.RegisterHandler("test_command", handler)

	ctx, cancel := context.WithCancel(context.Background())
	go p.Run(ctx)

	require.Eventually(t, func() bool {
		return p.IsRunning()
	}, time.Second, 10*time.Millisecond)

	// Submit a command
	err := p.Submit(newTestCommand(1))
	require.NoError(t, err)

	// Stop the processor
	cancel()
	p.Stop()

	assert.False(t, p.IsRunning())

	// Submitting after stop should fail
	err = p.Submit(newTestCommand(2))
	assert.ErrorIs(t, err, command.ErrQueueFull)
}

func TestProcessor_Drain(t *testing.T) {
	p := NewCommandProcessor()
	handler := newTestHandler()
	p.RegisterHandler("test_command", handler)

	ctx := context.Background()
	go p.Run(ctx)

	require.Eventually(t, func() bool {
		return p.IsRunning()
	}, time.Second, 10*time.Millisecond)

	// Submit multiple commands
	for i := 1; i <= 5; i++ {
		err := p.Submit(newTestCommand(i))
		require.NoError(t, err)
	}

	// Drain processes all pending commands
	p.Drain()

	assert.False(t, p.IsRunning())

	// All commands should have been processed
	processed := handler.getProcessed()
	assert.Len(t, processed, 5)
	assert.Contains(t, processed, 1)
	assert.Contains(t, processed, 5)
}

func TestProcessor_ShutdownDrain(t *testing.T) {
	p := NewCommandProcessor()

	// Handler with artificial delay
	var processed atomic.Int32
	slowHandler := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		time.Sleep(10 * time.Millisecond)
		processed.Add(1)
		return &command.CommandResult{Success: true}, nil
	})
	p.RegisterHandler("slow_command", slowHandler)

	ctx := context.Background()
	go p.Run(ctx)

	require.Eventually(t, func() bool {
		return p.IsRunning()
	}, time.Second, 10*time.Millisecond)

	// Submit commands
	for i := 0; i < 5; i++ {
		cmd := &simpleCommand{BaseCommand: baseCmd("slow_command")}
		err := p.Submit(cmd)
		require.NoError(t, err)
	}

	// Drain should process all
	p.Drain()

	assert.Equal(t, int32(5), processed.Load())
}

// ===========================================================================
// Metrics Tests
// ===========================================================================

func TestProcessor_Metrics(t *testing.T) {
	p, _, cleanup := startProcessor(t)
	defer cleanup()

	// Submit successful commands
	for i := 0; i < 3; i++ {
		_, err := p.SubmitAndWait(context.Background(), newTestCommand(i))
		require.NoError(t, err)
	}

	// Submit failing command
	cmd := newTestCommand(1)
	cmd.shouldFail = true
	_, err := p.SubmitAndWait(context.Background(), cmd)
	require.NoError(t, err)

	assert.Equal(t, int64(4), p.ProcessedCount())
	assert.Equal(t, int64(1), p.ErrorCount())
}

// ===========================================================================
// Cancellation Tests
// ===========================================================================

func TestProcessor_Cancellation(t *testing.T) {
	p := NewCommandProcessor()

	// Slow handler
	var started atomic.Bool
	var completed atomic.Bool
	slowHandler := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		started.Store(true)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			completed.Store(true)
			return &command.CommandResult{Success: true}, nil
		}
	})
	p.RegisterHandler("slow_command", slowHandler)

	ctx, cancel := context.WithCancel(context.Background())
	go p.Run(ctx)

	require.Eventually(t, func() bool {
		return p.IsRunning()
	}, time.Second, 10*time.Millisecond)

	// Submit slow command
	cmd := &simpleCommand{BaseCommand: baseCmd("slow_command")}
	err := p.Submit(cmd)
	require.NoError(t, err)

	// Wait for handler to start
	require.Eventually(t, func() bool {
		return started.Load()
	}, time.Second, 10*time.Millisecond)

	// Cancel processor
	cancel()
	p.Stop()

	// Handler should not complete (was cancelled)
	assert.False(t, completed.Load())
}

// ===========================================================================
// High Contention Tests
// ===========================================================================

func TestProcessor_HighContention(t *testing.T) {
	p, handler, cleanup := startProcessor(t, WithQueueCapacity(100))
	defer cleanup()

	const numGoroutines = 10
	const commandsPerGoroutine = 50

	var wg sync.WaitGroup
	var submitted atomic.Int32
	var failed atomic.Int32

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < commandsPerGoroutine; i++ {
				value := goroutineID*1000 + i
				cmd := newTestCommand(value)
				err := p.Submit(cmd)
				if err != nil {
					failed.Add(1)
				} else {
					submitted.Add(1)
				}
			}
		}(g)
	}

	wg.Wait()

	// Wait for processing to complete
	require.Eventually(t, func() bool {
		return p.ProcessedCount() >= int64(submitted.Load())
	}, 5*time.Second, 10*time.Millisecond)

	// Verify all submitted commands were processed
	processed := handler.getProcessed()
	assert.Equal(t, int(submitted.Load()), len(processed))
}

// ===========================================================================
// FIFO Property-Based Test - THE CRITICAL ARCHITECTURAL GUARANTEE
// ===========================================================================

func TestProcessor_FIFO_PropertyBased(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random number of commands (1-100)
		numCommands := rapid.IntRange(1, 100).Draw(rt, "numCommands")

		p := NewCommandProcessor()

		// Track submission and processing order
		var submissionOrder []int
		var processingOrder []int
		var mu sync.Mutex

		// Handler that records processing order
		handler := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
			tc := cmd.(*testCommand)
			mu.Lock()
			processingOrder = append(processingOrder, tc.value)
			mu.Unlock()
			return &command.CommandResult{Success: true, Data: tc.value}, nil
		})
		p.RegisterHandler("test_command", handler)

		ctx, cancel := context.WithCancel(context.Background())
		go p.Run(ctx)

		// Wait for processor to start
		for !p.IsRunning() {
			time.Sleep(time.Millisecond)
		}

		// Submit commands in order
		for i := 0; i < numCommands; i++ {
			submissionOrder = append(submissionOrder, i)
			cmd := newTestCommand(i)

			// Use SubmitAndWait to ensure ordering within rapid test
			// But we need to submit fast, so use Submit and track externally
			err := p.Submit(cmd)
			require.NoError(rt, err)
		}

		// Drain to process all commands
		p.Drain()
		cancel()

		// CRITICAL INVARIANT: Processing order MUST equal submission order
		mu.Lock()
		defer mu.Unlock()

		require.Equal(rt, submissionOrder, processingOrder,
			"FIFO violated! Submitted: %v, Processed: %v", submissionOrder, processingOrder)
	})
}

// TestProcessor_FIFO_StressTest is an additional non-rapid stress test
func TestProcessor_FIFO_StressTest(t *testing.T) {
	const numCommands = 1000

	p := NewCommandProcessor()

	var processingOrder []int
	var mu sync.Mutex

	handler := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		tc := cmd.(*testCommand)
		mu.Lock()
		processingOrder = append(processingOrder, tc.value)
		mu.Unlock()
		return &command.CommandResult{Success: true}, nil
	})
	p.RegisterHandler("test_command", handler)

	ctx := context.Background()
	go p.Run(ctx)

	for !p.IsRunning() {
		time.Sleep(time.Millisecond)
	}

	// Submit all commands
	for i := 0; i < numCommands; i++ {
		err := p.Submit(newTestCommand(i))
		require.NoError(t, err)
	}

	// Drain to completion
	p.Drain()

	// Verify strict FIFO ordering
	mu.Lock()
	defer mu.Unlock()

	require.Len(t, processingOrder, numCommands)
	for i := 0; i < numCommands; i++ {
		assert.Equal(t, i, processingOrder[i], "FIFO violated at position %d", i)
	}
}

// ===========================================================================
// Additional Helper Types
// ===========================================================================

// simpleCommand is a minimal command for testing.
type simpleCommand struct {
	*command.BaseCommand
}

func (c *simpleCommand) Validate() error {
	return nil
}

func baseCmd(cmdType command.CommandType) *command.BaseCommand {
	base := command.NewBaseCommand(cmdType, command.SourceInternal)
	return &base
}

// blockingCommand is a command that blocks until told to proceed.
type blockingCommand struct {
	*command.BaseCommand
}

func (c *blockingCommand) Validate() error {
	return nil
}

// blockingHandler signals when it starts and waits to proceed.
type blockingHandler struct {
	ready   chan struct{}
	proceed chan struct{}
}

func (h *blockingHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	close(h.ready)
	<-h.proceed
	return &command.CommandResult{Success: true}, nil
}

// ===========================================================================
// Middleware Integration Tests
// ===========================================================================

func TestCommandProcessor_WithMiddleware(t *testing.T) {
	var order []string
	var mu sync.Mutex

	recordMiddleware := func(name string) Middleware {
		return func(next CommandHandler) CommandHandler {
			return HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
				mu.Lock()
				order = append(order, name+"-before")
				mu.Unlock()

				result, err := next.Handle(ctx, cmd)

				mu.Lock()
				order = append(order, name+"-after")
				mu.Unlock()

				return result, err
			})
		}
	}

	handler := newTestHandler()
	p := NewCommandProcessor(
		WithMiddleware(recordMiddleware("first"), recordMiddleware("second")),
	)
	p.RegisterHandler("test_command", handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go p.Run(ctx)
	defer p.Stop()

	time.Sleep(10 * time.Millisecond)

	cmd := newTestCommand(42)
	result, err := p.SubmitAndWait(ctx, cmd)

	require.NoError(t, err)
	require.True(t, result.Success)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"first-before", "second-before", "second-after", "first-after"}, order)
}

func TestCommandProcessor_WithMiddleware_AppliedToAllHandlers(t *testing.T) {
	var callCount atomic.Int32

	countingMiddleware := func(next CommandHandler) CommandHandler {
		return HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
			callCount.Add(1)
			return next.Handle(ctx, cmd)
		})
	}

	p := NewCommandProcessor(WithMiddleware(countingMiddleware))

	handler1 := newTestHandler()
	handler2 := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return &command.CommandResult{Success: true}, nil
	})

	p.RegisterHandler("test_command", handler1)
	p.RegisterHandler("other_command", handler2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go p.Run(ctx)
	defer p.Stop()

	time.Sleep(10 * time.Millisecond)

	_, _ = p.SubmitAndWait(ctx, newTestCommand(1))
	_, _ = p.SubmitAndWait(ctx, &simpleCommand{baseCmd("other_command")})

	assert.Equal(t, int32(2), callCount.Load())
}
