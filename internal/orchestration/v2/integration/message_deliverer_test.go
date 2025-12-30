package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/client"
)

// mockSessionProvider implements SessionProvider for testing.
type mockSessionProvider struct {
	sessionID    string
	sessionErr   error
	mcpConfig    string
	mcpConfigErr error
	workDir      string
}

func (m *mockSessionProvider) GetProcessSessionID(processID string) (string, error) {
	return m.sessionID, m.sessionErr
}

func (m *mockSessionProvider) GenerateProcessMCPConfig(processID string) (string, error) {
	return m.mcpConfig, m.mcpConfigErr
}

func (m *mockSessionProvider) GetWorkDir() string {
	return m.workDir
}

// slowMockSessionProvider implements SessionProvider with configurable delays for timeout testing.
type slowMockSessionProvider struct {
	sessionID string
	mcpConfig string
	workDir   string
	delay     time.Duration
}

func (m *slowMockSessionProvider) GetProcessSessionID(processID string) (string, error) {
	time.Sleep(m.delay)
	return m.sessionID, nil
}

func (m *slowMockSessionProvider) GenerateProcessMCPConfig(processID string) (string, error) {
	return m.mcpConfig, nil
}

func (m *slowMockSessionProvider) GetWorkDir() string {
	return m.workDir
}

// mockHeadlessClient implements client.HeadlessClient for testing.
type mockHeadlessClient struct {
	mock.Mock
}

func (m *mockHeadlessClient) Type() client.ClientType {
	return client.ClientMock
}

func (m *mockHeadlessClient) Spawn(ctx context.Context, cfg client.Config) (client.HeadlessProcess, error) {
	args := m.Called(ctx, cfg)
	proc := args.Get(0)
	if proc == nil {
		return nil, args.Error(1)
	}
	return proc.(client.HeadlessProcess), args.Error(1)
}

// mockHeadlessProcess implements client.HeadlessProcess for testing.
type mockHeadlessProcess struct {
	mock.Mock
}

func (m *mockHeadlessProcess) Events() <-chan client.OutputEvent {
	ch := make(chan client.OutputEvent)
	close(ch)
	return ch
}

func (m *mockHeadlessProcess) Errors() <-chan error {
	ch := make(chan error)
	close(ch)
	return ch
}

func (m *mockHeadlessProcess) SessionRef() string {
	return "mock-session"
}

func (m *mockHeadlessProcess) Status() client.ProcessStatus {
	return client.StatusRunning
}

func (m *mockHeadlessProcess) IsRunning() bool {
	return true
}

func (m *mockHeadlessProcess) WorkDir() string {
	return "/test/workdir"
}

func (m *mockHeadlessProcess) PID() int {
	return 1234
}

func (m *mockHeadlessProcess) Cancel() error {
	args := m.Called()
	return args.Error(0)
}

func (m *mockHeadlessProcess) Wait() error {
	args := m.Called()
	return args.Error(0)
}

// mockProcessResumer implements ProcessResumer for testing.
type mockProcessResumer struct {
	mock.Mock
}

func (m *mockProcessResumer) ResumeProcess(processID string, proc client.HeadlessProcess) error {
	args := m.Called(processID, proc)
	return args.Error(0)
}

func TestProcessSessionDeliverer_Deliver_Success(t *testing.T) {
	// Setup
	sessionProvider := &mockSessionProvider{
		sessionID: "session-123",
		mcpConfig: `{"servers":[]}`,
		workDir:   "/test/workdir",
	}

	mockClient := &mockHeadlessClient{}
	mockProc := &mockHeadlessProcess{}
	mockResumer := &mockProcessResumer{}

	// Expect Spawn to be called with correct config
	mockClient.On("Spawn", mock.Anything, mock.MatchedBy(func(cfg client.Config) bool {
		return cfg.SessionID == "session-123" &&
			cfg.WorkDir == "/test/workdir" &&
			cfg.MCPConfig == `{"servers":[]}` &&
			cfg.Prompt == "Hello worker!" &&
			cfg.SkipPermissions == true
	})).Return(mockProc, nil)

	// Expect ResumeProcess to be called
	mockResumer.On("ResumeProcess", "worker-1", mockProc).Return(nil)

	// Create deliverer with the real implementation
	deliverer := NewProcessSessionDeliverer(sessionProvider, mockClient, mockResumer)

	// Execute
	err := deliverer.Deliver(context.Background(), "worker-1", "Hello worker!")

	// Assert
	require.NoError(t, err)
	mockClient.AssertExpectations(t)
	mockResumer.AssertExpectations(t)
}

func TestProcessSessionDeliverer_Deliver_SessionNotFound(t *testing.T) {
	// Setup
	sessionProvider := &mockSessionProvider{
		sessionErr: errors.New("process not found"),
	}

	deliverer := NewProcessSessionDeliverer(
		sessionProvider,
		&mockHeadlessClient{},
		&mockProcessResumer{},
	)

	// Execute
	err := deliverer.Deliver(context.Background(), "worker-1", "Hello")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get session for process worker-1")
}

func TestProcessSessionDeliverer_Deliver_EmptySessionID(t *testing.T) {
	// Setup - session exists but has empty ID (worker still starting)
	sessionProvider := &mockSessionProvider{
		sessionID: "", // Empty session ID
	}

	deliverer := NewProcessSessionDeliverer(
		sessionProvider,
		&mockHeadlessClient{},
		&mockProcessResumer{},
	)

	// Execute
	err := deliverer.Deliver(context.Background(), "worker-1", "Hello")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no session ID")
}

func TestProcessSessionDeliverer_Deliver_MCPConfigError(t *testing.T) {
	// Setup
	sessionProvider := &mockSessionProvider{
		sessionID:    "session-123",
		mcpConfigErr: errors.New("config generation failed"),
	}

	deliverer := NewProcessSessionDeliverer(
		sessionProvider,
		&mockHeadlessClient{},
		&mockProcessResumer{},
	)

	// Execute
	err := deliverer.Deliver(context.Background(), "worker-1", "Hello")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate MCP config")
}

func TestProcessSessionDeliverer_Deliver_SpawnError(t *testing.T) {
	// Setup
	sessionProvider := &mockSessionProvider{
		sessionID: "session-123",
		mcpConfig: `{}`,
		workDir:   "/test",
	}

	mockClient := &mockHeadlessClient{}
	mockClient.On("Spawn", mock.Anything, mock.Anything).Return(nil, errors.New("spawn failed"))

	deliverer := NewProcessSessionDeliverer(
		sessionProvider,
		mockClient,
		&mockProcessResumer{},
	)

	// Execute
	err := deliverer.Deliver(context.Background(), "worker-1", "Hello")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resume session")
	mockClient.AssertExpectations(t)
}

func TestProcessSessionDeliverer_Deliver_ResumeProcessError(t *testing.T) {
	// Setup
	sessionProvider := &mockSessionProvider{
		sessionID: "session-123",
		mcpConfig: `{}`,
		workDir:   "/test",
	}

	mockClient := &mockHeadlessClient{}
	mockProc := &mockHeadlessProcess{}
	mockResumer := &mockProcessResumer{}

	mockClient.On("Spawn", mock.Anything, mock.Anything).Return(mockProc, nil)
	mockResumer.On("ResumeProcess", "worker-1", mockProc).Return(errors.New("pool resume failed"))
	mockProc.On("Cancel").Return(nil) // Should try to cancel on failure

	deliverer := NewProcessSessionDeliverer(
		sessionProvider,
		mockClient,
		mockResumer,
	)

	// Execute
	err := deliverer.Deliver(context.Background(), "worker-1", "Hello")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resume process")
	mockClient.AssertExpectations(t)
	mockResumer.AssertExpectations(t)
	mockProc.AssertExpectations(t)
}

func TestProcessSessionDeliverer_Deliver_ContextCancellation(t *testing.T) {
	// Setup
	sessionProvider := &mockSessionProvider{
		sessionID: "session-123",
		mcpConfig: `{}`,
		workDir:   "/test",
	}

	// Note: We don't need to set up mockClient.On("Spawn") because
	// the function should return early when context is cancelled BEFORE spawn

	deliverer := NewProcessSessionDeliverer(
		sessionProvider,
		&mockHeadlessClient{},
		&mockProcessResumer{},
		WithDeliveryTimeout(100*time.Millisecond),
	)

	// Create already cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Execute
	err := deliverer.Deliver(ctx, "worker-1", "Hello")

	// Assert - should fail due to context cancellation check before spawn
	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestProcessSessionDeliverer_Deliver_Timeout(t *testing.T) {
	// Setup - use a slow session provider to trigger timeout
	slowSessionProvider := &slowMockSessionProvider{
		sessionID: "session-123",
		mcpConfig: `{}`,
		workDir:   "/test",
		delay:     100 * time.Millisecond, // Delay longer than timeout
	}

	deliverer := NewProcessSessionDeliverer(
		slowSessionProvider,
		&mockHeadlessClient{},
		&mockProcessResumer{},
		WithDeliveryTimeout(10*time.Millisecond), // Very short timeout
	)

	// Execute
	err := deliverer.Deliver(context.Background(), "worker-1", "Hello")

	// Assert - should fail due to timeout exceeded before spawn
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout exceeded before spawn")
}

func TestProcessSessionDeliverer_WithDeliveryTimeout(t *testing.T) {
	// Test custom timeout option
	sessionProvider := &mockSessionProvider{}
	mockClient := &mockHeadlessClient{}

	deliverer := NewProcessSessionDeliverer(
		sessionProvider,
		mockClient,
		nil, // resumer not used in this test
		WithDeliveryTimeout(5*time.Second),
	)

	assert.Equal(t, 5*time.Second, deliverer.timeout)
}

func TestDefaultDeliveryTimeout(t *testing.T) {
	// Verify default timeout is 3 seconds as specified in acceptance criteria
	assert.Equal(t, 3*time.Second, DefaultDeliveryTimeout)
}
