// Package playground provides a component showcase and theme token viewer.
package playground

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/xorchestrator/internal/keys"
	"github.com/zjrosen/xorchestrator/internal/ui/shared/panes"
	"github.com/zjrosen/xorchestrator/internal/ui/shared/quitmodal"
	"github.com/zjrosen/xorchestrator/internal/ui/styles"
)

// FocusPane represents which pane has focus.
type FocusPane int

const (
	// FocusSidebar means the sidebar has focus.
	FocusSidebar FocusPane = iota
	// FocusDemo means the demo area has focus.
	FocusDemo
)

// Model holds the playground state.
type Model struct {
	// View state
	focus         FocusPane
	selectedIndex int
	lastAction    string

	// Components
	demos          []ComponentDemo
	demoModel      DemoModel
	demoModelIndex int // tracks which demo is currently loaded

	// Quit confirmation modal
	quitModal quitmodal.Model

	// Dimensions
	width    int
	height   int
	quitting bool
}

// QuitMsg signals that the playground should exit.
type QuitMsg struct{}

// New creates a new playground model.
func New() Model {
	demos := GetComponentDemos()

	m := Model{
		focus:          FocusSidebar,
		selectedIndex:  0,
		demos:          demos,
		demoModelIndex: -1, // no demo loaded yet
		quitModal: quitmodal.New(quitmodal.Config{
			Title:   "Quit Playground",
			Message: "Are you sure you want to exit?",
		}),
	}

	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.EnableMouseCellMotion
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle quit modal first when visible
	if m.quitModal.IsVisible() {
		var cmd tea.Cmd
		var result quitmodal.Result
		m.quitModal, cmd, result = m.quitModal.Update(msg)
		switch result {
		case quitmodal.ResultQuit:
			m.quitting = true
			return m, tea.Quit
		case quitmodal.ResultCancel:
			return m, nil
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.quitModal.SetSize(msg.Width, msg.Height)
		// Initialize or resize demo model
		if m.demoModel != nil {
			demoWidth, demoHeight := m.getDemoAreaDimensions()
			m.demoModel = m.demoModel.SetSize(demoWidth, demoHeight)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case tea.MouseMsg:
		// Forward mouse events to demo model
		if m.demoModel != nil {
			var cmd tea.Cmd
			m.demoModel, cmd, _ = m.demoModel.Update(msg)
			return m, cmd
		}
		return m, nil

	default:
		// Forward other messages to the demo model
		if m.demoModel != nil {
			var cmd tea.Cmd
			var action string
			m.demoModel, cmd, action = m.demoModel.Update(msg)
			if action != "" {
				m.lastAction = action
			}
			return m, cmd
		}
	}

	return m, nil
}

// handleKeyMsg handles keyboard input.
func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Ctrl+C shows quit modal (force-quit when modal visible is handled by quitModal.Update)
	if key == "ctrl+c" {
		m.quitModal.Show()
		return m, nil
	}

	return m.handleComponentListKeys(msg)
}

// handleComponentListKeys handles keys in the component list view.
func (m Model) handleComponentListKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "right":
		if m.focus == FocusSidebar {
			m.focus = FocusDemo
			m.ensureDemoLoaded()
			return m, nil
		}
	case "left":
		if m.focus == FocusDemo {
			m.focus = FocusSidebar
			return m, nil
		}
	}

	// Focus-specific handling
	if m.focus == FocusSidebar {
		return m.handleSidebarKeys(msg)
	}

	return m.handleDemoKeys(msg)
}

// ensureDemoLoaded loads the demo for the current selection if not already loaded.
func (m *Model) ensureDemoLoaded() {
	if m.demoModelIndex != m.selectedIndex && m.selectedIndex < len(m.demos) {
		demoWidth, demoHeight := m.getDemoAreaDimensions()
		m.demoModel = m.demos[m.selectedIndex].Create(demoWidth, demoHeight)
		m.demoModelIndex = m.selectedIndex
	}
}

// handleSidebarKeys handles keys when sidebar is focused.
func (m Model) handleSidebarKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "j", "down":
		m.selectedIndex++
		if m.selectedIndex >= len(m.demos) {
			m.selectedIndex = 0 // Wrap to top
		}
		m.ensureDemoLoaded()
	case "k", "up":
		m.selectedIndex--
		if m.selectedIndex < 0 {
			m.selectedIndex = len(m.demos) - 1 // Wrap to bottom
		}
		m.ensureDemoLoaded()
	case "enter":
		// Switch focus to demo area
		m.ensureDemoLoaded()
		m.focus = FocusDemo
	}

	return m, nil
}

// handleDemoKeys handles keys when demo area is focused.
func (m Model) handleDemoKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Esc returns focus to sidebar (unless demo needs Esc key, e.g., vimtextarea)
	if key.Matches(msg, keys.Common.Escape) && (m.demoModel == nil || !m.demoModel.NeedsEscKey()) {
		m.focus = FocusSidebar
		return m, nil
	}

	// Forward to demo model
	if m.demoModel != nil {
		var cmd tea.Cmd
		var action string
		m.demoModel, cmd, action = m.demoModel.Update(msg)
		if action != "" {
			m.lastAction = action
		}
		return m, cmd
	}

	return m, nil
}

// getDemoAreaDimensions calculates the demo area dimensions.
func (m Model) getDemoAreaDimensions() (int, int) {
	sidebarWidth := m.getSidebarWidth()
	gap := 2
	demoWidth := m.width - sidebarWidth - gap - 4 // -4 for borders
	demoHeight := m.height - 4                    // -4 for footer (1) + borders (2) + newline (1)
	return max(demoWidth, 20), max(demoHeight, 10)
}

// getSidebarWidth returns the sidebar width (30% of total, min 20, max 30).
func (m Model) getSidebarWidth() int {
	w := m.width * 30 / 100
	return max(min(w, 30), 20)
}

// View implements tea.Model.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	content := (&m).renderComponentListView()

	// Overlay quit confirmation modal if showing
	if m.quitModal.IsVisible() {
		return m.quitModal.Overlay(content)
	}

	return content
}

// renderComponentListView renders the main component list view with sidebar + demo area.
func (m *Model) renderComponentListView() string {
	// Ensure a demo is loaded for the current selection
	m.ensureDemoLoaded()

	sidebarWidth := m.getSidebarWidth()
	gap := 2
	demoWidth := m.width - sidebarWidth - gap

	// Calculate content height (leaving room for footer - 1 line)
	contentHeight := m.height - 1

	// Render sidebar
	sidebarContent := renderSidebar(m.demos, m.selectedIndex, sidebarWidth, contentHeight, m.focus == FocusSidebar)
	sidebar := panes.BorderedPane(panes.BorderConfig{
		Content:            sidebarContent,
		Width:              sidebarWidth,
		Height:             contentHeight,
		Focused:            m.focus == FocusSidebar,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})

	// Render demo area
	var demoContent string
	var demoName string
	if m.selectedIndex < len(m.demos) {
		demoName = m.demos[m.selectedIndex].Name
		demoAreaWidth, demoAreaHeight := m.getDemoAreaDimensions()
		demoContent = renderDemoArea(m.demoModel, m.lastAction, demoAreaWidth, demoAreaHeight)
	}

	demoArea := panes.BorderedPane(panes.BorderConfig{
		Content:            demoContent,
		Width:              demoWidth,
		Height:             contentHeight,
		TopLeft:            demoName,
		Focused:            m.focus == FocusDemo,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})

	// Join sidebar and demo area
	gapStr := strings.Repeat(" ", gap)
	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, gapStr, demoArea)

	// Footer - single line, full width
	footerStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor).Width(m.width)
	footer := footerStyle.Render("Ctrl+C: Quit")

	return mainContent + "\n" + footer
}
