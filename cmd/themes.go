package cmd

import (
	"fmt"
	"sort"

	"github.com/zjrosen/xorchestrator/internal/ui/styles"

	"github.com/spf13/cobra"
)

var themesCmd = &cobra.Command{
	Use:   "themes",
	Short: "List available theme presets",
	Long:  `Display all built-in theme presets that can be used in your config file.`,
	Run:   runThemes,
}

func init() {
	rootCmd.AddCommand(themesCmd)
}

func runThemes(cmd *cobra.Command, args []string) {
	fmt.Println("Available theme presets:")
	fmt.Println()

	// Sort preset names for consistent output
	names := make([]string, 0, len(styles.Presets))
	for name := range styles.Presets {
		names = append(names, name)
	}
	sort.Strings(names)

	// Find max name length for alignment
	maxLen := 0
	for _, name := range names {
		if len(name) > maxLen {
			maxLen = len(name)
		}
	}

	// Print each preset
	for _, name := range names {
		preset := styles.Presets[name]
		fmt.Printf("  %-*s  %s\n", maxLen, name, preset.Description)
	}

	fmt.Println()
	fmt.Println("Use in config.yaml:")
	fmt.Println("  theme:")
	fmt.Println("    preset: catppuccin-mocha")
	fmt.Println()
	fmt.Println("Override specific colors:")
	fmt.Println("  theme:")
	fmt.Println("    preset: dracula")
	fmt.Println("    colors:")
	fmt.Println("      status.error: \"#FF0000\"")
}
