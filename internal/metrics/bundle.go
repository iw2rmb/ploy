package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// BundleRecorder captures IPFS bundle pin metrics for alerting hooks.
type BundleRecorder interface {
	// PinSuccess records a successful pin attempt for the provided artifact kind.
	PinSuccess(kind string, duration time.Duration)
	// PinFailure records a failed pin attempt for the provided artifact kind.
	PinFailure(kind string, err error)
	// PinRetry increments retry counters for the provided artifact kind.
	PinRetry(kind string)
}

// NoopBundleRecorder implements BundleRecorder without emitting metrics.
type NoopBundleRecorder struct{}

// PinSuccess is a no-op implementation.
func (NoopBundleRecorder) PinSuccess(string, time.Duration) {}

// PinFailure is a no-op implementation.
func (NoopBundleRecorder) PinFailure(string, error) {}

// PinRetry is a no-op implementation.
func (NoopBundleRecorder) PinRetry(string) {}

// NewNoopBundleRecorder constructs a BundleRecorder that discards measurements.
func NewNoopBundleRecorder() BundleRecorder {
	return NoopBundleRecorder{}
}

// BundleMetrics exports Prometheus collectors for IPFS bundle pin attempts.
type BundleMetrics struct {
	pinTotal    *prometheus.CounterVec
	pinRetry    *prometheus.CounterVec
	pinDuration *prometheus.HistogramVec
}

// NewBundleMetrics registers bundle metrics against the provided registry.
func NewBundleMetrics(reg prometheus.Registerer) (*BundleMetrics, error) {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	pinTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "ploy",
		Subsystem: "ipfs_bundle",
		Name:      "pin_total",
		Help:      "Count of IPFS bundle pin attempts partitioned by result.",
	}, []string{"kind", "result"})

	pinRetry := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "ploy",
		Subsystem: "ipfs_bundle",
		Name:      "pin_retry_total",
		Help:      "Count of IPFS bundle pin retries.",
	}, []string{"kind"})

	pinDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "ploy",
		Subsystem: "ipfs_bundle",
		Name:      "pin_duration_seconds",
		Help:      "Duration of IPFS bundle pin attempts.",
		Buckets:   prometheus.ExponentialBuckets(0.01, 2, 12),
	}, []string{"kind"})

	var err error
	pinTotal, err = registerCounterVec(reg, pinTotal)
	if err != nil {
		return nil, err
	}
	pinRetry, err = registerCounterVec(reg, pinRetry)
	if err != nil {
		return nil, err
	}
	pinDuration, err = registerHistogramVec(reg, pinDuration)
	if err != nil {
		return nil, err
	}

	return &BundleMetrics{
		pinTotal:    pinTotal,
		pinRetry:    pinRetry,
		pinDuration: pinDuration,
	}, nil
}

// PinSuccess records a successful pin attempt duration.
func (m *BundleMetrics) PinSuccess(kind string, duration time.Duration) {
	if m == nil {
		return
	}
	m.pinTotal.WithLabelValues(kind, "success").Inc()
	m.pinDuration.WithLabelValues(kind).Observe(duration.Seconds())
}

// PinFailure increments the failure counter for the provided kind.
func (m *BundleMetrics) PinFailure(kind string, err error) {
	if m == nil {
		return
	}
	_ = err
	m.pinTotal.WithLabelValues(kind, "failure").Inc()
}

// PinRetry increments the retry counter.
func (m *BundleMetrics) PinRetry(kind string) {
	if m == nil {
		return
	}
	m.pinRetry.WithLabelValues(kind).Inc()
}

func registerHistogramVec(reg prometheus.Registerer, collector *prometheus.HistogramVec) (*prometheus.HistogramVec, error) {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	if err := reg.Register(collector); err != nil {
		if already, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := already.ExistingCollector.(*prometheus.HistogramVec); ok {
				return existing, nil
			}
		}
		return nil, err
	}
	return collector, nil
}
