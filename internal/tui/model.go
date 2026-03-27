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
	"charm.land/lipgloss/v2"

	clitui "github.com/iw2rmb/ploy/internal/cli/tui"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// Screen represents the current TUI state in the six-screen navigation contract.
type Screen int

const (
	ScreenRoot             Screen = iota // PLOY root selector
	ScreenMigrationsList                 // PLOY | MIGRATIONS
	ScreenMigrationDetails               // PLOY (selected migration context)
	ScreenRunsList                       // PLOY | RUNS
	ScreenRunDetails                     // PLOY (selected run context)
	ScreenJobsList                       // PLOY | JOBS
)

// listWidth is the fixed width applied to standard non-PLOY lists in the TUI.
const listWidth = 24

// ployListWidth is the fixed width for the left PLOY list.
const ployListWidth = 30

// runsListWidth is the fixed width for RUNS items in the right panel.
const runsListWidth = 30

// jobsListWidth is the fixed width for JOBS items in the right panel.
const jobsListWidth = 30

// jobsContentWidth is an item text budget (excluding list delegate chrome).
const jobsContentWidth = 26

var (
	jobsCompleteGlyphStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#98c379"))
	jobsFailedGlyphStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#e06c75"))
)

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
	// (run details) as the right panel.
	detail list.Model

	// selectedMigID tracks the migration chosen in S2 for drill-down to S3.
	selectedMigID domaintypes.MigID

	// selectedMigName tracks migration name when selected through runs.
	selectedMigName string

	// selectedRunID tracks the run chosen in S4 for drill-down to S5.
	selectedRunID domaintypes.RunID

	// selectedJobID tracks the job chosen in S6.
	selectedJobID domaintypes.JobID

	// Selected entity flags control root PLOY item labels (plural vs singular).
	hasSelectedMigration bool
	hasSelectedRun       bool
	hasSelectedJob       bool

	// client and baseURL are used to fetch list data via internal/cli/tui commands.
	client  *http.Client
	baseURL *url.URL

	// windowHeight tracks the latest terminal height so every list can match it.
	windowHeight int

	// runs caches the latest runs list so run selection can resolve mig context.
	runs []runSummary

	// jobs caches the latest jobs list so job selection can resolve context.
	jobs []clitui.JobItem
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

// newPloyList creates the left PLOY list with fixed width.
func newPloyList(title string, items []list.Item) list.Model {
	return newListWithWidth(title, items, ployListWidth)
}

// newJobsList creates a jobs list with a 48-column width.
func newJobsList(title string, items []list.Item) list.Model {
	return newListWithWidth(title, items, jobsListWidth)
}

// newRunsList creates a runs list with a 30-column width.
func newRunsList(title string, items []list.Item) list.Model {
	return newListWithWidth(title, items, runsListWidth)
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

// buildPloyItems constructs the left PLOY list with context-aware labels.
func buildPloyItems(hasMigration, hasRun, hasJob bool) []list.Item {
	migrationsTitle := "Migrations"
	if hasMigration {
		migrationsTitle = "Migration"
	}
	runsTitle := "Runs"
	if hasRun {
		runsTitle = "Run"
	}
	jobsTitle := "Jobs"
	if hasJob {
		jobsTitle = "Job"
	}
	return []list.Item{
		listItem{title: migrationsTitle, description: "select migration"},
		listItem{title: runsTitle, description: "select run"},
		listItem{title: jobsTitle, description: "select job"},
	}
}

func buildRunDetailsPloyItems(migName string, migID domaintypes.MigID, runID domaintypes.RunID, jobsTotal string) []list.Item {
	migTitle := strings.TrimSpace(migName)
	if migTitle == "" {
		migTitle = "-"
	}
	migDesc := strings.TrimSpace(migID.String())
	if migDesc == "" {
		migDesc = "-"
	}
	runDesc := strings.TrimSpace(runID.String())
	if runDesc == "" {
		runDesc = "-"
	}
	return []list.Item{
		listItem{title: migTitle, description: migDesc},
		listItem{title: "Run", description: runDesc},
		listItem{title: "Jobs", description: jobsTotal},
	}
}

func buildMigrationDetailsPloyItems(migName string, migID domaintypes.MigID, runsTotal string) []list.Item {
	migTitle := strings.TrimSpace(migName)
	if migTitle == "" {
		migTitle = "-"
	}
	migDesc := strings.TrimSpace(migID.String())
	if migDesc == "" {
		migDesc = "-"
	}
	return []list.Item{
		listItem{title: migTitle, description: migDesc},
		listItem{title: "Runs", description: runsTotal},
		listItem{title: "Jobs", description: "select job"},
	}
}

func buildJobDetailsPloyItems(migName string, migID domaintypes.MigID, runID domaintypes.RunID, jobID domaintypes.JobID) []list.Item {
	migTitle := strings.TrimSpace(migName)
	if migTitle == "" {
		migTitle = "-"
	}
	migDesc := strings.TrimSpace(migID.String())
	if migDesc == "" {
		migDesc = "-"
	}
	runDesc := strings.TrimSpace(runID.String())
	if runDesc == "" {
		runDesc = "-"
	}
	jobDesc := strings.TrimSpace(jobID.String())
	if jobDesc == "" {
		jobDesc = "-"
	}
	return []list.Item{
		listItem{title: migTitle, description: migDesc},
		listItem{title: "Run", description: runDesc},
		listItem{title: "Job", description: jobDesc},
	}
}

// setPloySelectionState applies root item label state while preserving cursor.
func (m *model) setPloySelectionState(hasMigration, hasRun, hasJob bool) {
	m.hasSelectedMigration = hasMigration
	m.hasSelectedRun = hasRun
	m.hasSelectedJob = hasJob

	selectedIdx := m.ploy.Index()
	m.ploy.SetItems(buildPloyItems(hasMigration, hasRun, hasJob))
	if selectedIdx < 0 {
		selectedIdx = 0
	}
	if itemCount := len(m.ploy.Items()); itemCount > 0 {
		if selectedIdx >= itemCount {
			selectedIdx = itemCount - 1
		}
		m.ploy.Select(selectedIdx)
	}
}

// setWindowHeight updates cached terminal height and applies it to all lists.
func (m *model) setWindowHeight(height int) {
	if height <= 0 {
		return
	}
	m.windowHeight = height
	m.applyWindowHeight()
}

// InitialModel constructs the starting model for the TUI session.
func InitialModel(client *http.Client, baseURL *url.URL) model {
	ploy := newPloyList("PLOY", buildPloyItems(false, false, false))
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
		m.runs = sorted
		m.secondary = newRunsList("RUNS", items)
		m.applyWindowHeight()
		return m, nil

	case migDetailsLoadedMsg:
		if m.screen == ScreenMigrationDetails {
			m.ploy.SetItems(buildMigrationDetailsPloyItems(
				m.selectedMigName,
				m.selectedMigID,
				fmt.Sprintf("total: %d", msg.runTotal),
			))
			m.ploy.Select(0)
			return m, nil
		}
		items := m.detail.Items()
		if len(items) >= 2 {
			items[0] = listItem{title: "repositories", description: fmt.Sprintf("total: %d", msg.repoTotal)}
			items[1] = listItem{title: "runs", description: fmt.Sprintf("total: %d", msg.runTotal)}
			m.detail.SetItems(items)
		}
		return m, nil

	case runDetailsLoadedMsg:
		if m.screen == ScreenRunDetails {
			m.ploy.SetItems(buildRunDetailsPloyItems(
				m.selectedMigName,
				m.selectedMigID,
				m.selectedRunID,
				fmt.Sprintf("total: %d", msg.jobTotal),
			))
			m.ploy.Select(1)
			return m, nil
		}
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
		m.jobs = msg.jobs
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
		m.ploy, cmd = m.ploy.Update(msg)
	case ScreenRunsList:
		m.secondary, cmd = m.secondary.Update(msg)
	case ScreenRunDetails:
		m.ploy, cmd = m.ploy.Update(msg)
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
			m.secondary = newRunsList("RUNS", nil)
			m.applyWindowHeight()
			return m, loadRunsCmd(m.client, m.baseURL)
		case 2: // Jobs
			m.screen = ScreenJobsList
			m.secondary = newJobsList("JOBS", nil)
			m.applyWindowHeight()
			return m, tea.Batch(loadJobsCmd(m.client, m.baseURL, nil), loadRunsCmd(m.client, m.baseURL))
		}
	case ScreenMigrationsList:
		if item, ok := m.secondary.SelectedItem().(listItem); ok {
			m.selectedMigID = domaintypes.MigID(item.description)
			m.selectedMigName = item.title
			m.selectedRunID = ""
			m.selectedJobID = ""
			m.ploy.SetItems(buildMigrationDetailsPloyItems(
				m.selectedMigName,
				m.selectedMigID,
				"total: —",
			))
			m.ploy.Select(0)
			m.applyWindowHeight()
			m.screen = ScreenMigrationDetails
			return m, loadMigDetailsCmd(m.client, m.baseURL, m.selectedMigID)
		}
	case ScreenRunsList:
		if item, ok := m.secondary.SelectedItem().(listItem); ok {
			m.selectedRunID = domaintypes.RunID(item.title)
			m.selectedJobID = ""
			m.selectedMigID = ""
			m.selectedMigName = ""
			for _, run := range m.runs {
				if run.ID == m.selectedRunID {
					m.selectedMigID = run.MigID
					m.selectedMigName = run.MigName
					break
				}
			}
			m.ploy.SetItems(buildRunDetailsPloyItems(
				m.selectedMigName,
				m.selectedMigID,
				m.selectedRunID,
				"total: —",
			))
			m.ploy.Select(1)
			m.applyWindowHeight()
			m.screen = ScreenRunDetails
			return m, loadRunDetailsCmd(m.client, m.baseURL, m.selectedRunID)
		}
	case ScreenJobsList:
		if item, ok := m.secondary.SelectedItem().(listItem); ok {
			m.selectedJobID = domaintypes.JobID(item.description)
			var selectedJob clitui.JobItem
			foundJob := false
			for _, job := range m.jobs {
				if job.JobID == m.selectedJobID {
					selectedJob = job
					foundJob = true
					break
				}
			}
			if !foundJob {
				return m, nil
			}
			m.selectedRunID = selectedJob.RunID
			m.selectedMigName = selectedJob.MigName
			m.selectedMigID = ""
			for _, run := range m.runs {
				if run.ID == m.selectedRunID {
					m.selectedMigID = run.MigID
					break
				}
			}
			m.ploy.SetItems(buildJobDetailsPloyItems(
				m.selectedMigName,
				m.selectedMigID,
				m.selectedRunID,
				m.selectedJobID,
			))
			m.ploy.Select(2)
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
	duration := formatDurationShort(job.DurationMs)
	prefix := glyph + " "
	availableNameWidth := jobsContentWidth - lipgloss.Width(prefix) - 1 - lipgloss.Width(duration)
	if availableNameWidth < 1 {
		availableNameWidth = 1
	}
	name = truncateRunes(name, availableNameWidth)
	name = name + strings.Repeat(" ", availableNameWidth-lipgloss.Width(name))
	return prefix + name + " " + duration
}

func renderJobsSecondaryLine(job clitui.JobItem) string {
	jobID := strings.TrimSpace(job.JobID.String())
	if jobID == "" {
		jobID = "-"
	}
	return truncateRunes(jobID, jobsListWidth)
}

func jobsStatusGlyph(status domaintypes.JobStatus) string {
	switch status {
	case domaintypes.JobStatusSuccess:
		return jobsCompleteGlyphStyle.Render("⏺")
	case domaintypes.JobStatusFail, domaintypes.JobStatusCancelled:
		return jobsFailedGlyphStyle.Render("⏺")
	default:
		return "⣾"
	}
}

func formatDurationShort(durationMs int64) string {
	if durationMs <= 0 {
		return "-"
	}
	if durationMs < 1000 {
		return fmt.Sprintf("%dms", durationMs)
	}
	if durationMs < 60000 {
		return fmt.Sprintf("%ds", durationMs/1000)
	}
	if durationMs < 3600000 {
		return fmt.Sprintf("%dm", durationMs/60000)
	}
	return fmt.Sprintf("%dh", durationMs/3600000)
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
		m.setPloySelectionState(true, false, false)
	case ScreenRunsList:
		m.screen = ScreenRoot
	case ScreenRunDetails:
		m.screen = ScreenRunsList
		m.setPloySelectionState(true, true, false)
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
	MigID     domaintypes.MigID
	MigName   string
	CreatedAt time.Time
}
