// Package validation provides shared validation functions for the orchestration layer.
package validation

import "regexp"

// taskIDPattern validates bd task IDs to prevent command injection.
// Valid formats: "prefix-xxxx" or "prefix-xxxx.N" (for subtasks)
// Examples: "xorchestrator-abc1", "xorchestrator-abc1.2", "ms-e52", "pe-xorchestrator-xyz9.10"
var taskIDPattern = regexp.MustCompile(`^[a-z0-9]{2,}(-[a-z0-9]{2,})+(\.[0-9]+)*$`)

// IsValidTaskID validates that a task ID matches the expected format.
// Valid formats: "prefix-xxxx" or "prefix-xxxx.N" (for subtasks)
func IsValidTaskID(taskID string) bool {
	return taskIDPattern.MatchString(taskID)
}
