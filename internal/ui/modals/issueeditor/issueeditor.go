// Package issueeditor provides a unified modal for editing issue properties.
//
// This modal combines priority, status, and labels editing into a single form,
// replacing the previous three-modal architecture with a streamlined interface.
package issueeditor

import (
	"strconv"

	"github.com/zjrosen/xorchestrator/internal/beads"
	"github.com/zjrosen/xorchestrator/internal/mode/shared"
	"github.com/zjrosen/xorchestrator/internal/ui/shared/formmodal"
	"github.com/zjrosen/xorchestrator/internal/ui/shared/issuebadge"

	tea "github.com/charmbracelet/bubbletea"
)

// Model holds the issue editor state.
type Model struct {
	issue beads.Issue
	form  formmodal.Model
}

// SaveMsg is sent when the user confirms issue changes.
type SaveMsg struct {
	IssueID  string
	Priority beads.Priority
	Status   beads.Status
	Labels   []string
}

// CancelMsg is sent when the user cancels the editor.
type CancelMsg struct{}

// New creates a new issue editor with the given issue.
func New(issue beads.Issue) Model {
	m := Model{issue: issue}

	cfg := formmodal.FormConfig{
		Title: "Edit Issue",
		TitleContent: func(width int) string {
			return issuebadge.RenderBadge(m.issue)
		},
		Fields: []formmodal.FieldConfig{
			{
				Key:     "priority",
				Type:    formmodal.FieldTypeSelect,
				Label:   "Priority",
				Hint:    "Space to toggle",
				Options: priorityListOptions(issue.Priority),
			},
			{
				Key:     "status",
				Type:    formmodal.FieldTypeSelect,
				Label:   "Status",
				Hint:    "Space to toggle",
				Options: statusListOptions(issue.Status),
			},
			{
				Key:              "labels",
				Type:             formmodal.FieldTypeEditableList,
				Label:            "Labels",
				Hint:             "Space to toggle",
				Options:          labelsListOptions(issue.Labels),
				InputLabel:       "Add Label",
				InputHint:        "Enter to add",
				InputPlaceholder: "Enter label name...",
			},
		},
		SubmitLabel: "Save",
		MinWidth:    52,
		OnSubmit: func(values map[string]any) tea.Msg {
			return SaveMsg{
				IssueID:  m.issue.ID,
				Priority: parsePriority(values["priority"].(string)),
				Status:   beads.Status(values["status"].(string)),
				Labels:   values["labels"].([]string),
			}
		},
		OnCancel: func() tea.Msg { return CancelMsg{} },
	}

	m.form = formmodal.New(cfg)
	return m
}

// priorityListOptions converts shared.PriorityOptions to formmodal.ListOption
// with the current priority pre-selected, preserving colors.
func priorityListOptions(current beads.Priority) []formmodal.ListOption {
	opts := shared.PriorityOptions()
	result := make([]formmodal.ListOption, len(opts))
	for i, opt := range opts {
		result[i] = formmodal.ListOption{
			Label:    opt.Label,
			Value:    opt.Value,
			Selected: i == int(current),
			Color:    opt.Color,
		}
	}
	return result
}

// statusListOptions converts shared.StatusOptions to formmodal.ListOption
// with the current status pre-selected, preserving colors.
func statusListOptions(current beads.Status) []formmodal.ListOption {
	opts := shared.StatusOptions()
	result := make([]formmodal.ListOption, len(opts))
	for i, opt := range opts {
		result[i] = formmodal.ListOption{
			Label:    opt.Label,
			Value:    opt.Value,
			Selected: opt.Value == string(current),
			Color:    opt.Color,
		}
	}
	return result
}

// labelsListOptions converts a slice of labels to formmodal.ListOption
// with all labels initially selected.
func labelsListOptions(labels []string) []formmodal.ListOption {
	result := make([]formmodal.ListOption, len(labels))
	for i, label := range labels {
		result[i] = formmodal.ListOption{
			Label:    label,
			Value:    label,
			Selected: true,
		}
	}
	return result
}

// parsePriority parses a priority string value (e.g., "P0") to beads.Priority.
func parsePriority(value string) beads.Priority {
	if len(value) >= 2 && value[0] == 'P' {
		if p, err := strconv.Atoi(value[1:]); err == nil && p >= 0 && p <= 4 {
			return beads.Priority(p)
		}
	}
	return beads.PriorityMedium // default to medium if parsing fails
}

// SetSize sets the viewport dimensions for overlay rendering.
func (m Model) SetSize(width, height int) Model {
	m.form = m.form.SetSize(width, height)
	return m
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.form, cmd = m.form.Update(msg)
	return m, cmd
}

// View renders the issue editor modal.
func (m Model) View() string {
	return m.form.View()
}

// Overlay renders the issue editor on top of a background view.
func (m Model) Overlay(background string) string {
	return m.form.Overlay(background)
}
