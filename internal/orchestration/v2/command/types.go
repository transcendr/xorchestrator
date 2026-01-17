// Package command provides concrete command types for the v2 orchestration architecture.
// Each command embeds BaseCommand and adds domain-specific fields with validation.
package command

import (
	"fmt"

	"github.com/zjrosen/xorchestrator/internal/orchestration/events"
	"github.com/zjrosen/xorchestrator/internal/orchestration/validation"
)

// Verdict represents an approval or denial decision from a reviewer.
type Verdict string

const (
	// VerdictApproved indicates the reviewer approved the implementation.
	VerdictApproved Verdict = "APPROVED"
	// VerdictDenied indicates the reviewer denied the implementation.
	VerdictDenied Verdict = "DENIED"
)

// IsValid returns true if the verdict is a valid value.
func (v Verdict) IsValid() bool {
	return v == VerdictApproved || v == VerdictDenied
}

// String returns the string representation of the Verdict.
func (v Verdict) String() string {
	return string(v)
}

// ReviewType represents the complexity level for code reviews.
type ReviewType string

const (
	// ReviewTypeSimple indicates a streamlined review without sub-agents.
	ReviewTypeSimple ReviewType = "simple"
	// ReviewTypeComplex indicates a comprehensive review with parallel sub-agents.
	ReviewTypeComplex ReviewType = "complex"
)

// IsValid returns true if the review type is a valid value.
func (r ReviewType) IsValid() bool {
	return r == ReviewTypeSimple || r == ReviewTypeComplex
}

// ===========================================================================
// Task Assignment Commands
// ===========================================================================

// AssignTaskCommand assigns a bd task to an idle worker.
type AssignTaskCommand struct {
	*BaseCommand
	WorkerID string // Required: ID of the worker to assign the task to
	TaskID   string // Required: BD task ID to assign
	Summary  string // Optional: context or instructions for the worker
}

// NewAssignTaskCommand creates a new AssignTaskCommand.
func NewAssignTaskCommand(source CommandSource, workerID, taskID, summary string) *AssignTaskCommand {
	base := NewBaseCommand(CmdAssignTask, source)
	return &AssignTaskCommand{
		BaseCommand: &base,
		WorkerID:    workerID,
		TaskID:      taskID,
		Summary:     summary,
	}
}

// Validate checks that WorkerID and TaskID are provided, and TaskID format is valid.
func (c *AssignTaskCommand) Validate() error {
	if c.WorkerID == "" {
		return fmt.Errorf("worker_id is required")
	}
	if c.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}
	if !validation.IsValidTaskID(c.TaskID) {
		return fmt.Errorf("invalid task_id format: %s", c.TaskID)
	}
	return nil
}

// AssignReviewCommand assigns a reviewer to an implemented task.
type AssignReviewCommand struct {
	*BaseCommand
	ReviewerID    string     // Required: ID of the worker who will review
	TaskID        string     // Required: BD task ID being reviewed
	ImplementerID string     // Required: ID of the worker who implemented the task
	ReviewType    ReviewType // Optional: "simple" or "complex", defaults to "complex"
}

// NewAssignReviewCommand creates a new AssignReviewCommand.
func NewAssignReviewCommand(source CommandSource, reviewerID, taskID, implementerID string, reviewType ReviewType) *AssignReviewCommand {
	base := NewBaseCommand(CmdAssignReview, source)
	return &AssignReviewCommand{
		BaseCommand:   &base,
		ReviewerID:    reviewerID,
		TaskID:        taskID,
		ImplementerID: implementerID,
		ReviewType:    reviewType,
	}
}

// Validate checks that ReviewerID, TaskID, and ImplementerID are provided.
func (c *AssignReviewCommand) Validate() error {
	if c.ReviewerID == "" {
		return fmt.Errorf("reviewer_id is required")
	}
	if c.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}
	if c.ImplementerID == "" {
		return fmt.Errorf("implementer_id is required")
	}
	return nil
}

// ApproveCommitCommand approves implementation and triggers commit phase.
type ApproveCommitCommand struct {
	*BaseCommand
	ImplementerID string // Required: ID of the worker who implemented the task
	TaskID        string // Required: BD task ID to commit
}

// NewApproveCommitCommand creates a new ApproveCommitCommand.
func NewApproveCommitCommand(source CommandSource, implementerID, taskID string) *ApproveCommitCommand {
	base := NewBaseCommand(CmdApproveCommit, source)
	return &ApproveCommitCommand{
		BaseCommand:   &base,
		ImplementerID: implementerID,
		TaskID:        taskID,
	}
}

// Validate checks that ImplementerID and TaskID are provided.
func (c *ApproveCommitCommand) Validate() error {
	if c.ImplementerID == "" {
		return fmt.Errorf("implementer_id is required")
	}
	if c.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}
	return nil
}

// AssignReviewFeedbackCommand sends review feedback to an implementer after denial.
// This transitions the implementer to the AddressingFeedback phase.
type AssignReviewFeedbackCommand struct {
	*BaseCommand
	ImplementerID string // Required: ID of the worker who implemented the task
	TaskID        string // Required: BD task ID that was denied
	Feedback      string // Required: reviewer feedback explaining what needs to change
}

// NewAssignReviewFeedbackCommand creates a new AssignReviewFeedbackCommand.
func NewAssignReviewFeedbackCommand(source CommandSource, implementerID, taskID, feedback string) *AssignReviewFeedbackCommand {
	base := NewBaseCommand(CmdAssignReviewFeedback, source)
	return &AssignReviewFeedbackCommand{
		BaseCommand:   &base,
		ImplementerID: implementerID,
		TaskID:        taskID,
		Feedback:      feedback,
	}
}

// Validate checks that ImplementerID, TaskID, and Feedback are provided.
func (c *AssignReviewFeedbackCommand) Validate() error {
	if c.ImplementerID == "" {
		return fmt.Errorf("implementer_id is required")
	}
	if c.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}
	if c.Feedback == "" {
		return fmt.Errorf("feedback is required")
	}
	return nil
}

// ===========================================================================
// Message Routing Commands
// ===========================================================================

// BroadcastCommand broadcasts a message to all workers.
type BroadcastCommand struct {
	*BaseCommand
	Content        string   // Required: message content to broadcast
	ExcludeWorkers []string // Optional: worker IDs to exclude from the broadcast
}

// NewBroadcastCommand creates a new BroadcastCommand.
func NewBroadcastCommand(source CommandSource, content string, excludeWorkers []string) *BroadcastCommand {
	base := NewBaseCommand(CmdBroadcast, source)
	return &BroadcastCommand{
		BaseCommand:    &base,
		Content:        content,
		ExcludeWorkers: excludeWorkers,
	}
}

// Validate checks that Content is provided.
func (c *BroadcastCommand) Validate() error {
	if c.Content == "" {
		return fmt.Errorf("content is required")
	}
	return nil
}

// ===========================================================================
// State Transition Commands
// ===========================================================================

// ReportCompleteCommand signals that a worker's implementation is done.
type ReportCompleteCommand struct {
	*BaseCommand
	WorkerID string // Required: ID of the worker reporting completion
	Summary  string // Optional: summary of what was implemented
}

// NewReportCompleteCommand creates a new ReportCompleteCommand.
func NewReportCompleteCommand(source CommandSource, workerID, summary string) *ReportCompleteCommand {
	base := NewBaseCommand(CmdReportComplete, source)
	return &ReportCompleteCommand{
		BaseCommand: &base,
		WorkerID:    workerID,
		Summary:     summary,
	}
}

// Validate checks that WorkerID is provided.
func (c *ReportCompleteCommand) Validate() error {
	if c.WorkerID == "" {
		return fmt.Errorf("worker_id is required")
	}
	return nil
}

// ReportVerdictCommand signals a reviewer's approval or denial verdict.
type ReportVerdictCommand struct {
	*BaseCommand
	WorkerID string  // Required: ID of the reviewer reporting the verdict
	Verdict  Verdict // Required: APPROVED or DENIED
	Comments string  // Optional: review comments
}

// NewReportVerdictCommand creates a new ReportVerdictCommand.
func NewReportVerdictCommand(source CommandSource, workerID string, verdict Verdict, comments string) *ReportVerdictCommand {
	base := NewBaseCommand(CmdReportVerdict, source)
	return &ReportVerdictCommand{
		BaseCommand: &base,
		WorkerID:    workerID,
		Verdict:     verdict,
		Comments:    comments,
	}
}

// Validate checks that WorkerID and a valid Verdict are provided.
func (c *ReportVerdictCommand) Validate() error {
	if c.WorkerID == "" {
		return fmt.Errorf("worker_id is required")
	}
	if !c.Verdict.IsValid() {
		return fmt.Errorf("verdict must be APPROVED or DENIED, got: %s", c.Verdict)
	}
	return nil
}

// TransitionPhaseCommand is an internal command for phase changes.
type TransitionPhaseCommand struct {
	*BaseCommand
	WorkerID string              // Required: ID of the worker transitioning phases
	NewPhase events.ProcessPhase // Required: the new phase for the worker
}

// NewTransitionPhaseCommand creates a new TransitionPhaseCommand.
func NewTransitionPhaseCommand(source CommandSource, workerID string, newPhase events.ProcessPhase) *TransitionPhaseCommand {
	base := NewBaseCommand(CmdTransitionPhase, source)
	return &TransitionPhaseCommand{
		BaseCommand: &base,
		WorkerID:    workerID,
		NewPhase:    newPhase,
	}
}

// Validate checks that WorkerID and a valid NewPhase are provided.
func (c *TransitionPhaseCommand) Validate() error {
	if c.WorkerID == "" {
		return fmt.Errorf("worker_id is required")
	}
	if !isValidProcessPhase(c.NewPhase) {
		return fmt.Errorf("invalid worker phase: %s", c.NewPhase)
	}
	return nil
}

// isValidProcessPhase checks if a phase is a valid ProcessPhase value.
func isValidProcessPhase(phase events.ProcessPhase) bool {
	switch phase {
	case events.ProcessPhaseIdle,
		events.ProcessPhaseImplementing,
		events.ProcessPhaseAwaitingReview,
		events.ProcessPhaseReviewing,
		events.ProcessPhaseAddressingFeedback,
		events.ProcessPhaseCommitting:
		return true
	default:
		return false
	}
}

// ===========================================================================
// BD Task Status Commands
// ===========================================================================

// MarkTaskCompleteCommand marks a BD task as completed.
type MarkTaskCompleteCommand struct {
	*BaseCommand
	TaskID string // Required: BD task ID to mark as complete
}

// NewMarkTaskCompleteCommand creates a new MarkTaskCompleteCommand.
func NewMarkTaskCompleteCommand(source CommandSource, taskID string) *MarkTaskCompleteCommand {
	base := NewBaseCommand(CmdMarkTaskComplete, source)
	return &MarkTaskCompleteCommand{
		BaseCommand: &base,
		TaskID:      taskID,
	}
}

// Validate checks that TaskID is provided and has a valid format.
func (c *MarkTaskCompleteCommand) Validate() error {
	if c.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}
	if !validation.IsValidTaskID(c.TaskID) {
		return fmt.Errorf("invalid task_id format: %s", c.TaskID)
	}
	return nil
}

// MarkTaskFailedCommand marks a BD task as failed with a reason.
type MarkTaskFailedCommand struct {
	*BaseCommand
	TaskID string // Required: BD task ID to mark as failed
	Reason string // Required: reason for failure
}

// NewMarkTaskFailedCommand creates a new MarkTaskFailedCommand.
func NewMarkTaskFailedCommand(source CommandSource, taskID, reason string) *MarkTaskFailedCommand {
	base := NewBaseCommand(CmdMarkTaskFailed, source)
	return &MarkTaskFailedCommand{
		BaseCommand: &base,
		TaskID:      taskID,
		Reason:      reason,
	}
}

// Validate checks that TaskID and Reason are provided and TaskID has a valid format.
func (c *MarkTaskFailedCommand) Validate() error {
	if c.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}
	if !validation.IsValidTaskID(c.TaskID) {
		return fmt.Errorf("invalid task_id format: %s", c.TaskID)
	}
	if c.Reason == "" {
		return fmt.Errorf("reason is required")
	}
	return nil
}

// ===========================================================================
// Pause/Resume Commands
// ===========================================================================

// PauseProcessCommand pauses a coordinator or process.
// Transitions from Ready/Working → Paused.
type PauseProcessCommand struct {
	*BaseCommand
	ProcessID string // Required: ID of the process to pause
	Reason    string // Optional: reason for pausing
}

// NewPauseProcessCommand creates a new PauseProcessCommand.
func NewPauseProcessCommand(source CommandSource, processID, reason string) *PauseProcessCommand {
	base := NewBaseCommand(CmdPauseProcess, source)
	return &PauseProcessCommand{
		BaseCommand: &base,
		ProcessID:   processID,
		Reason:      reason,
	}
}

// Validate checks that ProcessID is provided.
func (c *PauseProcessCommand) Validate() error {
	if c.ProcessID == "" {
		return fmt.Errorf("process_id is required")
	}
	return nil
}

// ResumeProcessCommand resumes a paused coordinator or process.
// Transitions from Paused → Ready. Triggers queue drain if messages pending.
type ResumeProcessCommand struct {
	*BaseCommand
	ProcessID string // Required: ID of the process to resume
}

// NewResumeProcessCommand creates a new ResumeProcessCommand.
func NewResumeProcessCommand(source CommandSource, processID string) *ResumeProcessCommand {
	base := NewBaseCommand(CmdResumeProcess, source)
	return &ResumeProcessCommand{
		BaseCommand: &base,
		ProcessID:   processID,
	}
}

// Validate checks that ProcessID is provided.
func (c *ResumeProcessCommand) Validate() error {
	if c.ProcessID == "" {
		return fmt.Errorf("process_id is required")
	}
	return nil
}

// ===========================================================================
// Process Control Commands
// ===========================================================================

// StopProcessCommand requests termination of a process (coordinator or worker).
// Supports both graceful (Cancel with timeout) and forceful (SIGKILL) modes.
type StopProcessCommand struct {
	*BaseCommand
	ProcessID string // Required: ID of the process to stop (e.g., "coordinator", "worker-1")
	Force     bool   // false = graceful termination, true = immediate SIGKILL
	Reason    string // Optional: reason for stopping (for logging/audit)
}

// NewStopProcessCommand creates a new StopProcessCommand.
func NewStopProcessCommand(source CommandSource, processID string, force bool, reason string) *StopProcessCommand {
	base := NewBaseCommand(CmdStopProcess, source)
	return &StopProcessCommand{
		BaseCommand: &base,
		ProcessID:   processID,
		Force:       force,
		Reason:      reason,
	}
}

// Validate checks that ProcessID is provided.
func (c *StopProcessCommand) Validate() error {
	if c.ProcessID == "" {
		return fmt.Errorf("process_id is required")
	}
	return nil
}

// ===========================================================================
// Aggregation Commands
// ===========================================================================

// GenerateAccountabilitySummaryCommand sends an aggregation task to an existing worker.
// The worker reads individual worker summaries and produces a unified session summary.
type GenerateAccountabilitySummaryCommand struct {
	*BaseCommand
	WorkerID   string // Required: ID of the worker to assign the aggregation task
	SessionDir string // Required: path to the session directory containing worker summaries
}

// NewGenerateAccountabilitySummaryCommand creates a new GenerateAccountabilitySummaryCommand.
func NewGenerateAccountabilitySummaryCommand(source CommandSource, workerID, sessionDir string) *GenerateAccountabilitySummaryCommand {
	base := NewBaseCommand(CmdGenerateAccountabilitySummary, source)
	return &GenerateAccountabilitySummaryCommand{
		BaseCommand: &base,
		WorkerID:    workerID,
		SessionDir:  sessionDir,
	}
}

// Validate checks that WorkerID and SessionDir are provided.
func (c *GenerateAccountabilitySummaryCommand) Validate() error {
	if c.WorkerID == "" {
		return fmt.Errorf("worker_id is required")
	}
	if c.SessionDir == "" {
		return fmt.Errorf("session_dir is required")
	}
	return nil
}
