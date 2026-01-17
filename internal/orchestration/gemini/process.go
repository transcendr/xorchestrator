package gemini

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zjrosen/xorchestrator/internal/log"
	"github.com/zjrosen/xorchestrator/internal/orchestration/client"
)

// Process represents a headless Gemini CLI process.
// Process implements client.HeadlessProcess.
type Process struct {
	cmd        *exec.Cmd
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

	// stderrLines captures stderr output for inclusion in error messages.
	// Protected by mu.
	stderrLines []string
}

// ErrTimeout is returned when a Gemini process exceeds its configured timeout.
var ErrTimeout = fmt.Errorf("gemini process timed out")

// ErrNoAuth is returned when no valid authentication is found.
var ErrNoAuth = fmt.Errorf("gemini: no authentication found - set GEMINI_API_KEY, GOOGLE_API_KEY, or run 'gemini auth' for OAuth")

// ErrNotFound is returned when the gemini executable cannot be found.
var ErrNotFound = fmt.Errorf("gemini: executable not found - install with 'npm install -g @anthropic-ai/claude-code-gemini' or ensure 'gemini' is in PATH")

// validateAuth checks for valid Gemini authentication.
// It checks in order: OAuth token file, GEMINI_API_KEY env, GOOGLE_API_KEY env.
func validateAuth() error {
	// Check for OAuth token at ~/.gemini/mcp-oauth-tokens-v2.json (Gemini CLI OAuth storage)
	homeDir, err := os.UserHomeDir()
	if err == nil {
		tokenPath := filepath.Join(homeDir, ".gemini", "mcp-oauth-tokens-v2.json")
		if _, err := os.Stat(tokenPath); err == nil {
			log.Debug(log.CatOrch, "Found OAuth token", "subsystem", "gemini", "path", tokenPath)
			return nil
		}
	}

	// Check for GEMINI_API_KEY environment variable
	if os.Getenv("GEMINI_API_KEY") != "" {
		log.Debug(log.CatOrch, "Found GEMINI_API_KEY", "subsystem", "gemini")
		return nil
	}

	// Check for GOOGLE_API_KEY environment variable
	if os.Getenv("GOOGLE_API_KEY") != "" {
		log.Debug(log.CatOrch, "Found GOOGLE_API_KEY", "subsystem", "gemini")
		return nil
	}

	return ErrNoAuth
}

// findExecutable locates the gemini executable.
// It checks in order: ~/.npm/bin/gemini, /usr/local/bin/gemini, then exec.LookPath.
func findExecutable() (string, error) {
	// On Windows, executables need .exe extension
	execName := "gemini"
	if os.PathSeparator == '\\' {
		execName = "gemini.exe"
	}

	// Check ~/.npm/bin/gemini first
	homeDir, err := os.UserHomeDir()
	if err == nil {
		npmPath := filepath.Join(homeDir, ".npm", "bin", execName)
		if _, err := os.Stat(npmPath); err == nil {
			log.Debug(log.CatOrch, "Found gemini at npm path", "subsystem", "gemini", "path", npmPath)
			return npmPath, nil
		}
	}

	// Check /usr/local/bin/gemini
	localPath := "/usr/local/bin/gemini"
	if _, err := os.Stat(localPath); err == nil {
		log.Debug(log.CatOrch, "Found gemini at local bin", "subsystem", "gemini", "path", localPath)
		return localPath, nil
	}

	// Fall back to exec.LookPath
	path, err := exec.LookPath("gemini")
	if err == nil {
		log.Debug(log.CatOrch, "Found gemini via PATH", "subsystem", "gemini", "path", path)
		return path, nil
	}

	return "", ErrNotFound
}

// setupMCPConfig creates or updates the .gemini/settings.json file with MCP configuration.
// If cfg.MCPConfig is empty, it's a no-op.
// The function merges mcpServers entries without overwriting other settings.
func setupMCPConfig(cfg Config) error {
	// No-op if MCPConfig is empty
	if cfg.MCPConfig == "" {
		return nil
	}

	// Parse the provided MCP config to extract mcpServers
	var mcpConfig map[string]any
	if err := json.Unmarshal([]byte(cfg.MCPConfig), &mcpConfig); err != nil {
		return fmt.Errorf("failed to parse MCPConfig JSON: %w", err)
	}

	// Ensure .gemini directory exists
	geminiDir := filepath.Join(cfg.WorkDir, ".gemini")
	if err := os.MkdirAll(geminiDir, 0750); err != nil {
		return fmt.Errorf("failed to create .gemini directory: %w", err)
	}

	settingsPath := filepath.Join(geminiDir, "settings.json")

	// Read existing settings.json if it exists
	existingSettings := make(map[string]any)
	existingData, err := os.ReadFile(settingsPath) //#nosec G304 -- path is constructed from validated config
	if err == nil {
		// File exists, parse it
		if err := json.Unmarshal(existingData, &existingSettings); err != nil {
			return fmt.Errorf("failed to parse existing settings.json: %w", err)
		}
	} else if !os.IsNotExist(err) {
		// Some other error reading the file
		return fmt.Errorf("failed to read settings.json: %w", err)
	}
	// If file doesn't exist, existingSettings remains an empty map

	// Get or create mcpServers map in existing settings
	var existingMCPServers map[string]any
	if existing, ok := existingSettings["mcpServers"]; ok {
		existingMCPServers, ok = existing.(map[string]any)
		if !ok {
			return fmt.Errorf("existing mcpServers is not a valid object")
		}
	} else {
		existingMCPServers = make(map[string]any)
	}

	// Merge new mcpServers into existing
	if newMCPServers, ok := mcpConfig["mcpServers"]; ok {
		newServers, ok := newMCPServers.(map[string]any)
		if !ok {
			return fmt.Errorf("mcpServers in MCPConfig is not a valid object")
		}
		maps.Copy(existingMCPServers, newServers)
	}

	// Update settings with merged mcpServers
	existingSettings["mcpServers"] = existingMCPServers

	// Write the merged settings back with proper formatting
	outputData, err := json.MarshalIndent(existingSettings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, outputData, 0600); err != nil {
		return fmt.Errorf("failed to write settings.json: %w", err)
	}

	log.Debug(log.CatOrch, "Wrote MCP config to settings.json", "subsystem", "gemini", "path", settingsPath)
	return nil
}

// Spawn creates and starts a new headless Gemini process.
// Context is used for cancellation and timeout control.
func Spawn(ctx context.Context, cfg Config) (*Process, error) {
	return spawnProcess(ctx, cfg, false)
}

// Resume continues an existing Gemini session using --resume flag.
func Resume(ctx context.Context, sessionID string, cfg Config) (*Process, error) {
	cfg.SessionID = sessionID
	return spawnProcess(ctx, cfg, true)
}

// spawnProcess is the internal implementation for both Spawn and Resume.
func spawnProcess(ctx context.Context, cfg Config, _ bool) (*Process, error) {
	// Validate authentication first
	if err := validateAuth(); err != nil {
		return nil, err
	}

	// Find the gemini executable
	execPath, err := findExecutable()
	if err != nil {
		return nil, err
	}

	// Setup MCP configuration (creates/updates .gemini/settings.json)
	if err := setupMCPConfig(cfg); err != nil {
		return nil, fmt.Errorf("failed to setup MCP config: %w", err)
	}

	var procCtx context.Context
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		procCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
	} else {
		procCtx, cancel = context.WithCancel(ctx)
	}

	args := buildArgs(cfg)
	log.Debug(log.CatOrch, "Spawning gemini process", "subsystem", "gemini", "args", strings.Join(args, " "), "workDir", cfg.WorkDir)

	// #nosec G204 -- args are built from Config struct, not user input
	cmd := exec.CommandContext(procCtx, execPath, args...)
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
		stdout:     stdout,
		stderr:     stderr,
		sessionID:  "", // Will be set from init event
		workDir:    cfg.WorkDir,
		status:     client.StatusPending,
		events:     make(chan client.OutputEvent, 100),
		errors:     make(chan error, 10),
		cancelFunc: cancel,
		ctx:        procCtx,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		log.Debug(log.CatOrch, "Failed to start gemini process", "subsystem", "gemini", "error", err)
		return nil, fmt.Errorf("failed to start gemini process: %w", err)
	}

	log.Debug(log.CatOrch, "Gemini process started", "subsystem", "gemini", "pid", cmd.Process.Pid)
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

// SessionRef returns the session reference (session ID for Gemini).
// May be empty until the init event is received.
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

// PID returns the process ID of the Gemini process, or 0 if not running.
func (p *Process) PID() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// Cancel terminates the Gemini process.
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
		log.Debug(log.CatOrch, "Error channel full, dropping error", "subsystem", "gemini", "error", err)
	}
}

// parseOutput reads stdout and parses stream-json events.
func (p *Process) parseOutput() {
	defer p.wg.Done()
	defer close(p.events)

	log.Debug(log.CatOrch, "Starting output parser", "subsystem", "gemini")

	scanner := bufio.NewScanner(p.stdout)
	// Increase buffer size for large outputs (64KB initial, 1MB max)
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
		log.Debug(log.CatOrch, "RAW_JSON", "subsystem", "gemini", "lineNum", lineCount, "json", string(line))

		event, err := ParseEvent(line)
		if err != nil {
			log.Debug(log.CatOrch, "Failed to parse JSON", "subsystem", "gemini", "error", err, "line", string(line[:min(100, len(line))]))
			continue
		}

		log.Debug(log.CatOrch, "Parsed event", "subsystem", "gemini", "type", event.Type, "subtype", event.SubType, "hasTool", event.Tool != nil, "hasMessage", event.Message != nil)

		// Log Usage data for debugging token tracking
		if event.Type == client.EventResult || event.Usage != nil {
			log.Debug(log.CatOrch, "EVENT_USAGE",
				"subsystem", "gemini",
				"type", event.Type,
				"hasUsage", event.Usage != nil,
				"totalCostUSD", event.TotalCostUSD,
				"durationMs", event.DurationMs)
			if event.Usage != nil {
				log.Debug(log.CatOrch, "USAGE_DETAILS",
					"subsystem", "gemini",
					"tokensUsed", event.Usage.TokensUsed,
					"totalTokens", event.Usage.TotalTokens,
					"outputTokens", event.Usage.OutputTokens)
			}
		}

		event.Timestamp = time.Now()

		// Extract session_id from init event
		if event.Type == client.EventSystem && event.SubType == "init" && event.SessionID != "" {
			p.mu.Lock()
			p.sessionID = event.SessionID
			p.mu.Unlock()
			log.Debug(log.CatOrch, "Got session ID", "subsystem", "gemini", "sessionID", event.SessionID)
		}

		select {
		case p.events <- event:
			log.Debug(log.CatOrch, "Sent event to channel", "subsystem", "gemini", "type", event.Type)
		case <-p.ctx.Done():
			log.Debug(log.CatOrch, "Context done, stopping parser", "subsystem", "gemini")
			return
		}
	}

	log.Debug(log.CatOrch, "Scanner finished", "subsystem", "gemini", "totalLines", lineCount)

	if err := scanner.Err(); err != nil {
		log.Debug(log.CatOrch, "Scanner error", "subsystem", "gemini", "error", err)
		p.sendError(fmt.Errorf("stdout scanner error: %w", err))
	}
}

// parseStderr reads and logs stderr output, capturing lines for error messages.
func (p *Process) parseStderr() {
	defer p.wg.Done()

	scanner := bufio.NewScanner(p.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		log.Debug(log.CatOrch, "STDERR", "subsystem", "gemini", "line", line)

		// Capture stderr lines for inclusion in error messages
		p.mu.Lock()
		p.stderrLines = append(p.stderrLines, line)
		p.mu.Unlock()
	}
	if err := scanner.Err(); err != nil {
		log.Debug(log.CatOrch, "Stderr scanner error", "subsystem", "gemini", "error", err)
	}
}

// waitForCompletion waits for the process to exit and updates status.
// It closes the errors channel when done to signal completion to consumers.
func (p *Process) waitForCompletion() {
	defer p.wg.Done()
	defer close(p.errors) // Signal that no more errors will be sent

	log.Debug(log.CatOrch, "Waiting for process to complete", "subsystem", "gemini")
	err := p.cmd.Wait()
	log.Debug(log.CatOrch, "Process completed", "subsystem", "gemini", "error", err)

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.status == client.StatusCancelled {
		// Already cancelled, don't override
		log.Debug(log.CatOrch, "Process was cancelled", "subsystem", "gemini")
		return
	}

	// Check if this was a timeout
	if errors.Is(p.ctx.Err(), context.DeadlineExceeded) {
		p.status = client.StatusFailed
		log.Debug(log.CatOrch, "Process timed out", "subsystem", "gemini")
		p.sendError(ErrTimeout)
		return
	}

	if err != nil {
		p.status = client.StatusFailed
		log.Debug(log.CatOrch, "Process failed", "subsystem", "gemini", "error", err)
		// Include stderr output in error message if available
		if len(p.stderrLines) > 0 {
			stderrMsg := strings.Join(p.stderrLines, "\n")
			p.sendError(fmt.Errorf("gemini process failed: %s (exit: %w)", stderrMsg, err))
		} else {
			p.sendError(fmt.Errorf("gemini process exited: %w", err))
		}
	} else {
		p.status = client.StatusCompleted
		log.Debug(log.CatOrch, "Process completed successfully", "subsystem", "gemini")
	}
}

// Ensure Process implements client.HeadlessProcess at compile time.
var _ client.HeadlessProcess = (*Process)(nil)
