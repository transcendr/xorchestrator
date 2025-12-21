package beads

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUpdateStatus_InvalidIssue tests error handling for invalid issue IDs.
func TestUpdateStatus_InvalidIssue(t *testing.T) {
	// Check if bd command is available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd CLI not available, skipping integration test")
	}

	err := UpdateStatus("nonexistent-xyz", StatusInProgress)
	require.Error(t, err, "expected error for nonexistent issue")
}

// TestUpdatePriority_InvalidIssue tests error handling for invalid issue IDs.
func TestUpdatePriority_InvalidIssue(t *testing.T) {
	// Check if bd command is available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd CLI not available, skipping integration test")
	}

	err := UpdatePriority("nonexistent-xyz", PriorityHigh)
	require.Error(t, err, "expected error for nonexistent issue")
}

// TestCloseIssue_InvalidIssue tests error handling for invalid issue IDs.
func TestCloseIssue_InvalidIssue(t *testing.T) {
	// Check if bd command is available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd CLI not available, skipping integration test")
	}

	err := CloseIssue("nonexistent-xyz", "testing")
	require.Error(t, err, "expected error for nonexistent issue")
}

// TestReopenIssue_InvalidIssue tests error handling for invalid issue IDs.
func TestReopenIssue_InvalidIssue(t *testing.T) {
	// Check if bd command is available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd CLI not available, skipping integration test")
	}

	err := ReopenIssue("nonexistent-xyz")
	require.Error(t, err, "expected error for nonexistent issue")
}

// TestDeleteIssues_InvalidIssue tests error handling for invalid issue IDs.
func TestDeleteIssues_InvalidIssue(t *testing.T) {
	// Check if bd command is available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd CLI not available, skipping integration test")
	}

	err := DeleteIssues([]string{"nonexistent-xyz"})
	require.Error(t, err, "expected error for nonexistent issue")
}
