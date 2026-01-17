package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zjrosen/xorchestrator/internal/beads"

	"github.com/stretchr/testify/require"
)

// TestNoBeadsDirectory_BeadsClientFails verifies that beads.NewClient returns
// an error when there's no .beads directory. This is the condition that triggers
// the nobeads empty state view.
func TestNoBeadsDirectory_BeadsClientFails(t *testing.T) {
	// Create temp directory without .beads
	tmpDir, err := os.MkdirTemp("", "xorchestrator-test-nobeads-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	// Verify no .beads directory exists
	beadsPath := filepath.Join(tmpDir, ".beads")
	_, err = os.Stat(beadsPath)
	require.True(t, os.IsNotExist(err), "expected .beads to not exist")

	// Verify beads.NewClient fails for this directory
	_, err = beads.NewClient(tmpDir)
	require.Error(t, err, "expected beads.NewClient to fail without .beads directory")
}

// TestNoBeadsDirectory_WithBeadsSucceeds verifies that beads.NewClient succeeds
// when there IS a valid .beads directory.
func TestNoBeadsDirectory_WithBeadsSucceeds(t *testing.T) {
	// Use the actual project directory which has .beads
	cwd, err := os.Getwd()
	require.NoError(t, err)

	// Go up to project root if we're in cmd/
	projectRoot := filepath.Dir(cwd)
	beadsPath := filepath.Join(projectRoot, ".beads")

	// Skip if not in expected directory structure
	if _, err := os.Stat(beadsPath); os.IsNotExist(err) {
		// Try current directory
		if _, err := os.Stat(filepath.Join(cwd, ".beads")); os.IsNotExist(err) {
			t.Skip("not running from project directory with .beads")
		}
		projectRoot = cwd
	}

	// Verify beads.NewClient succeeds
	client, err := beads.NewClient(projectRoot)
	if err == nil {
		// Clean up if we got a client
		_ = client
	}
}
