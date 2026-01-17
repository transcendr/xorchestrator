package session

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// GitRemoteGetter is a minimal interface for getting git remote URLs.
// This is satisfied by git.RealExecutor and allows easy mocking in tests.
type GitRemoteGetter interface {
	// GetRemoteURL returns the URL for the named remote (e.g., "origin").
	// Returns empty string and nil error if remote doesn't exist.
	GetRemoteURL(name string) (string, error)
}

// SessionPathBuilder constructs session directory paths for centralized storage.
// It handles path construction for the structure:
//
//	{baseDir}/{applicationName}/{YYYY-MM-DD}/{sessionID}/
type SessionPathBuilder struct {
	baseDir         string
	applicationName string
}

// NewSessionPathBuilder creates a builder with the given base directory and application name.
// If baseDir is empty, DefaultBaseDir() is used.
// If applicationName is empty, it must be set later via DeriveApplicationName or directly.
func NewSessionPathBuilder(baseDir, applicationName string) *SessionPathBuilder {
	if baseDir == "" {
		baseDir = DefaultBaseDir()
	}
	return &SessionPathBuilder{
		baseDir:         baseDir,
		applicationName: applicationName,
	}
}

// DefaultBaseDir returns the default base directory for session storage: ~/.xorchestrator/sessions
// Returns empty string if home directory cannot be determined.
func DefaultBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".xorchestrator", "sessions")
}

// DeriveApplicationName derives the application name from available sources:
//  1. If name is already set on the builder, return it
//  2. Extract from git remote "origin" URL (repo name without .git suffix)
//  3. Fall back to the basename of the work directory
//
// The gitExecutor can be nil if git is not available.
func DeriveApplicationName(workDir string, gitExecutor GitRemoteGetter) string {
	// Try git remote first
	if gitExecutor != nil {
		url, err := gitExecutor.GetRemoteURL("origin")
		if err == nil && url != "" {
			if name := extractRepoNameFromURL(url); name != "" {
				return name
			}
		}
	}

	// Fall back to directory basename
	return filepath.Base(workDir)
}

// extractRepoNameFromURL extracts the repository name from various git URL formats.
// Handles:
//   - https://github.com/user/repo.git -> repo
//   - https://github.com/user/repo -> repo
//   - git@github.com:user/repo.git -> repo
//   - ssh://git@github.com/user/repo.git -> repo
//
// Returns empty string if extraction fails.
func extractRepoNameFromURL(url string) string {
	url = strings.TrimSpace(url)
	if url == "" {
		return ""
	}

	// Remove .git suffix if present
	url = strings.TrimSuffix(url, ".git")

	// Remove trailing slashes
	url = strings.TrimRight(url, "/")

	// Handle SSH format: git@github.com:user/repo or git@github.com:repo
	if strings.Contains(url, "@") && strings.Contains(url, ":") && !strings.HasPrefix(url, "ssh://") {
		colonIdx := strings.LastIndex(url, ":")
		if colonIdx != -1 {
			afterColon := url[colonIdx+1:]
			// Extract last path component after colon
			parts := strings.Split(afterColon, "/")
			if len(parts) > 0 && parts[len(parts)-1] != "" {
				return parts[len(parts)-1]
			}
		}
	}

	// Handle HTTPS/SSH URL format: extract last path component
	// Match pattern: anything/reponame at the end
	pathPattern := regexp.MustCompile(`[/:]([^/:]+)$`)
	matches := pathPattern.FindStringSubmatch(url)
	if len(matches) >= 2 && matches[1] != "" {
		return matches[1]
	}

	return ""
}

// SessionDir returns the full path for a session directory.
// Format: {baseDir}/{applicationName}/{YYYY-MM-DD}/{sessionID}
func (b *SessionPathBuilder) SessionDir(sessionID string, timestamp time.Time) string {
	date := timestamp.Format("2006-01-02")
	return filepath.Join(b.baseDir, b.applicationName, date, sessionID)
}

// IndexPath returns the path to the global sessions.json index.
// Format: {baseDir}/sessions.json
func (b *SessionPathBuilder) IndexPath() string {
	return filepath.Join(b.baseDir, "sessions.json")
}

// ApplicationIndexPath returns the path to the per-application sessions.json index.
// Format: {baseDir}/{applicationName}/sessions.json
func (b *SessionPathBuilder) ApplicationIndexPath() string {
	return filepath.Join(b.baseDir, b.applicationName, "sessions.json")
}

// DateDir returns the date partition directory for a given timestamp.
// Format: {baseDir}/{applicationName}/{YYYY-MM-DD}
func (b *SessionPathBuilder) DateDir(timestamp time.Time) string {
	date := timestamp.Format("2006-01-02")
	return filepath.Join(b.baseDir, b.applicationName, date)
}

// ApplicationName returns the stored application name.
func (b *SessionPathBuilder) ApplicationName() string {
	return b.applicationName
}

// BaseDir returns the stored base directory.
func (b *SessionPathBuilder) BaseDir() string {
	return b.baseDir
}
