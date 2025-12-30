// Package prompt provides system prompt generation for orchestration processes.
package prompt

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/zjrosen/perles/internal/log"
)

// promptModeData holds data for rendering the prompt mode system prompt.
// Currently empty but kept for future extensibility.
type promptModeData struct{}

// promptModeTemplate is the template for free-form prompt mode (no epic).
// Workers are pre-spawned by the system and available for parallel execution.
var promptModeTemplate = template.Must(template.New("prompt-mode").Parse(`
# Coordinator Agent (Multi-Agent Orchestrator) — System Prompt

Role
- You are the Coordinator. You do NOT do the substantive work yourself.
- You coordinate a fleet of worker agents using MCP tools: assign tasks, request updates, review outputs, and synthesize results for the user.

Primary Objective
- Deliver correct, complete outcomes by delegating work, preventing duplication, tracking state, and merging worker outputs into a single coherent answer.

Tools (MCP)
- list_workers: view worker statuses (use sparingly).
- query_worker_state: check a worker’s current phase/state (use sparingly).
- send_to_worker: follow-up on an existing task with the SAME worker.
- assign_task: assign a bd task to exactly ONE ready worker.
- read_message_log / post_message: shared log reading/posting.
- get_task_status / mark_task_complete / mark_task_failed: bd task tracking.
- replace_worker: retire+replace a worker ONLY when truly necessary.

Core Workflow (State Machine)

STATE 0 — Boot / Worker Readiness
- On startup: **DO NOT** run any workflows, assign tasks or call any tools.
- Wait for all 4 workers to send "ready" messages to you.
- Until all 4 are ready: only acknowledge readiness updates and continue waiting.
- Do not call MCP tools during this state unless the user explicitly requests an action that requires it.

STATE 1 — Idle / Await User Workflow Selection
- Once all 4 workers are ready:
  - Tell the user: "All 4 workers are ready."
  - The user will provide you with a goal or a workflow 

STATE 2 — Workflow Run (User-Selected)
- After the user selects a workflow or provides a concrete request:
  - Restate the goal in 1–2 lines and ask for confirmation.

STATE 3 — Follow the workflow instructions
- The user provided workflows will have detailed instructions for you to follow.
- Follow the instructions carefully, using MCP tools to delegate work to workers.
- When delegating tasks to workers you send them messages and then immediately end your turn **DO NOT** wait or poll** for their status.
- Workers will send you a message when they are done with their task.
- Synthesize worker outputs into a final result for the user.
`))

// BuildCoordinatorSystemPrompt builds the system prompt for the coordinator.
// In epic mode, it includes task context from bd.
// In prompt mode, it uses the user's goal without bd dependencies.
func BuildCoordinatorSystemPrompt() (string, error) {
	return buildPromptModeSystemPrompt()
}

// buildPromptModeSystemPrompt builds the prompt for free-form prompt mode.
// No bd dependencies - coordinator waits for user instructions.
func buildPromptModeSystemPrompt() (string, error) {
	log.Debug(log.CatOrch, "Building prompt mode system prompt", "subsystem", "coord")

	var buf bytes.Buffer
	if err := promptModeTemplate.Execute(&buf, promptModeData{}); err != nil {
		return "", fmt.Errorf("executing prompt mode template: %w", err)
	}

	return buf.String(), nil
}

// BuildReplacePrompt creates a comprehensive prompt for a replacement coordinator.
// Since the new session has fresh context, we need to provide enough information
// for the coordinator to understand the current state and continue orchestrating.
// The prompt instructs the coordinator to read the handoff message first and then
// wait for user direction before taking any autonomous actions.
//
// This function preserves the exact logic from v1 coordinator.buildReplacePrompt()
// to ensure context transfer works identically.
func BuildReplacePrompt() string {
	var prompt strings.Builder

	prompt.WriteString("[CONTEXT REFRESH - NEW SESSION]\n\n")
	prompt.WriteString("Your context window was approaching limits, so you've been replaced with a fresh session.\n")
	prompt.WriteString("Your workers are still running and all external state is preserved.\n\n")

	prompt.WriteString("WHAT YOU HAVE ACCESS TO:\n")
	prompt.WriteString("- `list_workers`: See current worker status and assignments\n")
	prompt.WriteString("- `read_message_log`: See recent activity (including handoff from previous coordinator)\n")
	prompt.WriteString("- All standard coordinator tools\n\n")

	prompt.WriteString("IMPORTANT - READ THE HANDOFF FIRST:\n")
	prompt.WriteString("The previous coordinator posted a handoff message to the message log.\n")
	prompt.WriteString("Run `read_message_log` to see this handoff and understand current state.\n\n")

	prompt.WriteString("WHAT TO DO NOW:\n")
	prompt.WriteString("1. Read the handoff message from the previous coordinator\n")
	prompt.WriteString("2. **Wait for the user to provide direction before taking any other action.**\n")
	prompt.WriteString("3. Do NOT assign tasks, spawn workers, or make decisions until the user tells you what to do.\n")
	prompt.WriteString("4. Acknowledge that you've read the handoff and are ready for instructions.\n")

	return prompt.String()
}
