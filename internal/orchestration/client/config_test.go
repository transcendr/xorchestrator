package client

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtensionKeys_CodexConstants(t *testing.T) {
	// Verify all Codex extension key constants are defined
	require.Equal(t, "codex.model", ExtCodexModel)
	require.Equal(t, "codex.sandbox", ExtCodexSandbox)
	require.Equal(t, "codex.skip_git_check", ExtCodexSkipGitCheck)
}

func TestConfig_CodexModel_Default(t *testing.T) {
	cfg := Config{}
	require.Equal(t, "gpt-5.2-codex", cfg.CodexModel())
}

func TestConfig_CodexModel_NilExtensions(t *testing.T) {
	cfg := Config{Extensions: nil}
	require.Equal(t, "gpt-5.2-codex", cfg.CodexModel())
}

func TestConfig_CodexModel_EmptyExtensions(t *testing.T) {
	cfg := Config{Extensions: map[string]any{}}
	require.Equal(t, "gpt-5.2-codex", cfg.CodexModel())
}

func TestConfig_CodexModel_EmptyString(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtCodexModel: "",
	}}
	require.Equal(t, "gpt-5.2-codex", cfg.CodexModel())
}

func TestConfig_CodexModel_CustomModel(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtCodexModel: "gpt-4o",
	}}
	require.Equal(t, "gpt-4o", cfg.CodexModel())
}

func TestConfig_CodexModel_WrongType(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtCodexModel: 123, // Not a string
	}}
	require.Equal(t, "gpt-5.2-codex", cfg.CodexModel())
}

func TestConfig_CodexModel_ViaSetExtension(t *testing.T) {
	cfg := Config{}
	cfg.SetExtension(ExtCodexModel, "o1")
	require.Equal(t, "o1", cfg.CodexModel())
}

func TestConfig_ClaudeModel_Default(t *testing.T) {
	cfg := Config{}
	require.Equal(t, "opus", cfg.ClaudeModel())
}

func TestConfig_ClaudeModel_CustomModel(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtClaudeModel: "sonnet",
	}}
	require.Equal(t, "sonnet", cfg.ClaudeModel())
}
