// Package chatpanel provides a lightweight AI chat panel component
// that can be toggled in Kanban and Search modes.
package chatpanel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	v2 "github.com/zjrosen/perles/internal/orchestration/v2"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/pubsub"
	"github.com/zjrosen/perles/internal/ui/shared/chatrender"
	"github.com/zjrosen/perles/internal/ui/shared/vimtextarea"
)

// ErrNoInfrastructure is returned when attempting to use infrastructure before it's set.
var ErrNoInfrastructure = errors.New("infrastructure not set")

// viewportKey is the map key for the single viewport.
// Using a map instead of a direct field allows changes to persist in View methods
// since maps are reference types.
const viewportKey = "main"

// Tab indices for the chat panel.
const (
	TabChat     = 0
	TabSessions = 1
	tabCount    = 2
)

// ChatPanelProcessID is the well-known ID for the chat panel's AI process.
const ChatPanelProcessID = "chat-panel"

// SessionData encapsulates all state for a single AI chat session.
// Multiple sessions can exist concurrently, with one being "active" at a time.
type SessionData struct {
	// ID is the unique session identifier (e.g., "session-1", "session-2").
	ID string
	// ProcessID is the mapped AI process ID for routing events.
	ProcessID string
	// Messages holds the chat history for this session.
	Messages []chatrender.Message
	// Viewport manages scroll state for this session.
	Viewport viewport.Model
	// Status indicates the process status (Ready, Working, etc.).
	Status events.ProcessStatus
	// Metrics tracks token usage for this session.
	Metrics *metrics.TokenMetrics
	// ContentDirty indicates the viewport needs re-rendering.
	ContentDirty bool
	// HasNewContent indicates new content arrived while scrolled up.
	HasNewContent bool
	// CreatedAt is when this session was created (for display/sorting).
	CreatedAt time.Time
	// LastActivity is when the last message was sent/received.
	LastActivity time.Time
}

// Model holds the chat panel state.
type Model struct {
	visible   bool
	focused   bool
	activeTab int // Current tab: TabChat or TabSessions
	width     int
	height    int
	messages  []chatrender.Message
	config    Config

	// UI components
	input     vimtextarea.Model
	viewports map[string]viewport.Model // Use map so changes persist in View (maps are reference types)

	// Assistant state (for UI feedback like border color)
	assistantWorking bool                  // True when AI is processing, false when ready
	queueCount       int                   // Number of messages queued for assistant (for UI display)
	metrics          *metrics.TokenMetrics // Token usage metrics for UI display

	// Infrastructure for AI communication (uses v2.SimpleInfrastructure)
	infra      *v2.SimpleInfrastructure
	v2Listener *pubsub.ContinuousListener[any]
	ctx        context.Context
	cancel     context.CancelFunc

	// Session persistence fields (legacy - being replaced by multi-session support)
	sessionRef          string    // Session reference from the AI process (for resuming sessions)
	lastInteractionTime time.Time // Time of last message send/receive (for session age calculation)

	// Multi-session support
	sessions          map[string]*SessionData // All sessions by ID
	sessionOrder      []string                // Session IDs in display order
	activeSessionID   string                  // Currently active session ID
	processToSession  map[string]string       // Reverse lookup: ProcessID -> SessionID for O(1) event routing
	sessionListCursor int                     // Cursor position in Sessions tab list

	// Confirmation state for session retirement
	pendingRetireSessionID string // Session ID pending retirement confirmation (empty = no pending)

	// Clock is the time source for testing. If nil, uses time.Now().
	Clock func() time.Time
}

// DefaultSessionID is the ID of the initial session created on startup.
const DefaultSessionID = "session-1"

// New creates a new chat panel model with the given configuration.
func New(cfg Config) Model {
	// Create vimtextarea input with vim mode enabled by default
	input := vimtextarea.New(vimtextarea.Config{
		VimEnabled:  true,
		DefaultMode: vimtextarea.ModeInsert,
		CharLimit:   0, // No limit
		MaxHeight:   4, // Allow up to 4 lines for input (matches input pane height)
	})

	// Create initial session
	now := time.Now()
	initialSession := &SessionData{
		ID:           DefaultSessionID,
		ProcessID:    ChatPanelProcessID,
		Messages:     make([]chatrender.Message, 0),
		Viewport:     viewport.New(0, 0),
		Status:       events.ProcessStatusPending,
		ContentDirty: true,
		CreatedAt:    now,
		LastActivity: now,
	}

	return Model{
		visible:   false,
		focused:   false,
		config:    cfg,
		messages:  make([]chatrender.Message, 0),
		input:     input,
		viewports: map[string]viewport.Model{viewportKey: viewport.New(0, 0)},
		// Initialize multi-session maps
		sessions:         map[string]*SessionData{DefaultSessionID: initialSession},
		sessionOrder:     []string{DefaultSessionID},
		activeSessionID:  DefaultSessionID,
		processToSession: map[string]string{ChatPanelProcessID: DefaultSessionID},
	}
}

// Visible returns whether the chat panel is currently showing.
func (m Model) Visible() bool {
	return m.visible
}

// Toggle flips the visibility state of the chat panel.
func (m Model) Toggle() Model {
	m.visible = !m.visible
	return m
}

// Focused returns whether the chat panel has focus.
func (m Model) Focused() bool {
	return m.focused
}

// Focus gives the chat panel focus and focuses the input.
func (m Model) Focus() Model {
	m.focused = true
	m.input.Focus()
	return m
}

// Blur removes focus from the chat panel and blurs the input.
func (m Model) Blur() Model {
	m.focused = false
	m.input.Blur()
	return m
}

// NextTab switches to the next tab.
func (m Model) NextTab() Model {
	m.activeTab = (m.activeTab + 1) % tabCount
	return m
}

// PrevTab switches to the previous tab.
func (m Model) PrevTab() Model {
	m.activeTab = (m.activeTab - 1 + tabCount) % tabCount
	return m
}

// ActiveTab returns the current active tab index.
func (m Model) ActiveTab() int {
	return m.activeTab
}

// SetSize updates the width and height of the chat panel.
// Note: Viewport dimensions are set at render time by ScrollablePane (single source of truth).
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height

	// Calculate internal dimensions (accounting for borders and padding)
	innerWidth := width - 2      // Remove left/right border
	inputWidth := innerWidth - 2 // Space for 1 char padding on each side

	m.input.SetSize(inputWidth, 4)

	// Mark active session's content as dirty for re-render
	if session := m.ActiveSession(); session != nil {
		session.ContentDirty = true
	}

	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model and handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	// Handle pubsub events from infrastructure (always, regardless of visibility)
	case pubsub.Event[any]:
		return m.handlePubSubEvent(msg)

	case tea.KeyMsg:
		// Only process key input if visible and focused
		if !m.visible || !m.focused {
			return m, nil
		}

		// Handle tab switching with Ctrl+] (cycles through tabs)
		// Note: Ctrl+[ is not used because it's indistinguishable from Escape in terminals
		if msg.Type == tea.KeyCtrlCloseBracket {
			m = m.NextTab()
			return m, nil
		}

		// Handle tab switching with Ctrl+H (previous tab) and Ctrl+L (next tab)
		// Vim-style directional navigation
		if msg.Type == tea.KeyCtrlH {
			m = m.PrevTab()
			return m, nil
		}
		if msg.Type == tea.KeyCtrlL {
			m = m.NextTab()
			return m, nil
		}

		// Handle Ctrl+C for quit - always show quit modal regardless of vim mode
		if msg.Type == tea.KeyCtrlC {
			return m, func() tea.Msg { return RequestQuitMsg{} }
		}

		// Handle Ctrl+N/P for session cycling (works in Chat and Sessions tabs)
		if msg.Type == tea.KeyCtrlN {
			m = m.NextSession()
			return m, nil
		}
		if msg.Type == tea.KeyCtrlP {
			m = m.PrevSession()
			return m, nil
		}

		// Handle Sessions tab navigation when Sessions tab is active
		// Note: Cursor index 0 is "Create new session", sessions start at index 1
		if m.activeTab == TabSessions {
			// Clamp cursor to valid range (handles case where sessions were removed)
			m = m.clampSessionListCursor()

			switch msg.String() {
			case "j", "down":
				// Move cursor down in session list - also clears pending retire
				m.pendingRetireSessionID = ""
				// Max cursor is len(sessions) since index 0 is "Create new session"
				if m.sessionListCursor < len(m.sessionOrder) {
					m.sessionListCursor++
				}
				return m, nil
			case "k", "up":
				// Move cursor up in session list - also clears pending retire
				m.pendingRetireSessionID = ""
				if m.sessionListCursor > 0 {
					m.sessionListCursor--
				}
				return m, nil
			case "enter":
				m.pendingRetireSessionID = ""
				if m.sessionListCursor == 0 {
					// "Create new session" option selected
					return m, func() tea.Msg { return NewSessionRequestMsg{} }
				}
				// Session selected (cursor index 1..len maps to sessionOrder[0..len-1])
				sessionIdx := m.sessionListCursor - 1
				if sessionIdx < len(m.sessionOrder) {
					selectedSessionID := m.sessionOrder[sessionIdx]
					m = m.switchToSession(selectedSessionID)
					m.activeTab = TabChat
				}
				return m, nil
			case "d":
				// Retire/delete the selected session (two-step confirmation)
				// Only works on session items (not "Create new session" at index 0)
				if m.sessionListCursor == 0 {
					return m, nil // Can't delete "Create new session"
				}
				sessionIdx := m.sessionListCursor - 1
				if sessionIdx < len(m.sessionOrder) {
					selectedSessionID := m.sessionOrder[sessionIdx]
					if m.pendingRetireSessionID == selectedSessionID {
						// Second 'd' press - confirm retirement
						var retireCmd tea.Cmd
						m, retireCmd = m.RetireSession(selectedSessionID)
						m.pendingRetireSessionID = ""
						return m, retireCmd
					} else {
						// First 'd' press - mark as pending confirmation
						m.pendingRetireSessionID = selectedSessionID
					}
				}
				return m, nil
			case "esc":
				// Cancel pending retirement
				m.pendingRetireSessionID = ""
				return m, nil
			}
			// Block all other keys when Sessions tab is active (don't forward to input)
			// Also clear pending retire on any other key
			m.pendingRetireSessionID = ""
			return m, nil
		}

		// Forward key events to input (only when Chat tab is active)
		var inputCmd tea.Cmd
		m.input, inputCmd = m.input.Update(msg)
		if inputCmd != nil {
			cmds = append(cmds, inputCmd)
		}

	case tea.MouseMsg:
		// Only handle mouse events if visible
		if !m.visible {
			return m, nil
		}

		// Only handle wheel events for scrolling
		if msg.Button != tea.MouseButtonWheelUp && msg.Button != tea.MouseButtonWheelDown {
			return m, nil
		}

		// Calculate message pane height (total height minus input pane)
		// Input pane is 6 lines (4 content + 2 borders)
		inputPaneHeight := 6
		messagePaneHeight := m.height - inputPaneHeight

		// Ignore mouse events in input area (bottom of panel)
		if msg.Y >= messagePaneHeight {
			return m, nil
		}

		// Get active session for viewport scrolling
		session := m.ActiveSession()
		if session == nil {
			return m, nil
		}

		// Scroll the viewport
		if msg.Button == tea.MouseButtonWheelUp {
			session.Viewport.ScrollUp(1)
		} else {
			session.Viewport.ScrollDown(1)
		}

		// Clear new content indicator when scrolled to bottom
		if session.Viewport.AtBottom() {
			session.HasNewContent = false
		}

		return m, nil

	case NewSessionCreatedMsg:
		// Switch to the newly created session and focus the input
		m = m.switchToSession(msg.SessionID)
		// Switch to Chat tab to show the new session
		m.activeTab = TabChat
		m.input.Focus()
		return m, nil

	case vimtextarea.SubmitMsg:
		// Only process submission if visible and focused
		if !m.visible || !m.focused {
			return m, nil
		}

		// Handle message submission
		content := strings.TrimSpace(msg.Content)
		if content != "" {
			// Don't add user message here - wait for ProcessIncoming event
			// which confirms the message was delivered (matches orchestration mode)

			// Reset input
			m.input.Reset()

			// Emit SendMessageMsg for parent to handle
			cmds = append(cmds, func() tea.Msg {
				return SendMessageMsg{Content: content}
			})
		}

	default:
		// Only process other messages if visible and focused
		if !m.visible || !m.focused {
			return m, nil
		}

		// Forward other messages to input
		var inputCmd tea.Cmd
		m.input, inputCmd = m.input.Update(msg)
		if inputCmd != nil {
			cmds = append(cmds, inputCmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// handlePubSubEvent processes events from the infrastructure's event bus.
// CRITICAL: Always returns v2Listener.Listen() to continue receiving events.
func (m Model) handlePubSubEvent(event pubsub.Event[any]) (Model, tea.Cmd) {
	if m.v2Listener == nil {
		return m, nil
	}

	// Type-assert to ProcessEvent
	processEvent, ok := event.Payload.(events.ProcessEvent)
	if !ok {
		// Unknown event type - continue listening
		return m, m.v2Listener.Listen()
	}

	// Route event to correct session by ProcessID using O(1) lookup
	m = m.handleProcessEvent(processEvent)

	// Update legacy top-level state for backwards compatibility and UI indicators
	// These fields are used for border color, queue count display, etc.
	switch processEvent.Type {
	case events.ProcessSpawned:
		// Process spawned - try to capture session ID from the process
		// The session ID may not be available yet (comes from init event),
		// so we'll also check on ProcessOutput
		m = m.captureSessionRef()

	case events.ProcessOutput:
		// Update last interaction time on message receive (legacy field)
		m = m.updateInteractionTime()
		// Try to capture session ID if we haven't yet
		// (it becomes available after the init event)
		if m.sessionRef == "" {
			m = m.captureSessionRef()
		}

	case events.ProcessIncoming:
		// Update last interaction time on message delivery (legacy field)
		m = m.updateInteractionTime()

	case events.ProcessReady:
		// Assistant finished its turn
		m.assistantWorking = false
		if m.sessionRef == "" {
			m = m.captureSessionRef()
		}

	case events.ProcessWorking:
		// Assistant is actively processing
		m.assistantWorking = true

	case events.ProcessQueueChanged:
		// Update queue count for UI display
		m.queueCount = processEvent.QueueCount

	case events.ProcessTokenUsage:
		// Update token metrics for UI display (legacy top-level field)
		if processEvent.Metrics != nil {
			m.metrics = processEvent.Metrics
		}

	case events.ProcessError:
		if processEvent.Error != nil {
			// Return error message for toast display
			return m, tea.Batch(
				m.v2Listener.Listen(),
				func() tea.Msg {
					return AssistantErrorMsg{Error: processEvent.Error}
				},
			)
		}
	}

	// CRITICAL: Always continue listening
	return m, m.v2Listener.Listen()
}

// captureSessionRef attempts to capture the session ID from the infrastructure's process registry.
func (m Model) captureSessionRef() Model {
	if m.infra == nil || m.infra.ProcessRegistry == nil {
		return m
	}
	proc := m.infra.ProcessRegistry.Get(ChatPanelProcessID)
	if proc == nil {
		return m
	}
	sessionID := proc.SessionID()
	if sessionID != "" {
		m.sessionRef = sessionID
	}
	return m
}

// AddMessage adds a message to the active session's chat history and marks it dirty.
func (m Model) AddMessage(msg chatrender.Message) Model {
	session := m.ActiveSession()
	if session == nil {
		return m
	}

	session.Messages = append(session.Messages, msg)
	session.ContentDirty = true

	// Track new content arrival when scrolled up
	if !session.Viewport.AtBottom() {
		session.HasNewContent = true
	}

	return m
}

// Messages returns the active session's chat messages.
func (m Model) Messages() []chatrender.Message {
	session := m.ActiveSession()
	if session == nil {
		return nil
	}
	return session.Messages
}

// ClearMessages removes all messages from the active session's chat history.
func (m Model) ClearMessages() Model {
	session := m.ActiveSession()
	if session == nil {
		return m
	}
	session.Messages = make([]chatrender.Message, 0)
	session.ContentDirty = true
	session.HasNewContent = false
	return m
}

// SetInfrastructure sets the v2 infrastructure and initializes the event listener.
// This should be called after creating the infrastructure and before the Model is used.
func (m Model) SetInfrastructure(infra *v2.SimpleInfrastructure) Model {
	m.infra = infra
	// Create context for subscription lifetime
	m.ctx, m.cancel = context.WithCancel(context.Background())
	// Initialize the continuous listener for event subscription
	m.v2Listener = pubsub.NewContinuousListener(m.ctx, infra.EventBus)
	return m
}

// InitListener returns a tea.Cmd to start listening for events.
// Call this after SetInfrastructure to begin receiving events.
func (m Model) InitListener() tea.Cmd {
	if m.v2Listener == nil {
		return nil
	}
	return m.v2Listener.Listen()
}

// SpawnAssistant submits a SpawnProcessCommand to create a new AI assistant process.
// The infrastructure's SimpleInfrastructureConfig contains the AI client, work dir,
// system prompt, and initial prompt - these are configured when creating the infrastructure.
// Returns a tea.Cmd that emits AssistantErrorMsg if submission fails.
func (m Model) SpawnAssistant() tea.Cmd {
	return m.SpawnAssistantForSession(ChatPanelProcessID)
}

// SpawnAssistantForSession submits a SpawnProcessCommand with a specific process ID.
// This is used for multi-session support where each session has its own process.
// Returns a tea.Cmd that emits AssistantErrorMsg if submission fails.
func (m Model) SpawnAssistantForSession(processID string) tea.Cmd {
	if m.infra == nil {
		return func() tea.Msg {
			return AssistantErrorMsg{Error: ErrNoInfrastructure}
		}
	}

	// Create spawn command using v2 command types
	// The SimpleInfrastructure's spawner already has the AI client config
	cmd := command.NewSpawnProcessCommand(command.SourceUser, repository.RoleWorker)
	cmd.ProcessID = processID

	// Submit to infrastructure
	if err := m.infra.Submit(cmd); err != nil {
		return func() tea.Msg {
			return AssistantErrorMsg{Error: err}
		}
	}

	return nil
}

// NextSessionID generates the next sequential session ID based on the number of existing sessions.
// IDs are formatted as "session-1", "session-2", etc.
func (m Model) NextSessionID() string {
	return fmt.Sprintf("session-%d", len(m.sessions)+1)
}

// SendMessage submits a SendToProcessCommand to send a user message to the active session's assistant.
// Returns a tea.Cmd that emits AssistantErrorMsg if submission fails.
func (m Model) SendMessage(content string) tea.Cmd {
	if m.infra == nil {
		return func() tea.Msg {
			return AssistantErrorMsg{Error: ErrNoInfrastructure}
		}
	}

	// Get the active session's ProcessID
	session := m.ActiveSession()
	if session == nil || session.ProcessID == "" {
		return func() tea.Msg {
			return AssistantErrorMsg{Error: errors.New("no active session or process")}
		}
	}

	// Create the send command using v2 command types
	cmd := command.NewSendToProcessCommand(command.SourceUser, session.ProcessID, content)

	// Submit to infrastructure
	if err := m.infra.Submit(cmd); err != nil {
		return func() tea.Msg {
			return AssistantErrorMsg{Error: err}
		}
	}

	return nil
}

// Cleanup releases resources and stops the AI process.
// Should be called when the chat panel is closed or the app exits.
func (m Model) Cleanup() {
	// Cancel the context to stop the listener
	if m.cancel != nil {
		m.cancel()
	}
	// Shutdown the infrastructure
	if m.infra != nil {
		m.infra.Shutdown()
	}
}

// HasInfrastructure returns whether the infrastructure has been set.
func (m Model) HasInfrastructure() bool {
	return m.infra != nil
}

// AssistantWorking returns whether the assistant is currently processing.
// Used for UI feedback like border color changes.
func (m Model) AssistantWorking() bool {
	return m.assistantWorking
}

// QueueCount returns the number of messages queued for the assistant.
// Used for UI feedback like the "[N queued]" indicator.
func (m Model) QueueCount() int {
	return m.queueCount
}

// Metrics returns the token usage metrics for UI display.
// Returns nil if no metrics have been received yet.
func (m Model) Metrics() *metrics.TokenMetrics {
	return m.metrics
}

// Config returns the chat panel configuration.
func (m Model) Config() Config {
	return m.config
}

// now returns the current time, using Clock if set, otherwise time.Now().
func (m Model) now() time.Time {
	if m.Clock != nil {
		return m.Clock()
	}
	return time.Now()
}

// SessionRef returns the session reference for the current session.
// Returns empty string if no session has been established.
func (m Model) SessionRef() string {
	return m.sessionRef
}

// SetSessionRef sets the session reference. This should be called when
// a session ID is received from the AI process (via ProcessSpawned or init event).
func (m Model) SetSessionRef(ref string) Model {
	m.sessionRef = ref
	return m
}

// LastInteractionTime returns the time of the last interaction.
func (m Model) LastInteractionTime() time.Time {
	return m.lastInteractionTime
}

// updateInteractionTime updates the last interaction time to now.
func (m Model) updateInteractionTime() Model {
	m.lastInteractionTime = m.now()
	return m
}

// ShouldResumeSession returns true if the session should be resumed
// (last interaction was within the session timeout period).
func (m Model) ShouldResumeSession() bool {
	// No previous interaction - can't resume
	if m.lastInteractionTime.IsZero() {
		return false
	}
	// No session ref - can't resume
	if m.sessionRef == "" {
		return false
	}
	// Check if within timeout
	elapsed := m.now().Sub(m.lastInteractionTime)
	return elapsed < m.config.SessionTimeout
}

// ClearSession clears the session state for a fresh start.
func (m Model) ClearSession() Model {
	m.sessionRef = ""
	m.lastInteractionTime = time.Time{}
	return m
}

// ActiveSession returns the currently active session.
// Returns nil if no active session exists (should not happen in normal operation).
func (m Model) ActiveSession() *SessionData {
	if m.activeSessionID == "" {
		return nil
	}
	return m.sessions[m.activeSessionID]
}

// CreateSession creates a new session with the given ID and returns it.
// The new session is added to the session maps and the reverse lookup is updated.
// The session is NOT automatically made active - call SwitchSession for that.
func (m Model) CreateSession(id string) (Model, *SessionData) {
	now := m.now()
	session := &SessionData{
		ID:           id,
		ProcessID:    "", // Will be set when process is spawned
		Messages:     make([]chatrender.Message, 0),
		Viewport:     viewport.New(0, 0),
		Status:       events.ProcessStatusPending,
		ContentDirty: true,
		CreatedAt:    now,
		LastActivity: now,
	}

	m.sessions[id] = session
	m.sessionOrder = append(m.sessionOrder, id)

	return m, session
}

// SwitchSession switches to the session with the given ID.
// Returns false if the session does not exist.
func (m Model) SwitchSession(id string) (Model, bool) {
	if _, exists := m.sessions[id]; !exists {
		return m, false
	}
	m.activeSessionID = id
	return m, true
}

// switchToSession is the internal method used by keyboard navigation.
// It switches to the specified session and clears its HasNewContent flag.
// This is the method called when user navigates via Sessions tab and presses Enter.
func (m Model) switchToSession(id string) Model {
	session, exists := m.sessions[id]
	if !exists {
		return m
	}
	m.activeSessionID = id
	// Clear HasNewContent since user is now viewing this session
	session.HasNewContent = false
	return m
}

// clampSessionListCursor ensures the cursor is within valid bounds.
// Called when navigating the Sessions tab to handle cases where sessions were removed.
// Note: cursor index 0 is "Create new session", sessions start at index 1.
func (m Model) clampSessionListCursor() Model {
	// Max cursor is len(sessions) because index 0 is "Create new session"
	maxCursor := len(m.sessionOrder) // sessions are at indices 1..len
	if m.sessionListCursor > maxCursor {
		m.sessionListCursor = maxCursor
	}
	if m.sessionListCursor < 0 {
		m.sessionListCursor = 0
	}
	return m
}

// NextSession cycles to the next session in the session order.
// Wraps around to the first session after the last one.
func (m Model) NextSession() Model {
	if len(m.sessionOrder) <= 1 {
		return m
	}
	// Find current session index
	currentIdx := m.activeSessionIndex()
	nextIdx := (currentIdx + 1) % len(m.sessionOrder)
	return m.switchToSession(m.sessionOrder[nextIdx])
}

// PrevSession cycles to the previous session in the session order.
// Wraps around to the last session from the first one.
func (m Model) PrevSession() Model {
	if len(m.sessionOrder) <= 1 {
		return m
	}
	// Find current session index
	currentIdx := m.activeSessionIndex()
	prevIdx := (currentIdx - 1 + len(m.sessionOrder)) % len(m.sessionOrder)
	return m.switchToSession(m.sessionOrder[prevIdx])
}

// activeSessionIndex returns the index of the active session in sessionOrder.
// Returns 0 if not found.
func (m Model) activeSessionIndex() int {
	for i, id := range m.sessionOrder {
		if id == m.activeSessionID {
			return i
		}
	}
	return 0
}

// SetSessionProcessID updates the ProcessID for a session and maintains the reverse lookup.
// This should be called when a process is spawned for a session.
func (m Model) SetSessionProcessID(sessionID, processID string) Model {
	session, exists := m.sessions[sessionID]
	if !exists {
		return m
	}
	// Update session's ProcessID
	session.ProcessID = processID
	// Update reverse lookup
	m.processToSession[processID] = sessionID
	return m
}

// SessionByProcessID returns the session associated with a process ID using O(1) lookup.
// Returns nil if no session is mapped to the process.
func (m Model) SessionByProcessID(processID string) *SessionData {
	sessionID, exists := m.processToSession[processID]
	if !exists {
		return nil
	}
	return m.sessions[sessionID]
}

// SessionCount returns the number of sessions.
func (m Model) SessionCount() int {
	return len(m.sessions)
}

// RetireSession removes a session from the chat panel and retires the underlying AI process.
// If the retired session is active, switches to another session first.
// Cannot retire the last remaining session.
// Returns the updated model and a tea.Cmd for error handling.
func (m Model) RetireSession(sessionID string) (Model, tea.Cmd) {
	// Can't retire if only one session remains
	if len(m.sessions) <= 1 {
		return m, nil
	}

	// Check session exists
	session, exists := m.sessions[sessionID]
	if !exists {
		return m, nil
	}

	// Submit RetireProcessCommand to properly stop the AI process
	var errorCmd tea.Cmd
	if session.ProcessID != "" && m.infra != nil {
		cmd := command.NewRetireProcessCommand(command.SourceUser, session.ProcessID, "session_retired")
		if err := m.infra.Submit(cmd); err != nil {
			errorCmd = func() tea.Msg {
				return AssistantErrorMsg{Error: err}
			}
		}
	}

	// If retiring the active session, switch to another first
	if sessionID == m.activeSessionID {
		// Find another session to switch to
		for _, otherID := range m.sessionOrder {
			if otherID != sessionID {
				m = m.switchToSession(otherID)
				break
			}
		}
	}

	// Remove from reverse lookup if process ID exists
	if session.ProcessID != "" {
		delete(m.processToSession, session.ProcessID)
	}

	// Remove from sessions map
	delete(m.sessions, sessionID)

	// Remove from session order slice
	for i, id := range m.sessionOrder {
		if id == sessionID {
			m.sessionOrder = append(m.sessionOrder[:i], m.sessionOrder[i+1:]...)
			break
		}
	}

	// Clamp cursor to valid range after removal
	m = m.clampSessionListCursor()

	return m, errorCmd
}

// ActiveSessionID returns the ID of the currently active session.
func (m Model) ActiveSessionID() string {
	return m.activeSessionID
}

// handleProcessEvent routes a ProcessEvent to the correct session by ProcessID using O(1) lookup.
// Returns the updated model. Unknown ProcessIDs are logged and ignored.
func (m Model) handleProcessEvent(event events.ProcessEvent) Model {
	// Use reverse lookup for O(1) session resolution
	session := m.SessionByProcessID(event.ProcessID)
	if session == nil {
		// Unknown process ID - log and ignore (don't crash)
		// In production, this could be logged to debug output
		return m
	}

	return m.appendToSession(session, event)
}

// appendToSession applies an event to the specified session.
// Handles ProcessOutput, ProcessStatusChange, ProcessTokenUsage, and ProcessStatusFailed events.
func (m Model) appendToSession(session *SessionData, event events.ProcessEvent) Model {
	isActiveSession := session.ID == m.activeSessionID

	switch event.Type {
	case events.ProcessOutput:
		if event.Output != "" {
			// Detect tool calls by 🔧 prefix (same as orchestration mode)
			isToolCall := strings.HasPrefix(event.Output, "🔧")
			session.Messages = append(session.Messages, chatrender.Message{
				Role:       RoleAssistant,
				Content:    event.Output,
				IsToolCall: isToolCall,
			})
			session.ContentDirty = true

			// Track new content if not viewing this session or scrolled up
			if !isActiveSession || !session.Viewport.AtBottom() {
				session.HasNewContent = true
			}
		}

	case events.ProcessIncoming:
		// Message was delivered to the assistant - show it in chat
		if event.Message != "" {
			session.Messages = append(session.Messages, chatrender.Message{
				Role:    RoleUser,
				Content: event.Message,
			})
			session.ContentDirty = true

			// Track new content if not viewing this session
			if !isActiveSession {
				session.HasNewContent = true
			}
		}

	case events.ProcessStatusChange:
		session.Status = event.Status
		// Mark content dirty if status change might affect UI
		session.ContentDirty = true

	case events.ProcessTokenUsage:
		if event.Metrics != nil {
			session.Metrics = event.Metrics
		}

	case events.ProcessReady:
		session.Status = events.ProcessStatusReady

	case events.ProcessWorking:
		session.Status = events.ProcessStatusWorking

	case events.ProcessError:
		// Mark session as failed on error
		if event.Error != nil {
			session.Status = events.ProcessStatusFailed
			session.ContentDirty = true
		}
	}

	// Update last activity timestamp for all event types
	session.LastActivity = m.now()

	return m
}
