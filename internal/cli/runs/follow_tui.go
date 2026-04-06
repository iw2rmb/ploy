package runs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/iw2rmb/ploy/internal/cli/stream"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

const defaultFollowPollInterval = 1 * time.Second

type followReportMsg struct {
	report RunReport
}

type followTerminalMsg struct {
	report RunReport
	state  migsapi.RunState
}

type followErrMsg struct {
	err error
}

type followJobPreviewMsg struct {
	jobID    domaintypes.JobID
	preview  RunJobIOPreview
	clearRow bool
}

type followModel struct {
	renderOpts      TextRenderOptions
	spinner         spinner.Model
	spinnerFrame    int
	report          *RunReport
	finalState      migsapi.RunState
	renderErr       error
	jobIOPreviews   map[domaintypes.JobID]RunJobIOPreview
	expandStdout    bool
	expandStderr    bool
	interactiveMode bool
}

func newFollowModel(opts TextRenderOptions, interactive bool) followModel {
	return followModel{
		renderOpts:      opts,
		spinner:         spinner.New(spinner.WithSpinner(spinner.Dot)),
		jobIOPreviews:   make(map[domaintypes.JobID]RunJobIOPreview),
		interactiveMode: interactive,
	}
}

func (m followModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m followModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case spinner.TickMsg:
		nextSpinner, cmd := m.spinner.Update(msg)
		m.spinner = nextSpinner
		m.spinnerFrame++
		return m, cmd
	case tea.KeyMsg:
		if strings.EqualFold(strings.TrimSpace(typed.String()), "ctrl+c") {
			return m, tea.Quit
		}
		if !m.interactiveMode {
			return m, nil
		}
		switch strings.ToLower(strings.TrimSpace(typed.String())) {
		case "o":
			m.expandStdout = !m.expandStdout
		case "e":
			m.expandStderr = !m.expandStderr
		}
		return m, nil
	case followReportMsg:
		report := typed.report
		m.report = &report
		return m, nil
	case followJobPreviewMsg:
		if typed.clearRow {
			delete(m.jobIOPreviews, typed.jobID)
			return m, nil
		}
		m.jobIOPreviews[typed.jobID] = typed.preview
		return m, nil
	case followTerminalMsg:
		report := typed.report
		m.report = &report
		m.finalState = typed.state
		return m, tea.Quit
	case followErrMsg:
		m.renderErr = typed.err
		return m, tea.Quit
	default:
		return m, nil
	}
}

func (m followModel) View() tea.View {
	if m.report == nil {
		return tea.NewView("Loading run status...\n")
	}

	opts := m.renderOpts
	opts.SpinnerFrame = m.spinnerFrame
	opts.LiveDurations = true
	opts.Now = time.Now()
	opts.JobIOPreviews = m.jobIOPreviews
	opts.ExpandStdout = m.expandStdout
	opts.ExpandStderr = m.expandStderr
	opts.FilterRunningRepos = true
	opts.EmptyReposLine = "No repos with in-progress jobs."
	layout, err := RenderRunReportTextLayout(*m.report, opts)
	if err != nil {
		return tea.NewView("")
	}
	return tea.NewView(layout.Text)
}

// FollowRunCommand drives `run --follow` rendering with Bubble Tea v2.
type FollowRunCommand struct {
	Client       *http.Client
	BaseURL      *url.URL
	RunID        domaintypes.RunID
	Output       io.Writer
	EnableOSC8   bool
	AuthToken    string
	MaxRetries   int
	PollInterval time.Duration
}

// Run executes follow-mode rendering until the run reaches a terminal state.
func (c FollowRunCommand) Run(ctx context.Context) (migsapi.RunState, error) {
	if c.Client == nil {
		return "", fmt.Errorf("run follow: client required")
	}
	if c.BaseURL == nil {
		return "", fmt.Errorf("run follow: base url required")
	}
	if c.RunID.IsZero() {
		return "", fmt.Errorf("run follow: run id required")
	}
	if c.Output == nil {
		return "", fmt.Errorf("run follow: output writer required")
	}

	interval := c.PollInterval
	if interval <= 0 {
		interval = defaultFollowPollInterval
	}

	coordCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	interactive := isTTYWriter(c.Output) && isTTYReader(os.Stdin)

	input := io.Reader(nil)
	if interactive {
		input = os.Stdin
	}

	program := tea.NewProgram(
		newFollowModel(TextRenderOptions{
			EnableOSC8: c.EnableOSC8,
			AuthToken:  c.AuthToken,
			BaseURL:    c.BaseURL,
		}, interactive),
		tea.WithContext(coordCtx),
		tea.WithInput(input),
		tea.WithOutput(c.Output),
		tea.WithoutSignalHandler(),
	)

	stateCh := make(chan migsapi.RunState, 1)
	errCh := make(chan error, 1)
	go c.coordinate(coordCtx, program, interval, stateCh, errCh)

	finalModel, runErr := program.Run()
	cancel()
	if runErr != nil {
		return "", runErr
	}

	model, ok := finalModel.(followModel)
	if !ok {
		return "", fmt.Errorf("run follow: unexpected final model type %T", finalModel)
	}
	if model.renderErr != nil {
		return "", model.renderErr
	}
	if model.report != nil && !isTTYWriter(c.Output) {
		opts := model.renderOpts
		opts.SpinnerFrame = model.spinnerFrame
		opts.LiveDurations = true
		opts.Now = time.Now()
		layout, err := RenderRunReportTextLayout(*model.report, opts)
		if err != nil {
			return "", err
		}
		_, _ = io.WriteString(c.Output, layout.Text)
	}
	if model.finalState != "" {
		return model.finalState, nil
	}

	select {
	case state := <-stateCh:
		return state, nil
	default:
	}
	select {
	case err := <-errCh:
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", err
		}
	default:
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	return "", context.Canceled
}

func (c FollowRunCommand) coordinate(
	ctx context.Context,
	program *tea.Program,
	pollInterval time.Duration,
	stateCh chan<- migsapi.RunState,
	errCh chan<- error,
) {
	defer close(stateCh)
	defer close(errCh)

	refreshCh := make(chan struct{}, 1)
	pollTicker := time.NewTicker(pollInterval)
	defer pollTicker.Stop()

	streamClient := stream.Client{
		HTTPClient: c.Client,
		MaxRetries: c.MaxRetries,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{})),
	}
	jobStreamClient := stream.Client{
		HTTPClient: c.Client,
		MaxRetries: -1,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{})),
	}
	var trackerMu sync.Mutex
	type jobLogTracker struct {
		cancel context.CancelFunc
	}
	jobTrackers := make(map[domaintypes.JobID]jobLogTracker)
	stopTracker := func(jobID domaintypes.JobID) {
		trackerMu.Lock()
		tracker, ok := jobTrackers[jobID]
		if ok {
			delete(jobTrackers, jobID)
		}
		trackerMu.Unlock()
		if ok {
			tracker.cancel()
		}
	}
	stopAllTrackers := func() {
		trackerMu.Lock()
		ids := make([]domaintypes.JobID, 0, len(jobTrackers))
		for id := range jobTrackers {
			ids = append(ids, id)
		}
		trackerMu.Unlock()
		for _, id := range ids {
			stopTracker(id)
		}
	}
	defer stopAllTrackers()

	previews := make(map[domaintypes.JobID]RunJobIOPreview)
	appendPreview := func(jobID domaintypes.JobID, streamName, line string) {
		trimmed := strings.TrimRight(line, "\r\n")
		if strings.TrimSpace(trimmed) == "" {
			return
		}
		trackerMu.Lock()
		current := previews[jobID]
		switch normalizeStatus(streamName) {
		case "stderr":
			current.Stderr = append(current.Stderr, trimmed)
			if len(current.Stderr) > 3 {
				current.Stderr = current.Stderr[len(current.Stderr)-3:]
			}
		default:
			current.Stdout = append(current.Stdout, trimmed)
			if len(current.Stdout) > 3 {
				current.Stdout = current.Stdout[len(current.Stdout)-3:]
			}
		}
		previews[jobID] = current
		trackerMu.Unlock()
		program.Send(followJobPreviewMsg{jobID: jobID, preview: current})
	}
	startJobTracker := func(jobID domaintypes.JobID) {
		trackerMu.Lock()
		if _, exists := jobTrackers[jobID]; exists {
			trackerMu.Unlock()
			return
		}
		jobCtx, cancelJob := context.WithCancel(ctx)
		jobTrackers[jobID] = jobLogTracker{cancel: cancelJob}
		trackerMu.Unlock()

		endpoint := strings.TrimRight(c.BaseURL.String(), "/") + "/v1/jobs/" + jobID.String() + "/logs"
		go func() {
			err := jobStreamClient.Stream(jobCtx, endpoint, func(evt stream.Event) error {
				switch normalizeStatus(evt.Type) {
				case "", "log":
					var rec struct {
						Stream string `json:"stream"`
						Line   string `json:"line"`
					}
					if err := json.Unmarshal(evt.Data, &rec); err != nil {
						return nil
					}
					appendPreview(jobID, rec.Stream, rec.Line)
					return nil
				case "done", "complete", "completed":
					return stream.ErrDone
				default:
					return nil
				}
			})
			if err != nil && !errors.Is(err, context.Canceled) {
				// Polling stays authoritative; preview stream failures are non-fatal.
			}
			stopTracker(jobID)
		}()
	}
	reconcileTrackers := func(report RunReport) {
		statusByJob := make(map[domaintypes.JobID]string)
		for _, repo := range report.Repos {
			for _, job := range repo.Jobs {
				if job.JobID.IsZero() {
					continue
				}
				statusByJob[job.JobID] = normalizeStatus(job.Status.String())
			}
		}
		for jobID, status := range statusByJob {
			if shouldTrackJobPreview(status) {
				startJobTracker(jobID)
				continue
			}
			stopTracker(jobID)
			trackerMu.Lock()
			_, hadPreview := previews[jobID]
			delete(previews, jobID)
			trackerMu.Unlock()
			if hadPreview {
				program.Send(followJobPreviewMsg{jobID: jobID, clearRow: true})
			}
		}
	}

	endpoint := strings.TrimRight(c.BaseURL.String(), "/") + "/v1/runs/" + c.RunID.String() + "/logs"

	sseErrCh := make(chan error, 1)
	go func() {
		handler := func(evt stream.Event) error {
			typ := strings.ToLower(strings.TrimSpace(evt.Type))
			if typ == "run" || typ == "stage" {
				select {
				case refreshCh <- struct{}{}:
				default:
				}
			}
			return nil
		}
		sseErrCh <- streamClient.Stream(ctx, endpoint, handler)
	}()

	consecutiveFailures := 0
	fetch := func() bool {
		report, err := GetRunReportCommand{
			Client:  c.Client,
			BaseURL: c.BaseURL,
			RunID:   c.RunID,
		}.Run(ctx)
		if err != nil {
			if ctx.Err() != nil {
				errCh <- ctx.Err()
				program.Send(followErrMsg{err: ctx.Err()})
				return true
			}
			if c.MaxRetries >= 0 && consecutiveFailures >= c.MaxRetries {
				errCh <- err
				program.Send(followErrMsg{err: err})
				return true
			}
			consecutiveFailures++
			return false
		}
		consecutiveFailures = 0
		reconcileTrackers(report)
		if state := DeriveRunStateFromReport(report); state != "" {
			stateCh <- state
			program.Send(followTerminalMsg{report: report, state: state})
			return true
		}
		program.Send(followReportMsg{report: report})
		return false
	}

	if done := fetch(); done {
		return
	}

	for {
		select {
		case <-ctx.Done():
			errCh <- ctx.Err()
			program.Send(followErrMsg{err: ctx.Err()})
			return
		case err := <-sseErrCh:
			// SSE failures should not terminate follow; polling remains authoritative.
			if err != nil && ctx.Err() == nil {
				// no-op: polling ticker keeps state fresh.
			}
		case <-refreshCh:
			if done := fetch(); done {
				return
			}
		case <-pollTicker.C:
			if done := fetch(); done {
				return
			}
		}
	}
}

// DeriveRunStateFromReport computes terminal run state from repo statuses.
func DeriveRunStateFromReport(report RunReport) migsapi.RunState {
	if len(report.Repos) == 0 {
		return ""
	}
	anyJobError := false
	anyJobFail := false
	for _, entry := range report.Repos {
		for _, job := range entry.Jobs {
			status := strings.ToLower(strings.TrimSpace(string(job.Status)))
			switch status {
			case "created", "queued", "running":
				return ""
			case "error":
				anyJobError = true
			case "fail", "failed":
				anyJobFail = true
			}
		}
	}
	if anyJobError {
		return migsapi.RunStateError
	}
	if anyJobFail {
		return migsapi.RunStateFailed
	}

	allSuccess := true
	allCancelled := true
	hasFailure := false

	for _, entry := range report.Repos {
		status := strings.ToLower(strings.TrimSpace(string(entry.Status)))
		switch status {
		case "success", "succeeded", "finished":
			allCancelled = false
		case "cancelled", "canceled":
			allSuccess = false
		case "fail", "failed", "error":
			hasFailure = true
			allSuccess = false
			allCancelled = false
		default:
			return ""
		}
	}

	if hasFailure {
		return migsapi.RunStateFailed
	}
	if allSuccess {
		return migsapi.RunStateSucceeded
	}
	if allCancelled {
		return migsapi.RunStateCancelled
	}
	return ""
}

var _ tea.Model = followModel{}

func isTTYWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func isTTYReader(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func shouldTrackJobPreview(status string) bool {
	switch normalizeStatus(status) {
	case "running", "started":
		return true
	default:
		return false
	}
}
