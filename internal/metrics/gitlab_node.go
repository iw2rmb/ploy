// Package metrics exposes internal metric helpers used across Ploy services.
package metrics

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// GitLabNodeRecorder captures GitLab token refresh health metrics for worker nodes.
type GitLabNodeRecorder interface {
	RefreshSuccess(secret string, expiresAt time.Time)
	RefreshFailure(secret string, err error)
	CacheFlushed(secret string)
}

// GitLabNodeMetrics exports GitLab node metrics via Prometheus collectors.
type GitLabNodeMetrics struct {
	refreshTotal *prometheus.CounterVec
	cacheFlush   *prometheus.CounterVec
	expiryGauge  *prometheus.GaugeVec
}

// NewGitLabNodeMetrics registers GitLab node Prometheus collectors against the provided registry.
func NewGitLabNodeMetrics(reg prometheus.Registerer) (*GitLabNodeMetrics, error) {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	refreshTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "ploy",
		Subsystem: "node_gitlab",
		Name:      "token_refresh_total",
		Help:      "Count of GitLab token refresh attempts partitioned by result.",
	}, []string{"secret", "result"})

	cacheFlush := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "ploy",
		Subsystem: "node_gitlab",
		Name:      "cache_flush_total",
		Help:      "Count of GitLab credential cache flush operations.",
	}, []string{"secret"})

	expiryGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "ploy",
		Subsystem: "node_gitlab",
		Name:      "token_expiry_timestamp",
		Help:      "Unix timestamp for the current GitLab token expiry.",
	}, []string{"secret"})

	var err error
	refreshTotal, err = registerCounterVec(reg, refreshTotal)
	if err != nil {
		return nil, err
	}
	cacheFlush, err = registerCounterVec(reg, cacheFlush)
	if err != nil {
		return nil, err
	}
	expiryGauge, err = registerGaugeVec(reg, expiryGauge)
	if err != nil {
		return nil, err
	}

	return &GitLabNodeMetrics{
		refreshTotal: refreshTotal,
		cacheFlush:   cacheFlush,
		expiryGauge:  expiryGauge,
	}, nil
}

func registerCounterVec(reg prometheus.Registerer, collector *prometheus.CounterVec) (*prometheus.CounterVec, error) {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	if err := reg.Register(collector); err != nil {
		if already, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := already.ExistingCollector.(*prometheus.CounterVec); ok {
				return existing, nil
			}
		}
		return nil, err
	}
	return collector, nil
}

func registerGaugeVec(reg prometheus.Registerer, collector *prometheus.GaugeVec) (*prometheus.GaugeVec, error) {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	if err := reg.Register(collector); err != nil {
		if already, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := already.ExistingCollector.(*prometheus.GaugeVec); ok {
				return existing, nil
			}
		}
		return nil, err
	}
	return collector, nil
}

// RefreshSuccess increments success counters and records the expiry timestamp.
func (m *GitLabNodeMetrics) RefreshSuccess(secret string, expiresAt time.Time) {
	if m == nil {
		return
	}
	m.refreshTotal.WithLabelValues(secret, "success").Inc()
	m.expiryGauge.WithLabelValues(secret).Set(float64(expiresAt.Unix()))
}

// RefreshFailure increments failure counters.
func (m *GitLabNodeMetrics) RefreshFailure(secret string, err error) {
	if m == nil {
		return
	}
	_ = err
	m.refreshTotal.WithLabelValues(secret, "failure").Inc()
}

// CacheFlushed increments cache flush counters for the provided secret.
func (m *GitLabNodeMetrics) CacheFlushed(secret string) {
	if m == nil {
		return
	}
	m.cacheFlush.WithLabelValues(secret).Inc()
}

// NoopGitLabNodeRecorder is a no-op metrics recorder.
type NoopGitLabNodeRecorder struct{}

// RefreshSuccess implements GitLabNodeRecorder.
func (NoopGitLabNodeRecorder) RefreshSuccess(string, time.Time) {}

// RefreshFailure implements GitLabNodeRecorder.
func (NoopGitLabNodeRecorder) RefreshFailure(string, error) {}

// CacheFlushed implements GitLabNodeRecorder.
func (NoopGitLabNodeRecorder) CacheFlushed(string) {}

// NewNoopGitLabNodeRecorder returns a no-op recorder.
func NewNoopGitLabNodeRecorder() GitLabNodeRecorder {
	return NoopGitLabNodeRecorder{}
}

type refreshTotals struct {
	success int
	failure int
	expiry  time.Time
}

// InMemoryGitLabNodeRecorder records metrics for testing without Prometheus.
type InMemoryGitLabNodeRecorder struct {
	mu      sync.Mutex
	refresh map[string]*refreshTotals
	flushes map[string]int
}

// NewInMemoryGitLabNodeRecorder constructs an in-memory GitLab recorder.
func NewInMemoryGitLabNodeRecorder() *InMemoryGitLabNodeRecorder {
	return &InMemoryGitLabNodeRecorder{
		refresh: make(map[string]*refreshTotals),
		flushes: make(map[string]int),
	}
}

// RefreshSuccess implements GitLabNodeRecorder.
func (r *InMemoryGitLabNodeRecorder) RefreshSuccess(secret string, expiresAt time.Time) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	totals := r.ensure(secret)
	totals.success++
	totals.expiry = expiresAt
}

// RefreshFailure implements GitLabNodeRecorder.
func (r *InMemoryGitLabNodeRecorder) RefreshFailure(secret string, err error) {
	if r == nil {
		return
	}
	_ = err
	r.mu.Lock()
	defer r.mu.Unlock()
	totals := r.ensure(secret)
	totals.failure++
}

// CacheFlushed implements GitLabNodeRecorder.
func (r *InMemoryGitLabNodeRecorder) CacheFlushed(secret string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flushes[secret]++
}

// RefreshTotals returns the success/failure counts for a secret.
func (r *InMemoryGitLabNodeRecorder) RefreshTotals(secret string) map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	totals := r.ensure(secret)
	return map[string]int{
		"success": totals.success,
		"failure": totals.failure,
	}
}

// CacheFlushes returns the cache flush count for a secret.
func (r *InMemoryGitLabNodeRecorder) CacheFlushes(secret string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.flushes[secret]
}

// LastExpiry returns the last recorded token expiry.
func (r *InMemoryGitLabNodeRecorder) LastExpiry(secret string) time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ensure(secret).expiry
}

func (r *InMemoryGitLabNodeRecorder) ensure(secret string) *refreshTotals {
	if totals, ok := r.refresh[secret]; ok {
		return totals
	}
	totals := &refreshTotals{}
	r.refresh[secret] = totals
	return totals
}
