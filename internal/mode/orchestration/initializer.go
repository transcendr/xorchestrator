package orchestration

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/amp"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/coordinator"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/mcp"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/pool"
	"github.com/zjrosen/perles/internal/pubsub"
)

// InitializerEventType represents the type of event emitted by the Initializer.
type InitializerEventType int

const (
	// InitEventPhaseChanged indicates a phase transition occurred.
	InitEventPhaseChanged InitializerEventType = iota
	// InitEventReady indicates initialization completed successfully.
	InitEventReady
	// InitEventFailed indicates initialization failed with an error.
	InitEventFailed
	// InitEventTimedOut indicates initialization timed out.
	InitEventTimedOut
)

// InitializerEvent represents events emitted by the Initializer.
type InitializerEvent struct {
	Type  InitializerEventType
	Phase InitPhase // Current phase after transition
	Error error     // Non-nil for Failed events
}

// InitializerConfig holds configuration for creating an Initializer.
type InitializerConfig struct {
	WorkDir         string
	ClientType      string
	ClaudeModel     string
	AmpModel        string
	AmpMode         string
	ExpectedWorkers int
	Timeout         time.Duration
}

// InitializerResources holds the resources created during initialization.
// These are transferred to the Model when initialization completes.
type InitializerResources struct {
	AIClient          client.HeadlessClient
	Extensions        map[string]any
	Pool              *pool.WorkerPool
	MessageLog        *message.Issue
	MCPServer         *http.Server
	Coordinator       *coordinator.Coordinator
	WorkerServerCache *workerServerCache
}

// Initializer manages the orchestration initialization lifecycle as a state machine.
// It subscribes to coordinator, worker, and message events to drive phase transitions,
// and publishes high-level events for the TUI to consume.
type Initializer struct {
	// Configuration
	cfg InitializerConfig

	// State (protected by mu)
	phase            InitPhase
	failedAtPhase    InitPhase // The phase we were in when failure/timeout occurred
	workersSpawned   int
	confirmedWorkers map[string]bool
	startTime        time.Time
	err              error

	// Resources created during initialization
	aiClient           client.HeadlessClient
	aiClientExtensions map[string]any
	pool               *pool.WorkerPool
	messageLog         *message.Issue
	mcpServer          *http.Server
	mcpCoordServer     *mcp.CoordinatorServer
	coord              *coordinator.Coordinator
	workerServers      *workerServerCache

	// Event broker for publishing state changes to TUI
	broker *pubsub.Broker[InitializerEvent]

	// Context for managing goroutine lifecycle
	ctx    context.Context
	cancel context.CancelFunc

	// Synchronization
	mu      sync.RWMutex
	started bool
}

// NewInitializer creates a new Initializer with the given configuration.
func NewInitializer(cfg InitializerConfig) *Initializer {
	if cfg.ExpectedWorkers == 0 {
		cfg.ExpectedWorkers = 4
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 20 * time.Second
	}

	return &Initializer{
		cfg:              cfg,
		phase:            InitNotStarted,
		confirmedWorkers: make(map[string]bool),
		broker:           pubsub.NewBroker[InitializerEvent](),
	}
}

// Broker returns the event broker for subscribing to state changes.
func (i *Initializer) Broker() *pubsub.Broker[InitializerEvent] {
	return i.broker
}

// Phase returns the current initialization phase.
func (i *Initializer) Phase() InitPhase {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.phase
}

// Error returns the error if initialization failed.
func (i *Initializer) Error() error {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.err
}

// FailedAtPhase returns the phase at which initialization failed or timed out.
// Only meaningful when Phase() returns InitFailed or InitTimedOut.
func (i *Initializer) FailedAtPhase() InitPhase {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.failedAtPhase
}

// SpinnerData returns data needed for spinner rendering.
func (i *Initializer) SpinnerData() (phase InitPhase, workersSpawned, expectedWorkers int, confirmedWorkers map[string]bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	// Return a copy of confirmedWorkers to avoid races
	confirmed := make(map[string]bool, len(i.confirmedWorkers))
	for k, v := range i.confirmedWorkers {
		confirmed[k] = v
	}

	return i.phase, i.workersSpawned, i.cfg.ExpectedWorkers, confirmed
}

// Resources returns the initialized resources.
// Only valid after receiving InitEventReady.
func (i *Initializer) Resources() InitializerResources {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return InitializerResources{
		AIClient:          i.aiClient,
		Extensions:        i.aiClientExtensions,
		Pool:              i.pool,
		MessageLog:        i.messageLog,
		MCPServer:         i.mcpServer,
		Coordinator:       i.coord,
		WorkerServerCache: i.workerServers,
	}
}

// GetCoordinator returns the coordinator if it has been created, nil otherwise.
// This allows the TUI to set up event subscriptions as soon as the coordinator exists.
func (i *Initializer) GetCoordinator() *coordinator.Coordinator {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.coord
}

// GetMessageLog returns the message log if it has been created, nil otherwise.
func (i *Initializer) GetMessageLog() *message.Issue {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.messageLog
}

// Start begins the initialization process.
// This runs asynchronously; progress is communicated via the event broker.
func (i *Initializer) Start() error {
	i.mu.Lock()
	if i.started {
		i.mu.Unlock()
		return fmt.Errorf("initializer already started")
	}
	i.started = true
	i.ctx, i.cancel = context.WithCancel(context.Background())
	i.startTime = time.Now()
	i.mu.Unlock()

	go i.run()
	return nil
}

// Retry restarts initialization from the beginning.
func (i *Initializer) Retry() error {
	i.Cancel()

	i.mu.Lock()
	i.phase = InitNotStarted
	i.failedAtPhase = InitNotStarted
	i.workersSpawned = 0
	i.confirmedWorkers = make(map[string]bool)
	i.err = nil
	i.started = false
	i.aiClient = nil
	i.aiClientExtensions = nil
	i.pool = nil
	i.messageLog = nil
	i.mcpServer = nil
	i.mcpCoordServer = nil
	i.coord = nil
	i.workerServers = nil
	i.mu.Unlock()

	return i.Start()
}

// Cancel stops initialization and cleans up resources.
func (i *Initializer) Cancel() {
	i.mu.Lock()
	if i.cancel != nil {
		i.cancel()
	}
	i.mu.Unlock()

	i.cleanup()
}

// run is the main initialization goroutine.
func (i *Initializer) run() {
	// Start timeout timer
	timeoutTimer := time.NewTimer(i.cfg.Timeout)
	defer timeoutTimer.Stop()

	// Phase 1: Create workspace
	i.transitionTo(InitCreatingWorkspace)
	if err := i.createWorkspace(); err != nil {
		i.fail(err)
		return
	}

	// Phase 2: Spawn coordinator
	i.transitionTo(InitSpawningCoordinator)
	if err := i.spawnCoordinator(); err != nil {
		i.fail(err)
		return
	}

	// Phase 3+: Event-driven phases
	// Subscribe to all event sources and process them
	coordSub := i.coord.Broker().Subscribe(i.ctx)
	workerSub := i.coord.Workers().Subscribe(i.ctx)
	msgSub := i.messageLog.Broker().Subscribe(i.ctx)

	// Transition to awaiting first message
	i.transitionTo(InitAwaitingFirstMessage)

	for {
		select {
		case <-i.ctx.Done():
			return

		case <-timeoutTimer.C:
			i.timeout()
			return

		case event, ok := <-coordSub:
			if !ok {
				return
			}
			if i.handleCoordinatorEvent(event) {
				return // Ready or terminal state
			}

		case event, ok := <-workerSub:
			if !ok {
				return
			}
			if i.handleWorkerEvent(event) {
				return // Ready or terminal state
			}

		case event, ok := <-msgSub:
			if !ok {
				return
			}
			if i.handleMessageEvent(event) {
				return // Ready or terminal state
			}
		}
	}
}

// createWorkspace creates the AI client, worker pool, message log, and MCP server.
func (i *Initializer) createWorkspace() error {
	// 1. Create AI client
	clientType := client.ClientType(i.cfg.ClientType)
	if clientType == "" {
		clientType = client.ClientClaude
	}

	aiClient, err := client.NewClient(clientType)
	if err != nil {
		return fmt.Errorf("failed to create AI client: %w", err)
	}

	// Build extensions map for provider-specific configuration
	extensions := make(map[string]any)
	switch clientType {
	case client.ClientClaude:
		if i.cfg.ClaudeModel != "" {
			extensions[client.ExtClaudeModel] = i.cfg.ClaudeModel
		}
	case client.ClientAmp:
		if i.cfg.AmpModel != "" {
			extensions[client.ExtAmpModel] = i.cfg.AmpModel
		}
		if i.cfg.AmpMode != "" {
			extensions[amp.ExtAmpMode] = i.cfg.AmpMode
		}
	}

	i.mu.Lock()
	i.aiClient = aiClient
	i.aiClientExtensions = extensions
	i.mu.Unlock()

	// 2. Create worker pool
	workerPool := pool.NewWorkerPool(pool.Config{
		Client: aiClient,
	})
	i.mu.Lock()
	i.pool = workerPool
	i.mu.Unlock()

	// 3. Create message log
	msgLog := message.New()
	i.mu.Lock()
	i.messageLog = msgLog
	i.mu.Unlock()

	// 4. Start MCP server
	mcpCoordServer := mcp.NewCoordinatorServer(aiClient, workerPool, msgLog, i.cfg.WorkDir, extensions)
	workerServers := newWorkerServerCache(msgLog)

	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpCoordServer.ServeHTTP())
	mux.HandleFunc("/worker/", workerServers.ServeHTTP)

	httpServer := &http.Server{
		Addr:              ":8765",
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	i.mu.Lock()
	i.mcpServer = httpServer
	i.mcpCoordServer = mcpCoordServer
	i.workerServers = workerServers
	i.mu.Unlock()

	// Start HTTP server in background
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("initializer", "MCP server error", "error", err)
		}
	}()

	return nil
}

// spawnCoordinator creates and starts the coordinator.
func (i *Initializer) spawnCoordinator() error {
	i.mu.RLock()
	aiClient := i.aiClient
	workerPool := i.pool
	msgLog := i.messageLog
	i.mu.RUnlock()

	if aiClient == nil || workerPool == nil || msgLog == nil {
		return fmt.Errorf("prerequisites not initialized")
	}

	coordCfg := coordinator.Config{
		WorkDir:      i.cfg.WorkDir,
		Client:       aiClient,
		Pool:         workerPool,
		MessageIssue: msgLog,
	}

	coord, err := coordinator.New(coordCfg)
	if err != nil {
		return err
	}

	i.mu.Lock()
	i.coord = coord
	i.mu.Unlock()

	if err := coord.Start(); err != nil {
		return err
	}

	log.Debug("initializer", "Coordinator started")
	return nil
}

// spawnWorkers spawns all workers programmatically.
// This runs in a goroutine to not block the event loop.
func (i *Initializer) spawnWorkers() {
	i.mu.RLock()
	mcpCoordServer := i.mcpCoordServer
	expected := i.cfg.ExpectedWorkers
	i.mu.RUnlock()

	if mcpCoordServer == nil {
		log.Error("initializer", "Cannot spawn workers: MCP server not initialized")
		return
	}

	for j := 0; j < expected; j++ {
		workerID, err := mcpCoordServer.SpawnIdleWorker()
		if err != nil {
			log.Error("initializer", "Failed to spawn worker", "index", j, "error", err)
			// Continue trying to spawn remaining workers
			continue
		}
		log.Debug("initializer", "Spawned worker programmatically", "workerID", workerID, "index", j)
	}
}

// handleCoordinatorEvent processes coordinator events.
// Returns true if initialization reached a terminal state.
func (i *Initializer) handleCoordinatorEvent(event pubsub.Event[events.CoordinatorEvent]) bool {
	payload := event.Payload

	i.mu.Lock()
	phase := i.phase
	i.mu.Unlock()

	// Detect first coordinator message for phase transition
	if phase == InitAwaitingFirstMessage && payload.Type == events.CoordinatorChat {
		log.Debug("initializer", "First coordinator message received, spawning workers")
		i.transitionTo(InitSpawningWorkers)

		// Spawn workers programmatically (don't rely on coordinator LLM)
		go i.spawnWorkers()
	}

	return false
}

// handleWorkerEvent processes worker events.
// Returns true if initialization reached a terminal state.
func (i *Initializer) handleWorkerEvent(event pubsub.Event[events.WorkerEvent]) bool {
	payload := event.Payload

	if payload.Type != events.WorkerSpawned {
		return false
	}

	i.mu.Lock()
	phase := i.phase
	i.workersSpawned++
	spawned := i.workersSpawned
	expected := i.cfg.ExpectedWorkers
	i.mu.Unlock()

	log.Debug("initializer", "Worker spawned",
		"workerID", payload.WorkerID,
		"spawned", spawned,
		"expected", expected)

	// Check if all workers spawned
	if phase == InitSpawningWorkers && spawned >= expected {
		i.transitionTo(InitWorkersReady)
		// Check if we already have enough confirmed workers (race condition handling)
		return i.checkWorkersConfirmed()
	}

	return false
}

// handleMessageEvent processes message events.
// Returns true if initialization reached a terminal state.
func (i *Initializer) handleMessageEvent(event pubsub.Event[message.Event]) bool {
	payload := event.Payload

	if payload.Type != message.EventPosted {
		return false
	}

	entry := payload.Entry

	// Only track worker messages
	if !strings.HasPrefix(strings.ToLower(entry.From), "worker") {
		return false
	}

	i.mu.Lock()
	phase := i.phase

	// Track during both SpawningWorkers and WorkersReady phases (messages may arrive early)
	if phase != InitSpawningWorkers && phase != InitWorkersReady {
		i.mu.Unlock()
		return false
	}

	if !i.confirmedWorkers[entry.From] {
		i.confirmedWorkers[entry.From] = true
		log.Debug("initializer", "Worker confirmed",
			"workerID", entry.From,
			"confirmed", len(i.confirmedWorkers),
			"expected", i.cfg.ExpectedWorkers)
	}
	i.mu.Unlock()

	// Only transition to ready if we're in WorkersReady phase
	return i.checkWorkersConfirmed()
}

// checkWorkersConfirmed checks if all workers are confirmed and transitions to Ready if so.
// Returns true if transitioned to Ready.
func (i *Initializer) checkWorkersConfirmed() bool {
	i.mu.Lock()
	phase := i.phase
	confirmed := len(i.confirmedWorkers)
	expected := i.cfg.ExpectedWorkers
	i.mu.Unlock()

	if phase == InitWorkersReady && confirmed >= expected {
		log.Debug("initializer", "All workers confirmed, transitioning to ready")
		i.transitionTo(InitReady)
		i.publishEvent(InitializerEvent{
			Type:  InitEventReady,
			Phase: InitReady,
		})
		return true
	}

	return false
}

// transitionTo updates the phase and publishes a phase change event.
func (i *Initializer) transitionTo(phase InitPhase) {
	i.mu.Lock()
	oldPhase := i.phase
	i.phase = phase
	i.mu.Unlock()

	log.Debug("initializer", "Phase transition", "from", oldPhase, "to", phase)

	i.publishEvent(InitializerEvent{
		Type:  InitEventPhaseChanged,
		Phase: phase,
	})
}

// fail transitions to failed state and publishes a failed event.
func (i *Initializer) fail(err error) {
	i.mu.Lock()
	i.failedAtPhase = i.phase
	i.phase = InitFailed
	i.err = err
	i.mu.Unlock()

	log.Error("initializer", "Initialization failed", "phase", i.failedAtPhase, "error", err)

	i.publishEvent(InitializerEvent{
		Type:  InitEventFailed,
		Phase: InitFailed,
		Error: err,
	})
}

// timeout transitions to timed out state and publishes a timeout event.
func (i *Initializer) timeout() {
	i.mu.Lock()
	i.failedAtPhase = i.phase
	i.phase = InitTimedOut
	i.mu.Unlock()

	log.Error("initializer", "Initialization timed out", "phase", i.failedAtPhase)

	i.publishEvent(InitializerEvent{
		Type:  InitEventTimedOut,
		Phase: InitTimedOut,
	})
}

// publishEvent publishes an event to subscribers.
func (i *Initializer) publishEvent(event InitializerEvent) {
	i.broker.Publish(pubsub.UpdatedEvent, event)
}

// cleanup releases all resources.
func (i *Initializer) cleanup() {
	i.mu.Lock()
	mcpServer := i.mcpServer
	coord := i.coord
	i.mu.Unlock()

	if mcpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = mcpServer.Shutdown(ctx)
		cancel()
	}

	if coord != nil {
		_ = coord.Cancel()
	}
}
