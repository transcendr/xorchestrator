package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantFM      frontmatter
		wantErr     bool
		errContains string
	}{
		{
			name: "valid frontmatter with all fields",
			content: `---
name: "Test Workflow"
description: "A test description"
category: "Testing"
---

# Content here
`,
			wantFM: frontmatter{
				Name:        "Test Workflow",
				Description: "A test description",
				Category:    "Testing",
			},
		},
		{
			name: "valid frontmatter with workers field",
			content: `---
name: "Test Workflow"
description: "A test description"
category: "Testing"
workers: 4
---

# Content here
`,
			wantFM: frontmatter{
				Name:        "Test Workflow",
				Description: "A test description",
				Category:    "Testing",
				Workers:     4,
			},
		},
		{
			name: "workers defaults to zero when omitted",
			content: `---
name: "Test Workflow"
description: "No workers field"
---

# Content here
`,
			wantFM: frontmatter{
				Name:        "Test Workflow",
				Description: "No workers field",
				Workers:     0,
			},
		},
		{
			name: "valid frontmatter without category",
			content: `---
name: "Simple Workflow"
description: "No category"
---

# Content
`,
			wantFM: frontmatter{
				Name:        "Simple Workflow",
				Description: "No category",
				Category:    "",
			},
		},
		{
			name: "valid frontmatter without description",
			content: `---
name: "Minimal"
---

# Content
`,
			wantFM: frontmatter{
				Name: "Minimal",
			},
		},
		{
			name:        "missing opening delimiter",
			content:     `name: "Test"`,
			wantErr:     true,
			errContains: "does not start with frontmatter delimiter",
		},
		{
			name: "missing closing delimiter",
			content: `---
name: "Test"
`,
			wantErr:     true,
			errContains: "no closing frontmatter delimiter found",
		},
		{
			name: "missing required name field",
			content: `---
description: "Has description but no name"
---

# Content
`,
			wantErr:     true,
			errContains: "missing required field: name",
		},
		{
			name: "invalid YAML syntax",
			content: `---
name: "Test
description: broken
---

# Content
`,
			wantErr:     true,
			errContains: "parsing YAML",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, err := parseFrontmatter(tt.content)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFM, fm)
		})
	}
}

func TestParseWorkflow(t *testing.T) {
	content := `---
name: "Technical Debate"
description: "Multi-perspective debate format"
category: "Analysis"
---

# Technical Debate Format

This is the content of the debate format.
`
	wf, err := parseWorkflow(content, "debate.md", SourceBuiltIn)
	require.NoError(t, err)

	assert.Equal(t, "debate", wf.ID)
	assert.Equal(t, "Technical Debate", wf.Name)
	assert.Equal(t, "Multi-perspective debate format", wf.Description)
	assert.Equal(t, "Analysis", wf.Category)
	assert.Equal(t, 0, wf.Workers) // Default when not specified
	assert.Equal(t, content, wf.Content)
	assert.Equal(t, SourceBuiltIn, wf.Source)
	assert.Empty(t, wf.FilePath)
}

func TestParseWorkflowWithWorkers(t *testing.T) {
	content := `---
name: "Technical Debate"
description: "Multi-perspective debate format"
category: "Analysis"
workers: 4
---

# Technical Debate Format

This is the content of the debate format.
`
	wf, err := parseWorkflow(content, "debate.md", SourceBuiltIn)
	require.NoError(t, err)

	assert.Equal(t, "debate", wf.ID)
	assert.Equal(t, "Technical Debate", wf.Name)
	assert.Equal(t, "Multi-perspective debate format", wf.Description)
	assert.Equal(t, "Analysis", wf.Category)
	assert.Equal(t, 4, wf.Workers) // Explicit worker count
	assert.Equal(t, content, wf.Content)
	assert.Equal(t, SourceBuiltIn, wf.Source)
	assert.Empty(t, wf.FilePath)
}

func TestParseWorkflowFile(t *testing.T) {
	content := `---
name: "Custom Workflow"
description: "User-defined workflow"
---

# Custom Content
`
	wf, err := ParseWorkflowFile(content, "custom.md", "/home/user/.xorchestrator/workflows/custom.md", SourceUser)
	require.NoError(t, err)

	assert.Equal(t, "custom", wf.ID)
	assert.Equal(t, "Custom Workflow", wf.Name)
	assert.Equal(t, "User-defined workflow", wf.Description)
	assert.Equal(t, SourceUser, wf.Source)
	assert.Equal(t, "/home/user/.xorchestrator/workflows/custom.md", wf.FilePath)
}

func TestLoadBuiltinWorkflows(t *testing.T) {
	workflows, err := LoadBuiltinWorkflows()
	require.NoError(t, err)

	// Should load at least our two built-in templates
	assert.GreaterOrEqual(t, len(workflows), 2, "expected at least 2 built-in workflows")

	// Check that we have both debate and research_proposal
	var foundDebate, foundResearch bool
	for _, wf := range workflows {
		switch wf.ID {
		case "debate":
			foundDebate = true
			assert.Equal(t, "Technical Debate", wf.Name)
			assert.Equal(t, SourceBuiltIn, wf.Source)
			assert.NotEmpty(t, wf.Description)
			assert.NotEmpty(t, wf.Content)
		case "research_proposal":
			foundResearch = true
			assert.Equal(t, "Research Proposal", wf.Name)
			assert.Equal(t, SourceBuiltIn, wf.Source)
			assert.NotEmpty(t, wf.Description)
			assert.NotEmpty(t, wf.Content)
		}
	}

	assert.True(t, foundDebate, "expected to find debate workflow")
	assert.True(t, foundResearch, "expected to find research_proposal workflow")
}

func TestLoadBuiltinWorkflowsHaveCorrectWorkerCounts(t *testing.T) {
	workflows, err := LoadBuiltinWorkflows()
	require.NoError(t, err)

	// Map workflow IDs to expected worker counts
	expectedWorkers := map[string]int{
		"debate":            4,
		"cook":              2,
		"quick_plan":        4,
		"research_proposal": 4,
		"research_to_tasks": 0, // Default when not specified
	}

	for _, wf := range workflows {
		expected, hasExpectation := expectedWorkers[wf.ID]
		if hasExpectation {
			assert.Equal(t, expected, wf.Workers, "workflow %s should have %d workers", wf.ID, expected)
		}
	}
}

func TestLoadUserWorkflowsFromDir(t *testing.T) {
	t.Run("non-existent directory returns empty slice", func(t *testing.T) {
		workflows, err := LoadUserWorkflowsFromDir("/non/existent/path")
		require.NoError(t, err)
		assert.Empty(t, workflows)
	})

	t.Run("empty directory returns empty slice", func(t *testing.T) {
		dir := t.TempDir()
		workflows, err := LoadUserWorkflowsFromDir(dir)
		require.NoError(t, err)
		assert.Empty(t, workflows)
	})

	t.Run("loads valid workflow files", func(t *testing.T) {
		dir := t.TempDir()

		// Create a valid workflow file
		content := `---
name: "Test Workflow"
description: "A test workflow"
category: "Testing"
---

# Test Content
`
		err := os.WriteFile(filepath.Join(dir, "test.md"), []byte(content), 0o644)
		require.NoError(t, err)

		workflows, err := LoadUserWorkflowsFromDir(dir)
		require.NoError(t, err)
		require.Len(t, workflows, 1)

		wf := workflows[0]
		assert.Equal(t, "test", wf.ID)
		assert.Equal(t, "Test Workflow", wf.Name)
		assert.Equal(t, "A test workflow", wf.Description)
		assert.Equal(t, "Testing", wf.Category)
		assert.Equal(t, content, wf.Content)
		assert.Equal(t, SourceUser, wf.Source)
		assert.Equal(t, filepath.Join(dir, "test.md"), wf.FilePath)
	})

	t.Run("loads multiple workflow files", func(t *testing.T) {
		dir := t.TempDir()

		// Create two valid workflow files
		content1 := `---
name: "First Workflow"
description: "First"
---

# First
`
		content2 := `---
name: "Second Workflow"
description: "Second"
---

# Second
`
		err := os.WriteFile(filepath.Join(dir, "first.md"), []byte(content1), 0o644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(dir, "second.md"), []byte(content2), 0o644)
		require.NoError(t, err)

		workflows, err := LoadUserWorkflowsFromDir(dir)
		require.NoError(t, err)
		assert.Len(t, workflows, 2)

		// Verify both were loaded (order may vary)
		ids := make(map[string]bool)
		for _, wf := range workflows {
			ids[wf.ID] = true
			assert.Equal(t, SourceUser, wf.Source)
		}
		assert.True(t, ids["first"])
		assert.True(t, ids["second"])
	})

	t.Run("skips invalid frontmatter", func(t *testing.T) {
		dir := t.TempDir()

		// Create a valid and an invalid workflow file
		validContent := `---
name: "Valid Workflow"
description: "Valid"
---

# Valid
`
		invalidContent := `no frontmatter here`

		err := os.WriteFile(filepath.Join(dir, "valid.md"), []byte(validContent), 0o644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(dir, "invalid.md"), []byte(invalidContent), 0o644)
		require.NoError(t, err)

		workflows, err := LoadUserWorkflowsFromDir(dir)
		require.NoError(t, err)
		require.Len(t, workflows, 1)
		assert.Equal(t, "valid", workflows[0].ID)
	})

	t.Run("skips non-md files", func(t *testing.T) {
		dir := t.TempDir()

		validContent := `---
name: "Valid Workflow"
description: "Valid"
---

# Valid
`
		err := os.WriteFile(filepath.Join(dir, "valid.md"), []byte(validContent), 0o644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a workflow"), 0o644)
		require.NoError(t, err)

		workflows, err := LoadUserWorkflowsFromDir(dir)
		require.NoError(t, err)
		require.Len(t, workflows, 1)
		assert.Equal(t, "valid", workflows[0].ID)
	})

	t.Run("skips subdirectories", func(t *testing.T) {
		dir := t.TempDir()

		validContent := `---
name: "Valid Workflow"
description: "Valid"
---

# Valid
`
		err := os.WriteFile(filepath.Join(dir, "valid.md"), []byte(validContent), 0o644)
		require.NoError(t, err)

		// Create a subdirectory with an md file (should be ignored)
		subdir := filepath.Join(dir, "subdir")
		err = os.MkdirAll(subdir, 0o755)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(subdir, "nested.md"), []byte(validContent), 0o644)
		require.NoError(t, err)

		workflows, err := LoadUserWorkflowsFromDir(dir)
		require.NoError(t, err)
		require.Len(t, workflows, 1)
		assert.Equal(t, "valid", workflows[0].ID)
	})

	t.Run("errors if path is a file", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "notadir")
		err := os.WriteFile(filePath, []byte("content"), 0o644)
		require.NoError(t, err)

		_, err = LoadUserWorkflowsFromDir(filePath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a directory")
	})
}

func TestUserWorkflowDir(t *testing.T) {
	dir := UserWorkflowDir()
	// Should return a path ending in .xorchestrator/workflows
	assert.Contains(t, dir, ".xorchestrator")
	assert.True(t, filepath.IsAbs(dir))
}

// --- Tests for AgentRoleConfig and agent_roles parsing ---

func TestAgentRoleConfig_Fields(t *testing.T) {
	// Verify that AgentRoleConfig has all expected fields
	config := AgentRoleConfig{
		SystemPromptAppend:   "append text",
		SystemPromptOverride: "override text",
		Constraints:          []string{"constraint1", "constraint2"},
	}

	assert.Equal(t, "append text", config.SystemPromptAppend)
	assert.Equal(t, "override text", config.SystemPromptOverride)
	assert.Equal(t, []string{"constraint1", "constraint2"}, config.Constraints)
}

func TestWorkflow_AgentRoles_DefaultsToNil(t *testing.T) {
	content := `---
name: "Test Workflow"
description: "No agent_roles"
---

# Content
`
	wf, err := parseWorkflow(content, "test.md", SourceBuiltIn)
	require.NoError(t, err)
	assert.Nil(t, wf.AgentRoles, "workflows without agent_roles should have nil AgentRoles")
}

func TestLoader_ParsesAgentRoles(t *testing.T) {
	content := `---
name: "Test Workflow"
description: "Has agent_roles"
agent_roles:
  implementer:
    system_prompt_append: "You are an implementer."
    constraints:
      - "Focus on implementation"
      - "Write tests"
  reviewer:
    system_prompt_append: "You are a reviewer."
---

# Content
`
	wf, err := parseWorkflow(content, "test.md", SourceBuiltIn)
	require.NoError(t, err)
	require.NotNil(t, wf.AgentRoles)
	assert.Len(t, wf.AgentRoles, 2)

	// Check implementer config
	implConfig, ok := wf.AgentRoles["implementer"]
	require.True(t, ok, "expected implementer key in AgentRoles")
	assert.Equal(t, "You are an implementer.", implConfig.SystemPromptAppend)
	assert.Equal(t, []string{"Focus on implementation", "Write tests"}, implConfig.Constraints)

	// Check reviewer config
	revConfig, ok := wf.AgentRoles["reviewer"]
	require.True(t, ok, "expected reviewer key in AgentRoles")
	assert.Equal(t, "You are a reviewer.", revConfig.SystemPromptAppend)
}

func TestLoader_ValidatesAgentRoleKeys(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid keys - implementer",
			content: `---
name: "Test"
agent_roles:
  implementer:
    system_prompt_append: "test"
---

# Content
`,
			wantErr: false,
		},
		{
			name: "valid keys - reviewer",
			content: `---
name: "Test"
agent_roles:
  reviewer:
    system_prompt_append: "test"
---

# Content
`,
			wantErr: false,
		},
		{
			name: "valid keys - researcher",
			content: `---
name: "Test"
agent_roles:
  researcher:
    system_prompt_append: "test"
---

# Content
`,
			wantErr: false,
		},
		{
			name: "invalid key - unknown type",
			content: `---
name: "Test"
agent_roles:
  hacker:
    system_prompt_append: "test"
---

# Content
`,
			wantErr:     true,
			errContains: "invalid agent_role key",
		},
		{
			name: "invalid key - shell injection attempt",
			content: `---
name: "Test"
agent_roles:
  "implementer; rm -rf /":
    system_prompt_append: "test"
---

# Content
`,
			wantErr:     true,
			errContains: "invalid agent_role key",
		},
		{
			name: "invalid key - path traversal attempt",
			content: `---
name: "Test"
agent_roles:
  "../../../etc/passwd":
    system_prompt_append: "test"
---

# Content
`,
			wantErr:     true,
			errContains: "invalid agent_role key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseWorkflow(tt.content, "test.md", SourceBuiltIn)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestLoader_AllowsOverrideForBuiltIn(t *testing.T) {
	content := `---
name: "Test Workflow"
description: "Built-in workflow with override"
agent_roles:
  implementer:
    system_prompt_override: "Override system prompt"
    system_prompt_append: "Additional text"
---

# Content
`
	wf, err := parseWorkflow(content, "test.md", SourceBuiltIn)
	require.NoError(t, err)
	require.NotNil(t, wf.AgentRoles)

	implConfig := wf.AgentRoles["implementer"]
	assert.Equal(t, "Override system prompt", implConfig.SystemPromptOverride, "system_prompt_override should be allowed for built-in workflows")
	assert.Equal(t, "Additional text", implConfig.SystemPromptAppend)
}

func TestLoader_ParsesConstraints(t *testing.T) {
	content := `---
name: "Test Workflow"
description: "Workflow with constraints"
agent_roles:
  reviewer:
    constraints:
      - "Always check for security issues"
      - "Verify test coverage"
      - "Check for dead code"
---

# Content
`
	wf, err := parseWorkflow(content, "test.md", SourceBuiltIn)
	require.NoError(t, err)
	require.NotNil(t, wf.AgentRoles)

	revConfig := wf.AgentRoles["reviewer"]
	require.Len(t, revConfig.Constraints, 3)
	assert.Equal(t, "Always check for security issues", revConfig.Constraints[0])
	assert.Equal(t, "Verify test coverage", revConfig.Constraints[1])
	assert.Equal(t, "Check for dead code", revConfig.Constraints[2])
}

func TestLoader_ExistingWorkflowsUnchanged(t *testing.T) {
	// Test that existing workflows (without agent_roles) work as before
	content := `---
name: "Legacy Workflow"
description: "Old workflow without agent_roles"
category: "Testing"
workers: 3
---

# Legacy content
This workflow has no agent_roles field and should work exactly as before.
`
	wf, err := parseWorkflow(content, "legacy.md", SourceBuiltIn)
	require.NoError(t, err)

	assert.Equal(t, "legacy", wf.ID)
	assert.Equal(t, "Legacy Workflow", wf.Name)
	assert.Equal(t, "Old workflow without agent_roles", wf.Description)
	assert.Equal(t, "Testing", wf.Category)
	assert.Equal(t, 3, wf.Workers)
	assert.Nil(t, wf.AgentRoles)
	assert.Equal(t, content, wf.Content)
	assert.Equal(t, SourceBuiltIn, wf.Source)
}

func TestLoader_EmptyAgentRolesHandled(t *testing.T) {
	// Test that empty agent_roles is handled correctly
	content := `---
name: "Test Workflow"
description: "Empty agent_roles"
agent_roles: {}
---

# Content
`
	wf, err := parseWorkflow(content, "test.md", SourceBuiltIn)
	require.NoError(t, err)
	// Empty map should result in nil AgentRoles (since len == 0)
	assert.Nil(t, wf.AgentRoles)
}

func TestLoader_MultipleAgentTypesWithAllFields(t *testing.T) {
	content := `---
name: "Full Workflow"
description: "All agent types with all fields"
agent_roles:
  implementer:
    system_prompt_append: "Implementer append"
    system_prompt_override: "Implementer override"
    constraints:
      - "Impl constraint 1"
  reviewer:
    system_prompt_append: "Reviewer append"
    constraints:
      - "Rev constraint 1"
      - "Rev constraint 2"
  researcher:
    system_prompt_append: "Researcher append"
---

# Content
`
	wf, err := parseWorkflow(content, "full.md", SourceBuiltIn)
	require.NoError(t, err)
	require.NotNil(t, wf.AgentRoles)
	assert.Len(t, wf.AgentRoles, 3)

	// Verify implementer
	impl := wf.AgentRoles["implementer"]
	assert.Equal(t, "Implementer append", impl.SystemPromptAppend)
	assert.Equal(t, "Implementer override", impl.SystemPromptOverride)
	assert.Equal(t, []string{"Impl constraint 1"}, impl.Constraints)

	// Verify reviewer
	rev := wf.AgentRoles["reviewer"]
	assert.Equal(t, "Reviewer append", rev.SystemPromptAppend)
	assert.Empty(t, rev.SystemPromptOverride)
	assert.Equal(t, []string{"Rev constraint 1", "Rev constraint 2"}, rev.Constraints)

	// Verify researcher
	res := wf.AgentRoles["researcher"]
	assert.Equal(t, "Researcher append", res.SystemPromptAppend)
	assert.Empty(t, res.SystemPromptOverride)
	assert.Nil(t, res.Constraints)
}

// --- Tests for TargetMode type and constants ---

func TestTargetMode_Constants(t *testing.T) {
	t.Run("TargetOrchestration has correct value", func(t *testing.T) {
		assert.Equal(t, TargetMode("orchestration"), TargetOrchestration)
	})

	t.Run("TargetChat has correct value", func(t *testing.T) {
		assert.Equal(t, TargetMode("chat"), TargetChat)
	})

	t.Run("TargetBoth is empty string for backwards compatibility", func(t *testing.T) {
		assert.Equal(t, TargetMode(""), TargetBoth)
	})
}

func TestWorkflow_TargetMode_Field(t *testing.T) {
	t.Run("Workflow struct includes TargetMode field", func(t *testing.T) {
		wf := Workflow{
			ID:         "test",
			Name:       "Test Workflow",
			TargetMode: TargetOrchestration,
		}
		assert.Equal(t, TargetOrchestration, wf.TargetMode)
	})

	t.Run("TargetMode can be set to each constant value", func(t *testing.T) {
		wfOrch := Workflow{TargetMode: TargetOrchestration}
		assert.Equal(t, TargetOrchestration, wfOrch.TargetMode)

		wfChat := Workflow{TargetMode: TargetChat}
		assert.Equal(t, TargetChat, wfChat.TargetMode)

		wfBoth := Workflow{TargetMode: TargetBoth}
		assert.Equal(t, TargetBoth, wfBoth.TargetMode)
	})

	t.Run("empty string default works for TargetBoth", func(t *testing.T) {
		// When TargetMode is not set, it defaults to empty string (TargetBoth)
		wf := Workflow{
			ID:   "test",
			Name: "Test Workflow",
		}
		assert.Equal(t, TargetBoth, wf.TargetMode)
		assert.Equal(t, TargetMode(""), wf.TargetMode)
	})
}

// --- Tests for target_mode parsing in loader ---

func TestLoader_ParsesTargetMode_Chat(t *testing.T) {
	content := `---
name: "Chat Workflow"
description: "A chat-only workflow"
target_mode: "chat"
---

# Content
`
	wf, err := parseWorkflow(content, "chat_workflow.md", SourceBuiltIn)
	require.NoError(t, err)
	assert.Equal(t, TargetChat, wf.TargetMode)
}

func TestLoader_ParsesTargetMode_Orchestration(t *testing.T) {
	content := `---
name: "Orchestration Workflow"
description: "An orchestration-only workflow"
target_mode: "orchestration"
---

# Content
`
	wf, err := parseWorkflow(content, "orchestration_workflow.md", SourceBuiltIn)
	require.NoError(t, err)
	assert.Equal(t, TargetOrchestration, wf.TargetMode)
}

func TestLoader_ParsesTargetMode_DefaultsToEmpty(t *testing.T) {
	content := `---
name: "Both Modes Workflow"
description: "A workflow without target_mode specified"
---

# Content
`
	wf, err := parseWorkflow(content, "both_workflow.md", SourceBuiltIn)
	require.NoError(t, err)
	assert.Equal(t, TargetBoth, wf.TargetMode)
	assert.Equal(t, TargetMode(""), wf.TargetMode)
}

func TestLoader_ParsesTargetMode_RejectsInvalid(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		errContains string
	}{
		{
			name: "invalid value 'invalid'",
			content: `---
name: "Invalid Workflow"
target_mode: "invalid"
---

# Content
`,
			errContains: "invalid target_mode",
		},
		{
			name: "invalid value 'CHAT' (wrong case)",
			content: `---
name: "Invalid Workflow"
target_mode: "CHAT"
---

# Content
`,
			errContains: "invalid target_mode",
		},
		{
			name: "invalid value 'ORCHESTRATION' (wrong case)",
			content: `---
name: "Invalid Workflow"
target_mode: "ORCHESTRATION"
---

# Content
`,
			errContains: "invalid target_mode",
		},
		{
			name: "invalid value 'both' (not allowed)",
			content: `---
name: "Invalid Workflow"
target_mode: "both"
---

# Content
`,
			errContains: "invalid target_mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseWorkflow(tt.content, "invalid.md", SourceBuiltIn)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

// --- Tests for chat_research_to_tasks.md built-in template ---

func TestLoadBuiltinWorkflows_IncludesChatResearchToTasks(t *testing.T) {
	workflows, err := LoadBuiltinWorkflows()
	require.NoError(t, err)

	var found bool
	var chatWorkflow Workflow
	for _, wf := range workflows {
		if wf.ID == "chat_research_to_tasks" {
			found = true
			chatWorkflow = wf
			break
		}
	}

	require.True(t, found, "expected to find chat_research_to_tasks workflow in built-in workflows")
	assert.Equal(t, "Research to Tasks", chatWorkflow.Name)
	assert.Equal(t, TargetChat, chatWorkflow.TargetMode, "chat_research_to_tasks should have target_mode: chat")
	assert.Equal(t, SourceBuiltIn, chatWorkflow.Source)
	assert.NotEmpty(t, chatWorkflow.Description)
	assert.NotEmpty(t, chatWorkflow.Content)
	assert.Equal(t, "Planning", chatWorkflow.Category)
}
