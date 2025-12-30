package orchestration

import (
	"context"
	"fmt"
	"maps"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/amp"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/mcp"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/session"
	"github.com/zjrosen/perles/internal/orchestration/v2/adapter"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/handler"
	"github.com/zjrosen/perles/internal/orchestration/v2/integration"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
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
	MessageRepo    repository.MessageRepository // Message repository for inter-agent messaging
	MCPServer      *http.Server
	MCPPort        int                          // Dynamic port the MCP server is listening on
	Session        *session.Session             // Session tracking for this orchestration run
	MCPCoordServer *mcp.CoordinatorServer       // MCP coordinator server for direct worker messaging
	ProcessRepo    repository.ProcessRepository // Process repository for unified state management
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
	messageRepo    *repository.MemoryMessageRepository // Message repository for inter-agent messaging
	mcpPort        int                                 // Assigned port
	mcpServer      *http.Server
	mcpCoordServer *mcp.CoordinatorServer
	workerServers  *workerServerCache // Used by HTTP handler; not transferred to Model
	session        *session.Session   // Session tracking

	// V2 orchestration infrastructure
	cmdProcessor    *processor.CommandProcessor
	dedupMiddleware *processor.DeduplicationMiddleware
	v2EventBus      *pubsub.Broker[any] // Event bus for v2 command events (TUI subscription)

	// Unified process infrastructure (coordinator + workers as Process entities)
	processRepo     repository.ProcessRepository
	processRegistry *process.ProcessRegistry
	cmdSubmitter    process.CommandSubmitter

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
	maps.Copy(confirmed, i.confirmedWorkers)

	return i.phase, i.workersSpawned, i.cfg.ExpectedWorkers, confirmed
}

// Resources returns the initialized resources.
// Only valid after receiving InitEventReady.
func (i *Initializer) Resources() InitializerResources {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return InitializerResources{
		MessageRepo:    i.messageRepo,
		MCPServer:      i.mcpServer,
		MCPPort:        i.mcpPort,
		Session:        i.session,
		MCPCoordServer: i.mcpCoordServer,
		ProcessRepo:    i.processRepo,
	}
}

// GetMessageRepo returns the message repository if it has been created, nil otherwise.
// The returned repository provides inter-agent messaging with pub/sub broker support.
func (i *Initializer) GetMessageRepo() repository.MessageRepository {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.messageRepo
}

// GetV2EventBus returns the v2 event bus for TUI subscription, nil if not yet created.
// The event bus is created during createWorkspace() and can be subscribed to for
// receiving v2 orchestration events (WorkerEvent, CommandErrorEvent, etc.).
func (i *Initializer) GetV2EventBus() *pubsub.Broker[any] {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.v2EventBus
}

// GetCmdSubmitter returns the command submitter for v2 command submission.
// Returns nil if not yet created. Used by TUI to submit v2 commands directly.
func (i *Initializer) GetCmdSubmitter() process.CommandSubmitter {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.cmdSubmitter
}

// GetProcessRepository returns the process repository for unified state management.
// Returns nil if not yet created.
func (i *Initializer) GetProcessRepository() repository.ProcessRepository {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.processRepo
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
	i.messageRepo = nil
	i.mcpPort = 0
	i.mcpServer = nil
	i.mcpCoordServer = nil
	i.workerServers = nil
	i.session = nil
	i.cmdProcessor = nil
	i.dedupMiddleware = nil
	i.v2EventBus = nil
	i.processRepo = nil
	i.processRegistry = nil
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
	// Subscribe to v2EventBus for all process events (coordinator and workers)
	// Coordinator events flow through v2EventBus as ProcessEvent with Role=RoleCoordinator
	v2Sub := i.v2EventBus.Subscribe(i.ctx)
	msgSub := i.messageRepo.Broker().Subscribe(i.ctx)

	// Transition to awaiting first message
	i.transitionTo(InitAwaitingFirstMessage)

	for {
		select {
		case <-i.ctx.Done():
			return

		case <-timeoutTimer.C:
			i.timeout()
			return

		case event, ok := <-v2Sub:
			if !ok {
				return
			}
			if i.handleV2Event(event) {
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

// createWorkspace creates the AI client, message log, and MCP server.
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

	// 2. Create message repository
	msgRepo := repository.NewMemoryMessageRepository()
	i.mu.Lock()
	i.messageRepo = msgRepo
	i.mu.Unlock()

	// 4. Create session for tracking this orchestration run
	sessionID := uuid.New().String()
	sessionDir := filepath.Join(i.cfg.WorkDir, ".perles", "sessions", sessionID)
	sess, err := session.New(sessionID, sessionDir)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	i.mu.Lock()
	i.session = sess
	i.mu.Unlock()

	log.Debug(log.CatOrch, "Session created", "subsystem", "init", "sessionID", sessionID, "dir", sessionDir)

	// 5. Create V2 orchestration infrastructure
	// Create repositories
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(repository.DefaultQueueMaxSize)
	processRepo := repository.NewMemoryProcessRepository() // Unified process repository

	// Create event bus for v2 command events (propagates to TUI subscribers)
	// Store as field so TUI can subscribe via GetV2EventBus()
	i.v2EventBus = pubsub.NewBroker[any]()

	// Attach session to brokers:
	// 1. Message broker - attached here (exists now)
	// 2. v2EventBus for all process events (coordinator and workers) - attached here
	// 3. MCP broker - attached below after mcpCoordServer is created
	// Note: Coordinator events flow through v2EventBus as ProcessEvent with Role=RoleCoordinator
	sess.AttachToBrokers(i.ctx, nil, msgRepo.Broker(), nil)
	sess.AttachV2EventBus(i.ctx, i.v2EventBus)

	// Create middleware for command processing
	loggingMiddleware := processor.NewLoggingMiddleware(processor.LoggingMiddlewareConfig{})
	//dedupMiddleware := processor.NewDeduplicationMiddleware(processor.DeduplicationMiddlewareConfig{
	//	TTL: 5 * time.Second,
	//})
	timeoutMiddleware := processor.NewTimeoutMiddleware(processor.TimeoutMiddlewareConfig{
		WarningThreshold: 500 * time.Millisecond,
	})

	// Create command processor with event bus for TUI event propagation
	cmdProcessor := processor.NewCommandProcessor(
		processor.WithQueueCapacity(1000),
		processor.WithTaskRepository(taskRepo),
		processor.WithQueueRepository(queueRepo),
		processor.WithEventBus(i.v2EventBus),
		processor.WithMiddleware(
			loggingMiddleware,
			// dedupMiddleware.Middleware(),
			timeoutMiddleware,
		),
	)

	// 6. Start MCP server with dynamic port
	// Create listener on localhost:0 to get a random available port
	// Using localhost (127.0.0.1) to avoid binding to all interfaces
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to create MCP listener: %w", err)
	}

	// Extract the assigned port
	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return fmt.Errorf("failed to get TCP address from listener")
	}
	port := tcpAddr.Port
	log.Debug(log.CatOrch, "MCP server listening on dynamic port", "subsystem", "init", "port", port)

	// Create unified ProcessRegistry for coordinator and workers
	// This single registry replaces the old worker.ProcessRegistry
	processRegistry := process.NewProcessRegistry()

	// Create MessageDeliverer for delivering messages to processes via session resume.
	// This uses ProcessRegistrySessionProvider and ProcessRegistry (as ProcessResumer)
	// for v2 command-based processing.
	sessionProvider := handler.NewProcessRegistrySessionProvider(processRegistry, aiClient, i.cfg.WorkDir, port)
	messageDeliverer := integration.NewProcessSessionDeliverer(sessionProvider, aiClient, processRegistry)

	// Create BDTaskExecutor for syncing v2 state changes to BD tracker.
	// This bridges v2 handlers to the BD CLI for task status updates and comments.
	beadsExec := beads.NewRealExecutor(i.cfg.WorkDir)

	// Register all handlers with the command processor

	// Task Assignment handlers (3) - wired with BDExecutor for BD task status sync
	// and QueueRepository for sending prompts to workers
	cmdProcessor.RegisterHandler(command.CmdAssignTask,
		handler.NewAssignTaskHandler(processRepo, taskRepo,
			handler.WithBDExecutor(beadsExec),
			handler.WithQueueRepository(queueRepo)))
	cmdProcessor.RegisterHandler(command.CmdAssignReview,
		handler.NewAssignReviewHandler(processRepo, taskRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdApproveCommit,
		handler.NewApproveCommitHandler(processRepo, taskRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdAssignReviewFeedback,
		handler.NewAssignReviewFeedbackHandler(processRepo, taskRepo, queueRepo))

	// State Transition handlers (3) - wired with BDExecutor for BD comment sync
	cmdProcessor.RegisterHandler(command.CmdReportComplete,
		handler.NewReportCompleteHandler(processRepo, taskRepo, queueRepo,
			handler.WithReportCompleteBDExecutor(beadsExec)))
	cmdProcessor.RegisterHandler(command.CmdReportVerdict,
		handler.NewReportVerdictHandler(processRepo, taskRepo, queueRepo,
			handler.WithReportVerdictBDExecutor(beadsExec)))
	cmdProcessor.RegisterHandler(command.CmdTransitionPhase,
		handler.NewTransitionPhaseHandler(processRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdProcessTurnComplete,
		handler.NewProcessTurnCompleteHandler(processRepo, queueRepo))

	// BD Task Status handlers (2) - for updating task status in beads database
	cmdProcessor.RegisterHandler(command.CmdMarkTaskComplete,
		handler.NewMarkTaskCompleteHandler(beadsExec))
	cmdProcessor.RegisterHandler(command.CmdMarkTaskFailed,
		handler.NewMarkTaskFailedHandler(beadsExec))

	// Create UnifiedProcessSpawner for spawning AI processes
	// This requires a CommandSubmitter adapter to bridge to the processor
	cmdSubmitter := handler.NewProcessorSubmitterAdapter(cmdProcessor)
	processSpawner := handler.NewUnifiedProcessSpawner(handler.UnifiedSpawnerConfig{
		Client:     aiClient,
		WorkDir:    i.cfg.WorkDir,
		Port:       port,
		Extensions: extensions,
		Submitter:  cmdSubmitter,
		EventBus:   i.v2EventBus,
	})

	// Unified Process handlers (7) - for coordinator and workers as Process entities
	cmdProcessor.RegisterHandler(command.CmdSpawnProcess,
		handler.NewSpawnProcessHandler(processRepo, processRegistry,
			handler.WithSpawnMaxWorkers(i.cfg.ExpectedWorkers),
			handler.WithUnifiedSpawner(processSpawner)))
	cmdProcessor.RegisterHandler(command.CmdSendToProcess,
		handler.NewSendToProcessHandler(processRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdDeliverProcessQueued,
		handler.NewDeliverProcessQueuedHandler(processRepo, queueRepo, processRegistry,
			handler.WithProcessDeliverer(messageDeliverer)))
	cmdProcessor.RegisterHandler(command.CmdProcessTurnComplete,
		handler.NewProcessTurnCompleteHandler(processRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdRetireProcess,
		handler.NewRetireProcessHandler(processRepo, processRegistry))
	cmdProcessor.RegisterHandler(command.CmdStopProcess,
		handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, processRegistry))
	cmdProcessor.RegisterHandler(command.CmdReplaceProcess,
		handler.NewReplaceProcessHandler(processRepo, processRegistry,
			handler.WithReplaceSpawner(processSpawner)))

	// Create V2Adapter with repositories for read-only operations and message repository for COORDINATOR routing
	v2Adapter := adapter.NewV2Adapter(cmdProcessor,
		adapter.WithProcessRepository(processRepo),
		adapter.WithTaskRepository(taskRepo),
		adapter.WithQueueRepository(queueRepo),
		adapter.WithMessageRepository(msgRepo),
	)

	// Start processor loop in background
	go cmdProcessor.Run(i.ctx)

	// Wait for processor to be ready before continuing
	if err := cmdProcessor.WaitForReady(i.ctx); err != nil {
		return fmt.Errorf("waiting for command processor: %w", err)
	}

	log.Debug(log.CatOrch, "V2 orchestration infrastructure initialized", "subsystem", "init",
		"handlers", 22, "queueCapacity", 1000)

	// Store v2 infrastructure references
	i.mu.Lock()
	i.cmdProcessor = cmdProcessor
	// i.dedupMiddleware = dedupMiddleware
	i.processRepo = processRepo
	i.processRegistry = processRegistry
	i.cmdSubmitter = cmdSubmitter
	i.mu.Unlock()

	// Create coordinator server with the dynamic port and v2 adapter
	mcpCoordServer := mcp.NewCoordinatorServerWithV2Adapter(
		aiClient, msgRepo, i.cfg.WorkDir, port, extensions,
		beads.NewRealExecutor(i.cfg.WorkDir), v2Adapter)
	// Pass the session as the reflection writer so workers can save reflections
	// Pass the v2Adapter so all worker servers route through v2
	workerServers := newWorkerServerCache(msgRepo, sess, v2Adapter)

	// Attach session to MCP broker now that mcpCoordServer exists
	sess.AttachMCPBroker(i.ctx, mcpCoordServer.Broker())

	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpCoordServer.ServeHTTP())
	mux.HandleFunc("/worker/", workerServers.ServeHTTP)

	httpServer := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	i.mu.Lock()
	i.mcpPort = port
	i.mcpServer = httpServer
	i.mcpCoordServer = mcpCoordServer
	i.workerServers = workerServers
	i.mu.Unlock()

	// Start HTTP server in background using the listener
	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Error(log.CatOrch, "MCP server error", "subsystem", "init", "error", err)
		}
	}()

	return nil
}

// spawnCoordinator creates and starts the coordinator using the v2 command processor.
// This submits a SpawnProcessCommand which handles all the AI spawning, registry registration,
// and ProcessRepository updates through the unified command pattern.
func (i *Initializer) spawnCoordinator() error {
	i.mu.RLock()
	cmdProcessor := i.cmdProcessor
	i.mu.RUnlock()

	if cmdProcessor == nil {
		return fmt.Errorf("command processor not initialized")
	}

	// Create and submit SpawnProcessCommand for coordinator
	spawnCmd := command.NewSpawnProcessCommand(command.SourceUser, repository.RoleCoordinator)

	// Use SubmitAndWait to ensure coordinator is fully spawned before continuing
	result, err := cmdProcessor.SubmitAndWait(i.ctx, spawnCmd)
	if err != nil {
		return fmt.Errorf("spawning coordinator via command: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("spawn coordinator command failed: %w", result.Error)
	}

	log.Debug(log.CatOrch, "Coordinator spawned via v2 command processor", "subsystem", "init")

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
		log.Error(log.CatOrch, "Cannot spawn workers: MCP server not initialized", "subsystem", "init")
		return
	}

	for j := range expected {
		workerID, err := mcpCoordServer.SpawnIdleWorker()
		if err != nil {
			log.Error(log.CatOrch, "Failed to spawn worker", "subsystem", "init", "index", j, "error", err)
			// Continue trying to spawn remaining workers
			continue
		}
		log.Debug(log.CatOrch, "Spawned worker programmatically", "subsystem", "init", "workerID", workerID, "index", j)
	}
}

// handleV2Event processes v2 orchestration events from the unified v2EventBus.
// Returns true if initialization reached a terminal state.
func (i *Initializer) handleV2Event(event pubsub.Event[any]) bool {
	// Type-assert to ProcessEvent (unified event type)
	if payload, ok := event.Payload.(events.ProcessEvent); ok {
		return i.handleProcessEventPayload(payload)
	}
	return false
}

// handleProcessEventPayload processes unified process events from the v2EventBus.
// This handles events from both coordinator and worker processes via the unified architecture.
// Returns true if initialization reached a terminal state.
//
// Note: Worker confirmation is handled via MessageWorkerReady messages in handleMessageEvent,
// not via ProcessReady events. This is because workers call signal_ready which posts a message.
func (i *Initializer) handleProcessEventPayload(payload events.ProcessEvent) bool {
	// Handle coordinator events
	if payload.Role == events.RoleCoordinator {
		return i.handleCoordinatorProcessEvent(payload)
	}

	// Handle worker events
	switch payload.Type {
	case events.ProcessSpawned:
		return i.handleProcessSpawned(payload)
	}

	return false
}

// handleCoordinatorProcessEvent processes coordinator events from the v2EventBus.
// This replaces the legacy handleCoordinatorEvent function that used CoordinatorEvent type.
// Returns true if initialization reached a terminal state.
func (i *Initializer) handleCoordinatorProcessEvent(payload events.ProcessEvent) bool {
	i.mu.Lock()
	phase := i.phase
	i.mu.Unlock()

	// Detect first coordinator message for phase transition
	// ProcessOutput is equivalent to the legacy CoordinatorChat event
	if phase == InitAwaitingFirstMessage && payload.Type == events.ProcessOutput {
		log.Debug(log.CatOrch, "First coordinator message received, spawning workers", "subsystem", "init")
		i.transitionTo(InitSpawningWorkers)

		// Spawn workers programmatically (don't rely on coordinator LLM)
		go i.spawnWorkers()
	}

	return false
}

// handleProcessSpawned processes unified ProcessSpawned events for workers.
// Note: Worker confirmation happens via MessageWorkerReady in handleMessageEvent,
// not here. This only tracks spawn count.
func (i *Initializer) handleProcessSpawned(payload events.ProcessEvent) bool {
	i.mu.Lock()
	phase := i.phase
	i.workersSpawned++
	spawned := i.workersSpawned
	expected := i.cfg.ExpectedWorkers
	i.mu.Unlock()

	log.Debug(log.CatOrch, "Worker process spawned (unified)",
		"subsystem", "init",
		"processID", payload.ProcessID,
		"spawned", spawned,
		"expected", expected,
		"status", payload.Status)

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

	// Only track worker ready messages
	if entry.Type != message.MessageWorkerReady {
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
		log.Debug(log.CatOrch, "Worker confirmed",
			"subsystem", "init",
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
		log.Debug(log.CatOrch, "All workers confirmed, transitioning to ready", "subsystem", "init")
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

	log.Debug(log.CatOrch, "Phase transition", "subsystem", "init", "from", oldPhase, "to", phase)

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

	log.Error(log.CatOrch, "Initialization failed", "subsystem", "init", "phase", i.failedAtPhase, "error", err)

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

	log.Error(log.CatOrch, "Initialization timed out", "subsystem", "init", "phase", i.failedAtPhase)

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
	cmdProcessor := i.cmdProcessor
	dedupMiddleware := i.dedupMiddleware
	processRegistry := i.processRegistry
	i.mu.Unlock()

	// Drain the v2 command processor first to complete in-flight commands
	// This must happen before shutting down MCP server
	if cmdProcessor != nil {
		cmdProcessor.Drain()
	}

	// Stop deduplication middleware background cleanup goroutine
	if dedupMiddleware != nil {
		dedupMiddleware.Stop()
	}

	if mcpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = mcpServer.Shutdown(ctx)
		cancel()
	}

	// Stop coordinator process via registry
	if processRegistry != nil {
		if coordProcess := processRegistry.GetCoordinator(); coordProcess != nil {
			coordProcess.Stop()
		}
	}
}
