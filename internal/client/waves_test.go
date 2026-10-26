package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
)

func TestWaveCommandsDecodeFullSummary(t *testing.T) {
	createdBy, startedAt, finishedAt := "alice", "Mon, 26 Oct 2026 08:00:00 GMT", "Mon, 26 Oct 2026 09:00:00 GMT"
	want := domainapi.WaveSummary{
		ID: domaintypes.NewWaveID(), MigID: domaintypes.NewMigID(), SpecID: domaintypes.NewSpecID(),
		CreatedBy: &createdBy, Status: domaintypes.WaveStatusFinished, CreatedAt: startedAt,
		StartedAt: &startedAt, FinishedAt: &finishedAt, Counts: &domaintypes.RunCounts{Total: 2, Fail: 1, Success: 1},
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := "/api/v1/waves/" + want.ID.String()
				if method == http.MethodPost {
					path += "/cancel"
				}
				if r.Method != method || r.URL.Path != path || r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("unexpected request: %s %s, authorization %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
				}
				_ = json.NewEncoder(w).Encode(want)
			}))
			defer server.Close()
			base, _ := url.Parse(server.URL + "/api")
			client := server.Client()
			client.Transport = httpx.NewHeaderTransport(client.Transport, http.Header{"Authorization": {"Bearer test-token"}})
			var got domainapi.WaveSummary
			var err error
			if method == http.MethodGet {
				got, err = (GetWaveCommand{Client: client, BaseURL: base, WaveID: want.ID}).Run(context.Background())
			} else {
				got, err = (CancelWaveCommand{Client: client, BaseURL: base, WaveID: want.ID}).Run(context.Background())
			}
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("Run() = %#v, %v; want %#v", got, err, want)
			}
		})
	}
}

func TestWaveCommandsRejectHTTPAndDecodeFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"non-200", http.StatusAccepted, `{"error":"not complete"}`, "unexpected status 202: not complete"},
		{"invalid JSON", http.StatusOK, `{`, "decode response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			base, _ := url.Parse(server.URL)
			for _, run := range []func(context.Context) (domainapi.WaveSummary, error){
				(GetWaveCommand{Client: server.Client(), BaseURL: base}).Run,
				(CancelWaveCommand{Client: server.Client(), BaseURL: base}).Run,
			} {
				if _, err := run(context.Background()); err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("Run() error = %v, want %q", err, tc.want)
				}
			}
		})
	}
}

func TestFollowWaveDeliversSnapshotsThroughTerminalState(t *testing.T) {
	for _, terminal := range []domaintypes.WaveStatus{domaintypes.WaveStatusFinished, domaintypes.WaveStatusCancelled} {
		t.Run(string(terminal), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				status := domaintypes.WaveStatusStarted
				if calls == 2 {
					status = terminal
				}
				_ = json.NewEncoder(w).Encode(domainapi.WaveSummary{ID: domaintypes.NewWaveID(), MigID: domaintypes.NewMigID(), SpecID: domaintypes.NewSpecID(), Status: status})
			}))
			defer server.Close()
			base, _ := url.Parse(server.URL)
			var updates []domaintypes.WaveStatus
			got, err := (FollowWaveCommand{
				Get:      GetWaveCommand{Client: server.Client(), BaseURL: base},
				OnUpdate: func(wave domainapi.WaveSummary) error { updates = append(updates, wave.Status); return nil },
			}).Run(context.Background())
			if err != nil || got.Status != terminal || !reflect.DeepEqual(updates, []domaintypes.WaveStatus{domaintypes.WaveStatusStarted, terminal}) {
				t.Fatalf("Run() = %+v, %v; updates = %v", got, err, updates)
			}
		})
	}
}

func TestFollowWaveStopsOnCallbackErrorOrCancellation(t *testing.T) {
	for _, tc := range []string{"callback error", "cancel during wait", "deadline during request", "request failure", "missing callback"} {
		t.Run(tc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			callbackErr := errors.New("output failed")
			want := callbackErr
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc == "deadline during request" {
					<-r.Context().Done()
					return
				}
				_ = json.NewEncoder(w).Encode(domainapi.WaveSummary{ID: domaintypes.NewWaveID(), MigID: domaintypes.NewMigID(), SpecID: domaintypes.NewSpecID(), Status: domaintypes.WaveStatusStarted})
			}))
			defer server.Close()
			base, _ := url.Parse(server.URL)
			command := FollowWaveCommand{Get: GetWaveCommand{Client: server.Client(), BaseURL: base}, OnUpdate: func(domainapi.WaveSummary) error { return callbackErr }}
			switch tc {
			case "cancel during wait":
				want = context.Canceled
				command.OnUpdate = func(domainapi.WaveSummary) error { cancel(); return nil }
			case "deadline during request":
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer deadlineCancel()
				want = context.DeadlineExceeded
			case "request failure":
				server.Close()
				want = nil
			case "missing callback":
				command.OnUpdate = nil
				want = nil
			}
			_, err := command.Run(ctx)
			if err == nil || (want != nil && !errors.Is(err, want)) {
				t.Fatalf("Run() error = %v, want %v", err, want)
			}
			if tc == "missing callback" && err.Error() != "wave follow: update callback required" {
				t.Fatal(err)
			}
			if tc == "request failure" && !strings.Contains(err.Error(), "http request") {
				t.Fatal(err)
			}
		})
	}
}

func TestWaveCommandsRequireTransport(t *testing.T) {
	for _, run := range []func(context.Context) (domainapi.WaveSummary, error){
		(GetWaveCommand{}).Run, (CancelWaveCommand{}).Run,
		(GetWaveCommand{Client: http.DefaultClient}).Run, (CancelWaveCommand{Client: http.DefaultClient}).Run,
	} {
		if _, err := run(context.Background()); err == nil {
			t.Fatal("expected transport error")
		}
	}
}
