// Package orchestration implements the three-pane orchestration mode TUI.
// resume.go provides session resumption functionality for restoring a previous
// orchestration session.
package orchestration

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zjrosen/xorchestrator/internal/log"
	"github.com/zjrosen/xorchestrator/internal/orchestration/session"
)

// ResumeSessionMsg triggers loading of a previous session for resumption.
// This message initiates the session restoration flow.
type ResumeSessionMsg struct {
	// SessionDir is the path to the session directory to resume.
	SessionDir string
}

// StartRestoredSessionMsg signals that session data has been loaded and the
// restored session flow should begin. This triggers infrastructure creation
// with the RestoredSession set in InitializerConfig.
type StartRestoredSessionMsg struct {
	// Session contains all loaded session data ready for restoration.
	Session *session.ResumableSession
}

// handleResumeSession processes a ResumeSessionMsg by loading the session data,
// building UI state, and restoring the model. It stores the loaded session for
// later infrastructure creation and returns a command to start the restored session.
//
// Error handling: If loading fails, an error is displayed to the user via SetError
// rather than silently failing.
func (m Model) handleResumeSession(msg ResumeSessionMsg) (Model, tea.Cmd) {
	// Load session data from disk
	loadedSession, err := session.LoadResumableSession(msg.SessionDir)
	if err != nil {
		log.Error(log.CatOrch, "Failed to load session for resumption",
			"subsystem", "resume", "sessionDir", msg.SessionDir, "error", err)
		m = m.SetError("Failed to load session: " + err.Error())
		return m, nil
	}

	log.Debug(log.CatOrch, "Session loaded for resumption",
		"subsystem", "resume",
		"sessionDir", msg.SessionDir,
		"sessionID", loadedSession.Metadata.SessionID,
		"activeWorkers", len(loadedSession.ActiveWorkers),
		"retiredWorkers", len(loadedSession.RetiredWorkers))

	// Build UI state from loaded session
	uiState := session.BuildRestoredUIState(loadedSession)
	if uiState == nil {
		log.Error(log.CatOrch, "Failed to build UI state from session",
			"subsystem", "resume", "sessionDir", msg.SessionDir)
		m = m.SetError("Failed to build UI state from session")
		return m, nil
	}

	log.Debug(log.CatOrch, "UI state built from session",
		"subsystem", "resume",
		"coordinatorMsgs", len(uiState.CoordinatorMessages),
		"workerCount", len(uiState.WorkerIDs),
		"messageLogEntries", len(uiState.MessageLogEntries))

	// Restore model from UI state (populates coordinator, worker, message panes)
	m = m.RestoreFromSession(uiState)

	// Store loaded session for infrastructure creation
	m.resumedSession = loadedSession

	// Skip worktree prompt - decision was already made in original session
	m.worktreeDecisionMade = true

	// Set resume indicator state
	m.isResumedSession = true
	m.resumedAt = time.Now()
	m.originalStartTime = uiState.StartTime

	// Add separator message to coordinator pane
	separatorContent := formatResumedSeparator(m.resumedAt)
	m = m.AddChatMessage("system", separatorContent, false)

	log.Debug(log.CatOrch, "Resume indicator state set",
		"subsystem", "resume",
		"isResumedSession", m.isResumedSession,
		"resumedAt", m.resumedAt,
		"originalStartTime", m.originalStartTime)

	// Return command to start the restored session flow
	return m, startRestoredSession(loadedSession)
}

// startRestoredSession returns a tea.Cmd that emits a StartRestoredSessionMsg.
// This triggers the infrastructure creation with restoration mode enabled.
func startRestoredSession(loadedSession *session.ResumableSession) tea.Cmd {
	return func() tea.Msg {
		return StartRestoredSessionMsg{
			Session: loadedSession,
		}
	}
}

// handleStartRestoredSession handles the StartRestoredSessionMsg by initiating
// the initialization flow with the restored session. It creates an Initializer
// with RestoredSession set, which will use session.Reopen() and populate
// repositories from the loaded session data.
func (m Model) handleStartRestoredSession(msg StartRestoredSessionMsg) (Model, tea.Cmd) {
	if msg.Session == nil {
		m = m.SetError("No session data available for restoration")
		return m, nil
	}

	log.Debug(log.CatOrch, "Starting restored session initialization",
		"subsystem", "resume",
		"sessionID", msg.Session.Metadata.SessionID)

	// The StartCoordinatorMsg handler will create the Initializer
	// with m.resumedSession, which triggers restoration mode.
	// We return a StartCoordinatorMsg to reuse the existing initialization flow.
	return m, func() tea.Msg {
		return StartCoordinatorMsg{}
	}
}

// formatResumedSeparator creates the separator message content for resumed sessions.
// Format: "─── Session Resumed (Jan 2 15:04) ───"
func formatResumedSeparator(resumedAt time.Time) string {
	timeStr := resumedAt.Format("Jan 2 15:04")
	return fmt.Sprintf("─── Session Resumed (%s) ───", timeStr)
}
