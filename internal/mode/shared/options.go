package shared

import (
	"perles/internal/beads"
	"perles/internal/ui/shared/picker"
	"perles/internal/ui/styles"
)

// PriorityOptions returns picker options for priority levels.
func PriorityOptions() []picker.Option {
	return []picker.Option{
		{Label: "P0 - Critical", Value: "P0", Color: styles.PriorityCriticalColor},
		{Label: "P1 - High", Value: "P1", Color: styles.PriorityHighColor},
		{Label: "P2 - Medium", Value: "P2", Color: styles.PriorityMediumColor},
		{Label: "P3 - Low", Value: "P3", Color: styles.PriorityLowColor},
		{Label: "P4 - Backlog", Value: "P4", Color: styles.PriorityBacklogColor},
	}
}

// StatusOptions returns picker options for status values.
func StatusOptions() []picker.Option {
	return []picker.Option{
		{Label: "Open", Value: string(beads.StatusOpen), Color: styles.StatusOpenColor},
		{Label: "In Progress", Value: string(beads.StatusInProgress), Color: styles.StatusInProgressColor},
		{Label: "Closed", Value: string(beads.StatusClosed), Color: styles.StatusClosedColor},
	}
}
