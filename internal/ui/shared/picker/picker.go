// Package picker provides a generic option picker component.
package picker

import (
	"strings"

	"github.com/zjrosen/xorchestrator/internal/keys"
	"github.com/zjrosen/xorchestrator/internal/ui/shared/overlay"
	"github.com/zjrosen/xorchestrator/internal/ui/styles"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Option represents a picker option with label and value.
type Option struct {
	Label string
	Value string
	Color lipgloss.TerminalColor // Optional color for the label
}

// Config defines picker configuration with optional callbacks.
type Config struct {
	Title    string
	Options  []Option
	Selected int // Initially selected index

	// OnSelect produces a custom message when an option is selected (Enter pressed).
	// If nil, picker produces SelectMsg{Option: selected}.
	OnSelect func(selected Option) tea.Msg

	// OnCancel produces a custom message when cancelled (Esc/q pressed).
	// If nil, picker produces CancelMsg{}.
	OnCancel func() tea.Msg
}

// SelectMsg is sent when an option is selected (if OnSelect is nil).
type SelectMsg struct {
	Option Option
}

// Model holds the picker state.
type Model struct {
	config         Config
	selected       int
	boxWidth       int // Width of the picker box itself
	viewportWidth  int // Full viewport width for overlay centering
	viewportHeight int // Full viewport height for overlay centering
}

// NewWithConfig creates a new picker with the given configuration.
// This is the preferred constructor for new code.
func NewWithConfig(cfg Config) Model {
	selected := cfg.Selected
	if selected < 0 || selected >= len(cfg.Options) {
		selected = 0
	}
	return Model{
		config:   cfg,
		selected: selected,
	}
}

// New creates a new picker with title and options (legacy constructor).
// Use NewWithConfig for callback support.
func New(title string, options []Option) Model {
	return NewWithConfig(Config{
		Title:   title,
		Options: options,
	})
}

// SetSize sets the viewport dimensions for overlay rendering.
func (m Model) SetSize(width, height int) Model {
	m.viewportWidth = width
	m.viewportHeight = height
	return m
}

// SetBoxWidth sets the width of the picker box itself.
func (m Model) SetBoxWidth(width int) Model {
	m.boxWidth = width
	return m
}

// SetSelected sets the initially selected index.
func (m Model) SetSelected(index int) Model {
	if index >= 0 && index < len(m.config.Options) {
		m.selected = index
	}
	return m
}

// Selected returns the currently selected option.
func (m Model) Selected() Option {
	if m.selected >= 0 && m.selected < len(m.config.Options) {
		return m.config.Options[m.selected]
	}
	return Option{}
}

// Update handles messages including enter/esc.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Common.Down), key.Matches(msg, keys.Component.Next):
			if m.selected < len(m.config.Options)-1 {
				m.selected++
			}
		case key.Matches(msg, keys.Common.Up), key.Matches(msg, keys.Component.Prev):
			if m.selected > 0 {
				m.selected--
			}
		case key.Matches(msg, keys.Common.Enter):
			return m, m.selectCmd()
		case key.Matches(msg, keys.Common.Escape), key.Matches(msg, keys.Common.Quit):
			return m, m.cancelCmd()
		}
	}
	return m, nil
}

// selectCmd returns the appropriate select command.
func (m Model) selectCmd() tea.Cmd {
	selected := m.Selected()
	if m.config.OnSelect != nil {
		return func() tea.Msg { return m.config.OnSelect(selected) }
	}
	return func() tea.Msg { return SelectMsg{Option: selected} }
}

// cancelCmd returns the appropriate cancel command.
func (m Model) cancelCmd() tea.Cmd {
	if m.config.OnCancel != nil {
		return func() tea.Msg { return m.config.OnCancel() }
	}
	return func() tea.Msg { return CancelMsg{} }
}

// View renders the picker box (without positioning).
func (m Model) View() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.OverlayTitleColor).
		PaddingLeft(1)

	// Build picker box (use boxWidth or default to 25)
	width := m.boxWidth
	if width == 0 {
		width = 25
	}

	// Build options
	var options strings.Builder
	for i, opt := range m.config.Options {
		var line string
		if i == m.selected {
			// Selected: white bold "> " prefix, then bold label (with optional color)
			labelStyle := lipgloss.NewStyle().Bold(true)
			if opt.Color != nil {
				labelStyle = labelStyle.Foreground(opt.Color)
			}
			line = styles.SelectionIndicatorStyle.Render(">") + labelStyle.Render(opt.Label)
		} else {
			// Not selected: " " prefix, then label (with optional color)
			labelStyle := lipgloss.NewStyle()
			if opt.Color != nil {
				labelStyle = labelStyle.Foreground(opt.Color)
			}
			line = " " + labelStyle.Render(opt.Label)
		}
		options.WriteString(line)
		if i < len(m.config.Options)-1 {
			options.WriteString("\n")
		}
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.OverlayBorderColor).
		Width(width)

	// Divider spans full width (no padding)
	dividerStyle := lipgloss.NewStyle().Foreground(styles.OverlayBorderColor)
	divider := dividerStyle.Render(strings.Repeat("─", width))
	content := titleStyle.Render(m.config.Title) + "\n" +
		divider + "\n" +
		options.String()

	return boxStyle.Render(content)
}

// Overlay renders the picker on top of a background view.
func (m Model) Overlay(background string) string {
	pickerBox := m.View()

	if background == "" {
		return lipgloss.Place(
			m.viewportWidth, m.viewportHeight,
			lipgloss.Center, lipgloss.Center,
			pickerBox,
		)
	}

	return overlay.Place(overlay.Config{
		Width:    m.viewportWidth,
		Height:   m.viewportHeight,
		Position: overlay.Center,
	}, pickerBox, background)
}

// CancelMsg is sent when the picker is cancelled.
type CancelMsg struct{}

// FindIndexByValue returns the index of the option with the given value.
func FindIndexByValue(options []Option, value string) int {
	for i, opt := range options {
		if opt.Value == value {
			return i
		}
	}
	return 0
}
