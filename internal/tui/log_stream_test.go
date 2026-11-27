package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestJobLogFromBackfillThroughCompletion(t *testing.T) {
	live := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/jobs/job-1/logs" || r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("unexpected request %s %s", r.URL.Path, r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: log\ndata: {\"stream\":\"stdout\",\"line\":\"first\"}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			return
		case <-live:
		}
		fmt.Fprint(w, "event: log\ndata: {\"stream\":\"stderr\",\"line\":\"second\"}\n\nevent: retention\ndata: {}\n\nevent: done\ndata: {}\n\n")
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	sub, open := openJobLog(t.Context(), server.Client(), base, "job-1")
	defer sub.cancel()
	msg := open().(jobLogMsg)
	if msg.sub != sub || msg.done || len(msg.records) != 1 || msg.records[0].Line != "first" {
		t.Fatalf("backfill: %+v", msg)
	}
	close(live)
	msg = sub.next().(jobLogMsg)
	if len(msg.records) != 1 || msg.records[0].Stream != "stderr" || msg.records[0].Line != "second" {
		t.Fatalf("live output: %+v", msg)
	}
	if !msg.done {
		msg = sub.next().(jobLogMsg)
	}
	if !msg.done || msg.err != nil {
		t.Fatalf("completion: %+v", msg)
	}
}

func TestJobLogEmptyCompletionAndError(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		wantErr bool
	}{
		{"empty", http.StatusOK, false},
		{"forbidden", http.StatusForbidden, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, "event: done\ndata: {}\n\n")
			}))
			defer server.Close()
			base, _ := url.Parse(server.URL)
			sub, open := openJobLog(t.Context(), server.Client(), base, "job-1")
			defer sub.cancel()
			msg := open().(jobLogMsg)
			if !msg.done || len(msg.records) != 0 || (msg.err != nil) != tc.wantErr {
				t.Fatalf("completion: %+v", msg)
			}
		})
	}
}

func TestJobLogCancellationStopsIdleStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: log\ndata: {\"line\":\"running\"}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	sub, open := openJobLog(t.Context(), server.Client(), base, "job-1")
	defer sub.cancel()
	if msg := open().(jobLogMsg); msg.done || len(msg.records) != 1 {
		t.Fatalf("first record: %+v", msg)
	}
	sub.cancel()
	if msg := sub.next().(jobLogMsg); !msg.done || !errors.Is(msg.err, context.Canceled) {
		t.Fatalf("cancelled stream: %+v", msg)
	}
}
