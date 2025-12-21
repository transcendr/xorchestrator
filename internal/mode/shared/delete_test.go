package shared

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"perles/internal/beads"
)

// mockLoader implements IssueLoader for testing.
type mockLoader struct {
	executeFunc func(query string) ([]beads.Issue, error)
}

func (m *mockLoader) Execute(query string) ([]beads.Issue, error) {
	if m.executeFunc != nil {
		return m.executeFunc(query)
	}
	return nil, nil
}

func TestGetAllDescendants_NoChildren(t *testing.T) {
	loader := &mockLoader{
		executeFunc: func(query string) ([]beads.Issue, error) {
			// Return just the root issue (no children)
			return []beads.Issue{
				{ID: "epic-1", Type: beads.TypeEpic},
			}, nil
		},
	}

	result := GetAllDescendants(loader, "epic-1")

	require.Len(t, result, 1)
	require.Equal(t, "epic-1", result[0].ID)
}

func TestGetAllDescendants_WithChildren(t *testing.T) {
	loader := &mockLoader{
		executeFunc: func(query string) ([]beads.Issue, error) {
			// Return root + immediate children
			return []beads.Issue{
				{ID: "epic-1", Type: beads.TypeEpic},
				{ID: "task-1", Type: beads.TypeTask},
				{ID: "task-2", Type: beads.TypeTask},
			}, nil
		},
	}

	result := GetAllDescendants(loader, "epic-1")

	require.Len(t, result, 3, "should have 3 issues (root + 2 children)")
	ids := make([]string, len(result))
	for i, issue := range result {
		ids[i] = issue.ID
	}
	require.Contains(t, ids, "epic-1")
	require.Contains(t, ids, "task-1")
	require.Contains(t, ids, "task-2")
}

func TestGetAllDescendants_NestedChildren(t *testing.T) {
	loader := &mockLoader{
		executeFunc: func(query string) ([]beads.Issue, error) {
			// Return root + children + grandchildren (2 levels)
			return []beads.Issue{
				{ID: "epic-1", Type: beads.TypeEpic},
				{ID: "sub-epic-1", Type: beads.TypeEpic},
				{ID: "task-1", Type: beads.TypeTask},
				{ID: "task-2", Type: beads.TypeTask},
				{ID: "grandchild-1", Type: beads.TypeTask},
			}, nil
		},
	}

	result := GetAllDescendants(loader, "epic-1")

	require.Len(t, result, 5, "should have 5 issues (root + 2 children + 2 grandchildren)")
	ids := make([]string, len(result))
	for i, issue := range result {
		ids[i] = issue.ID
	}
	require.Contains(t, ids, "epic-1")
	require.Contains(t, ids, "sub-epic-1")
	require.Contains(t, ids, "task-1")
	require.Contains(t, ids, "task-2")
	require.Contains(t, ids, "grandchild-1")
}

func TestGetAllDescendants_BQLError(t *testing.T) {
	loader := &mockLoader{
		executeFunc: func(query string) ([]beads.Issue, error) {
			return nil, errors.New("BQL query failed")
		},
	}

	result := GetAllDescendants(loader, "epic-1")

	require.Nil(t, result, "BQL error should return nil")
}

func TestGetAllDescendants_EmptyBQLResult(t *testing.T) {
	loader := &mockLoader{
		executeFunc: func(query string) ([]beads.Issue, error) {
			// Return empty slice (edge case)
			return []beads.Issue{}, nil
		},
	}

	result := GetAllDescendants(loader, "epic-1")

	require.Empty(t, result, "empty BQL result should return empty slice")
}

func TestCreateDeleteModal_RegularIssue_ReturnsCorrectIDs(t *testing.T) {
	loader := &mockLoader{}

	issue := &beads.Issue{
		ID:        "task-1",
		TitleText: "Test Task",
		Type:      beads.TypeTask,
	}

	modal, isCascade, issueIDs := CreateDeleteModal(issue, loader)

	require.NotNil(t, modal)
	require.False(t, isCascade, "regular issue should not be cascade")
	require.Equal(t, []string{"task-1"}, issueIDs, "should return single-element slice")
}

func TestCreateDeleteModal_EpicWithChildren_ReturnsAllDescendants(t *testing.T) {
	loader := &mockLoader{
		executeFunc: func(query string) ([]beads.Issue, error) {
			return []beads.Issue{
				{ID: "epic-1", Type: beads.TypeEpic, TitleText: "Root Epic"},
				{ID: "task-1", Type: beads.TypeTask, TitleText: "Child Task 1"},
				{ID: "task-2", Type: beads.TypeTask, TitleText: "Child Task 2"},
			}, nil
		},
	}

	issue := &beads.Issue{
		ID:        "epic-1",
		TitleText: "Root Epic",
		Type:      beads.TypeEpic,
		Children:  []string{"task-1", "task-2"},
	}

	modal, isCascade, issueIDs := CreateDeleteModal(issue, loader)

	require.NotNil(t, modal)
	require.True(t, isCascade, "epic with children should be cascade")
	require.Len(t, issueIDs, 3, "should return all 3 IDs")
	require.Contains(t, issueIDs, "epic-1")
	require.Contains(t, issueIDs, "task-1")
	require.Contains(t, issueIDs, "task-2")
}

func TestCreateDeleteModal_EpicWithNestedChildren_ReturnsAllDescendants(t *testing.T) {
	loader := &mockLoader{
		executeFunc: func(query string) ([]beads.Issue, error) {
			return []beads.Issue{
				{ID: "epic-1", Type: beads.TypeEpic, TitleText: "Root Epic"},
				{ID: "sub-epic-1", Type: beads.TypeEpic, TitleText: "Sub Epic"},
				{ID: "task-1", Type: beads.TypeTask, TitleText: "Child Task"},
				{ID: "grandchild-1", Type: beads.TypeTask, TitleText: "Grandchild Task"},
			}, nil
		},
	}

	issue := &beads.Issue{
		ID:        "epic-1",
		TitleText: "Root Epic",
		Type:      beads.TypeEpic,
		Children:  []string{"sub-epic-1", "task-1"}, // Immediate children
	}

	modal, isCascade, issueIDs := CreateDeleteModal(issue, loader)

	require.NotNil(t, modal)
	require.True(t, isCascade)
	require.Len(t, issueIDs, 4, "should return all 4 IDs including grandchild")
	require.Contains(t, issueIDs, "epic-1")
	require.Contains(t, issueIDs, "sub-epic-1")
	require.Contains(t, issueIDs, "task-1")
	require.Contains(t, issueIDs, "grandchild-1")
}

func TestCreateDeleteModal_EpicWithoutChildren_NotCascade(t *testing.T) {
	loader := &mockLoader{}

	issue := &beads.Issue{
		ID:        "epic-1",
		TitleText: "Empty Epic",
		Type:      beads.TypeEpic,
		Children:  []string{}, // No children
	}

	modal, isCascade, issueIDs := CreateDeleteModal(issue, loader)

	require.NotNil(t, modal)
	require.False(t, isCascade, "epic without children should not be cascade")
	require.Equal(t, []string{"epic-1"}, issueIDs)
}
