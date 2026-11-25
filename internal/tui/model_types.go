package tui

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	"github.com/iw2rmb/ploy/internal/cli/common"
	cliruns "github.com/iw2rmb/ploy/internal/cli/runs"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/tui/joblist"
)

type Screen int

const (
	ScreenPloyList Screen = iota
	ScreenRunsList
	ScreenRunDetails
	ScreenJobsList
	ScreenUsersList
)

const (
	ployListWidth = 30
	runsListWidth = 30
	runsIndex     = 0
	jobsIndex     = 1
	usersIndex    = 2
)

type listItem struct {
	title       string
	description string
}

func (i listItem) Title() string       { return i.title }
func (i listItem) Description() string { return i.description + "\n" }
func (i listItem) FilterValue() string { return i.title }

type runItem struct {
	run          domaintypes.RunSummary
	spinnerFrame int
}

func (i runItem) Title() string {
	spec := strings.TrimSpace(i.run.SpecName)
	if spec == "" {
		spec = normalizeLabel(i.run.SpecID.String())
	}
	return cliruns.StatusGlyph(string(i.run.Status), i.spinnerFrame) + " " + spec
}
func (i runItem) Description() string {
	var creator string
	if i.run.CreatedBy != nil {
		if username := strings.TrimSpace(*i.run.CreatedBy); username != "" {
			creator = "@" + username
		}
	}
	return common.FormatRunListRepo(i.run.RepoURL, "") + "\n" + creator
}
func (i runItem) FilterValue() string {
	return strings.Join([]string{
		i.Title(), string(i.run.Status), i.Description(), i.run.ID.String(),
		common.FormatRunListRepo(i.run.RepoURL, i.run.SourceCommitSHA), common.FormatRunListSpec(i.run),
	}, " ")
}

type model struct {
	screen           Screen
	rootList         list.Model
	usersList        list.Model
	usersEnabled     bool
	usersLoading     bool
	usersErr         error
	selectedUser     string
	rightPaneList    list.Model
	runDetailsList   list.Model
	selectedRunID    domaintypes.RunID
	runDetails       runDetails
	detailsRequest   uint64
	detailsLoading   bool
	detailsStatus    domaintypes.RunStatus
	jobsRequest      uint64
	runsRequest      uint64
	runsLoading      bool
	jobsLoading      bool
	refreshScheduled bool
	now              time.Time
	spinnerFrame     int
	ctx              context.Context
	client           *http.Client
	baseURL          *url.URL
	windowHeight     int
	windowWidth      int
	jobList          joblist.Model
	lastErr          error
}

type runsLoadedMsg struct {
	offPageRun *domaintypes.RunSummary
	runs       []domaintypes.RunSummary
	requestID  uint64
	err        error
}
type jobsLoadedMsg struct {
	requestID uint64
	err       error
	runID     domaintypes.RunID
	jobs      []domainapi.JobListItem
}
