package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/config"
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
