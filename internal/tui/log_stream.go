package tui

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	tea "charm.land/bubbletea/v2"
	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/cli/stream"
	"github.com/iw2rmb/ploy/internal/domain/types"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

type jobLogSubscription struct {
	jobID   types.JobID
	cancel  context.CancelFunc
	records chan logstream.LogRecord
	// The producer stores err before closing records. Only read it after observing closure.
	err error
}

type jobLogMsg struct {
	sub     *jobLogSubscription
	records []logstream.LogRecord
	done    bool
	err     error
}

func openJobLog(parent context.Context, client *http.Client, base *url.URL, jobID types.JobID) (*jobLogSubscription, tea.Cmd) {
	ctx, cancel := context.WithCancel(parent)
	sub := &jobLogSubscription{jobID: jobID, cancel: cancel, records: make(chan logstream.LogRecord, 128)}
	return sub, func() tea.Msg {
		go func() {
			defer close(sub.records)
			defer cancel()
			reader := stream.Client{HTTPClient: common.CloneForStream(client), Logger: slog.New(slog.DiscardHandler)}
			endpoint := base.JoinPath("v1", "jobs", jobID.String(), "logs")
			sub.err = reader.Stream(ctx, endpoint.String(), func(event stream.Event) error {
				switch event.Type {
				case "done":
					return stream.ErrDone
				case "log":
					var record logstream.LogRecord
					if err := json.Unmarshal(event.Data, &record); err != nil {
						return err
					}
					select {
					case sub.records <- record:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				return nil
			})
		}()
		return sub.next()
	}
}

func (s *jobLogSubscription) next() tea.Msg {
	msg := jobLogMsg{sub: s}
	record, open := <-s.records
	for open {
		msg.records = append(msg.records, record)
		// Deliver buffered backfill in batches while returning live output immediately.
		if len(msg.records) >= cap(s.records) {
			return msg
		}
		select {
		case record, open = <-s.records:
		default:
			return msg
		}
	}
	msg.done = true
	msg.err = s.err
	return msg
}
