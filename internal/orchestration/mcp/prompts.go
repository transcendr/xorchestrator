package mcp

import "fmt"

// WorkerIdlePrompt generates the initial prompt for an idle worker.
// This is sent when spawning a worker that has no task yet.
func WorkerIdlePrompt(workerID string) string {
	return fmt.Sprintf(`You are %s. You are now in IDLE state waiting for task assignment.

Use signal_ready to tell the coordinator you are ready, then STOP.
Do NOT run any other tools. Do NOT check for tasks. Do NOT start any work.

You will receive task assignments from the coordinator in a follow-up message.`, workerID)
}

// WorkerSystemPrompt generates the system prompt for a worker agent.
// This is used as AppendSystemPrompt in Claude config.
func WorkerSystemPrompt(workerID string) string {
	return fmt.Sprintf(`You are %s in orchestration mode.

**WORK CYCLE:**
1. Wait for task assignment from coordinator
2. When assigned a task, work on it thoroughly to completion
3. **MANDATORY**: You must end your turn with a tool call either post_message or report_implementation_complete to notify the coordinator of task completion
4. Return to ready state for next task

**MCP Tools**
- check_messages: Check for new messages addressed to you
- post_message: Send a message to the coordinator (REQUIRED when task complete)
- signal_ready: Signal that you are ready for task assignment (call on startup)
- report_implementation_complete: Signal implementation is done (for state-driven workflow)
- report_review_verdict: Report code review verdict: APPROVED or DENIED (for reviewers)

**HOW TO REPORT COMPLETION:**
- If the coordinator assigned you a bd task use the report_implementation_complete tool to signal completion.
	- Call: report_implementation_complete(summary="[brief summary of what was done]")
- If the coordinator assigned you a non-bd task or did not specify, use post_message to notify completion.
	- Call: post_message(to="COORDINATOR", content="Task completed! [brief summary]")

**CRITICAL RULES:**
- You **MUST ALWAYS** end your turn with either a post_message or report_implementation_complete tool call
- NEVER update any bd task status yourself; coordinator handles that
- NEVER use bd to update tasks
- If stuck, use post_message to ask coordinator for help`, workerID)
}

// TaskAssignmentPrompt generates the prompt sent to a worker when assigning a task.
// The summary parameter is optional and provides additional instructions/context from the coordinator.
func TaskAssignmentPrompt(taskID, title, summary string) string {
	prompt := fmt.Sprintf(`[TASK ASSIGNMENT]

**Goal** Complete the task assigned to you with the highest possible quality effort.

Task ID: %s
Title: %s

**IMPORTANT: Before starting, if your tasks parent epic references a proposal document, you must read the full context of the proposal to understand the work.**
Your instructions are in the task description which you can read using the bd tool with "bd show <task-id>" read the full description this is your work and contains
import acceptance criteria to adhere to.`, taskID, title)

	if summary != "" {
		prompt += fmt.Sprintf(`

Coordinator Instructions:
%s`, summary)
	}

	prompt += `

**CRITICAL*: Work on this task thoroughly. When complete, Before committing your changes, use report_implementation_complete to signal you're done.
Example: report_implementation_complete(summary="Implemented feature X with tests")`

	return prompt
}

// ReviewAssignmentPrompt generates the prompt sent to a reviewer when assigning a code review.
func ReviewAssignmentPrompt(taskID, implementerID, summary string) string {
	return fmt.Sprintf(`[REVIEW ASSIGNMENT]

You are being assigned to **review** the work completed by %s on task **%s**.

## What was implemented:
%s

## Your Review Process

### Step 1: Gather Context
- Read the task description using: bd show %s
- Examine the changes made by the implementer
- Check that acceptance criteria are met

### Step 2: Verify the Implementation
- Check for correctness and completeness
- Look for edge cases and error handling
- Verify tests exist and pass (if applicable)
- Check code style and conventions

### Step 3: Report Your Verdict
Use the report_review_verdict tool to submit your verdict:
- **APPROVED**: The implementation is complete and correct
- **DENIED**: Changes are required (include specific feedback in comments)

Example: report_review_verdict(verdict="APPROVED", comments="Code looks good, tests pass")`, implementerID, taskID, summary, taskID)
}

// ReviewFeedbackPrompt generates the prompt sent to an implementer when their code was denied.
func ReviewFeedbackPrompt(taskID, feedback string) string {
	return fmt.Sprintf(`[REVIEW FEEDBACK]

Your implementation of task **%s** was **DENIED** during code review.

## Required Changes:
%s

Please address the feedback above and make the necessary changes.

When you have addressed all feedback, report via post_message to COORDINATOR that you are ready for re-review.`, taskID, feedback)
}

// CommitApprovalPrompt generates the prompt sent to an implementer when their code is approved.
func CommitApprovalPrompt(taskID, commitMessage string) string {
	prompt := fmt.Sprintf(`[COMMIT APPROVED]

Your implementation of task **%s** has been **APPROVED** by the reviewer.

Please create a git commit for your changes.`, taskID)

	if commitMessage != "" {
		prompt += fmt.Sprintf(`

Suggested commit message:
%s`, commitMessage)
	}

	prompt += fmt.Sprintf(`

## After Committing

Please reflect on your work using post_reflections. This helps capture valuable learnings for future sessions.

**What to include:**
- **summary**: What you actually implemented (not just the task title)
- **insights**: Patterns, techniques, or approaches that worked well
- **mistakes**: Any errors you made and had to correct (helps avoid repeating them)
- **learnings**: Key takeaways - architectural decisions, gotchas, or tips for similar work

Example:
post_reflections(
    task_id="%s",
    summary="Added validation layer with regex patterns for user input sanitization",
    insights="Using table-driven tests made edge case coverage much easier",
    mistakes="Initially forgot to handle empty string case, caught by reviewer",
    learnings="This codebase prefers returning errors over panicking for invalid input"
)

Then report via post_message to COORDINATOR with the commit hash.`, taskID)

	return prompt
}
