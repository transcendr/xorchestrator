// Package kanban implements the kanban board mode controller.
package kanban

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"perles/internal/beads"
	"perles/internal/config"
	"perles/internal/mode"
	"perles/internal/mode/shared"
	"perles/internal/ui/board"
	"perles/internal/ui/coleditor"
	"perles/internal/ui/details"
	"perles/internal/ui/modals/editissue"
	"perles/internal/ui/modals/help"
	"perles/internal/ui/modals/labeleditor"
	"perles/internal/ui/modals/viewmenu"
	"perles/internal/ui/shared/colorpicker"
	"perles/internal/ui/shared/modal"
	"perles/internal/ui/shared/picker"
	"perles/internal/ui/shared/toaster"
	"perles/internal/ui/styles"
)

// ViewMode determines which view is active within the kanban mode.
type ViewMode int

const (
	ViewBoard ViewMode = iota
	ViewDetails
	ViewHelp
	ViewDetailsPriorityPicker
	ViewDetailsStatusPicker
	ViewColumnEditor
	ViewNewViewModal
	ViewDeleteViewModal
	ViewDeleteConfirm
	ViewLabelEditor
	ViewViewMenu
	ViewDeleteColumnModal
	ViewRenameViewModal
	ViewDetailsEditMenu
)

// cursorState tracks the current selection for restoration after refresh.
type cursorState struct {
	column  board.ColumnIndex
	issueID string
}

// Model is the kanban mode state.
type Model struct {
	services mode.Services

	board       board.Model
	details     details.Model
	help        help.Model
	picker      picker.Model
	colEditor   coleditor.Model
	modal       modal.Model
	labelEditor labeleditor.Model
	viewMenu    viewmenu.Model
	editMenu    editissue.Model
	spinner     spinner.Model
	view        ViewMode
	width       int
	height      int
	loading     bool
	err         error
	errContext  string // Context for the error (e.g., "updating status")

	// Currently selected issue for picker operations
	selectedIssue *beads.Issue

	// Delete operation state
	deleteIsCascade     bool // True if deleting an epic with children
	pendingDeleteColumn int  // Index of column to delete, -1 if none

	// Pending cursor restoration after refresh
	pendingCursor *cursorState

	// Refresh state tracking
	autoRefreshed   bool // Set when refresh triggered by file watcher
	manualRefreshed bool // Set when refresh triggered by 'r' key

	// UI visibility toggles
	showStatusBar bool
}

// New creates a new kanban mode controller.
func New(services mode.Services) Model {
	// Apply theme colors from config
	themeCfg := styles.ThemeConfig{
		Preset: services.Config.Theme.Preset,
		Mode:   services.Config.Theme.Mode,
		Colors: services.Config.Theme.Colors,
	}
	_ = styles.ApplyTheme(themeCfg) // Ignore error for now, validation will be added

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.SpinnerColor)

	// Create board - use views if configured, otherwise fall back to columns
	var boardModel board.Model
	if len(services.Config.Views) > 0 {
		boardModel = board.NewFromViews(services.Config.Views, services.Executor).SetShowCounts(services.Config.UI.ShowCounts)
	} else {
		columns := services.Config.GetColumns()
		boardModel = board.NewFromConfigWithExecutor(columns, services.Executor).SetShowCounts(services.Config.UI.ShowCounts)
	}

	return Model{
		services:            services,
		view:                ViewBoard,
		board:               boardModel,
		help:                help.New(),
		spinner:             s,
		loading:             true,
		showStatusBar:       services.Config.UI.ShowStatusBar,
		pendingDeleteColumn: -1,
	}
}

// Init returns initial commands for the mode.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spinner.Tick}

	// Trigger initial column load via BQL
	if loadCmd := m.board.LoadAllColumns(); loadCmd != nil {
		cmds = append(cmds, loadCmd)
	}

	return tea.Batch(cmds...)
}

// Refresh triggers a data reload.
func (m Model) Refresh() tea.Cmd {
	// Note: m.loading is set but doesn't persist (receiver is value type)
	// The actual loading state is managed through the board's LoadAllColumns
	return m.board.InvalidateViews().LoadAllColumns()
}

// RefreshFromConfig rebuilds the board from the current config.
// Use this when columns have been added/removed externally.
func (m Model) RefreshFromConfig() (Model, tea.Cmd) {
	m.rebuildBoard()
	m.loading = true
	return m, m.loadBoardCmd()
}

// SetSize handles terminal resize.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height
	m.board = m.board.SetSize(width, m.boardHeight())
	m.help = m.help.SetSize(width, height)
	// Update details if we're viewing it (handles terminal resize while in details view)
	if m.view == ViewDetails {
		m.details = m.details.SetSize(width, height)
	}
	// Update column editor if we're viewing it
	if m.view == ViewColumnEditor {
		m.colEditor = m.colEditor.SetSize(width, height)
	}
	// Update modal if we're viewing it
	if m.view == ViewNewViewModal || m.view == ViewDeleteViewModal || m.view == ViewDeleteConfirm || m.view == ViewDeleteColumnModal || m.view == ViewRenameViewModal {
		m.modal.SetSize(width, height)
	}
	// Update view menu if we're viewing it
	if m.view == ViewViewMenu {
		m.viewMenu = m.viewMenu.SetSize(width, height)
	}
	return m
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case board.ColumnLoadedMsg:
		return m.handleColumnLoaded(msg)

	case statusChangedMsg:
		return m.handleStatusChanged(msg)

	case priorityChangedMsg:
		return m.handlePriorityChanged(msg)

	case issueDeletedMsg:
		return m.handleIssueDeleted(msg)

	// Open picker messages from details view
	case details.OpenPriorityPickerMsg:
		m.picker = picker.New("Priority", shared.PriorityOptions()).
			SetSize(m.width, m.height).
			SetSelected(int(msg.Current))
		m.selectedIssue = m.getIssueByID(msg.IssueID)
		m.view = ViewDetailsPriorityPicker
		return m, nil

	case details.OpenStatusPickerMsg:
		m.picker = picker.New("Status", shared.StatusOptions()).
			SetSize(m.width, m.height).
			SetSelected(picker.FindIndexByValue(shared.StatusOptions(), string(msg.Current)))
		m.selectedIssue = m.getIssueByID(msg.IssueID)
		m.view = ViewDetailsStatusPicker
		return m, nil

	case details.DeleteIssueMsg:
		issue := m.getIssueByID(msg.IssueID)
		if issue == nil {
			return m, nil
		}
		m.modal, m.deleteIsCascade = shared.CreateDeleteModal(issue, m.services.Client)
		m.modal.SetSize(m.width, m.height)
		m.selectedIssue = issue
		m.view = ViewDeleteConfirm
		return m, m.modal.Init()

	case details.NavigateToDependencyMsg:
		issue := m.getIssueByID(msg.IssueID)
		if issue == nil {
			return m, nil
		}
		// Create new details view for the dependency (pass client for deps and comments)
		m.details = details.New(*issue, m.services.Client, m.services.Client).SetSize(m.width, m.height)
		return m, nil

	case details.OpenLabelEditorMsg:
		m.labelEditor = labeleditor.New(msg.IssueID, msg.Labels).
			SetSize(m.width, m.height)
		m.view = ViewLabelEditor
		return m, m.labelEditor.Init()

	case details.OpenEditMenuMsg:
		m.editMenu = editissue.New().SetSize(m.width, m.height)
		m.selectedIssue = m.getIssueByID(msg.IssueID)
		m.view = ViewDetailsEditMenu
		return m, nil

	case editissue.SelectMsg:
		return m.handleEditMenuSelect(msg)

	case editissue.CancelMsg:
		m.view = ViewDetails
		return m, nil

	case labeleditor.SaveMsg:
		m.view = ViewDetails
		return m, setLabelsCmd(msg.IssueID, msg.Labels)

	case labeleditor.CancelMsg:
		m.view = ViewDetails
		return m, nil

	case labelsChangedMsg:
		return m.handleLabelsChanged(msg)

	case errMsg:
		return m.handleErrMsg(msg)

	case clearErrorMsg:
		m.err = nil
		m.errContext = ""
		return m, nil

	case clearRefreshIndicatorMsg:
		m.autoRefreshed = false
		m.manualRefreshed = false
		return m, nil

	case colorpicker.SelectMsg, colorpicker.CancelMsg:
		// Route colorpicker messages to column editor when it's active
		if m.view == ViewColumnEditor {
			var cmd tea.Cmd
			m.colEditor, cmd = m.colEditor.Update(msg)
			return m, cmd
		}
		return m, nil

	case coleditor.SaveMsg:
		return m.handleColEditorSave(msg)

	case coleditor.DeleteMsg:
		return m.handleColEditorDelete(msg)

	case coleditor.AddMsg:
		return m.handleColEditorAdd(msg)

	case coleditor.CancelMsg:
		m.view = ViewBoard
		return m, nil

	case viewmenu.SelectMsg:
		return m.handleViewMenuSelect(msg)

	case viewmenu.CancelMsg:
		m.view = ViewBoard
		return m, nil

	case modal.SubmitMsg:
		return m.handleModalSubmit(msg)

	case modal.CancelMsg:
		return m.handleModalCancel()
	}

	return m, nil
}

// View renders the kanban mode.
func (m Model) View() string {
	switch m.view {
	case ViewDetails:
		return m.details.View()
	case ViewDetailsPriorityPicker, ViewDetailsStatusPicker:
		// Render picker overlay on top of details view
		return m.picker.Overlay(m.details.View())
	case ViewHelp:
		// Render help overlay on top of board
		bg := m.board.View()
		if m.showStatusBar {
			bg += "\n" + m.renderStatusBar()
		}
		return m.help.Overlay(bg)
	case ViewColumnEditor:
		// Full-screen column editor
		return m.colEditor.View()
	case ViewNewViewModal, ViewDeleteViewModal, ViewRenameViewModal:
		// Render modal overlay on top of board
		bg := m.board.View()
		if m.showStatusBar {
			bg += "\n" + m.renderStatusBar()
		}
		return m.modal.Overlay(bg)
	case ViewDeleteConfirm:
		// Render modal overlay on top of details view
		return m.modal.Overlay(m.details.View())
	case ViewLabelEditor:
		// Render label editor overlay on top of details view
		return m.labelEditor.Overlay(m.details.View())
	case ViewDetailsEditMenu:
		// Render edit menu overlay on top of details view
		return m.editMenu.Overlay(m.details.View())
	case ViewViewMenu:
		// Render view menu overlay on top of board
		bg := m.board.View()
		if m.showStatusBar {
			bg += "\n" + m.renderStatusBar()
		}
		return m.viewMenu.Overlay(bg)
	case ViewDeleteColumnModal:
		// Render delete column modal overlay on top of board
		bg := m.board.View()
		if m.showStatusBar {
			bg += "\n" + m.renderStatusBar()
		}
		return m.modal.Overlay(bg)
	default:
		// Add top margin for spacing
		view := m.board.View()
		if m.showStatusBar {
			view += "\n"
			if m.err != nil {
				view += m.renderErrorBar()
			} else {
				view += m.renderStatusBar()
			}
		}
		return view
	}
}

// Close releases resources held by the kanban mode.
func (m *Model) Close() error {
	// No resources to clean up - app owns the watcher now
	return nil
}

// saveCursor captures the current selection state.
func (m Model) saveCursor() *cursorState {
	selected := m.board.SelectedIssue()
	if selected == nil {
		return nil
	}
	return &cursorState{
		column:  m.board.FocusedColumn(),
		issueID: selected.ID,
	}
}

// restoreCursor attempts to restore selection to the saved issue.
func (m Model) restoreCursor(state *cursorState) Model {
	if state == nil {
		return m
	}

	// Try to find the issue by ID (may have moved columns)
	newBoard, found := m.board.SelectByID(state.issueID)
	if found {
		m.board = newBoard
	} else {
		// Issue not found, stay in same column
		m.board = m.board.SetFocus(state.column)
	}
	return m
}

// boardHeight returns the available height for the board, accounting for status bar.
func (m Model) boardHeight() int {
	if m.showStatusBar {
		return m.height - 1 // Reserve 1 line for status bar
	}
	return m.height
}

// rebuildBoard recreates the board from the current config.
func (m *Model) rebuildBoard() {
	currentView := m.board.CurrentViewIndex()

	if len(m.services.Config.Views) > 0 {
		m.board = board.NewFromViews(m.services.Config.Views, m.services.Executor).
			SetShowCounts(m.services.Config.UI.ShowCounts).
			SetSize(m.width, m.boardHeight())
		// Restore view index if valid
		if currentView > 0 && currentView < m.board.ViewCount() {
			m.board, _ = m.board.SwitchToView(currentView)
		}
	} else {
		m.board = board.NewFromConfigWithExecutor(m.services.Config.GetColumns(), m.services.Executor).
			SetShowCounts(m.services.Config.UI.ShowCounts).
			SetSize(m.width, m.boardHeight())
	}
}

// rebuildBoardWithFocus recreates the board and sets focus to a specific column.
func (m *Model) rebuildBoardWithFocus(focusColumn int) {
	m.rebuildBoard()
	m.board = m.board.SetFocus(focusColumn)
}

// loadBoardCmd returns the appropriate command to load the board.
func (m Model) loadBoardCmd() tea.Cmd {
	if m.board.ViewCount() > 0 {
		return m.board.LoadCurrentViewCmd()
	}
	return m.board.LoadAllColumns()
}

// currentViewIndex returns the current view index, or 0 if no views.
func (m Model) currentViewIndex() int {
	return m.board.CurrentViewIndex()
}

// currentViewColumns returns the columns for the current view.
func (m Model) currentViewColumns() []config.ColumnConfig {
	return m.services.Config.GetColumnsForView(m.currentViewIndex())
}

// configPath returns the config path or default.
func (m Model) configPath() string {
	if m.services.ConfigPath == "" {
		return ".perles.yaml"
	}
	return m.services.ConfigPath
}

// findIssueByIDFromColumns searches loaded columns for an issue by ID.
func (m Model) findIssueByIDFromColumns(id string) *beads.Issue {
	for i := 0; i < m.board.ColCount(); i++ {
		col := m.board.Column(i)
		for _, issue := range col.Items() {
			if issue.ID == id {
				return &issue
			}
		}
	}
	return nil
}

// getIssueByID fetches an issue from the existing columns first.
func (m Model) getIssueByID(id string) *beads.Issue {
	issue := m.findIssueByIDFromColumns(id)
	if issue != nil {
		return issue
	}

	if m.services.Client != nil {
		issues, err := m.services.Client.ListIssuesByIds([]string{id})
		if err == nil && len(issues) == 1 {
			return &issues[0]
		}
	}

	return nil
}

func (m Model) renderStatusBar() string {
	// Build left section with view indicator (if multiple views)
	var content string
	if m.board.ViewCount() > 1 {
		viewName := m.board.CurrentViewName()
		viewNum := m.board.CurrentViewIndex() + 1
		viewTotal := m.board.ViewCount()
		content = fmt.Sprintf("[%s] (%d/%d)", viewName, viewNum, viewTotal)
	}

	return styles.StatusBarStyle.Width(m.width).Render(content)
}

func (m Model) renderErrorBar() string {
	msg := "Error"
	if m.errContext != "" {
		msg += " " + m.errContext
	}
	msg += ": " + m.err.Error() + "  [Press any key to dismiss]"
	return styles.ErrorStyle.Width(m.width).Render(msg)
}

// deleteColumn handles the deletion of a column after modal confirmation.
func (m Model) deleteColumn() (Model, tea.Cmd) {
	colIndex := m.pendingDeleteColumn
	m.pendingDeleteColumn = -1

	if colIndex < 0 {
		m.view = ViewBoard
		return m, nil
	}

	viewIndex := m.currentViewIndex()
	columns := m.currentViewColumns()

	if colIndex >= len(columns) {
		m.view = ViewBoard
		return m, nil
	}

	err := config.DeleteColumnInView(m.configPath(), viewIndex, colIndex, columns, m.services.Config.Views)
	if err != nil {
		m.err = err
		m.errContext = "deleting column"
		m.view = ViewBoard
		return m, scheduleErrorClear()
	}

	// Update in-memory config (remove the column)
	newColumns := append(columns[:colIndex], columns[colIndex+1:]...)
	m.services.Config.SetColumnsForView(viewIndex, newColumns)

	// Rebuild board with new config
	m.rebuildBoard()

	m.view = ViewBoard
	m.loading = true
	cmds := []tea.Cmd{
		m.spinner.Tick,
		func() tea.Msg { return mode.ShowToastMsg{Message: "Column deleted", Style: toaster.StyleSuccess} },
	}
	if loadCmd := m.loadBoardCmd(); loadCmd != nil {
		cmds = append(cmds, loadCmd)
	}
	return m, tea.Batch(cmds...)
}

// handleViewMenuSelect routes view menu selections to appropriate actions.
func (m Model) handleViewMenuSelect(msg viewmenu.SelectMsg) (Model, tea.Cmd) {
	switch msg.Option {
	case viewmenu.OptionCreate:
		// Open new view modal
		m.modal = modal.New(modal.Config{
			Title:          "Create New View",
			ConfirmVariant: modal.ButtonPrimary,
			Inputs: []modal.InputConfig{
				{Key: "name", Label: "View Name", Placeholder: "Enter view name...", MaxLength: 50},
			},
		})
		m.modal.SetSize(m.width, m.height)
		m.view = ViewNewViewModal
		return m, m.modal.Init()

	case viewmenu.OptionDelete:
		// Prevent deletion of last view
		if len(m.services.Config.Views) <= 1 {
			m.view = ViewBoard
			return m, func() tea.Msg {
				return mode.ShowToastMsg{Message: "Cannot delete the only view", Style: toaster.StyleError}
			}
		}

		viewName := m.board.CurrentViewName()
		m.modal = modal.New(modal.Config{
			Title:          "Delete View",
			Message:        fmt.Sprintf("Delete view '%s'? This cannot be undone.", viewName),
			ConfirmVariant: modal.ButtonDanger,
		})
		m.modal.SetSize(m.width, m.height)
		m.view = ViewDeleteViewModal
		return m, m.modal.Init()

	case viewmenu.OptionRename:
		// Open rename modal with current view name pre-filled
		currentViewName := m.board.CurrentViewName()
		m.modal = modal.New(modal.Config{
			Title:          "Rename View",
			ConfirmVariant: modal.ButtonPrimary,
			Inputs: []modal.InputConfig{
				{Key: "name", Label: "View Name", Value: currentViewName, MaxLength: 50},
			},
		})
		m.modal.SetSize(m.width, m.height)
		m.view = ViewRenameViewModal
		return m, m.modal.Init()
	}

	m.view = ViewBoard
	return m, nil
}

// handleEditMenuSelect routes edit menu selections to appropriate actions.
func (m Model) handleEditMenuSelect(msg editissue.SelectMsg) (Model, tea.Cmd) {
	if m.selectedIssue == nil {
		m.view = ViewDetails
		return m, nil
	}

	switch msg.Option {
	case editissue.OptionLabels:
		m.labelEditor = labeleditor.New(m.selectedIssue.ID, m.selectedIssue.Labels).
			SetSize(m.width, m.height)
		m.view = ViewLabelEditor
		return m, m.labelEditor.Init()

	case editissue.OptionPriority:
		m.picker = picker.New("Priority", shared.PriorityOptions()).
			SetSize(m.width, m.height).
			SetSelected(int(m.selectedIssue.Priority))
		m.view = ViewDetailsPriorityPicker
		return m, nil

	case editissue.OptionStatus:
		m.picker = picker.New("Status", shared.StatusOptions()).
			SetSize(m.width, m.height).
			SetSelected(picker.FindIndexByValue(shared.StatusOptions(), string(m.selectedIssue.Status)))
		m.view = ViewDetailsStatusPicker
		return m, nil
	}

	m.view = ViewDetails
	return m, nil
}

// createNewView handles the creation of a new view after modal submission.
func (m Model) createNewView(viewName string) (Model, tea.Cmd) {
	newView := config.ViewConfig{
		Name:    viewName,
		Columns: []config.ColumnConfig{},
	}

	err := config.AddView(m.configPath(), newView, m.services.Config.Views)
	if err != nil {
		m.err = err
		m.errContext = "creating view"
		m.view = ViewBoard
		return m, scheduleErrorClear()
	}

	// Update in-memory config
	m.services.Config.Views = append(m.services.Config.Views, newView)

	// Rebuild board and switch to new view
	m.rebuildBoard()
	newViewIndex := len(m.services.Config.Views) - 1
	m.board, _ = m.board.SwitchToView(newViewIndex)

	m.view = ViewBoard
	m.loading = true
	cmds := []tea.Cmd{
		m.spinner.Tick,
		func() tea.Msg {
			return mode.ShowToastMsg{Message: "Created view: " + viewName, Style: toaster.StyleSuccess}
		},
	}
	if loadCmd := m.board.LoadCurrentViewCmd(); loadCmd != nil {
		cmds = append(cmds, loadCmd)
	}
	return m, tea.Batch(cmds...)
}

// deleteCurrentView handles the deletion of the current view after modal confirmation.
func (m Model) deleteCurrentView() (Model, tea.Cmd) {
	viewIndex := m.board.CurrentViewIndex()
	viewName := m.board.CurrentViewName()

	err := config.DeleteView(m.configPath(), viewIndex, m.services.Config.Views)
	if err != nil {
		m.err = err
		m.errContext = "deleting view"
		m.view = ViewBoard
		return m, scheduleErrorClear()
	}

	// Update in-memory config
	m.services.Config.Views = append(m.services.Config.Views[:viewIndex], m.services.Config.Views[viewIndex+1:]...)

	// Determine which view to switch to
	newViewIndex := max(viewIndex-1, 0)

	// Rebuild board with updated views
	m.view = ViewBoard
	m.rebuildBoard()
	m.board, _ = m.board.SwitchToView(newViewIndex)

	m.loading = true
	cmds := []tea.Cmd{
		m.spinner.Tick,
		func() tea.Msg {
			return mode.ShowToastMsg{Message: "Deleted view: " + viewName, Style: toaster.StyleSuccess}
		},
	}
	if loadCmd := m.board.LoadCurrentViewCmd(); loadCmd != nil {
		cmds = append(cmds, loadCmd)
	}
	return m, tea.Batch(cmds...)
}

// renameCurrentView handles renaming the current view after modal submission.
func (m Model) renameCurrentView(newName string) (Model, tea.Cmd) {
	viewIndex := m.board.CurrentViewIndex()

	err := config.RenameView(m.configPath(), viewIndex, newName, m.services.Config.Views)
	if err != nil {
		m.err = err
		m.errContext = "renaming view"
		m.view = ViewBoard
		return m, scheduleErrorClear()
	}

	m.services.Config.Views[viewIndex].Name = newName
	m.board = m.board.SetCurrentViewName(newName)

	m.view = ViewBoard
	return m, func() tea.Msg {
		return mode.ShowToastMsg{Message: "Renamed view to: " + newName, Style: toaster.StyleSuccess}
	}
}

// Message types

// SwitchToSearchMsg requests switching to search mode with an initial query.
type SwitchToSearchMsg struct {
	Query string
}

type errMsg struct {
	err     error
	context string
}

type clearErrorMsg struct{}

type clearRefreshIndicatorMsg struct{}

type statusChangedMsg struct {
	issueID string
	status  beads.Status
	err     error
}

type priorityChangedMsg struct {
	issueID  string
	priority beads.Priority
	err      error
}

type issueDeletedMsg struct {
	issueID string
	err     error
}

type labelsChangedMsg struct {
	issueID string
	labels  []string
	err     error
}

// Async commands

func updateStatusCmd(issueID string, status beads.Status) tea.Cmd {
	return func() tea.Msg {
		err := beads.UpdateStatus(issueID, status)
		return statusChangedMsg{issueID, status, err}
	}
}

func updatePriorityCmd(issueID string, priority beads.Priority) tea.Cmd {
	return func() tea.Msg {
		err := beads.UpdatePriority(issueID, priority)
		return priorityChangedMsg{issueID, priority, err}
	}
}

func deleteIssueCmd(issueID string, cascade bool) tea.Cmd {
	return func() tea.Msg {
		var err error
		if cascade {
			err = beads.DeleteIssueCascade(issueID)
		} else {
			err = beads.DeleteIssue(issueID)
		}
		return issueDeletedMsg{issueID: issueID, err: err}
	}
}

func setLabelsCmd(issueID string, labels []string) tea.Cmd {
	return func() tea.Msg {
		err := beads.SetLabels(issueID, labels)
		return labelsChangedMsg{issueID: issueID, labels: labels, err: err}
	}
}

func scheduleErrorClear() tea.Cmd {
	return tea.Tick(3*time.Second, func(_ time.Time) tea.Msg {
		return clearErrorMsg{}
	})
}
