// Package nobeads provides the empty state view shown when no .beads directory exists.
package nobeads

import (
	"strings"

	"github.com/zjrosen/xorchestrator/internal/keys"
	"github.com/zjrosen/xorchestrator/internal/ui/shared/chainart"
	"github.com/zjrosen/xorchestrator/internal/ui/styles"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model holds the nobeads view state.
type Model struct {
	width  int
	height int
}

// New creates a new nobeads view.
func New() Model {
	return Model{}
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Common.Quit), key.Matches(msg, keys.Common.Escape):
			return m, tea.Quit
		}
	}
	return m, nil
}

// View renders the empty state.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	art := chainart.BuildChainArt()

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.TextPrimaryColor).
		MarginTop(1)

	messageStyle := lipgloss.NewStyle().
		Foreground(styles.TextDescriptionColor)

	hintStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor).
		Italic(true).
		MarginTop(2)

	// Build content
	var content strings.Builder

	content.WriteString(art)
	content.WriteString("\n\n")
	content.WriteString(titleStyle.Render("Oh no! Looks like there's a break in the chain!"))
	content.WriteString("\n\n")
	content.WriteString(messageStyle.Render("No .beads directory found in the current directory."))
	content.WriteString("\n\n")
	content.WriteString(messageStyle.Render("Try one of these options:"))
	content.WriteString("\n\n")
	content.WriteString(messageStyle.Render("  1. (Recommended) Run xorchestrator from a directory containing .beads/"))
	content.WriteString("\n")
	content.WriteString(messageStyle.Render("  2. Use the --beads-dir flag: xorchestrator --beads-dir /path/to/project"))
	content.WriteString("\n")
	content.WriteString(messageStyle.Render("  3. Run 'xorchestrator init' to create a local config file, then set beads_dir"))
	content.WriteString("\n")
	content.WriteString(messageStyle.Render("  4. Set beads_dir in your config file (~/.config/xorchestrator/config.yaml)"))
	content.WriteString("\n\n")
	content.WriteString(hintStyle.Render("Press q to quit"))

	// Center the content
	containerStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center)

	return containerStyle.Render(content.String())
}

// SetSize updates the view dimensions.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height
	return m
}
