package tree

import (
	"testing"

	"perles/internal/beads"

	"github.com/stretchr/testify/require"
)

// Helper to create test issues
func makeIssue(id string, status beads.Status, children []string, blocks []string, blockedBy []string, parentID string) *beads.Issue {
	return &beads.Issue{
		ID:        id,
		TitleText: "Issue " + id,
		Status:    status,
		Children:  children,
		Blocks:    blocks,
		BlockedBy: blockedBy,
		ParentID:  parentID,
	}
}

// Helper to create test issues with directional discovered-from fields
func makeIssueWithDiscovered(id string, status beads.Status, children []string, blocks []string, blockedBy []string, discoveredFrom []string, discovered []string, parentID string) *beads.Issue {
	return &beads.Issue{
		ID:             id,
		TitleText:      "Issue " + id,
		Status:         status,
		Children:       children,
		Blocks:         blocks,
		BlockedBy:      blockedBy,
		DiscoveredFrom: discoveredFrom,
		Discovered:     discovered,
		ParentID:       parentID,
	}
}

func TestBuildTree_Down_Basic(t *testing.T) {
	// Epic -> Task1, Task2
	issueMap := map[string]*beads.Issue{
		"epic-1": makeIssue("epic-1", beads.StatusOpen, []string{"task-1", "task-2"}, nil, nil, ""),
		"task-1": makeIssue("task-1", beads.StatusClosed, nil, nil, nil, "epic-1"),
		"task-2": makeIssue("task-2", beads.StatusOpen, nil, nil, nil, "epic-1"),
	}

	root, err := BuildTree(issueMap, "epic-1", DirectionDown, ModeDeps)
	require.NoError(t, err)
	require.NotNil(t, root)
	require.Equal(t, "epic-1", root.Issue.ID)
	require.Len(t, root.Children, 2)
	require.Equal(t, 0, root.Depth)
	require.Nil(t, root.Parent)

	// Check children
	require.Equal(t, "task-1", root.Children[0].Issue.ID)
	require.Equal(t, 1, root.Children[0].Depth)
	require.Equal(t, root, root.Children[0].Parent)

	require.Equal(t, "task-2", root.Children[1].Issue.ID)
	require.Equal(t, 1, root.Children[1].Depth)
}

func TestBuildTree_Down_WithBlocks(t *testing.T) {
	// Task blocks another task
	issueMap := map[string]*beads.Issue{
		"task-1": makeIssue("task-1", beads.StatusClosed, nil, []string{"task-2"}, nil, ""),
		"task-2": makeIssue("task-2", beads.StatusOpen, nil, nil, []string{"task-1"}, ""),
	}

	root, err := BuildTree(issueMap, "task-1", DirectionDown, ModeDeps)
	require.NoError(t, err)
	require.Len(t, root.Children, 1)
	require.Equal(t, "task-2", root.Children[0].Issue.ID)
}

func TestBuildTree_Down_Combined(t *testing.T) {
	// Epic with children AND blocks
	issueMap := map[string]*beads.Issue{
		"epic-1":     makeIssue("epic-1", beads.StatusOpen, []string{"task-1"}, []string{"external-1"}, nil, ""),
		"task-1":     makeIssue("task-1", beads.StatusOpen, nil, nil, nil, "epic-1"),
		"external-1": makeIssue("external-1", beads.StatusOpen, nil, nil, []string{"epic-1"}, ""),
	}

	root, err := BuildTree(issueMap, "epic-1", DirectionDown, ModeDeps)
	require.NoError(t, err)
	require.Len(t, root.Children, 2) // Both child and blocked issue
	ids := []string{root.Children[0].Issue.ID, root.Children[1].Issue.ID}
	require.Contains(t, ids, "task-1")
	require.Contains(t, ids, "external-1")
}

func TestBuildTree_Up_Basic(t *testing.T) {
	// Task -> Parent epic
	issueMap := map[string]*beads.Issue{
		"epic-1": makeIssue("epic-1", beads.StatusOpen, []string{"task-1"}, nil, nil, ""),
		"task-1": makeIssue("task-1", beads.StatusOpen, nil, nil, nil, "epic-1"),
	}

	root, err := BuildTree(issueMap, "task-1", DirectionUp, ModeDeps)
	require.NoError(t, err)
	require.Equal(t, "task-1", root.Issue.ID)
	require.Len(t, root.Children, 1)
	require.Equal(t, "epic-1", root.Children[0].Issue.ID)
}

func TestBuildTree_Up_WithBlockedBy(t *testing.T) {
	// Task blocked by another task
	issueMap := map[string]*beads.Issue{
		"task-1":    makeIssue("task-1", beads.StatusOpen, nil, nil, []string{"blocker-1"}, ""),
		"blocker-1": makeIssue("blocker-1", beads.StatusClosed, nil, []string{"task-1"}, nil, ""),
	}

	root, err := BuildTree(issueMap, "task-1", DirectionUp, ModeDeps)
	require.NoError(t, err)
	require.Len(t, root.Children, 1)
	require.Equal(t, "blocker-1", root.Children[0].Issue.ID)
}

func TestBuildTree_Up_Combined(t *testing.T) {
	// Task with parent AND blockedBy
	issueMap := map[string]*beads.Issue{
		"epic-1":    makeIssue("epic-1", beads.StatusOpen, []string{"task-1"}, nil, nil, ""),
		"task-1":    makeIssue("task-1", beads.StatusOpen, nil, nil, []string{"blocker-1"}, "epic-1"),
		"blocker-1": makeIssue("blocker-1", beads.StatusClosed, nil, []string{"task-1"}, nil, ""),
	}

	root, err := BuildTree(issueMap, "task-1", DirectionUp, ModeDeps)
	require.NoError(t, err)
	require.Len(t, root.Children, 2) // Both parent and blocker
	ids := []string{root.Children[0].Issue.ID, root.Children[1].Issue.ID}
	require.Contains(t, ids, "epic-1")
	require.Contains(t, ids, "blocker-1")
}

func TestBuildTree_MissingRoot(t *testing.T) {
	issueMap := map[string]*beads.Issue{
		"task-1": makeIssue("task-1", beads.StatusOpen, nil, nil, nil, ""),
	}

	_, err := BuildTree(issueMap, "nonexistent", DirectionDown, ModeDeps)
	require.Error(t, err)
	require.Contains(t, err.Error(), "root issue nonexistent not found")
}

func TestBuildTree_CycleDetection(t *testing.T) {
	// Create a cycle: A -> B -> C -> A
	issueMap := map[string]*beads.Issue{
		"a": makeIssue("a", beads.StatusOpen, []string{"b"}, nil, nil, ""),
		"b": makeIssue("b", beads.StatusOpen, []string{"c"}, nil, nil, "a"),
		"c": makeIssue("c", beads.StatusOpen, []string{"a"}, nil, nil, "b"), // Cycle back to A
	}

	// Should not infinite loop - cycle detection should prevent
	root, err := BuildTree(issueMap, "a", DirectionDown, ModeDeps)
	require.NoError(t, err)
	require.NotNil(t, root)

	// Tree should have a -> b -> c, but c should NOT have a as child (cycle prevented)
	require.Len(t, root.Children, 1) // b
	require.Equal(t, "b", root.Children[0].Issue.ID)
	require.Len(t, root.Children[0].Children, 1) // c
	require.Equal(t, "c", root.Children[0].Children[0].Issue.ID)
	require.Empty(t, root.Children[0].Children[0].Children) // No children - cycle stopped
}

func TestBuildTree_MissingRelated(t *testing.T) {
	// Issue references a child not in the map (shouldn't crash)
	issueMap := map[string]*beads.Issue{
		"epic-1": makeIssue("epic-1", beads.StatusOpen, []string{"missing-task"}, nil, nil, ""),
	}

	root, err := BuildTree(issueMap, "epic-1", DirectionDown, ModeDeps)
	require.NoError(t, err)
	require.Empty(t, root.Children) // Missing reference skipped
}

func TestBuildTree_DeepTree(t *testing.T) {
	// Create 5-level deep tree
	issueMap := map[string]*beads.Issue{
		"l0": makeIssue("l0", beads.StatusOpen, []string{"l1"}, nil, nil, ""),
		"l1": makeIssue("l1", beads.StatusOpen, []string{"l2"}, nil, nil, "l0"),
		"l2": makeIssue("l2", beads.StatusOpen, []string{"l3"}, nil, nil, "l1"),
		"l3": makeIssue("l3", beads.StatusOpen, []string{"l4"}, nil, nil, "l2"),
		"l4": makeIssue("l4", beads.StatusOpen, nil, nil, nil, "l3"),
	}

	root, err := BuildTree(issueMap, "l0", DirectionDown, ModeDeps)
	require.NoError(t, err)

	// Traverse and verify depths
	node := root
	for depth := 0; depth <= 4; depth++ {
		require.Equal(t, depth, node.Depth)
		if depth < 4 {
			require.Len(t, node.Children, 1)
			node = node.Children[0]
		}
	}
}

func TestFlatten(t *testing.T) {
	issueMap := map[string]*beads.Issue{
		"root": makeIssue("root", beads.StatusOpen, []string{"a", "b"}, nil, nil, ""),
		"a":    makeIssue("a", beads.StatusOpen, []string{"a1"}, nil, nil, "root"),
		"b":    makeIssue("b", beads.StatusOpen, nil, nil, nil, "root"),
		"a1":   makeIssue("a1", beads.StatusOpen, nil, nil, nil, "a"),
	}

	root, _ := BuildTree(issueMap, "root", DirectionDown, ModeDeps)
	flat := root.Flatten()

	require.Len(t, flat, 4)
	require.Equal(t, "root", flat[0].Issue.ID)
	require.Equal(t, "a", flat[1].Issue.ID)
	require.Equal(t, "a1", flat[2].Issue.ID)
	require.Equal(t, "b", flat[3].Issue.ID)
}

func TestCalculateProgress_AllOpen(t *testing.T) {
	issueMap := map[string]*beads.Issue{
		"root": makeIssue("root", beads.StatusOpen, []string{"a", "b"}, nil, nil, ""),
		"a":    makeIssue("a", beads.StatusOpen, nil, nil, nil, "root"),
		"b":    makeIssue("b", beads.StatusOpen, nil, nil, nil, "root"),
	}

	root, _ := BuildTree(issueMap, "root", DirectionDown, ModeDeps)
	closed, total := root.CalculateProgress()

	require.Equal(t, 0, closed)
	require.Equal(t, 3, total)
}

func TestCalculateProgress_AllClosed(t *testing.T) {
	issueMap := map[string]*beads.Issue{
		"root": makeIssue("root", beads.StatusClosed, []string{"a", "b"}, nil, nil, ""),
		"a":    makeIssue("a", beads.StatusClosed, nil, nil, nil, "root"),
		"b":    makeIssue("b", beads.StatusClosed, nil, nil, nil, "root"),
	}

	root, _ := BuildTree(issueMap, "root", DirectionDown, ModeDeps)
	closed, total := root.CalculateProgress()

	require.Equal(t, 3, closed)
	require.Equal(t, 3, total)
}

func TestCalculateProgress_Mixed(t *testing.T) {
	issueMap := map[string]*beads.Issue{
		"root": makeIssue("root", beads.StatusOpen, []string{"a", "b", "c"}, nil, nil, ""),
		"a":    makeIssue("a", beads.StatusClosed, nil, nil, nil, "root"),
		"b":    makeIssue("b", beads.StatusOpen, nil, nil, nil, "root"),
		"c":    makeIssue("c", beads.StatusClosed, nil, nil, nil, "root"),
	}

	root, _ := BuildTree(issueMap, "root", DirectionDown, ModeDeps)
	closed, total := root.CalculateProgress()

	require.Equal(t, 2, closed)
	require.Equal(t, 4, total)
}

func TestCalculateProgress_Nested(t *testing.T) {
	issueMap := map[string]*beads.Issue{
		"root": makeIssue("root", beads.StatusOpen, []string{"a"}, nil, nil, ""),
		"a":    makeIssue("a", beads.StatusClosed, []string{"b"}, nil, nil, "root"),
		"b":    makeIssue("b", beads.StatusClosed, nil, nil, nil, "a"),
	}

	root, _ := BuildTree(issueMap, "root", DirectionDown, ModeDeps)
	closed, total := root.CalculateProgress()

	require.Equal(t, 2, closed)
	require.Equal(t, 3, total)
}

func TestDirection_String(t *testing.T) {
	require.Equal(t, "down", DirectionDown.String())
	require.Equal(t, "up", DirectionUp.String())
}

func TestBuildTree_Down_SiblingBlocking(t *testing.T) {
	// Epic with two task children where one blocks the other.
	// task-impl blocks task-tests (tests must wait for impl).
	// In the tree, task-impl should appear first and task-tests
	// should be nested under it (not a sibling).
	issueMap := map[string]*beads.Issue{
		"epic-1":     makeIssue("epic-1", beads.StatusOpen, []string{"task-tests", "task-impl"}, nil, nil, ""),
		"task-impl":  makeIssue("task-impl", beads.StatusOpen, nil, []string{"task-tests"}, nil, "epic-1"),
		"task-tests": makeIssue("task-tests", beads.StatusOpen, nil, nil, []string{"task-impl"}, "epic-1"),
	}

	root, err := BuildTree(issueMap, "epic-1", DirectionDown, ModeDeps)
	require.NoError(t, err)
	require.Equal(t, "epic-1", root.Issue.ID)

	// The epic should have only ONE direct child: task-impl (the blocker)
	// task-tests should be a child of task-impl, not a direct child of epic
	require.Len(t, root.Children, 1, "epic should have 1 direct child (the blocker)")
	require.Equal(t, "task-impl", root.Children[0].Issue.ID)

	// task-impl should have task-tests as its child (via blocking relationship)
	require.Len(t, root.Children[0].Children, 1, "blocker should have blocked issue as child")
	require.Equal(t, "task-tests", root.Children[0].Children[0].Issue.ID)

	// Verify depths
	require.Equal(t, 0, root.Depth)
	require.Equal(t, 1, root.Children[0].Depth)
	require.Equal(t, 2, root.Children[0].Children[0].Depth)
}

func TestBuildTree_Down_ChainedBlocking(t *testing.T) {
	// Epic with three tasks in a blocking chain: A -> B -> C
	// All are children of epic, but the tree should show the chain.
	issueMap := map[string]*beads.Issue{
		"epic":   makeIssue("epic", beads.StatusOpen, []string{"task-c", "task-b", "task-a"}, nil, nil, ""),
		"task-a": makeIssue("task-a", beads.StatusOpen, nil, []string{"task-b"}, nil, "epic"),
		"task-b": makeIssue("task-b", beads.StatusOpen, nil, []string{"task-c"}, []string{"task-a"}, "epic"),
		"task-c": makeIssue("task-c", beads.StatusOpen, nil, nil, []string{"task-b"}, "epic"),
	}

	root, err := BuildTree(issueMap, "epic", DirectionDown, ModeDeps)
	require.NoError(t, err)

	// Epic should have only task-a as direct child (first in chain)
	require.Len(t, root.Children, 1, "epic should have 1 direct child")
	require.Equal(t, "task-a", root.Children[0].Issue.ID)

	// task-a -> task-b
	require.Len(t, root.Children[0].Children, 1)
	require.Equal(t, "task-b", root.Children[0].Children[0].Issue.ID)

	// task-b -> task-c
	require.Len(t, root.Children[0].Children[0].Children, 1)
	require.Equal(t, "task-c", root.Children[0].Children[0].Children[0].Issue.ID)
}

func TestBuildTree_Down_WithDiscovered(t *testing.T) {
	// Issue with discovered-from relationships
	// bug-1 was discovered while working on feature-1 (discovered-from relationship)
	// Down traversal from feature-1 shows bug-1 via Discovered field
	issueMap := map[string]*beads.Issue{
		"feature-1": makeIssueWithDiscovered("feature-1", beads.StatusOpen, nil, nil, nil, nil, []string{"bug-1"}, ""),
		"bug-1":     makeIssueWithDiscovered("bug-1", beads.StatusOpen, nil, nil, nil, []string{"feature-1"}, nil, ""),
	}

	root, err := BuildTree(issueMap, "feature-1", DirectionDown, ModeDeps)
	require.NoError(t, err)
	require.Len(t, root.Children, 1)
	require.Equal(t, "bug-1", root.Children[0].Issue.ID)
}

func TestBuildTree_Up_WithDiscoveredFrom(t *testing.T) {
	// Issue with discovered-from relationships (up direction)
	// bug-1 was discovered from feature-1, so traversing up from bug-1 shows feature-1
	// Up traversal from bug-1 shows feature-1 via DiscoveredFrom field
	issueMap := map[string]*beads.Issue{
		"feature-1": makeIssueWithDiscovered("feature-1", beads.StatusOpen, nil, nil, nil, nil, []string{"bug-1"}, ""),
		"bug-1":     makeIssueWithDiscovered("bug-1", beads.StatusOpen, nil, nil, nil, []string{"feature-1"}, nil, ""),
	}

	root, err := BuildTree(issueMap, "bug-1", DirectionUp, ModeDeps)
	require.NoError(t, err)
	require.Len(t, root.Children, 1)
	require.Equal(t, "feature-1", root.Children[0].Issue.ID)
}

func TestBuildTree_Down_DiscoveredNotInChildrenMode(t *testing.T) {
	// Discovered issues should NOT appear in ModeChildren (only parent-child hierarchy)
	issueMap := map[string]*beads.Issue{
		"feature-1": makeIssueWithDiscovered("feature-1", beads.StatusOpen, nil, nil, nil, nil, []string{"bug-1"}, ""),
		"bug-1":     makeIssueWithDiscovered("bug-1", beads.StatusOpen, nil, nil, nil, []string{"feature-1"}, nil, ""),
	}

	root, err := BuildTree(issueMap, "feature-1", DirectionDown, ModeChildren)
	require.NoError(t, err)
	// No children in children-only mode since there's no parent-child relationship
	require.Empty(t, root.Children)
}

func TestBuildTree_Down_CombinedDiscoveredAndBlocks(t *testing.T) {
	// Issue with both blocks and discovered-from relationships
	issueMap := map[string]*beads.Issue{
		"feature-1": makeIssueWithDiscovered("feature-1", beads.StatusOpen, nil, []string{"feature-2"}, nil, nil, []string{"bug-1"}, ""),
		"feature-2": makeIssue("feature-2", beads.StatusOpen, nil, nil, []string{"feature-1"}, ""),
		"bug-1":     makeIssueWithDiscovered("bug-1", beads.StatusOpen, nil, nil, nil, []string{"feature-1"}, nil, ""),
	}

	root, err := BuildTree(issueMap, "feature-1", DirectionDown, ModeDeps)
	require.NoError(t, err)
	require.Len(t, root.Children, 2) // Both blocked issue and discovered issue
	ids := []string{root.Children[0].Issue.ID, root.Children[1].Issue.ID}
	require.Contains(t, ids, "feature-2") // blocked by feature-1
	require.Contains(t, ids, "bug-1")     // discovered from feature-1
}
