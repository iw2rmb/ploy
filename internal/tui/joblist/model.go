// Package joblist implements the JobList TUI domain component.
// It owns job rows state, selected job identity, and jobs-specific view rendering.
package joblist

import (
	"sort"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

const ListWidth = 50

type row struct {
	job          domainapi.JobListItem
	now          time.Time
	spinnerFrame int
}

func (r row) Title() string       { return renderPrimaryLine(r.job, r.spinnerFrame) }
func (r row) Description() string { return renderDescription(r.job, r.now) }
func (r row) FilterValue() string {
	return r.Title() + " " + r.Description() + " " + r.job.JobID.String()
}

// Model is the JobList domain component. It owns job row state, list cursor,
// and confirmed selected job identity for the jobs pane.
type Model struct {
	inner         list.Model
	jobs          []domainapi.JobListItem
	selectedJobID domaintypes.JobID
}

// New creates an initialized JobList with the given title.
func New(title string) Model {
	l := list.New(nil, newDelegate(true, time.Time{}, 0), ListWidth, 0)
	l.Title = title
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()
	l.Filter = list.UnsortedFilter
	return Model{inner: l}
}

func (m Model) SetJobs(jobs []domainapi.JobListItem) Model {
	m.jobs = append([]domainapi.JobListItem(nil), jobs...)
	sort.SliceStable(m.jobs, func(i, j int) bool {
		runningI := m.jobs[i].Status == domaintypes.JobStatusRunning
		runningJ := m.jobs[j].Status == domaintypes.JobStatusRunning
		if runningI != runningJ {
			return runningI
		}
		a, b := m.jobs[i].StartedAt, m.jobs[j].StartedAt
		if a == nil {
			return false
		}
		if b == nil {
			return true
		}
		return a.After(*b)
	})
	items := make([]list.Item, len(m.jobs))
	for i, job := range m.jobs {
		items[i] = row{job: job}
	}
	m.selectedJobID = ""
	m.inner.ResetFilter()
	m.inner.SetItems(items)
	m.inner.Select(0)
	return m
}

func (m Model) RefreshJobs(jobs []domainapi.JobListItem) Model {
	highlighted, confirmed := m.SelectedJobID(), m.selectedJobID
	index := m.inner.Index()
	state, input := m.inner.FilterState(), m.inner.FilterInput
	m = m.SetJobs(jobs)
	m.refilter(state, input)
	m.restoreHighlight(highlighted, index)
	for _, job := range m.jobs {
		if job.JobID == confirmed {
			m.selectedJobID = confirmed
			break
		}
	}
	return m
}

func (m *Model) refilter(state list.FilterState, input textinput.Model) {
	if state == list.Unfiltered {
		return
	}
	m.inner.SetFilterText(input.Value())
	m.inner.SetFilterState(state)
	m.inner.FilterInput = input
}

func (m *Model) restoreHighlight(id domaintypes.JobID, index int) {
	visible := m.inner.VisibleItems()
	if len(visible) > 0 {
		m.inner.Select(min(index, len(visible)-1))
	}
	for i, item := range visible {
		if item.(row).job.JobID == id {
			m.inner.Select(i)
			break
		}
	}
}

func (m Model) HasActiveJobs() bool {
	for _, job := range m.jobs {
		switch job.Status {
		case domaintypes.JobStatusCreated, domaintypes.JobStatusQueued, domaintypes.JobStatusRunning:
			return true
		}
	}
	return false
}

// SelectedJob returns the job at the current cursor position, if any.
func (m Model) SelectedJob() (domainapi.JobListItem, bool) {
	item, ok := m.inner.SelectedItem().(row)
	if !ok {
		return domainapi.JobListItem{}, false
	}
	return item.job, true
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

func (m *Model) SetWidth(width int) { m.inner.SetWidth(width) }

// Title returns the list title.
func (m Model) Title() string { return m.inner.Title }

// Width returns the list panel width.
func (m Model) Width() int { return m.inner.Width() }

// Items returns the current list items, exposing the underlying row slice.
func (m Model) Items() []list.Item { return m.inner.Items() }

// Index returns the current cursor index.
func (m Model) Index() int { return m.inner.Index() }

func (m Model) SettingFilter() bool { return m.inner.SettingFilter() }

func (m Model) IsFiltered() bool { return m.inner.IsFiltered() }

// Jobs returns the raw job items backing the list.
func (m Model) Jobs() []domainapi.JobListItem { return m.jobs }

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

func (m Model) ConfirmedItem(now time.Time, frame int) (list.DefaultItem, bool) {
	if !m.selectedJobID.IsZero() {
		for _, job := range m.jobs {
			if job.JobID == m.selectedJobID {
				return row{job: job, now: now, spinnerFrame: frame}, true
			}
		}
	}
	return nil, false
}

// Update routes messages into the inner list.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if _, ok := msg.(list.FilterMatchesMsg); ok {
		// Async filter results can contain rows replaced by a newer jobs response.
		// Recompute from current items while retaining the user's input and selection.
		if m.inner.FilterState() != list.Unfiltered {
			highlighted, index := m.SelectedJobID(), m.inner.Index()
			m.refilter(m.inner.FilterState(), m.inner.FilterInput)
			m.restoreHighlight(highlighted, index)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.inner, cmd = m.inner.Update(msg)
	return m, cmd
}

func (m Model) View(focused bool, now time.Time, spinnerFrame int) string {
	if !focused {
		m.inner.Styles.Title = m.inner.Styles.Title.UnsetBackground().UnsetForeground()
	}
	m.inner.SetDelegate(newDelegate(focused, now, spinnerFrame))
	return m.inner.View()
}
