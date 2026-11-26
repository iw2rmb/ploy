package joblist

import (
	"fmt"
	"io"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"

	"github.com/iw2rmb/ploy/internal/cli/common"
	cliruns "github.com/iw2rmb/ploy/internal/cli/runs"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/tui/statusrow"
)

type delegate struct {
	list.DefaultDelegate
	now          time.Time
	spinnerFrame int
}

func newDelegate(focused bool, now time.Time, spinnerFrame int) delegate {
	d := list.NewDefaultDelegate()
	d.SetHeight(3)
	if !focused {
		d.Styles.SelectedTitle = d.Styles.SelectedTitle.BorderStyle(lipgloss.HiddenBorder())
		d.Styles.SelectedDesc = d.Styles.SelectedDesc.BorderStyle(lipgloss.HiddenBorder())
	}
	return delegate{DefaultDelegate: d, now: now, spinnerFrame: spinnerFrame}
}

func (d delegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	r, ok := item.(row)
	if !ok {
		return
	}
	r.now, r.spinnerFrame = d.now, d.spinnerFrame
	statusrow.Render(w, d.DefaultDelegate, m, index, r, r.job.Status.String())
}

func renderPrimaryLine(job domainapi.JobListItem, spinnerFrame int) string {
	glyph := cliruns.StatusGlyph(job.Status.String(), spinnerFrame)
	if job.StartedAt == nil && (job.Status == domaintypes.JobStatusCreated || job.Status == domaintypes.JobStatusQueued) {
		glyph = "-"
	}
	return glyph + " " + cliruns.FormatStepName(job.DisplayName, job.JobType.String())
}

func renderDescription(job domainapi.JobListItem, now time.Time) string {
	duration := formatDuration(jobDuration(job, now))
	if job.Status == domaintypes.JobStatusRunning && job.StartedAt == nil {
		duration = "--h --m --s"
	}
	return common.FormatRunListRepo(job.RepoURL, "") + "\n" + strings.TrimSpace(duration)
}

func jobDuration(job domainapi.JobListItem, now time.Time) time.Duration {
	switch job.Status {
	case domaintypes.JobStatusCreated, domaintypes.JobStatusQueued:
		return 0
	case domaintypes.JobStatusRunning:
		if job.StartedAt != nil {
			return max(time.Duration(0), now.Sub(*job.StartedAt))
		}
		return 0
	default:
		if job.DurationMs > 0 {
			return time.Duration(job.DurationMs) * time.Millisecond
		}
		if job.StartedAt != nil && job.FinishedAt != nil {
			return max(time.Duration(0), job.FinishedAt.Sub(*job.StartedAt))
		}
		return 0
	}
}

func formatDuration(duration time.Duration) string {
	seconds := max(int64(0), int64(duration/time.Second))
	hours, minutes := "   ", "   "
	if value := seconds / 3600; value > 0 {
		hours = fmt.Sprintf("%2dh", value)
	}
	if value := seconds % 3600 / 60; value > 0 {
		minutes = fmt.Sprintf("%2dm", value)
	}
	return fmt.Sprintf("%s %s %2ds", hours, minutes, seconds%60)
}
