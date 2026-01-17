package handler

import (
	"context"
	"strings"

	"github.com/zjrosen/xorchestrator/internal/orchestration/client"
)

// ===========================================================================
// Mock Client for Testing
// ===========================================================================

// mockHeadlessClient implements client.HeadlessClient for testing.
type mockHeadlessClient struct {
	clientType  client.ClientType
	spawnErr    error
	spawnedProc *mockHeadlessProcess
}

func (m *mockHeadlessClient) Type() client.ClientType {
	return m.clientType
}

func (m *mockHeadlessClient) Spawn(ctx context.Context, cfg client.Config) (client.HeadlessProcess, error) {
	if m.spawnErr != nil {
		return nil, m.spawnErr
	}
	return m.spawnedProc, nil
}

// mockHeadlessProcess implements client.HeadlessProcess for testing.
type mockHeadlessProcess struct {
	sessionRef string
	events     chan client.OutputEvent
	errors     chan error
	running    bool
	status     client.ProcessStatus
	pid        int
	workDir    string
}

func newMockHeadlessProcess(sessionRef string) *mockHeadlessProcess {
	return &mockHeadlessProcess{
		sessionRef: sessionRef,
		events:     make(chan client.OutputEvent, 10),
		errors:     make(chan error, 10),
		running:    true,
		status:     client.StatusRunning,
		pid:        12345,
		workDir:    "/test/workdir",
	}
}

func (m *mockHeadlessProcess) Events() <-chan client.OutputEvent {
	return m.events
}

func (m *mockHeadlessProcess) Errors() <-chan error {
	return m.errors
}

func (m *mockHeadlessProcess) SessionRef() string {
	return m.sessionRef
}

func (m *mockHeadlessProcess) Status() client.ProcessStatus {
	return m.status
}

func (m *mockHeadlessProcess) IsRunning() bool {
	return m.running
}

func (m *mockHeadlessProcess) WorkDir() string {
	return m.workDir
}

func (m *mockHeadlessProcess) PID() int {
	return m.pid
}

func (m *mockHeadlessProcess) Cancel() error {
	m.running = false
	m.status = client.StatusCancelled
	close(m.events)
	return nil
}

func (m *mockHeadlessProcess) Wait() error {
	return nil
}

// SendInitEvent sends an init event with the given session ID to the events channel.
// This allows tests to set the session ID on a process through the normal init flow.
func (m *mockHeadlessProcess) SendInitEvent(sessionID string) {
	m.events <- client.OutputEvent{
		Type:      client.EventSystem,
		SubType:   "init",
		SessionID: sessionID,
	}
}

// ===========================================================================
// Helper Functions
// ===========================================================================

// containsString checks if a string contains a substring.
func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}
