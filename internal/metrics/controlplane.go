package metrics

import (
	"math"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// SchedulerRecorder captures control plane scheduler metrics.
type SchedulerRecorder interface {
	QueueEnqueued(priority string)
	QueueDequeued(priority string)
	ObserveClaimLatency(priority string, latency time.Duration)
	ObserveJobRetry(priority, reason string)
	ObserveGateDuration(stepID, result string, duration time.Duration)
}

// SchedulerMetrics exports Prometheus collectors for the control plane scheduler.
type SchedulerMetrics struct {
	queueDepth   *prometheus.GaugeVec
	claimLatency *prometheus.HistogramVec
	jobRetry     *prometheus.CounterVec
	gateDuration *prometheus.HistogramVec
}

// NewSchedulerMetrics registers scheduler Prometheus collectors against the provided registry.
func NewSchedulerMetrics(reg prometheus.Registerer) (*SchedulerMetrics, error) {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	queueDepth := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "ploy",
		Subsystem: "controlplane",
		Name:      "queue_depth",
		Help:      "Number of jobs waiting to be claimed partitioned by priority.",
	}, []string{"priority"})

	claimLatency := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "ploy",
		Subsystem: "controlplane",
		Name:      "claim_latency_seconds",
		Help:      "Latency between job enqueue and claim.",
		Buckets:   []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120},
	}, []string{"priority"})

	jobRetry := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "ploy",
		Subsystem: "controlplane",
		Name:      "job_retry_total",
		Help:      "Count of job retries triggered by the scheduler partitioned by priority and reason.",
	}, []string{"priority", "reason"})

	gateDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "ploy",
		Subsystem: "controlplane",
		Name:      "gate_duration_seconds",
		Help:      "Duration of Build Gate execution per step.",
		Buckets:   []float64{0.25, 0.5, 1, 2, 5, 10, 20, 40, 80, 160},
	}, []string{"step_id", "result"})

	var err error
	queueDepth, err = registerGaugeVec(reg, queueDepth)
	if err != nil {
		return nil, err
	}
	claimLatency, err = registerHistogramVec(reg, claimLatency)
	if err != nil {
		return nil, err
	}
	jobRetry, err = registerCounterVec(reg, jobRetry)
	if err != nil {
		return nil, err
	}
	gateDuration, err = registerHistogramVec(reg, gateDuration)
	if err != nil {
		return nil, err
	}

	return &SchedulerMetrics{
		queueDepth:   queueDepth,
		claimLatency: claimLatency,
		jobRetry:     jobRetry,
		gateDuration: gateDuration,
	}, nil
}

// NewNoopSchedulerRecorder returns a scheduler recorder that discards observations.
func NewNoopSchedulerRecorder() SchedulerRecorder {
	return noopSchedulerRecorder{}
}

// QueueEnqueued increments the queue depth gauge for the provided priority.
func (m *SchedulerMetrics) QueueEnqueued(priority string) {
	if m == nil {
		return
	}
	m.queueDepth.WithLabelValues(priority).Inc()
}

// QueueDequeued decrements the queue depth gauge for the provided priority.
func (m *SchedulerMetrics) QueueDequeued(priority string) {
	if m == nil {
		return
	}
	m.queueDepth.WithLabelValues(priority).Dec()
}

// ObserveClaimLatency records the observed latency between enqueue and claim.
func (m *SchedulerMetrics) ObserveClaimLatency(priority string, latency time.Duration) {
	if m == nil {
		return
	}
	seconds := math.Max(latency.Seconds(), 0)
	m.claimLatency.WithLabelValues(priority).Observe(seconds)
}

// ObserveJobRetry increments the retry counter for the provided priority and reason.
func (m *SchedulerMetrics) ObserveJobRetry(priority, reason string) {
	if m == nil {
		return
	}
	m.jobRetry.WithLabelValues(priority, reason).Inc()
}

// ObserveShiftDuration records the duration of SHIFT execution per step.
func (m *SchedulerMetrics) ObserveGateDuration(stepID, result string, duration time.Duration) {
	if m == nil || duration <= 0 {
		return
	}
	seconds := math.Max(duration.Seconds(), 0)
	if strings.TrimSpace(result) == "" {
		result = "unknown"
	}
	m.gateDuration.WithLabelValues(stepID, result).Observe(seconds)
}

type noopSchedulerRecorder struct{}

func (noopSchedulerRecorder) QueueEnqueued(string)                              {}
func (noopSchedulerRecorder) QueueDequeued(string)                              {}
func (noopSchedulerRecorder) ObserveClaimLatency(string, time.Duration)         {}
func (noopSchedulerRecorder) ObserveJobRetry(string, string)                    {}
func (noopSchedulerRecorder) ObserveGateDuration(string, string, time.Duration) {}
