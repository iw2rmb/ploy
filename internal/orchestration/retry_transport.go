package orchestration

import (
	"io"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// RetryTransport wraps an http.RoundTripper to add retry/backoff for 429 and 5xx responses.
type RetryTransport struct {
	Base       http.RoundTripper
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	JitterFrac float64 // 0.0..1.0 fraction of delay to jitter
}

func envInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i >= 0 {
			return i
		}
	}
	return def
}

func envDur(name string, def time.Duration) time.Duration {
	if v := os.Getenv(name); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// NewDefaultRetryTransport builds a transport with sane defaults configurable via env:
// NOMAD_HTTP_MAX_RETRIES, NOMAD_HTTP_BASE_DELAY, NOMAD_HTTP_MAX_DELAY
func NewDefaultRetryTransport(base http.RoundTripper) *RetryTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &RetryTransport{
		Base:       base,
		MaxRetries: envInt("NOMAD_HTTP_MAX_RETRIES", 5),
		BaseDelay:  envDur("NOMAD_HTTP_BASE_DELAY", 500*time.Millisecond),
		MaxDelay:   envDur("NOMAD_HTTP_MAX_DELAY", 30*time.Second),
		JitterFrac: 0.2,
	}
}

func (t *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var bodyBytes []byte
	if req.Body != nil {
		// Preserve body for retries
		b, _ := io.ReadAll(req.Body)
		bodyBytes = b
		req.Body = io.NopCloser(strings.NewReader(string(b)))
	}

	// attempt 0..MaxRetries
	for attempt := 0; attempt <= t.MaxRetries; attempt++ {
		// Recreate body
		if bodyBytes != nil {
			req.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
		}

		resp, err := t.base().RoundTrip(req)
		if err != nil {
			if attempt == t.MaxRetries {
				return nil, err
			}
			t.sleepBackoff(attempt, 0)
			continue
		}

		// Retry on 429 and 5xx
		if resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode >= 500 && resp.StatusCode <= 599) {
			// Respect Retry-After if provided
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			if attempt < t.MaxRetries {
				// Drain and close body before retrying
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				t.sleepBackoff(attempt, retryAfter)
				continue
			}
		}
		return resp, err
	}
	// Should not reach here
	return nil, nil
}

func (t *RetryTransport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
}

func (t *RetryTransport) sleepBackoff(attempt int, retryAfter time.Duration) {
	if retryAfter > 0 {
		time.Sleep(retryAfter)
		return
	}
	// exponential backoff with jitter
	pow := math.Pow(2, float64(attempt))
	delay := time.Duration(float64(t.BaseDelay) * pow)
	if delay > t.MaxDelay {
		delay = t.MaxDelay
	}
	if t.JitterFrac > 0 {
		jitter := rand.Float64()*2*t.JitterFrac - t.JitterFrac // [-frac, +frac]
		delay = time.Duration(float64(delay) * (1 + jitter))
	}
	time.Sleep(delay)
}

func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}
