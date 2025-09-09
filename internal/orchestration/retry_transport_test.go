package orchestration

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type stubRT struct {
	calls int
	codes []int
}

func (s *stubRT) RoundTrip(req *http.Request) (*http.Response, error) {
	code := 200
	if s.calls < len(s.codes) {
		code = s.codes[s.calls]
	}
	s.calls++
	return &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestRetryTransport_RetriesOn429ThenSucceeds(t *testing.T) {
	base := &stubRT{codes: []int{429, 429, 200}}
	rt := &RetryTransport{Base: base, MaxRetries: 5, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond, JitterFrac: 0}
	req, _ := http.NewRequest("GET", "http://example/", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if base.calls < 3 {
		t.Fatalf("expected at least 3 calls, got %d", base.calls)
	}
}

func TestRetryTransport_RetriesOn5xxThenFails(t *testing.T) {
	base := &stubRT{codes: []int{503, 503, 503, 503, 503, 503}}
	rt := &RetryTransport{Base: base, MaxRetries: 3, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond, JitterFrac: 0}
	req, _ := http.NewRequest("GET", "http://example/", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// After 4 attempts (0..3), last response should be 503 and returned
	if resp.StatusCode != 503 {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}
