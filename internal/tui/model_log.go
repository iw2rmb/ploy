package tui

import (
	tea "charm.land/bubbletea/v2"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

type jobLogState struct {
	sub   *jobLogSubscription
	lines []logstream.LogRecord
	done  bool
	err   error
}

func (m model) highlightedLogJob() domaintypes.JobID {
	if m.jobsVisible() {
		if job, ok := m.jobList.SelectedJob(); ok && job.JobType == domaintypes.JobTypePostGate {
			return job.JobID
		}
	}
	return ""
}

func (m *model) syncJobLog() tea.Cmd {
	id := m.highlightedLogJob()
	if sub := m.jobLog.sub; sub != nil {
		if sub.jobID == id {
			return nil
		}
		sub.cancel()
		m.jobLog = jobLogState{}
	}
	if id.IsZero() {
		return nil
	}
	sub, cmd := openJobLog(m.ctx, m.client, m.baseURL, id)
	m.jobLog.sub = sub
	return cmd
}

func (m *model) applyJobLog(msg jobLogMsg) tea.Cmd {
	if msg.sub != m.jobLog.sub || m.jobLog.done {
		return nil
	}
	m.jobLog.lines = append(m.jobLog.lines, msg.records...)
	// Each record consumes at least one row. Reserve extra rows for terminal resizes.
	if drop := len(m.jobLog.lines) - max(1024, m.windowHeight); drop > 0 {
		clear(m.jobLog.lines[:drop])
		m.jobLog.lines = m.jobLog.lines[drop:]
	}
	m.jobLog.done = msg.done || msg.err != nil
	m.jobLog.err = msg.err
	if m.jobLog.done {
		return nil
	}
	return msg.sub.next
}

func jobLogRestarted(previous, current domaintypes.JobStatus) bool {
	switch previous {
	case domaintypes.JobStatusSuccess, domaintypes.JobStatusCancelled, domaintypes.JobStatusFail, domaintypes.JobStatusError:
		return current == domaintypes.JobStatusRunning || current == domaintypes.JobStatusQueued || current == domaintypes.JobStatusCreated
	}
	return false
}
