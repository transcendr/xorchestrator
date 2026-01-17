---
name: "Context Builder"
description: "Progressive context building from vague intent to actionable plans through iterative multi-agent collaboration integrated with Claudex workstreams"
category: "Planning"
workers: 3
target_mode: "orchestration"
---

# Context Builder Workflow

## Overview

A multi-agent iterative workflow that builds context progressively from minimal user input through structured collaboration. Breaks through planning friction by using multiple specialized agents to research, synthesize, and plan collaboratively.

**Claudex Integration:**
- Uses `.ai/tasks/<workstream>/planning.md` for planning documents
- Integrates with Claudex workstream system
- Supports ephemeral planning at `.ai/tasks/_scratch/[timestamp]-[slug]/planning.md`

**Flow:**
```
Iteration Loop (repeat until HIGH confidence):
  Worker 1 (Researcher)    → Gather codebase context
  Worker 2 (Synthesizer)   → Transform findings into options
  Worker 3 (Planner)       → Update planning doc, assess confidence
  Coordinator              → Ask user questions, integrate answers
```

**Output:**
- `.ai/tasks/<workstream>/planning.md` - Living planning document with research findings and confidence tracking
- Actionable plan with HIGH confidence or implementation tasks when ready

---

## Roles

### Worker 1: Research Specialist
**Goal:** Gather context from codebase to answer open questions.

**Responsibilities:**
- Explore codebase using Grep/Glob/Read tools
- Find existing patterns, files, and conventions
- Document findings with specific file paths and line numbers
- Identify constraints and dependencies
- Build mental model of relevant architecture

**Output Format:**
```markdown
## Research Findings - Iteration {N}

### Question: {research question}

**Findings:**
- `path/to/file.ts:123` - {pattern or finding}
- `path/to/other.ts:456` - {pattern or finding}

**Patterns Identified:**
- {Pattern name}: {description with examples}

**Constraints:**
- {Constraint or dependency}
```

---

### Worker 2: Synthesis Specialist
**Goal:** Transform research findings into actionable insights and generate options.

**Responsibilities:**
- Read research findings from Worker 1
- Identify key insights and implications
- Generate structured options for user decisions
- Frame options with clear trade-offs
- Prepare option format for coordinator to present

**Output Format:**
```markdown
## Synthesis - Iteration {N}

### Key Insights:
1. {Insight from research}
2. {Insight from research}

### Options Generated:

**Question:** {question for user}
**Header:** {short label for question}

**Options:**
1. **{Option 1}** - {description}
   - Trade-offs: {pros and cons}
2. **{Option 2}** - {description}
   - Trade-offs: {pros and cons}
```

---

### Worker 3: Planning Specialist
**Goal:** Maintain planning document and track confidence levels.

**Responsibilities:**
- Create and update planning document at `.ai/tasks/<workstream>/planning.md`
- Track confidence levels (LOW → MEDIUM → HIGH)
- Integrate user answers into planning doc
- Determine when enough context has been gathered
- Signal readiness for final plan creation

**Planning Document Location:**
- **With workstream**: `.ai/tasks/<workstream>/planning.md`
- **Ephemeral**: `.ai/tasks/_scratch/[timestamp]-[slug]/planning.md`

**Output Format:**
```markdown
## Planning Document Update - Iteration {N}

### Current Confidence: {LOW|MEDIUM|HIGH}

### What's Clear:
- {Clear decision or constraint}
- {Clear decision or constraint}

### What's Still Unclear:
- {Open question}
- {Open question}

### Assessment:
{1-2 paragraphs explaining confidence level and what's needed to progress}
```

**Confidence Criteria:**
- **LOW**: Many unknowns, need significant exploration
- **MEDIUM**: Some clarity, key decisions still needed
- **HIGH**: Clear path forward, ready for task breakdown

---

## Workflow Phases

### Phase 0: Initialization (Coordinator)

**Coordinator gets user's vague intent and workstream context:**

#### Step 1: Workstream Setup
```
Ask user:
Question: Do you want to attach this planning to a workstream?

Options:
A) Use active workstream – Attach to current work – Best when: planning next phase of ongoing work
B) Create new workstream – Start fresh – Best when: new initiative, separate from current work
C) No workstream (ephemeral) – Plan without formal tracking – Best when: exploratory, not ready to commit
```

**Workstream Resolution:**
- **Option A**: Detect active workstream from `.claudex/.active-session`, use `.ai/tasks/<workstream>/planning.md`
- **Option B**: User provides workstream name, create via workstream system, use `.ai/tasks/<workstream>/planning.md`
- **Option C**: Generate timestamp slug, use `.ai/tasks/_scratch/[timestamp]-[slug]/planning.md`

#### Step 2: Capture Initial Input
```
Ask user:
What do you want to accomplish? (Can be as vague as you'd like - I'll help build it up)
```

**Coordinator assigns to Worker 3 (Planner) with prompt:**
```
You are the Planning Specialist. A new context-building session has started.

**Workstream**: {workstream-name or "ephemeral"}
**Planning Document**: .ai/tasks/{path}/planning.md
**User Intent**: {user's request}

**Your Task:**
1. Create the initial planning document from template
2. Initialize with user's vague intent in Goals section
3. Set confidence to LOW
4. Identify 2-3 initial research questions to explore

**Planning Document Template:**
```markdown
# Planning: {Feature/Change Name}

## Goals
- {User's initial vague statement}
  - Confidence: LOW
  - Signals needed: {what would increase confidence}

## Current Understanding
{Empty initially, will be populated by research}

## Open Questions
1. {Initial research question}
2. {Initial research question}
3. {Initial research question}

## Iteration Log
### Iteration 0 - Initialization
- Created planning document
- Set initial goals based on user input
- Identified {N} research questions

## Micro-Summary
Planning started. Confidence: LOW. Next: Research initial questions.
```

Begin now.
```

---

### Phase 1: Research Cycle (Worker 1 → Worker 2 → Worker 3 → Coordinator Loop)

**Iteration Pattern (repeat until HIGH confidence):**

#### Step 1.1: Research (Worker 1)
**Coordinator assigns Worker 1 with prompt:**
```
You are the Research Specialist for iteration {N}.

**Planning Document**: .ai/tasks/{path}/planning.md

**Research Questions:**
{questions from Worker 3's Open Questions section}

**Your Task:**
1. Read the planning document to understand current state
2. Use Grep/Glob/Read to explore the codebase
3. Answer each research question with specific findings
4. Cite file paths and line numbers
5. Identify patterns and constraints

**Post your findings when complete.**
```

#### Step 1.2: Synthesis (Worker 2)
**Coordinator assigns Worker 2 with prompt:**
```
You are the Synthesis Specialist for iteration {N}.

**Planning Document**: .ai/tasks/{path}/planning.md
**Research Findings**: {Worker 1's latest findings}

**Your Task:**
1. Read the research findings
2. Extract key insights and implications
3. Generate 2-4 structured options for user decisions
4. Frame each option with clear trade-offs using format:

**Question**: {question text}
**Header**: {short label, max 12 chars}
**Options**:
1. **{Option label}** - {description} - Best when: {context}
   - Trade-offs: {pros and cons}
2. **{Option label}** - {description} - Best when: {context}
   - Trade-offs: {pros and cons}

**Post your synthesis when complete.**
```

#### Step 1.3: Planning Update (Worker 3)
**Coordinator assigns Worker 3 with prompt:**
```
You are the Planning Specialist for iteration {N}.

**Planning Document**: .ai/tasks/{path}/planning.md
**Research**: {Worker 1's findings}
**Synthesis**: {Worker 2's insights and options}

**Your Task:**
1. Read the current planning document
2. Update Current Understanding with research findings
3. Append iteration log entry (2-3 sentences + net change)
4. Update confidence level based on clarity gained
5. Update "What's Clear" and "What's Still Unclear"
6. Determine if ready for final plan or need another iteration

**Confidence Assessment:**
- LOW: Need another research cycle (many unknowns)
- MEDIUM: Getting closer (some key decisions made, 1-2 more iterations likely)
- HIGH: Ready to create final actionable plan (zero blocking questions)

**Post your assessment when complete.**
```

#### Step 1.4: User Questions (Coordinator)
**Coordinator reads Worker 2's synthesis and Worker 3's assessment:**

If confidence is LOW or MEDIUM:
1. Extract questions from Worker 2's synthesis
2. Use `ask_user_question` MCP tool to present options
3. Wait for user responses
4. Send answers to Worker 3 to integrate into planning document
5. Start next iteration (go to Step 1.1)

If confidence is HIGH:
- **Do NOT auto-proceed to finalization**
- Ask user via `ask_user_question`:
  ```
  Question: I've built up substantial context (all goals at HIGH confidence). Ready to finalize, or continue exploring?

  Options:
  A) Finalize now - Create final plan - Best when: ready to move to implementation
  B) Continue exploring - Build more context - Best when: want deeper understanding
  ```
- If user chooses A: Proceed to Phase 2
- If user chooses B: Continue iteration (go to Step 1.1)

---

### Phase 2: Final Plan Creation (Worker 3)

**Coordinator assigns Worker 3 with prompt:**
```
You are the Planning Specialist. The context-building phase is complete.

**Planning Document**: .ai/tasks/{path}/planning.md
**Confidence**: HIGH

**Your Task:**
1. Read the complete planning document with all iterations
2. Create a final actionable plan section with:
   - Clear problem statement
   - Research summary (key findings with file paths)
   - Decisions made (from user answers across all iterations)
   - Implementation approach
   - Files to create/modify
   - Testing strategy
   - Next steps

**Append this to the planning document:**

```markdown
## Final Plan

### Problem Statement
{2-3 paragraphs summarizing what needs to be built and why}

### Research Summary
**Key Findings:**
- `path/to/file:line` - {finding from iteration X}
- `path/to/file:line` - {finding from iteration Y}

**Patterns Identified:**
- {Pattern name}: {description with examples}

### Decisions Made
1. **{Decision topic}** (Iteration {N}): Chose {option} - {rationale}
2. **{Decision topic}** (Iteration {N}): Chose {option} - {rationale}

### Implementation Approach
{2-3 paragraphs explaining the strategy based on research and decisions}

### Files to Create/Modify
- `path/to/file` - {what changes and why}
- `path/to/file` - {what changes and why}

### Testing Strategy
{How this will be tested - unit tests, integration tests, manual testing}

### Next Steps
1. {Immediate action with file reference}
2. {Follow-up action with file reference}
```

**Signal completion when done.**
```

---

### Phase 3: Task Breakdown (Optional - Worker 3)

**Ask user:**
```
Question: Do you want immediate task breakdown?

Options:
A) Yes, create tasks now - Generate beads tasks - Best when: ready to start implementation
B) No, keep as-is - Just planning document - Best when: need approval or further discussion first
```

**If user chooses A:**

**Coordinator assigns Worker 3 with prompt:**
```
You are the Planning Specialist. The user wants immediate task breakdown.

**Planning Document**: .ai/tasks/{path}/planning.md
**Workstream**: {workstream-name}

**Your Task:**
1. Read the final plan section
2. Create beads epic and tasks for this workstream

**Beads Task Creation Pattern:**

**Step 1: Create Epic**
```bash
bd create "{Epic Title from plan}" -t epic --labels="workstream:{workstream}" -d "
## Overview
{Brief summary from final plan}

## Planning Document
See: .ai/tasks/{workstream}/planning.md

## Implementation Approach
{From final plan}
" --json
```

**Step 2: Create Tasks with --parent flag**
For each task identified in final plan:
```bash
bd create "{Task Title}" -t task --parent {epic-id} --labels="workstream:{workstream}" -d "
## Goal
{What this task accomplishes from plan}

## Implementation
{Specific steps from plan with file paths}

## Files to Modify/Create
- `path/to/file` - {changes from plan}

## Tests Required
- [ ] {Test case from testing strategy}
- [ ] {Test case from testing strategy}

## Acceptance Criteria
- [ ] {Criterion from plan}
- [ ] All tests pass
" --json
```

**Step 3: Set Task Dependencies**
If tasks have logical sequence:
```bash
bd dep add {task-2-id} {task-1-id}  # task-2 depends on task-1
```

**Verification:**
```bash
bd show {epic-id} --json  # Shows all linked tasks
bd ready --json           # Shows which tasks are unblocked
```

**Signal completion and provide task summary.**
```

---

### Phase 4: Completion (Coordinator)

**Coordinator summarizes to user:**
```
## Context Builder Complete

**Planning Document**: .ai/tasks/{path}/planning.md
**Workstream**: {workstream-name or "ephemeral"}

**Iterations**: {count} cycles
**Confidence**: HIGH
**Decisions Made**: {count}

{If tasks created:}
**Epic Created**: {epic-id} - {epic-title}
**Tasks**: {count} tasks ready for implementation
**Next**: Use `bd ready` to see unblocked tasks or `/cook` workflow to start implementation

{If no tasks:}
**Next Steps**:
1. Review planning document for full context
2. Create tasks manually or use another workflow
3. If this was ephemeral planning, consider creating a workstream for tracking
```

**Signal workflow complete:**
```
signal_workflow_complete(
    status="success",
    summary="Completed context building with {count} iterations. Planning document at .ai/tasks/{path}/planning.md. Confidence: HIGH."
)
```

---

## Coordinator Instructions

### Setup
```
1. Spawn 3 workers (researcher, synthesizer, planner)
2. Get user's vague intent/problem statement
3. Determine workstream context (active, new, or ephemeral)
4. Track worker IDs for each role
5. Initialize planning document via Worker 3
```

### Iteration Loop
```
REPEAT until Worker 3 signals HIGH confidence AND user confirms readiness:
  Step 1: Assign Worker 1 (Researcher) → wait for findings
  Step 2: Assign Worker 2 (Synthesizer) → wait for options
  Step 3: Assign Worker 3 (Planner) → wait for assessment and planning doc update
  Step 4: Present Worker 2's options to user via ask_user_question
  Step 5: Send user answers to Worker 3 to integrate into planning doc
  Step 6: Check Worker 3's confidence level

  IF confidence = HIGH:
    Ask user if ready to finalize or continue exploring
    IF finalize: Break loop, proceed to final plan
    IF continue: Continue to next iteration
  ELSE:
    Continue to next iteration

MAX_ITERATIONS: 10 (safety limit, bias toward continuation)
```

### Final Plan & Task Breakdown
```
Step 1: Assign Worker 3 to create final plan → wait
Step 2: Ask user if they want task breakdown
  IF yes: Assign Worker 3 to create epic and tasks → wait
  IF no: Skip to completion
Step 3: Summarize to user and signal complete
```

---

## Workflow Configuration

### Iteration Control
- **Max Iterations**: 10 (safety limit)
- **Min Iterations**: 2 (ensure thorough exploration)
- **Confidence Threshold**: HIGH (before offering finalization)
- **Bias**: Continue building context over premature conclusion

### Question Guidelines
- **Questions per iteration**: 2-4 max (cognitive load limit)
- **Options per question**: 2-4 (not counting "Other")
- **Use multiSelect**: true when choices not mutually exclusive
- **Format**: What - Trade-offs - Best when

### File Management
- **Planning Document**: `.ai/tasks/<workstream>/planning.md` (workstream mode)
- **Ephemeral**: `.ai/tasks/_scratch/[timestamp]-[slug]/planning.md`
- **Single file**: All iterations append to same doc
- **Sequential writes**: Only one worker writes at a time

---

## Claudex Integration Points

### Workstream System
- Detects active workstream from `.claudex/.active-session`
- Creates new workstreams via Claudex workstream commands
- Uses standard `.ai/tasks/<workstream>/` directory structure
- Integrates with beads task tracking system

### Beads Task Creation
- Uses `--parent` flag for proper epic-task relationships
- Sets `workstream:{name}` labels for isolation
- Creates dependencies via `bd dep add`
- Verifies dependency chain with `bd ready`

### Planning Document Location
- **Not** in arbitrary `docs/planning/` directory
- **Always** in `.ai/tasks/<workstream>/planning.md` (or `_scratch/` for ephemeral)
- Follows Claudex conventions for task documentation

---

## When to Use This Workflow

**Good for:**
- Vague or ambiguous requirements
- User unsure of approach
- Need to explore codebase before committing to design
- Complex features requiring progressive clarification
- Planning friction preventing start
- Building context from minimal input

**Use quick_plan.md instead for:**
- Clear scope and requirements
- User knows what they want
- Medium complexity (1-5 tasks)
- Want tasks ready immediately without exploration

---

## Success Metrics

- ✅ Started from minimal user input (< 50 words acceptable)
- ✅ Each iteration reduced ambiguity
- ✅ Confidence level progressed LOW → MEDIUM → HIGH
- ✅ Planning document has concrete file paths cited
- ✅ User made clear decisions via structured options
- ✅ Final plan is actionable (ready for tasks)
- ✅ Iteration count reasonable (2-10 cycles)
- ✅ Planning document in correct Claudex location (`.ai/tasks/<workstream>/planning.md`)

---

## Example Session

```
[User Request]
User: "I want to do something with clipboard"

[Initialization]
Coordinator: Asks about workstream → User chooses "Use active workstream: ux/clipboard-feature"
Coordinator: Asks for initial intent → User: "I want to do something with clipboard"
Worker 3 (Planner): Creates .ai/tasks/ux/clipboard-feature/planning.md with vague initial goal

[Iteration 1: LOW Confidence]
Worker 1 (Research): Finds existing clipboard usage, cites keyboard/main.go:89
Worker 2 (Synthesis): Options: "Copy issue ID" vs "Copy full issue details" vs "General clipboard API"
Worker 3 (Planner): Updates planning.md - Confidence LOW, need to clarify what to copy
Coordinator: Asks user → User chooses "Copy issue ID"

[Iteration 2: MEDIUM Confidence]
Worker 1 (Research): Explores keybinding patterns, cites keyboard package
Worker 2 (Synthesis): Options: Keybinding "y" vs "ctrl+c" vs "cmd+c"
Worker 3 (Planner): Updates planning.md - Confidence MEDIUM, need keybinding decision
Coordinator: Asks user → User chooses "y"

[Iteration 3: HIGH Confidence]
Worker 1 (Research): Confirms no conflicts with "y", finds clipboard libs
Worker 2 (Synthesis): Visual feedback options: toast vs status bar vs none
Worker 3 (Planner): Updates planning.md - Confidence HIGH after this decision
Coordinator: Asks user → User chooses "toast"
Coordinator: Asks user "Ready to finalize?" → User: "Yes, finalize now"

[Final Plan]
Worker 3 (Planner): Creates final plan in .ai/tasks/ux/clipboard-feature/planning.md
  - Problem: Add keyboard shortcut to copy issue ID to clipboard
  - Decisions: Keybinding "y", visual feedback "toast"
  - Files: keyboard/main.go, ui/feedback.go
  - Testing: Unit tests for keybinding, integration test for clipboard

[Task Breakdown]
Coordinator: Asks user "Create tasks?" → User: "Yes"
Worker 3 (Planner): Creates epic xorchestrator-xyz with 3 tasks
  - Task 1: Add clipboard package
  - Task 2: Add "y" keybinding with clipboard integration
  - Task 3: Add toast notification on copy

[Completion]
Coordinator: "Context Builder Complete. Planning: .ai/tasks/ux/clipboard-feature/planning.md"
            "Epic: xorchestrator-xyz with 3 tasks ready. Use /cook to start."
```

---

## Common Pitfalls

1. **Don't proceed without questions** - If synthesis doesn't generate options, prompt Worker 2 again
2. **Don't rush to HIGH confidence** - Ensure genuine clarity, not just iteration fatigue
3. **Don't skip research** - Worker 1 must explore codebase, not guess
4. **Don't overwhelm user** - Max 4 questions per iteration
5. **Don't create vague tasks** - If making tasks, each must be independently executable
6. **Don't use wrong file paths** - Must be `.ai/tasks/<workstream>/planning.md`, not `docs/planning/`
7. **Don't forget workstream labels** - All beads tasks must have `workstream:{name}` label

---

## Integration with Existing Workflows

**After Context Builder completes:**
- Use `cook.md` workflow to implement the created tasks
- Use `research_proposal.md` for more complex architectural validation
- Use `quick_plan.md` if scope expands and needs more detailed breakdown

**Before Context Builder:**
- Best as first workflow when requirements are vague
- Can skip if user already has clear requirements (go straight to quick_plan or cook)
