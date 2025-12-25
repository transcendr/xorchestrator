package workflow

import (
	"sort"
	"strings"

	"github.com/zjrosen/perles/internal/config"
)

// Registry manages workflow templates from multiple sources.
type Registry struct {
	workflows map[string]Workflow
}

// NewRegistry creates a new empty workflow registry.
func NewRegistry() *Registry {
	return &Registry{
		workflows: make(map[string]Workflow),
	}
}

// NewRegistryWithBuiltins creates a registry pre-populated with built-in workflows.
func NewRegistryWithBuiltins() (*Registry, error) {
	r := NewRegistry()

	builtins, err := LoadBuiltinWorkflows()
	if err != nil {
		return nil, err
	}

	for _, wf := range builtins {
		r.Add(wf)
	}

	return r, nil
}

// NewRegistryWithConfig creates a registry with built-in workflows, user workflows,
// and applies configuration overrides.
//
// Loading order (later sources can shadow earlier):
//  1. Built-in workflows (from embedded templates)
//  2. User workflows (from ~/.perles/workflows/)
//
// Config overrides:
//   - If workflow has enabled=false, it's removed from the registry
//   - If workflow has name/description overrides, those fields are updated
func NewRegistryWithConfig(cfg config.OrchestrationConfig) (*Registry, error) {
	return NewRegistryWithConfigFromDir(cfg, UserWorkflowDir())
}

// NewRegistryWithConfigFromDir creates a registry with a custom user workflow directory.
// This is primarily useful for testing.
func NewRegistryWithConfigFromDir(cfg config.OrchestrationConfig, userDir string) (*Registry, error) {
	r := NewRegistry()

	// 1. Load built-in workflows first
	builtins, err := LoadBuiltinWorkflows()
	if err != nil {
		return nil, err
	}
	for _, wf := range builtins {
		r.Add(wf)
	}

	// 2. Load user workflows (can shadow built-ins by using same ID)
	userWorkflows, err := LoadUserWorkflowsFromDir(userDir)
	if err != nil {
		return nil, err
	}
	for _, wf := range userWorkflows {
		r.Add(wf)
	}

	// 3. Apply config overrides
	applyConfigOverrides(r, cfg.Workflows)

	return r, nil
}

// applyConfigOverrides applies workflow configuration overrides to the registry.
// - Disables workflows with enabled=false
// - Overrides name/description for matching workflows
func applyConfigOverrides(r *Registry, configs []config.WorkflowConfig) {
	for _, cfg := range configs {
		// Find matching workflow by name (case-insensitive match on Name field)
		var matchedID string
		for _, wf := range r.workflows {
			if strings.EqualFold(wf.Name, cfg.Name) {
				matchedID = wf.ID
				break
			}
		}

		// No matching workflow found - skip this config
		// (This allows defining config for workflows not yet created)
		if matchedID == "" {
			continue
		}

		wf := r.workflows[matchedID]

		// Check if this workflow should be disabled
		if !cfg.IsEnabled() {
			r.Remove(matchedID)
			continue
		}

		// Apply name override if the config has a different name
		// (The name in config might be used for matching, so only override
		// if it's explicitly different from original)
		// Note: Since we matched by name, we don't override the name here
		// The name in config is used for identification

		// Apply description override if specified
		if cfg.Description != "" {
			wf.Description = cfg.Description
			r.workflows[matchedID] = wf
		}
	}
}

// Add adds a workflow to the registry.
// If a workflow with the same ID already exists, it will be replaced.
func (r *Registry) Add(wf Workflow) {
	r.workflows[wf.ID] = wf
}

// Get retrieves a workflow by ID.
// Returns the workflow and true if found, or an empty workflow and false if not.
func (r *Registry) Get(id string) (Workflow, bool) {
	wf, ok := r.workflows[id]
	return wf, ok
}

// List returns all workflows in the registry, sorted by name.
func (r *Registry) List() []Workflow {
	workflows := make([]Workflow, 0, len(r.workflows))
	for _, wf := range r.workflows {
		workflows = append(workflows, wf)
	}

	sort.Slice(workflows, func(i, j int) bool {
		return workflows[i].Name < workflows[j].Name
	})

	return workflows
}

// ListBySource returns workflows filtered by source, sorted by name.
func (r *Registry) ListBySource(source Source) []Workflow {
	var workflows []Workflow
	for _, wf := range r.workflows {
		if wf.Source == source {
			workflows = append(workflows, wf)
		}
	}

	sort.Slice(workflows, func(i, j int) bool {
		return workflows[i].Name < workflows[j].Name
	})

	return workflows
}

// ListByCategory returns workflows filtered by category, sorted by name.
// An empty category matches workflows with no category set.
func (r *Registry) ListByCategory(category string) []Workflow {
	var workflows []Workflow
	for _, wf := range r.workflows {
		if wf.Category == category {
			workflows = append(workflows, wf)
		}
	}

	sort.Slice(workflows, func(i, j int) bool {
		return workflows[i].Name < workflows[j].Name
	})

	return workflows
}

// Search returns workflows matching the query in name or description.
// Search is case-insensitive and matches substrings.
func (r *Registry) Search(query string) []Workflow {
	if query == "" {
		return r.List()
	}

	query = strings.ToLower(query)
	var workflows []Workflow
	for _, wf := range r.workflows {
		nameLower := strings.ToLower(wf.Name)
		descLower := strings.ToLower(wf.Description)
		if strings.Contains(nameLower, query) || strings.Contains(descLower, query) {
			workflows = append(workflows, wf)
		}
	}

	// Sort with name matches first, then by name
	sort.Slice(workflows, func(i, j int) bool {
		iNameMatch := strings.Contains(strings.ToLower(workflows[i].Name), query)
		jNameMatch := strings.Contains(strings.ToLower(workflows[j].Name), query)
		if iNameMatch != jNameMatch {
			return iNameMatch // Name matches come first
		}
		return workflows[i].Name < workflows[j].Name
	})

	return workflows
}

// Remove removes a workflow by ID.
// Returns true if the workflow was found and removed, false otherwise.
func (r *Registry) Remove(id string) bool {
	if _, ok := r.workflows[id]; ok {
		delete(r.workflows, id)
		return true
	}
	return false
}

// Count returns the total number of workflows in the registry.
func (r *Registry) Count() int {
	return len(r.workflows)
}

// Categories returns a sorted list of unique categories in the registry.
func (r *Registry) Categories() []string {
	categorySet := make(map[string]struct{})
	for _, wf := range r.workflows {
		if wf.Category != "" {
			categorySet[wf.Category] = struct{}{}
		}
	}

	categories := make([]string, 0, len(categorySet))
	for cat := range categorySet {
		categories = append(categories, cat)
	}

	sort.Strings(categories)
	return categories
}
