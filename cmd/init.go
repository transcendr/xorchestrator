package cmd

import (
	"fmt"
	"os"

	"github.com/zjrosen/xorchestrator/internal/config"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a xorchestrator config file in the current directory",
	Long:  `Creates a .xorchestrator/config.yaml file in the current directory with default settings.`,
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	configPath := ".xorchestrator/config.yaml"

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config file already exists: %s", configPath)
	}

	// Create the config file
	if err := config.WriteDefaultConfig(configPath); err != nil {
		return fmt.Errorf("creating config file: %w", err)
	}

	fmt.Printf("Created %s\n", configPath)
	return nil
}
