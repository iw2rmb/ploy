package tui

import (
	"net/http"
	"net/url"
	"time"

	"charm.land/bubbles/v2/list"
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

	// lastErr holds the most recent error returned by an async command.
	lastErr error
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
