// Package joblist implements the JobList TUI domain component.
// It owns job rows state, selected job identity, and jobs-specific view rendering.
package joblist

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	clitui "github.com/iw2rmb/ploy/internal/cli/tui"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

const (
	// ListWidth is the fixed column width for the jobs list panel.
	ListWidth = 30
	// ContentWidth is the text budget per row (excluding list delegate chrome).
	ContentWidth = 26
)

// row is a list.Item implementation for a single job entry.
type row struct {
	title string
	desc  string
}

func (r row) Title() string       { return r.title }
func (r row) Description() string { return r.desc }
func (r row) FilterValue() string { return r.title }

// Model is the JobList domain component. It owns job row state, list cursor,
// confirmed selected job identity, and job detail payload cache for the jobs pane.
type Model struct {
	inner          list.Model
	jobs           []clitui.JobItem
	selectedJobID  domaintypes.JobID
	details        *clitui.JobItem
}

// New creates an initialized JobList with the given title.
func New(title string) Model {
	l := list.New(nil, list.NewDefaultDelegate(), ListWidth, 0)
	l.Title = title
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()
	return Model{inner: l}
}

// SetJobs replaces the job items, rebuilds the list rows, and clears the details cache.
func (m Model) SetJobs(jobs []clitui.JobItem) Model {
	items := make([]list.Item, len(jobs))
	for i, job := range jobs {
		items[i] = row{
			title: renderPrimaryLine(job),
			desc:  renderSecondaryLine(job),
		}
	}
	m.jobs = jobs
	m.details = nil
	m.inner.SetItems(items)
	return m
}

// SelectedJob returns the job at the current cursor position, if any.
func (m Model) SelectedJob() (clitui.JobItem, bool) {
	idx := m.inner.Index()
	if idx < 0 || idx >= len(m.jobs) {
		return clitui.JobItem{}, false
	}
	return m.jobs[idx], true
}

// Select moves the list cursor to the given index.
func (m Model) Select(idx int) Model {
	m.inner.Select(idx)
	return m
}

// SetHeight updates the list height (pointer receiver for in-place mutation).
func (m *Model) SetHeight(h int) {
	m.inner.SetHeight(h)
}

// Title returns the list title.
func (m Model) Title() string { return m.inner.Title }

// Width returns the list panel width.
func (m Model) Width() int { return m.inner.Width() }

// Items returns the current list items, exposing the underlying row slice.
func (m Model) Items() []list.Item { return m.inner.Items() }

// Index returns the current cursor index.
func (m Model) Index() int { return m.inner.Index() }

// Jobs returns the raw job items backing the list.
func (m Model) Jobs() []clitui.JobItem { return m.jobs }

// SelectedJobID returns the job ID of the currently highlighted job.
func (m Model) SelectedJobID() domaintypes.JobID {
	job, ok := m.SelectedJob()
	if !ok {
		return ""
	}
	return job.JobID
}

// SetSelectedJobID records the confirmed job selection (set when the user presses Enter).
func (m Model) SetSelectedJobID(id domaintypes.JobID) Model {
	m.selectedJobID = id
	return m
}

// ConfirmedJobID returns the job ID that was explicitly confirmed via Enter.
// This is distinct from SelectedJobID, which follows the cursor.
func (m Model) ConfirmedJobID() domaintypes.JobID {
	return m.selectedJobID
}

// SetDetails stores the fetched job detail payload for the confirmed selection.
func (m Model) SetDetails(item *clitui.JobItem) Model {
	m.details = item
	return m
}

// Details returns the cached job detail payload, or nil if not yet loaded.
func (m Model) Details() *clitui.JobItem {
	return m.details
}

// Update routes messages into the inner list.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.inner, cmd = m.inner.Update(msg)
	return m, cmd
}

// View renders the jobs list.
func (m Model) View() string { return m.inner.View() }
