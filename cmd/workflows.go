package cmd

import (
	"fmt"

	"github.com/zjrosen/perles/internal/orchestration/workflow"

	"github.com/spf13/cobra"
)

var workflowsCmd = &cobra.Command{
	Use:   "workflows",
	Short: "List available workflow templates",
	Long:  `Display all workflow templates available for orchestration mode, including built-in and user-defined workflows.`,
	RunE:  runWorkflows,
}

func init() {
	rootCmd.AddCommand(workflowsCmd)
}

func runWorkflows(cmd *cobra.Command, args []string) error {
	// Load workflow registry with both built-in and user workflows
	registry, err := workflow.NewRegistryWithBuiltins()
	if err != nil {
		return fmt.Errorf("loading workflows: %w", err)
	}

	// Also load user workflows
	userWorkflows, err := workflow.LoadUserWorkflows()
	if err != nil {
		return fmt.Errorf("loading user workflows: %w", err)
	}
	for _, wf := range userWorkflows {
		registry.Add(wf)
	}

	// Get workflows grouped by source
	builtinWorkflows := registry.ListBySource(workflow.SourceBuiltIn)
	userDefinedWorkflows := registry.ListBySource(workflow.SourceUser)

	// Print built-in workflows
	fmt.Println("Built-in Workflows:")
	if len(builtinWorkflows) == 0 {
		fmt.Println("  (none)")
	} else {
		// Find max name length for alignment
		maxLen := 0
		for _, wf := range builtinWorkflows {
			if len(wf.ID) > maxLen {
				maxLen = len(wf.ID)
			}
		}
		for _, wf := range builtinWorkflows {
			fmt.Printf("  %-*s  %s\n", maxLen, wf.ID, wf.Description)
		}
	}

	fmt.Println()

	// Print user workflows
	userDir := workflow.UserWorkflowDir()
	fmt.Printf("User Workflows (%s):\n", userDir)
	if len(userDefinedWorkflows) == 0 {
		fmt.Println("  (none)")
	} else {
		// Find max name length for alignment
		maxLen := 0
		for _, wf := range userDefinedWorkflows {
			if len(wf.ID) > maxLen {
				maxLen = len(wf.ID)
			}
		}
		for _, wf := range userDefinedWorkflows {
			fmt.Printf("  %-*s  %s\n", maxLen, wf.ID, wf.Description)
		}
	}

	fmt.Println()
	fmt.Println("Use workflows in orchestration mode with Ctrl+P")

	return nil
}
