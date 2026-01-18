package config

import (
	"os"
	"path/filepath"
	"strings"
)

const defaultHistoryBackend = "xorchestrator"

// HistoryBackend returns a normalized backend name with defaults applied.
func HistoryBackend(orch OrchestrationConfig) string {
	if strings.TrimSpace(orch.HistoryBackend) == "" {
		return defaultHistoryBackend
	}
	return orch.HistoryBackend
}

// HistoryPath returns the history snapshot path based on the config location.
// If config is project-local (.xorchestrator/config.yaml), stores history alongside it.
// Otherwise, uses ~/.config/xorchestrator/history.json.
func HistoryPath(configPath string) string {
	home, _ := os.UserHomeDir()
	fallback := filepath.Join(home, ".config", "xorchestrator", "history.json")
	if configPath == "" {
		return fallback
	}

	clean := filepath.Clean(configPath)
	suffix := filepath.Join(".xorchestrator", "config.yaml")
	if strings.HasSuffix(clean, suffix) {
		return filepath.Join(filepath.Dir(clean), "history.json")
	}

	return fallback
}
