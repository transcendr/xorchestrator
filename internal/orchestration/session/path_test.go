package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// mockGitRemoteGetter implements GitRemoteGetter for testing.
type mockGitRemoteGetter struct {
	url string
	err error
}

func (m *mockGitRemoteGetter) GetRemoteURL(name string) (string, error) {
	return m.url, m.err
}

func TestDefaultBaseDir(t *testing.T) {
	baseDir := DefaultBaseDir()
	require.NotEmpty(t, baseDir, "DefaultBaseDir should return non-empty path")

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	expected := filepath.Join(home, ".xorchestrator", "sessions")
	require.Equal(t, expected, baseDir, "DefaultBaseDir should return ~/.xorchestrator/sessions")
}

func TestNewSessionPathBuilder(t *testing.T) {
	t.Run("with explicit base dir", func(t *testing.T) {
		builder := NewSessionPathBuilder("/custom/base", "myapp")
		require.Equal(t, "/custom/base", builder.BaseDir())
		require.Equal(t, "myapp", builder.ApplicationName())
	})

	t.Run("with empty base dir uses default", func(t *testing.T) {
		builder := NewSessionPathBuilder("", "myapp")
		require.Equal(t, DefaultBaseDir(), builder.BaseDir())
		require.Equal(t, "myapp", builder.ApplicationName())
	})

	t.Run("with empty application name", func(t *testing.T) {
		builder := NewSessionPathBuilder("/base", "")
		require.Equal(t, "/base", builder.BaseDir())
		require.Equal(t, "", builder.ApplicationName())
	})
}

func TestSessionDir(t *testing.T) {
	baseDir := filepath.Join("home", "user", ".xorchestrator", "sessions")
	builder := NewSessionPathBuilder(baseDir, "xorchestrator")
	timestamp := time.Date(2026, 1, 11, 14, 30, 0, 0, time.UTC)
	sessionID := "abc123-uuid"

	result := builder.SessionDir(sessionID, timestamp)
	expected := filepath.Join(baseDir, "xorchestrator", "2026-01-11", "abc123-uuid")
	require.Equal(t, expected, result, "SessionDir should construct correct path")
}

func TestSessionDir_DateFormat(t *testing.T) {
	builder := NewSessionPathBuilder("/base", "app")

	tests := []struct {
		name      string
		timestamp time.Time
		wantDate  string
	}{
		{
			name:      "single digit month and day",
			timestamp: time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
			wantDate:  "2026-01-05",
		},
		{
			name:      "double digit month and day",
			timestamp: time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC),
			wantDate:  "2026-12-25",
		},
		{
			name:      "leap year date",
			timestamp: time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
			wantDate:  "2024-02-29",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := builder.SessionDir("session-id", tc.timestamp)
			require.Contains(t, result, tc.wantDate, "Date format should be YYYY-MM-DD")
		})
	}
}

func TestIndexPath(t *testing.T) {
	baseDir := filepath.Join("home", "user", ".xorchestrator", "sessions")
	builder := NewSessionPathBuilder(baseDir, "xorchestrator")
	result := builder.IndexPath()
	expected := filepath.Join(baseDir, "sessions.json")
	require.Equal(t, expected, result, "IndexPath should return {baseDir}/sessions.json")
}

func TestApplicationIndexPath(t *testing.T) {
	baseDir := filepath.Join("home", "user", ".xorchestrator", "sessions")
	builder := NewSessionPathBuilder(baseDir, "xorchestrator")
	result := builder.ApplicationIndexPath()
	expected := filepath.Join(baseDir, "xorchestrator", "sessions.json")
	require.Equal(t, expected, result, "ApplicationIndexPath should return {baseDir}/{app}/sessions.json")
}

func TestDateDir(t *testing.T) {
	baseDir := filepath.Join("home", "user", ".xorchestrator", "sessions")
	builder := NewSessionPathBuilder(baseDir, "xorchestrator")
	timestamp := time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC)
	result := builder.DateDir(timestamp)
	expected := filepath.Join(baseDir, "xorchestrator", "2026-01-11")
	require.Equal(t, expected, result, "DateDir should return {baseDir}/{app}/{date}")
}

func TestDeriveApplicationName_FromGitRemote(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		// HTTPS URLs
		{
			name:     "https with .git suffix",
			url:      "https://github.com/user/repo.git",
			expected: "repo",
		},
		{
			name:     "https without .git suffix",
			url:      "https://github.com/user/repo",
			expected: "repo",
		},
		{
			name:     "https with organization",
			url:      "https://github.com/anthropic/claude-code.git",
			expected: "claude-code",
		},
		// SSH URLs
		{
			name:     "ssh format with .git",
			url:      "git@github.com:user/repo.git",
			expected: "repo",
		},
		{
			name:     "ssh format without .git",
			url:      "git@github.com:user/repo",
			expected: "repo",
		},
		{
			name:     "ssh:// prefix format",
			url:      "ssh://git@github.com/user/repo.git",
			expected: "repo",
		},
		// GitLab URLs
		{
			name:     "gitlab https",
			url:      "https://gitlab.com/group/subgroup/repo.git",
			expected: "repo",
		},
		{
			name:     "gitlab ssh",
			url:      "git@gitlab.com:group/subgroup/repo.git",
			expected: "repo",
		},
		// Bitbucket URLs
		{
			name:     "bitbucket https",
			url:      "https://bitbucket.org/team/repo.git",
			expected: "repo",
		},
		// Self-hosted
		{
			name:     "self hosted https",
			url:      "https://git.company.com/projects/myrepo.git",
			expected: "myrepo",
		},
		// Edge cases
		{
			name:     "url with trailing slash",
			url:      "https://github.com/user/repo/",
			expected: "repo",
		},
		{
			name:     "url with extra whitespace",
			url:      "  https://github.com/user/repo.git  ",
			expected: "repo",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockGitRemoteGetter{url: tc.url, err: nil}
			result := DeriveApplicationName("/some/work/dir", mock)
			require.Equal(t, tc.expected, result, "Should extract repo name from URL")
		})
	}
}

func TestDeriveApplicationName_FallbackToDirectory(t *testing.T) {
	t.Run("git remote returns empty", func(t *testing.T) {
		mock := &mockGitRemoteGetter{url: "", err: nil}
		result := DeriveApplicationName("/path/to/my-project", mock)
		require.Equal(t, "my-project", result, "Should fall back to directory basename")
	})

	t.Run("git remote returns error", func(t *testing.T) {
		mock := &mockGitRemoteGetter{url: "", err: os.ErrNotExist}
		result := DeriveApplicationName("/path/to/my-project", mock)
		require.Equal(t, "my-project", result, "Should fall back to directory basename on error")
	})

	t.Run("git executor is nil", func(t *testing.T) {
		result := DeriveApplicationName("/path/to/my-project", nil)
		require.Equal(t, "my-project", result, "Should fall back to directory basename when nil")
	})
}

func TestExtractRepoNameFromURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		// Basic cases
		{"empty string", "", ""},
		{"whitespace only", "   ", ""},

		// HTTPS variations
		{"https basic", "https://github.com/user/repo.git", "repo"},
		{"https no git suffix", "https://github.com/user/repo", "repo"},
		{"https deep path", "https://gitlab.com/a/b/c/repo.git", "repo"},

		// SSH variations
		{"ssh colon format", "git@github.com:user/repo.git", "repo"},
		{"ssh colon no suffix", "git@github.com:user/repo", "repo"},
		{"ssh protocol", "ssh://git@github.com/user/repo.git", "repo"},

		// Edge cases
		{"trailing slash", "https://github.com/user/repo/", "repo"},
		{"double .git", "https://github.com/user/repo.git.git", "repo.git"},
		{"ssh no repo after colon", "git@github.com:", ""},
		{"ssh colon only", "user@host:", ""},
		{"no path extracts hostname", "https://github.com", "github.com"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractRepoNameFromURL(tc.url)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestApplicationName_Getter(t *testing.T) {
	builder := NewSessionPathBuilder("/base", "test-app")
	require.Equal(t, "test-app", builder.ApplicationName())
}

func TestBaseDir_Getter(t *testing.T) {
	builder := NewSessionPathBuilder("/custom/path", "app")
	require.Equal(t, "/custom/path", builder.BaseDir())
}

func TestHomeDirectoryExpansion(t *testing.T) {
	// Verify DefaultBaseDir actually expands home directory
	baseDir := DefaultBaseDir()
	require.NotContains(t, baseDir, "~", "DefaultBaseDir should expand ~ to actual home")
	require.True(t, filepath.IsAbs(baseDir), "DefaultBaseDir should return absolute path")
}

func TestPathBuilder_CompleteWorkflow(t *testing.T) {
	// Test a complete workflow: create builder, derive name, construct paths
	baseDir := filepath.Join("home", "testuser", ".xorchestrator", "sessions")
	mock := &mockGitRemoteGetter{url: "git@github.com:anthropic/claude-code.git", err: nil}

	appName := DeriveApplicationName("/path/to/claude-code", mock)
	require.Equal(t, "claude-code", appName)

	builder := NewSessionPathBuilder(baseDir, appName)
	timestamp := time.Date(2026, 1, 11, 10, 30, 0, 0, time.UTC)
	sessionID := "a1b2c3d4-e5f6-7890"

	// Verify all path methods
	require.Equal(t, baseDir, builder.BaseDir())
	require.Equal(t, "claude-code", builder.ApplicationName())
	require.Equal(t,
		filepath.Join(baseDir, "sessions.json"),
		builder.IndexPath())
	require.Equal(t,
		filepath.Join(baseDir, "claude-code", "sessions.json"),
		builder.ApplicationIndexPath())
	require.Equal(t,
		filepath.Join(baseDir, "claude-code", "2026-01-11"),
		builder.DateDir(timestamp))
	require.Equal(t,
		filepath.Join(baseDir, "claude-code", "2026-01-11", "a1b2c3d4-e5f6-7890"),
		builder.SessionDir(sessionID, timestamp))
}
