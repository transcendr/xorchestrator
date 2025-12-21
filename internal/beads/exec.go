package beads

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"perles/internal/log"
)

// UpdateStatus changes an issue's status via bd CLI.
func UpdateStatus(issueID string, status Status) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "UpdateStatus completed", "issueID", issueID, "status", status, "duration", time.Since(start))
	}()

	//nolint:gosec // G204: issueID comes from bd database, not user input
	cmd := exec.Command("bd", "update", issueID,
		"--status", string(status), "--json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("bd update status failed: %s", stderr.String())
			log.Error(log.CatBeads, "UpdateStatus failed", "issueID", issueID, "error", err)
			return err
		}
		err = fmt.Errorf("bd update status failed: %w", err)
		log.Error(log.CatBeads, "UpdateStatus failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// UpdatePriority changes an issue's priority via bd CLI.
func UpdatePriority(issueID string, priority Priority) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "UpdatePriority completed", "issueID", issueID, "priority", priority, "duration", time.Since(start))
	}()

	//nolint:gosec // G204: issueID comes from bd database, not user input
	cmd := exec.Command("bd", "update", issueID,
		"--priority", fmt.Sprintf("%d", priority), "--json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("bd update priority failed: %s", stderr.String())
			log.Error(log.CatBeads, "UpdatePriority failed", "issueID", issueID, "error", err)
			return err
		}
		err = fmt.Errorf("bd update priority failed: %w", err)
		log.Error(log.CatBeads, "UpdatePriority failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// UpdateType changes an issue's type via bd CLI.
func UpdateType(issueID string, issueType IssueType) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "UpdateType completed", "issueID", issueID, "type", issueType, "duration", time.Since(start))
	}()

	//nolint:gosec // G204: issueID comes from bd database, not user input
	cmd := exec.Command("bd", "update", issueID,
		"--type", string(issueType), "--json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("bd update type failed: %s", stderr.String())
			log.Error(log.CatBeads, "UpdateType failed", "issueID", issueID, "error", err)
			return err
		}
		err = fmt.Errorf("bd update type failed: %w", err)
		log.Error(log.CatBeads, "UpdateType failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// CloseIssue marks an issue as closed with a reason via bd CLI.
func CloseIssue(issueID, reason string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "CloseIssue completed", "issueID", issueID, "duration", time.Since(start))
	}()

	cmd := exec.Command("bd", "close", issueID,
		"--reason", reason, "--json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("bd close failed: %s", stderr.String())
			log.Error(log.CatBeads, "CloseIssue failed", "issueID", issueID, "error", err)
			return err
		}
		err = fmt.Errorf("bd close failed: %w", err)
		log.Error(log.CatBeads, "CloseIssue failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// ReopenIssue reopens a closed issue via bd CLI.
func ReopenIssue(issueID string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "ReopenIssue completed", "issueID", issueID, "duration", time.Since(start))
	}()

	//nolint:gosec // G204: issueID comes from bd database, not user input
	cmd := exec.Command("bd", "update", issueID,
		"--status", string(StatusOpen), "--json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("bd reopen failed: %s", stderr.String())
			log.Error(log.CatBeads, "ReopenIssue failed", "issueID", issueID, "error", err)
			return err
		}
		err = fmt.Errorf("bd reopen failed: %w", err)
		log.Error(log.CatBeads, "ReopenIssue failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// DeleteIssues deletes one or more issues in a single bd CLI call.
func DeleteIssues(issueIDs []string) error {
	if len(issueIDs) == 0 {
		return nil
	}

	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "DeleteIssues completed",
			"count", len(issueIDs),
			"duration", time.Since(start))
	}()

	args := append([]string{"delete"}, issueIDs...)
	args = append(args, "--force", "--json")

	//nolint:gosec // G204: issueIDs come from bd database, not user input
	cmd := exec.Command("bd", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("bd delete failed: %s", stderr.String())
			log.Error(log.CatBeads, "DeleteIssues failed", "count", len(issueIDs), "error", err)
			return err
		}
		err = fmt.Errorf("bd delete failed: %w", err)
		log.Error(log.CatBeads, "DeleteIssues failed", "count", len(issueIDs), "error", err)
		return err
	}
	return nil
}

// SetLabels replaces all labels on an issue via bd CLI.
// Pass an empty slice to remove all labels.
func SetLabels(issueID string, labels []string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "SetLabels completed", "issueID", issueID, "labels", strings.Join(labels, ","), "duration", time.Since(start))
	}()

	//nolint:gosec // G204: issueID and labels come from bd database, not user input
	cmd := exec.Command("bd", "update", issueID,
		"--set-labels", strings.Join(labels, ","), "--json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("bd update set-labels failed: %s", stderr.String())
			log.Error(log.CatBeads, "SetLabels failed", "issueID", issueID, "error", err)
			return err
		}
		err = fmt.Errorf("bd update set-labels failed: %w", err)
		log.Error(log.CatBeads, "SetLabels failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}
