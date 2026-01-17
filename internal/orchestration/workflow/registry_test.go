package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zjrosen/xorchestrator/internal/config"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	assert.NotNil(t, r)
	assert.Equal(t, 0, r.Count())
}

func TestNewRegistryWithBuiltins(t *testing.T) {
	r, err := NewRegistryWithBuiltins()
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.GreaterOrEqual(t, r.Count(), 2, "expected at least 2 built-in workflows")

	// Verify built-ins are present
	debate, ok := r.Get("debate")
	assert.True(t, ok)
	assert.Equal(t, "Technical Debate", debate.Name)

	research, ok := r.Get("research_proposal")
	assert.True(t, ok)
	assert.Equal(t, "Research Proposal", research.Name)
}

func TestRegistryAdd(t *testing.T) {
	r := NewRegistry()

	wf := Workflow{
		ID:          "test",
		Name:        "Test Workflow",
		Description: "A test workflow",
		Source:      SourceUser,
	}

	r.Add(wf)
	assert.Equal(t, 1, r.Count())

	// Can retrieve it
	got, ok := r.Get("test")
	assert.True(t, ok)
	assert.Equal(t, wf, got)
}

func TestRegistryAddReplace(t *testing.T) {
	r := NewRegistry()

	wf1 := Workflow{
		ID:   "test",
		Name: "Original",
	}
	wf2 := Workflow{
		ID:   "test",
		Name: "Replacement",
	}

	r.Add(wf1)
	r.Add(wf2)

	assert.Equal(t, 1, r.Count())
	got, _ := r.Get("test")
	assert.Equal(t, "Replacement", got.Name)
}

func TestRegistryGet(t *testing.T) {
	r := NewRegistry()

	// Not found
	_, ok := r.Get("nonexistent")
	assert.False(t, ok)

	// Found
	r.Add(Workflow{ID: "test", Name: "Test"})
	wf, ok := r.Get("test")
	assert.True(t, ok)
	assert.Equal(t, "Test", wf.Name)
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()

	// Empty list
	assert.Empty(t, r.List())

	// Add some workflows
	r.Add(Workflow{ID: "z", Name: "Zebra"})
	r.Add(Workflow{ID: "a", Name: "Alpha"})
	r.Add(Workflow{ID: "m", Name: "Middle"})

	list := r.List()
	require.Len(t, list, 3)

	// Should be sorted by name
	assert.Equal(t, "Alpha", list[0].Name)
	assert.Equal(t, "Middle", list[1].Name)
	assert.Equal(t, "Zebra", list[2].Name)
}

func TestRegistryListBySource(t *testing.T) {
	r := NewRegistry()

	r.Add(Workflow{ID: "builtin1", Name: "Built-in 1", Source: SourceBuiltIn})
	r.Add(Workflow{ID: "builtin2", Name: "Built-in 2", Source: SourceBuiltIn})
	r.Add(Workflow{ID: "user1", Name: "User 1", Source: SourceUser})

	builtins := r.ListBySource(SourceBuiltIn)
	assert.Len(t, builtins, 2)
	assert.Equal(t, "Built-in 1", builtins[0].Name)
	assert.Equal(t, "Built-in 2", builtins[1].Name)

	users := r.ListBySource(SourceUser)
	assert.Len(t, users, 1)
	assert.Equal(t, "User 1", users[0].Name)
}

func TestRegistryListByCategory(t *testing.T) {
	r := NewRegistry()

	r.Add(Workflow{ID: "a1", Name: "Analysis 1", Category: "Analysis"})
	r.Add(Workflow{ID: "a2", Name: "Analysis 2", Category: "Analysis"})
	r.Add(Workflow{ID: "p1", Name: "Planning 1", Category: "Planning"})
	r.Add(Workflow{ID: "n1", Name: "No Category"}) // Empty category

	analysis := r.ListByCategory("Analysis")
	assert.Len(t, analysis, 2)
	assert.Equal(t, "Analysis 1", analysis[0].Name)
	assert.Equal(t, "Analysis 2", analysis[1].Name)

	planning := r.ListByCategory("Planning")
	assert.Len(t, planning, 1)
	assert.Equal(t, "Planning 1", planning[0].Name)

	noCategory := r.ListByCategory("")
	assert.Len(t, noCategory, 1)
	assert.Equal(t, "No Category", noCategory[0].Name)
}

func TestRegistrySearch(t *testing.T) {
	r := NewRegistry()

	r.Add(Workflow{ID: "debate", Name: "Technical Debate", Description: "Multi-agent debate format"})
	r.Add(Workflow{ID: "research", Name: "Research Proposal", Description: "Collaborative research"})
	r.Add(Workflow{ID: "custom", Name: "Custom Workflow", Description: "User custom template"})

	t.Run("empty query returns all", func(t *testing.T) {
		results := r.Search("")
		assert.Len(t, results, 3)
	})

	t.Run("search by name", func(t *testing.T) {
		results := r.Search("debate")
		require.Len(t, results, 1)
		assert.Equal(t, "Technical Debate", results[0].Name)
	})

	t.Run("search by description", func(t *testing.T) {
		results := r.Search("collaborative")
		require.Len(t, results, 1)
		assert.Equal(t, "Research Proposal", results[0].Name)
	})

	t.Run("case insensitive", func(t *testing.T) {
		results := r.Search("DEBATE")
		require.Len(t, results, 1)
		assert.Equal(t, "Technical Debate", results[0].Name)
	})

	t.Run("partial match", func(t *testing.T) {
		results := r.Search("work")
		require.Len(t, results, 1)
		assert.Equal(t, "Custom Workflow", results[0].Name)
	})

	t.Run("name matches before description matches", func(t *testing.T) {
		// "research" appears in name of Research Proposal and description of another
		r.Add(Workflow{ID: "other", Name: "Other Thing", Description: "Contains research word"})
		results := r.Search("research")
		require.Len(t, results, 2)
		// Name match should come first
		assert.Equal(t, "Research Proposal", results[0].Name)
	})

	t.Run("no matches", func(t *testing.T) {
		results := r.Search("nonexistent")
		assert.Empty(t, results)
	})
}

func TestRegistryRemove(t *testing.T) {
	r := NewRegistry()

	r.Add(Workflow{ID: "test", Name: "Test"})
	assert.Equal(t, 1, r.Count())

	// Remove existing
	removed := r.Remove("test")
	assert.True(t, removed)
	assert.Equal(t, 0, r.Count())

	// Remove non-existing
	removed = r.Remove("test")
	assert.False(t, removed)
}

func TestRegistryCategories(t *testing.T) {
	r := NewRegistry()

	// Empty registry
	assert.Empty(t, r.Categories())

	// Add workflows with categories
	r.Add(Workflow{ID: "a1", Name: "A1", Category: "Analysis"})
	r.Add(Workflow{ID: "a2", Name: "A2", Category: "Analysis"}) // Duplicate category
	r.Add(Workflow{ID: "p1", Name: "P1", Category: "Planning"})
	r.Add(Workflow{ID: "n1", Name: "N1"}) // No category (should not appear)

	categories := r.Categories()
	require.Len(t, categories, 2)
	assert.Equal(t, "Analysis", categories[0])
	assert.Equal(t, "Planning", categories[1])
}

func TestRegistry_ListByTargetMode_FiltersCorrectly(t *testing.T) {
	r := NewRegistry()

	r.Add(Workflow{ID: "orch1", Name: "Orchestration 1", TargetMode: TargetOrchestration})
	r.Add(Workflow{ID: "orch2", Name: "Orchestration 2", TargetMode: TargetOrchestration})
	r.Add(Workflow{ID: "chat1", Name: "Chat 1", TargetMode: TargetChat})

	orchWorkflows := r.ListByTargetMode(TargetOrchestration)
	assert.Len(t, orchWorkflows, 2)
	assert.Equal(t, "Orchestration 1", orchWorkflows[0].Name)
	assert.Equal(t, "Orchestration 2", orchWorkflows[1].Name)

	chatWorkflows := r.ListByTargetMode(TargetChat)
	assert.Len(t, chatWorkflows, 1)
	assert.Equal(t, "Chat 1", chatWorkflows[0].Name)
}

func TestRegistry_ListByTargetMode_IncludesTargetBoth(t *testing.T) {
	r := NewRegistry()

	r.Add(Workflow{ID: "orch", Name: "Orchestration Only", TargetMode: TargetOrchestration})
	r.Add(Workflow{ID: "chat", Name: "Chat Only", TargetMode: TargetChat})
	r.Add(Workflow{ID: "both", Name: "Both Modes", TargetMode: TargetBoth})

	// TargetBoth should appear in orchestration results
	orchWorkflows := r.ListByTargetMode(TargetOrchestration)
	require.Len(t, orchWorkflows, 2)
	assert.Equal(t, "Both Modes", orchWorkflows[0].Name)
	assert.Equal(t, "Orchestration Only", orchWorkflows[1].Name)

	// TargetBoth should appear in chat results
	chatWorkflows := r.ListByTargetMode(TargetChat)
	require.Len(t, chatWorkflows, 2)
	assert.Equal(t, "Both Modes", chatWorkflows[0].Name)
	assert.Equal(t, "Chat Only", chatWorkflows[1].Name)

	// TargetBoth mode query should only return exact matches (TargetBoth workflows)
	bothWorkflows := r.ListByTargetMode(TargetBoth)
	require.Len(t, bothWorkflows, 1)
	assert.Equal(t, "Both Modes", bothWorkflows[0].Name)
}

func TestRegistry_ListByTargetMode_EmptyResult(t *testing.T) {
	r := NewRegistry()

	// Empty registry returns empty slice, not nil
	result := r.ListByTargetMode(TargetOrchestration)
	assert.NotNil(t, result, "should return empty slice, not nil")
	assert.Len(t, result, 0)

	// Registry with workflows of other modes also returns empty slice
	r.Add(Workflow{ID: "chat", Name: "Chat Only", TargetMode: TargetChat})
	result = r.ListByTargetMode(TargetOrchestration)
	assert.NotNil(t, result, "should return empty slice, not nil")
	assert.Len(t, result, 0)
}

func TestRegistry_ListByTargetMode_SortedByName(t *testing.T) {
	r := NewRegistry()

	// Add workflows out of alphabetical order
	r.Add(Workflow{ID: "z", Name: "Zebra", TargetMode: TargetOrchestration})
	r.Add(Workflow{ID: "a", Name: "Alpha", TargetMode: TargetOrchestration})
	r.Add(Workflow{ID: "m", Name: "Middle", TargetMode: TargetOrchestration})
	r.Add(Workflow{ID: "b", Name: "Both Mode", TargetMode: TargetBoth}) // Should also be sorted

	result := r.ListByTargetMode(TargetOrchestration)
	require.Len(t, result, 4)

	// Verify alphabetical order
	assert.Equal(t, "Alpha", result[0].Name)
	assert.Equal(t, "Both Mode", result[1].Name)
	assert.Equal(t, "Middle", result[2].Name)
	assert.Equal(t, "Zebra", result[3].Name)
}

func TestNewRegistryWithConfigFromDir(t *testing.T) {
	t.Run("loads built-ins when user dir is empty", func(t *testing.T) {
		cfg := config.OrchestrationConfig{}
		r, err := NewRegistryWithConfigFromDir(cfg, "/non/existent/path")
		require.NoError(t, err)

		// Should have built-in workflows
		assert.GreaterOrEqual(t, r.Count(), 2)

		debate, ok := r.Get("debate")
		assert.True(t, ok)
		assert.Equal(t, SourceBuiltIn, debate.Source)
	})

	t.Run("user workflow shadows built-in with same ID", func(t *testing.T) {
		dir := t.TempDir()

		// Create a user workflow with same ID as built-in
		content := `---
name: "Custom Debate"
description: "User's custom debate format"
---

# Custom Content
`
		err := os.WriteFile(filepath.Join(dir, "debate.md"), []byte(content), 0o644)
		require.NoError(t, err)

		cfg := config.OrchestrationConfig{}
		r, err := NewRegistryWithConfigFromDir(cfg, dir)
		require.NoError(t, err)

		debate, ok := r.Get("debate")
		assert.True(t, ok)
		assert.Equal(t, "Custom Debate", debate.Name)
		assert.Equal(t, SourceUser, debate.Source)
	})

	t.Run("user workflows appear alongside built-ins", func(t *testing.T) {
		dir := t.TempDir()

		// Create a unique user workflow
		content := `---
name: "Code Review"
description: "Multi-perspective code review"
---

# Code Review Content
`
		err := os.WriteFile(filepath.Join(dir, "code_review.md"), []byte(content), 0o644)
		require.NoError(t, err)

		cfg := config.OrchestrationConfig{}
		r, err := NewRegistryWithConfigFromDir(cfg, dir)
		require.NoError(t, err)

		// Should have both built-in and user workflows
		codeReview, ok := r.Get("code_review")
		assert.True(t, ok)
		assert.Equal(t, SourceUser, codeReview.Source)

		debate, ok := r.Get("debate")
		assert.True(t, ok)
		assert.Equal(t, SourceBuiltIn, debate.Source)
	})

	t.Run("config can disable built-in workflow", func(t *testing.T) {
		enabled := false
		cfg := config.OrchestrationConfig{
			Workflows: []config.WorkflowConfig{
				{
					Name:    "Technical Debate", // Match by display name
					Enabled: &enabled,
				},
			},
		}

		r, err := NewRegistryWithConfigFromDir(cfg, "/non/existent")
		require.NoError(t, err)

		// Debate should be removed
		_, ok := r.Get("debate")
		assert.False(t, ok, "debate workflow should be disabled")

		// Research Proposal should still exist
		_, ok = r.Get("research_proposal")
		assert.True(t, ok, "research_proposal should still exist")
	})

	t.Run("config can override description", func(t *testing.T) {
		cfg := config.OrchestrationConfig{
			Workflows: []config.WorkflowConfig{
				{
					Name:        "Technical Debate",
					Description: "Custom override description",
				},
			},
		}

		r, err := NewRegistryWithConfigFromDir(cfg, "/non/existent")
		require.NoError(t, err)

		debate, ok := r.Get("debate")
		assert.True(t, ok)
		assert.Equal(t, "Custom override description", debate.Description)
		assert.Equal(t, "Technical Debate", debate.Name) // Name unchanged
	})

	t.Run("config matching is case-insensitive", func(t *testing.T) {
		enabled := false
		cfg := config.OrchestrationConfig{
			Workflows: []config.WorkflowConfig{
				{
					Name:    "technical debate", // lowercase
					Enabled: &enabled,
				},
			},
		}

		r, err := NewRegistryWithConfigFromDir(cfg, "/non/existent")
		require.NoError(t, err)

		_, ok := r.Get("debate")
		assert.False(t, ok, "debate should be disabled even with case mismatch")
	})

	t.Run("config for non-existent workflow is ignored", func(t *testing.T) {
		enabled := false
		cfg := config.OrchestrationConfig{
			Workflows: []config.WorkflowConfig{
				{
					Name:    "Non Existent Workflow",
					Enabled: &enabled,
				},
			},
		}

		r, err := NewRegistryWithConfigFromDir(cfg, "/non/existent")
		require.NoError(t, err)

		// Should still have all built-ins
		assert.GreaterOrEqual(t, r.Count(), 2)
	})
}

// --- Tests for chat_research_to_tasks.md built-in template filtering ---

func TestRegistry_ChatResearchToTasks_AppearsInChatMode(t *testing.T) {
	r, err := NewRegistryWithBuiltins()
	require.NoError(t, err)

	chatWorkflows := r.ListByTargetMode(TargetChat)

	var found bool
	for _, wf := range chatWorkflows {
		if wf.ID == "chat_research_to_tasks" {
			found = true
			assert.Equal(t, TargetChat, wf.TargetMode)
			break
		}
	}
	assert.True(t, found, "chat_research_to_tasks should appear in ListByTargetMode(TargetChat)")
}

func TestRegistry_ChatResearchToTasks_NotInOrchestrationMode(t *testing.T) {
	r, err := NewRegistryWithBuiltins()
	require.NoError(t, err)

	orchWorkflows := r.ListByTargetMode(TargetOrchestration)

	for _, wf := range orchWorkflows {
		assert.NotEqual(t, "chat_research_to_tasks", wf.ID,
			"chat_research_to_tasks should NOT appear in ListByTargetMode(TargetOrchestration)")
	}
}

// --- Cross-mode isolation integration tests ---
// These tests verify that workflow mode targeting works correctly with real built-in templates.

func TestCrossMode_OrchestrationNotInChat(t *testing.T) {
	// Integration test: workflows with TargetMode=TargetOrchestration should NOT appear in chat mode.
	// Uses real built-in workflows: debate, quick_plan, cook, etc. all have target_mode: "orchestration".
	r, err := NewRegistryWithBuiltins()
	require.NoError(t, err)

	chatWorkflows := r.ListByTargetMode(TargetChat)

	// Verify NO orchestration-only workflows appear in chat results
	for _, wf := range chatWorkflows {
		assert.NotEqual(t, TargetOrchestration, wf.TargetMode,
			"workflow %q with TargetOrchestration should NOT appear in chat mode", wf.ID)
	}

	// Collect chat workflow IDs
	chatIDs := make(map[string]bool)
	for _, wf := range chatWorkflows {
		chatIDs[wf.ID] = true
	}

	// Verify orchestration-targeted workflows are excluded from chat mode
	orchestrationWorkflows := []string{"debate", "quick_plan", "cook", "research_proposal", "research_to_tasks", "mediated_investigation"}
	for _, id := range orchestrationWorkflows {
		assert.False(t, chatIDs[id], "orchestration workflow %q should NOT appear in chat mode", id)
	}

	// Verify chat-targeted workflow IS present
	assert.True(t, chatIDs["chat_research_to_tasks"], "chat_research_to_tasks should appear in chat mode")
}

func TestCrossMode_ChatNotInOrchestration(t *testing.T) {
	// Integration test: workflows with TargetMode=TargetChat should NOT appear in orchestration mode.
	// Uses real built-in workflows (chat_research_to_tasks has target_mode: "chat").
	r, err := NewRegistryWithBuiltins()
	require.NoError(t, err)

	orchWorkflows := r.ListByTargetMode(TargetOrchestration)

	// Verify no chat-only workflows appear in orchestration results
	for _, wf := range orchWorkflows {
		assert.NotEqual(t, TargetChat, wf.TargetMode,
			"workflow %q with TargetChat should NOT appear in orchestration mode", wf.ID)
	}

	// Specifically verify chat_research_to_tasks is excluded
	orchIDs := make(map[string]bool)
	for _, wf := range orchWorkflows {
		orchIDs[wf.ID] = true
	}
	assert.False(t, orchIDs["chat_research_to_tasks"],
		"chat_research_to_tasks should NOT appear in orchestration mode")

	// Verify orchestration-targeted built-in workflows ARE present
	assert.True(t, orchIDs["debate"], "debate should appear in orchestration mode")
	assert.True(t, orchIDs["quick_plan"], "quick_plan should appear in orchestration mode")
	assert.True(t, orchIDs["cook"], "cook should appear in orchestration mode")
}

func TestCrossMode_BothInBothModes(t *testing.T) {
	// Integration test: workflows with TargetMode=TargetBoth (empty string) appear in BOTH modes.
	// This verifies backwards compatibility - workflows without explicit target_mode work everywhere.
	r := NewRegistry()

	// Add workflows for each target mode
	r.Add(Workflow{ID: "orch_only", Name: "Orchestration Only", TargetMode: TargetOrchestration})
	r.Add(Workflow{ID: "chat_only", Name: "Chat Only", TargetMode: TargetChat})
	r.Add(Workflow{ID: "both_1", Name: "Both Modes 1", TargetMode: TargetBoth})
	r.Add(Workflow{ID: "both_2", Name: "Both Modes 2", TargetMode: TargetBoth})
	r.Add(Workflow{ID: "both_3", Name: "Both Modes 3", TargetMode: ""}) // Explicit empty string = TargetBoth

	chatWorkflows := r.ListByTargetMode(TargetChat)
	orchWorkflows := r.ListByTargetMode(TargetOrchestration)

	// Collect IDs for each mode
	chatIDs := make(map[string]bool)
	for _, wf := range chatWorkflows {
		chatIDs[wf.ID] = true
	}
	orchIDs := make(map[string]bool)
	for _, wf := range orchWorkflows {
		orchIDs[wf.ID] = true
	}

	// TargetBoth workflows should appear in BOTH modes
	bothModeWorkflows := []string{"both_1", "both_2", "both_3"}
	for _, id := range bothModeWorkflows {
		assert.True(t, chatIDs[id], "workflow %q with TargetBoth should appear in chat mode", id)
		assert.True(t, orchIDs[id], "workflow %q with TargetBoth should appear in orchestration mode", id)
	}

	// Mode-specific workflows should only appear in their respective modes
	assert.True(t, orchIDs["orch_only"], "orch_only should appear in orchestration mode")
	assert.False(t, chatIDs["orch_only"], "orch_only should NOT appear in chat mode")
	assert.True(t, chatIDs["chat_only"], "chat_only should appear in chat mode")
	assert.False(t, orchIDs["chat_only"], "chat_only should NOT appear in orchestration mode")
}

func TestApplyConfigOverrides(t *testing.T) {
	t.Run("disable removes workflow", func(t *testing.T) {
		r := NewRegistry()
		r.Add(Workflow{ID: "test", Name: "Test Workflow", Description: "Original"})

		enabled := false
		configs := []config.WorkflowConfig{
			{Name: "Test Workflow", Enabled: &enabled},
		}

		applyConfigOverrides(r, configs)
		assert.Equal(t, 0, r.Count())
	})

	t.Run("description override works", func(t *testing.T) {
		r := NewRegistry()
		r.Add(Workflow{ID: "test", Name: "Test Workflow", Description: "Original"})

		configs := []config.WorkflowConfig{
			{Name: "Test Workflow", Description: "New Description"},
		}

		applyConfigOverrides(r, configs)

		wf, _ := r.Get("test")
		assert.Equal(t, "New Description", wf.Description)
	})

	t.Run("empty description does not override", func(t *testing.T) {
		r := NewRegistry()
		r.Add(Workflow{ID: "test", Name: "Test Workflow", Description: "Original"})

		configs := []config.WorkflowConfig{
			{Name: "Test Workflow", Description: ""}, // Empty description
		}

		applyConfigOverrides(r, configs)

		wf, _ := r.Get("test")
		assert.Equal(t, "Original", wf.Description)
	})

	t.Run("nil enabled means enabled", func(t *testing.T) {
		r := NewRegistry()
		r.Add(Workflow{ID: "test", Name: "Test Workflow", Description: "Original"})

		configs := []config.WorkflowConfig{
			{Name: "Test Workflow", Description: "Updated"}, // Enabled is nil
		}

		applyConfigOverrides(r, configs)

		// Should still exist
		wf, ok := r.Get("test")
		assert.True(t, ok)
		assert.Equal(t, "Updated", wf.Description)
	})
}
