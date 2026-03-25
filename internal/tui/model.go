package tui

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	clitui "github.com/iw2rmb/ploy/internal/cli/tui"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// Screen represents the current TUI state in the six-screen navigation contract.
type Screen int

const (
	ScreenRoot             Screen = iota // PLOY root selector
	ScreenMigrationsList                 // PLOY | MIGRATIONS
	ScreenMigrationDetails               // MIGRATION <name>
	ScreenRunsList                       // PLOY | RUNS
	ScreenRunDetails                     // RUN
	ScreenJobsList                       // PLOY | JOBS
)

// listWidth is the fixed width applied to standard lists in the TUI.
const listWidth = 24

// jobsListWidth is the fixed width for JOBS items in the right panel.
const jobsListWidth = 48

// listItem is the default item type used in all TUI lists.
type listItem struct {
	title       string
	description string
}

func (i listItem) Title() string       { return i.title }
func (i listItem) Description() string { return i.description }
func (i listItem) FilterValue() string { return i.title }

// model holds all TUI state. It implements tea.Model.
type model struct {
	screen Screen

	// ploy is the root list (PLOY), always rendered in S1 and as the left
	// panel in S2/S4/S6.
	ploy list.Model

	// secondary is the right-panel list rendered in S2 (MIGRATIONS), S4
	// (RUNS), and S6 (JOBS).
	secondary list.Model

	// detail is the single-list rendered in S3 (migration details) and S5
	// (run details).
	detail list.Model

	// selectedMigID tracks the migration chosen in S2 for drill-down to S3.
	selectedMigID domaintypes.MigID

	// selectedRunID tracks the run chosen in S4 for drill-down to S5.
	selectedRunID domaintypes.RunID

	// client and baseURL are used to fetch list data via internal/cli/tui commands.
	client  *http.Client
	baseURL *url.URL

	// windowHeight tracks the latest terminal height so every list can match it.
	windowHeight int
}

// newListWithWidth creates a list with the shared TUI invariants applied:
// - help disabled
// - quit keybindings disabled
func newListWithWidth(title string, items []list.Item, width int) list.Model {
	l := list.New(items, list.NewDefaultDelegate(), width, 0)
	l.Title = title
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()
	return l
}

// newList creates a standard-width list (24 columns).
func newList(title string, items []list.Item) list.Model {
	return newListWithWidth(title, items, listWidth)
}

// newJobsList creates a jobs list with a 48-column width.
func newJobsList(title string, items []list.Item) list.Model {
	return newListWithWidth(title, items, jobsListWidth)
}

// setWindowHeight updates cached terminal height and applies it to all lists.
func (m *model) setWindowHeight(height int) {
	if height <= 0 {
		return
	}
	m.windowHeight = height
	m.applyWindowHeight()
}

// applyWindowHeight reapplies the cached terminal height to all lists.
func (m *model) applyWindowHeight() {
	if m.windowHeight <= 0 {
		return
	}
	m.ploy.SetHeight(m.windowHeight)
	m.secondary.SetHeight(m.windowHeight)
	m.detail.SetHeight(m.windowHeight)
}

// ployItems are the fixed root-level items for the PLOY list.
var ployItems = []list.Item{
	listItem{title: "Migrations", description: "select migration"},
	listItem{title: "Runs", description: "select run"},
	listItem{title: "Jobs", description: "select job"},
}

// InitialModel constructs the starting model for the TUI session.
func InitialModel(client *http.Client, baseURL *url.URL) model {
	ploy := newList("PLOY", ployItems)
	ploy.SetFilteringEnabled(false)

	return model{
		screen:    ScreenRoot,
		ploy:      ploy,
		secondary: newList("", nil),
		detail:    newList("", nil),
		client:    client,
		baseURL:   baseURL,
	}
}

// Init satisfies tea.Model. The base shell issues no async commands on start.
func (m model) Init() tea.Cmd {
	return nil
}

// Update satisfies tea.Model and implements the six-screen navigation contract.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			return m.handleEnter()
		case "esc":
			return m.handleEsc()
		}

	case tea.WindowSizeMsg:
		m.setWindowHeight(msg.Height)
		return m, nil

	case migsLoadedMsg:
		sorted := make([]clitui.MigItem, len(msg.migs))
		copy(sorted, msg.migs)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].CreatedAt > sorted[j].CreatedAt
		})
		items := make([]list.Item, len(sorted))
		for i, mig := range sorted {
			items[i] = listItem{
				title:       mig.Name,
				description: mig.ID.String(),
			}
		}
		m.secondary = newList("MIGRATIONS", items)
		m.applyWindowHeight()
		return m, nil

	case runsLoadedMsg:
		sorted := make([]runSummary, len(msg.runs))
		copy(sorted, msg.runs)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
		})
		items := make([]list.Item, len(sorted))
		for i, run := range sorted {
			items[i] = listItem{
				title:       run.ID.String(),
				description: run.MigName + "  " + run.CreatedAt.Format("02 01 15:04"),
			}
		}
		m.secondary = newList("RUNS", items)
		m.applyWindowHeight()
		return m, nil

	case migDetailsLoadedMsg:
		items := m.detail.Items()
		if len(items) >= 2 {
			items[0] = listItem{title: "repositories", description: fmt.Sprintf("total: %d", msg.repoTotal)}
			items[1] = listItem{title: "runs", description: fmt.Sprintf("total: %d", msg.runTotal)}
			m.detail.SetItems(items)
		}
		return m, nil

	case runDetailsLoadedMsg:
		items := m.detail.Items()
		if len(items) >= 2 {
			items[0] = listItem{title: "Repositories", description: fmt.Sprintf("total: %d", msg.repoTotal)}
			items[1] = listItem{title: "Jobs", description: fmt.Sprintf("total: %d", msg.jobTotal)}
			m.detail.SetItems(items)
		}
		return m, nil

	case jobsLoadedMsg:
		items := make([]list.Item, len(msg.jobs))
		for i, job := range msg.jobs {
			items[i] = listItem{
				title:       renderJobsPrimaryLine(job),
				description: renderJobsSecondaryLine(job),
			}
		}
		m.secondary = newJobsList("JOBS", items)
		m.applyWindowHeight()
		return m, nil
	}

	// Delegate key input to the active list.
	var cmd tea.Cmd
	switch m.screen {
	case ScreenRoot:
		m.ploy, cmd = m.ploy.Update(msg)
	case ScreenMigrationsList:
		m.secondary, cmd = m.secondary.Update(msg)
	case ScreenMigrationDetails:
		m.detail, cmd = m.detail.Update(msg)
	case ScreenRunsList:
		m.secondary, cmd = m.secondary.Update(msg)
	case ScreenRunDetails:
		m.detail, cmd = m.detail.Update(msg)
	case ScreenJobsList:
		m.secondary, cmd = m.secondary.Update(msg)
	}
	return m, cmd
}

// handleEnter implements Enter-key transitions per the state machine contract.
func (m model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenRoot:
		switch m.ploy.Index() {
		case 0: // Migrations
			m.screen = ScreenMigrationsList
			m.secondary = newList("MIGRATIONS", nil)
			m.applyWindowHeight()
			return m, loadMigsCmd(m.client, m.baseURL)
		case 1: // Runs
			m.screen = ScreenRunsList
			m.secondary = newList("RUNS", nil)
			m.applyWindowHeight()
			return m, loadRunsCmd(m.client, m.baseURL)
		case 2: // Jobs
			m.screen = ScreenJobsList
			m.secondary = newJobsList("JOBS", nil)
			m.applyWindowHeight()
			return m, loadJobsCmd(m.client, m.baseURL, nil)
		}
	case ScreenMigrationsList:
		if item, ok := m.secondary.SelectedItem().(listItem); ok {
			m.selectedMigID = domaintypes.MigID(item.description)
			label := item.title
			if label == "" {
				label = item.description
			}
			title := "MIGRATION " + label
			m.detail = newList(title, []list.Item{
				listItem{title: "repositories", description: "total: —"},
				listItem{title: "runs", description: "total: —"},
			})
			m.applyWindowHeight()
			m.screen = ScreenMigrationDetails
			return m, loadMigDetailsCmd(m.client, m.baseURL, m.selectedMigID)
		}
	case ScreenRunsList:
		if item, ok := m.secondary.SelectedItem().(listItem); ok {
			m.selectedRunID = domaintypes.RunID(item.title)
			m.detail = newList("RUN", []list.Item{
				listItem{title: "Repositories", description: "total: —"},
				listItem{title: "Jobs", description: "total: —"},
			})
			m.applyWindowHeight()
			m.screen = ScreenRunDetails
			return m, loadRunDetailsCmd(m.client, m.baseURL, m.selectedRunID)
		}
	}
	return m, nil
}

func renderJobsPrimaryLine(job clitui.JobItem) string {
	glyph := jobsStatusGlyph(job.Status)
	name := strings.TrimSpace(job.Name)
	if name == "" {
		name = "-"
	}
	duration := formatDurationCompact(job.DurationMs)
	prefix := glyph + " "
	availableNameWidth := jobsListWidth - utf8.RuneCountInString(prefix) - 1 - utf8.RuneCountInString(duration)
	if availableNameWidth < 1 {
		availableNameWidth = 1
	}
	name = truncateRunes(name, availableNameWidth)
	name = name + strings.Repeat(" ", availableNameWidth-utf8.RuneCountInString(name))
	return prefix + name + " " + duration
}

func renderJobsSecondaryLine(job clitui.JobItem) string {
	image := strings.TrimSpace(job.JobImage)
	if image == "" {
		image = "-"
	}
	node := "-"
	if job.NodeID != nil && !job.NodeID.IsZero() {
		node = job.NodeID.String()
	}
	suffix := " @ " + node
	maxImageWidth := jobsListWidth - utf8.RuneCountInString(suffix)
	if maxImageWidth < 1 {
		return truncateRunes(suffix, jobsListWidth)
	}
	return truncateRunes(image, maxImageWidth) + suffix
}

func jobsStatusGlyph(status domaintypes.JobStatus) string {
	switch strings.ToLower(strings.TrimSpace(status.String())) {
	case "success", "succeeded", "finished", "completed":
		return "✓"
	case "fail", "failed", "crash", "crashed", "error", "cancelled", "canceled":
		return "X"
	default:
		return "⣾"
	}
}

func formatDurationCompact(durationMs int64) string {
	if durationMs <= 0 {
		return "-"
	}
	if durationMs < 1000 {
		return fmt.Sprintf("%dms", durationMs)
	}
	return fmt.Sprintf("%.1fs", float64(durationMs)/1000.0)
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max])
}

// handleEsc implements Esc-key transitions per the state machine contract.
func (m model) handleEsc() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenMigrationsList:
		m.screen = ScreenRoot
	case ScreenMigrationDetails:
		m.screen = ScreenMigrationsList
	case ScreenRunsList:
		m.screen = ScreenRoot
	case ScreenRunDetails:
		m.screen = ScreenRunsList
	case ScreenJobsList:
		m.screen = ScreenRoot
	case ScreenRoot:
		return m, tea.Quit
	}
	return m, nil
}

// migsLoadedMsg carries migrations fetched from the API.
type migsLoadedMsg struct{ migs []clitui.MigItem }

// runsLoadedMsg carries runs fetched from the API.
type runsLoadedMsg struct{ runs []runSummary }

// jobsLoadedMsg carries jobs fetched from the API.
type jobsLoadedMsg struct{ jobs []clitui.JobItem }

// runSummary is a minimal run representation used in the TUI.
type runSummary struct {
	ID        domaintypes.RunID
	MigName   string
	CreatedAt time.Time
}
