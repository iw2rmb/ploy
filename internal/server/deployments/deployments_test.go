package deployments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenUsesOneUserForTrinoAndBasicAuthentication(t *testing.T) {
	// URL punctuation in credentials must reach both authentication fields unchanged.
	user, password := "service+@/%", "password@:/?#&+%"
	previousClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = previousClient })
	var calls atomic.Int32
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.Scheme != "https" || r.URL.Host != "trino.dlh.t-infra.ru" {
			t.Errorf("unexpected Trino destination")
		}
		basicUser, basicPassword, ok := r.BasicAuth()
		if !ok || basicUser != user || basicPassword != password || r.Header.Get("X-Trino-User") != user {
			t.Error("Trino authentication did not preserve configured credentials")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"test"}`)), Request: r}, nil
	})}
	db, err := Open(user, password)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := New(db).Lookup(context.Background(), Request{For: []Coordinate{{"g", "a"}}, JVM: 21}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() == 0 {
		t.Fatal("driver made no authenticated request")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestLookupReturnsSortedDistinctObservedSets(t *testing.T) {
	// The response preserves complete sets across Trino result pages.
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			writePage(t, w, server.URL+"/first")
			return
		}
		if r.URL.Path == "/first" {
			writePage(t, w, server.URL+"/next", `["g:a@1","g:b@3","g:a@1"]`)
			return
		}
		writePage(t, w, "", `["g:b@2","g:a@1"]`, `["g:a@1","g:b@2"]`, `["g:b@1","g:a@2"]`)
	}))
	defer server.Close()
	db := openTestDB(t, server.URL)
	got, err := New(db).Lookup(context.Background(), Request{For: []Coordinate{{"g", "a"}}, JVM: 21})
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"g:a@1", "g:b@2"}, {"g:a@1", "g:b@3"}, {"g:a@2", "g:b@1"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("versions = %v, want %v", got, want)
	}
}

func TestLookupReturnsEmptyArrayWhenNoRowsMatch(t *testing.T) {
	// No observations produce a non-nil empty result.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writePage(t, w, "")
	}))
	defer server.Close()
	got, err := New(openTestDB(t, server.URL)).Lookup(context.Background(), Request{For: []Coordinate{{"g", "a"}}, JVM: 21})
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("Lookup = %v, %v; want empty non-nil result", got, err)
	}
}

func TestLookupDiscardsRowsOnFailure(t *testing.T) {
	for _, failure := range []string{"iteration", "malformed JSON", "null column"} {
		t.Run(failure, func(t *testing.T) {
			// An incomplete or malformed result never returns partial observations.
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				if r.Method == http.MethodPost {
					writePage(t, w, server.URL+"/first")
					return
				}
				if r.URL.Path == "/first" {
					writePage(t, w, server.URL+"/next", `["g:a@1"]`)
					return
				}
				switch failure {
				case "iteration":
					_ = json.NewEncoder(w).Encode(map[string]any{"id": "test", "error": map[string]any{"message": "failed", "errorCode": 1, "errorName": "GENERIC_INTERNAL_ERROR", "errorType": "INTERNAL_ERROR"}})
				case "malformed JSON":
					writePage(t, w, "", "not JSON")
				case "null column":
					page := resultPage("")
					page["data"] = [][]any{{nil}}
					_ = json.NewEncoder(w).Encode(page)
				}
			}))
			defer server.Close()
			got, err := New(openTestDB(t, server.URL)).Lookup(context.Background(), Request{For: []Coordinate{{"g", "a"}}, JVM: 21})
			if err == nil || got != nil {
				t.Fatalf("Lookup = %v, %v; want error without observations", got, err)
			}
		})
	}
}

func TestLookupCancelsUpstreamWhenContextEnds(t *testing.T) {
	for _, ending := range []string{"cancelled", "deadline"} {
		t.Run(ending, func(t *testing.T) {
			// Ending the caller context cancels upstream work and discards partial rows.
			var cancelled atomic.Int32
			started := make(chan struct{})
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodPost:
					writePage(t, w, server.URL+"/first")
				case http.MethodGet:
					if r.URL.Path == "/first" {
						writePage(t, w, server.URL+"/next", `["g:a@1"]`)
						return
					}
					close(started)
					<-r.Context().Done()
				case http.MethodDelete:
					cancelled.Add(1)
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			wantErr := context.Canceled
			if ending == "deadline" {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
				wantErr = context.DeadlineExceeded
			} else {
				go func() { <-started; cancel() }()
			}
			defer cancel()
			got, err := New(openTestDB(t, server.URL)).Lookup(ctx, Request{For: []Coordinate{{"g", "a"}}, JVM: 21})
			if !errors.Is(err, wantErr) || got != nil || cancelled.Load() == 0 {
				t.Fatalf("Lookup = %v, %v; cancellations = %d", got, err, cancelled.Load())
			}
		})
	}
}

func openTestDB(t *testing.T, serverURL string) *sql.DB {
	t.Helper()
	db, err := sql.Open("trino", serverURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func resultPage(next string) map[string]any {
	page := map[string]any{
		"id":      "test",
		"columns": []map[string]any{{"name": "versions", "type": "varchar", "typeSignature": map[string]any{"rawType": "varchar", "arguments": []any{}}}},
	}
	if next != "" {
		page["nextUri"] = next
	}
	return page
}

func writePage(t *testing.T, w http.ResponseWriter, next string, values ...string) {
	t.Helper()
	page := resultPage(next)
	rows := make([][]string, 0, len(values))
	for _, value := range values {
		rows = append(rows, []string{value})
	}
	page["data"] = rows
	if err := json.NewEncoder(w).Encode(page); err != nil {
		t.Errorf("write query response: %v", err)
	}
}
