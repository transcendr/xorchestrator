package codex

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/client"
)

// Process represents a headless Codex CLI process.
// Process implements client.HeadlessProcess.
type Process struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	stderr     io.ReadCloser
	sessionID  string
	workDir    string
	status     client.ProcessStatus
	events     chan client.OutputEvent
	errors     chan error
	cancelFunc context.CancelFunc
	ctx        context.Context
	mu         sync.RWMutex
	wg         sync.WaitGroup
}

// ErrTimeout is returned when a Codex process exceeds its configured timeout.
var ErrTimeout = fmt.Errorf("codex process timed out")

// Spawn creates and starts a new headless Codex process.
// Context is used for cancellation and timeout control.
func Spawn(ctx context.Context, cfg Config) (*Process, error) {
	return spawnProcess(ctx, cfg, false)
}

// Resume continues an existing Codex session.
func Resume(ctx context.Context, sessionID string, cfg Config) (*Process, error) {
	cfg.SessionID = sessionID
	return spawnProcess(ctx, cfg, true)
}

// spawnProcess is the internal implementation for both Spawn and Resume.
func spawnProcess(ctx context.Context, cfg Config, isResume bool) (*Process, error) {
	var procCtx context.Context
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		procCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
	} else {
		procCtx, cancel = context.WithCancel(ctx)
	}

	args := buildArgs(cfg, isResume)
	log.Debug(log.CatOrch, "Spawning codex process", "subsystem", "codex", "args", strings.Join(args, " "), "workDir", cfg.WorkDir)

	// #nosec G204 -- args are built from Config struct, not user input
	cmd := exec.CommandContext(procCtx, "codex", args...)
	// Set working directory via cmd.Dir as belt-and-suspenders with -C flag
	cmd.Dir = cfg.WorkDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	p := &Process{
		cmd:        cmd,
		stdin:      nil, // Codex uses positional prompt argument, not stdin
		stdout:     stdout,
		stderr:     stderr,
		sessionID:  cfg.SessionID,
		workDir:    cfg.WorkDir,
		status:     client.StatusPending,
		events:     make(chan client.OutputEvent, 100),
		errors:     make(chan error, 10),
		cancelFunc: cancel,
		ctx:        procCtx,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		log.Debug(log.CatOrch, "Failed to start codex process", "subsystem", "codex", "error", err)
		return nil, fmt.Errorf("failed to start codex process: %w", err)
	}

	log.Debug(log.CatOrch, "Codex process started", "subsystem", "codex", "pid", cmd.Process.Pid)
	p.setStatus(client.StatusRunning)

	// Start output parser goroutines
	p.wg.Add(3)
	go p.parseOutput()
	go p.parseStderr()
	go p.waitForCompletion()

	return p, nil
}

// Events returns a channel that receives parsed output events.
func (p *Process) Events() <-chan client.OutputEvent {
	return p.events
}

// Errors returns a channel that receives errors.
func (p *Process) Errors() <-chan error {
	return p.errors
}

// SessionRef returns the session reference (thread_id for Codex).
// May be empty until the thread.started event is received.
func (p *Process) SessionRef() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sessionID
}

// Status returns the current process status.
func (p *Process) Status() client.ProcessStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

// IsRunning returns true if the process is currently running.
func (p *Process) IsRunning() bool {
	return p.Status() == client.StatusRunning
}

// WorkDir returns the working directory of the process.
func (p *Process) WorkDir() string {
	return p.workDir
}

// PID returns the process ID of the Codex process, or 0 if not running.
func (p *Process) PID() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// Cancel terminates the Codex process.
// The status is set before calling cancelFunc to prevent race with waitForCompletion.
func (p *Process) Cancel() error {
	p.mu.Lock()
	// Only set to cancelled if not already in a terminal state
	if !p.status.IsTerminal() {
		p.status = client.StatusCancelled
	}
	p.mu.Unlock()
	p.cancelFunc()
	return nil
}

// Wait blocks until the process completes.
func (p *Process) Wait() error {
	p.wg.Wait()
	return nil
}

// setStatus updates the process status thread-safely.
func (p *Process) setStatus(s client.ProcessStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = s
}

// sendError attempts to send an error to the errors channel.
// If the channel is full, the error is logged but not sent to avoid blocking.
func (p *Process) sendError(err error) {
	select {
	case p.errors <- err:
		// Error sent successfully
	default:
		// Channel full, log the dropped error
		log.Debug(log.CatOrch, "Error channel full, dropping error", "subsystem", "codex", "error", err)
	}
}

// parseOutput reads stdout and parses JSONL events.
func (p *Process) parseOutput() {
	defer p.wg.Done()
	defer close(p.events)

	log.Debug(log.CatOrch, "Starting output parser", "subsystem", "codex")

	scanner := bufio.NewScanner(p.stdout)
	// Increase buffer size for large outputs (1MB max line size)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	lineCount := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		lineCount++

		if len(line) == 0 {
			continue
		}

		// Log raw JSON for debugging
		log.Debug(log.CatOrch, "RAW_JSON", "subsystem", "codex", "lineNum", lineCount, "json", string(line))

		event, err := ParseEvent(line)
		if err != nil {
			log.Debug(log.CatOrch, "Failed to parse JSON", "subsystem", "codex", "error", err, "line", string(line[:min(100, len(line))]))
			continue
		}

		log.Debug(log.CatOrch, "Parsed event", "subsystem", "codex", "type", event.Type, "subtype", event.SubType, "hasTool", event.Tool != nil, "hasMessage", event.Message != nil)

		// Log Usage data for debugging token tracking
		if event.Type == client.EventResult || event.Usage != nil {
			log.Debug(log.CatOrch, "EVENT_USAGE",
				"subsystem", "codex",
				"type", event.Type,
				"hasUsage", event.Usage != nil,
				"totalCostUSD", event.TotalCostUSD,
				"durationMs", event.DurationMs)
			if event.Usage != nil {
				log.Debug(log.CatOrch, "USAGE_DETAILS",
					"subsystem", "codex",
					"inputTokens", event.Usage.InputTokens,
					"outputTokens", event.Usage.OutputTokens,
					"cacheReadInputTokens", event.Usage.CacheReadInputTokens,
					"cacheCreationInputTokens", event.Usage.CacheCreationInputTokens)
			}
		}

		event.Timestamp = time.Now()

		// Extract thread_id from thread.started event for SessionRef()
		if event.Type == client.EventSystem && event.SubType == "init" && event.SessionID != "" {
			p.mu.Lock()
			p.sessionID = event.SessionID
			p.mu.Unlock()
			log.Debug(log.CatOrch, "Got session ID (thread_id)", "subsystem", "codex", "sessionID", event.SessionID)
		}

		select {
		case p.events <- event:
			log.Debug(log.CatOrch, "Sent event to channel", "subsystem", "codex", "type", event.Type)
		case <-p.ctx.Done():
			log.Debug(log.CatOrch, "Context done, stopping parser", "subsystem", "codex")
			return
		}
	}

	log.Debug(log.CatOrch, "Scanner finished", "subsystem", "codex", "totalLines", lineCount)

	if err := scanner.Err(); err != nil {
		log.Debug(log.CatOrch, "Scanner error", "subsystem", "codex", "error", err)
		p.sendError(fmt.Errorf("stdout scanner error: %w", err))
	}
}

// parseStderr reads and logs stderr output.
func (p *Process) parseStderr() {
	defer p.wg.Done()

	scanner := bufio.NewScanner(p.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		log.Debug(log.CatOrch, "STDERR", "subsystem", "codex", "line", line)
	}
	if err := scanner.Err(); err != nil {
		log.Debug(log.CatOrch, "Stderr scanner error", "subsystem", "codex", "error", err)
	}
}

// waitForCompletion waits for the process to exit and updates status.
func (p *Process) waitForCompletion() {
	defer p.wg.Done()

	log.Debug(log.CatOrch, "Waiting for process to complete", "subsystem", "codex")
	err := p.cmd.Wait()
	log.Debug(log.CatOrch, "Process completed", "subsystem", "codex", "error", err)

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.status == client.StatusCancelled {
		// Already cancelled, don't override
		log.Debug(log.CatOrch, "Process was cancelled", "subsystem", "codex")
		return
	}

	// Check if this was a timeout
	if p.ctx.Err() == context.DeadlineExceeded {
		p.status = client.StatusFailed
		log.Debug(log.CatOrch, "Process timed out", "subsystem", "codex")
		p.sendError(ErrTimeout)
		return
	}

	if err != nil {
		p.status = client.StatusFailed
		log.Debug(log.CatOrch, "Process failed", "subsystem", "codex", "error", err)
		p.sendError(fmt.Errorf("codex process exited: %w", err))
	} else {
		p.status = client.StatusCompleted
		log.Debug(log.CatOrch, "Process completed successfully", "subsystem", "codex")
	}
}

// Ensure Process implements client.HeadlessProcess at compile time.
var _ client.HeadlessProcess = (*Process)(nil)
