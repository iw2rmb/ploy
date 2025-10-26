package metrics

import "github.com/prometheus/client_golang/prometheus"

// ArtifactPinRecorder captures pin-state observations and retry counts.
type ArtifactPinRecorder interface {
	UpdateState(counts map[string]int)
	ObserveRetry(kind string)
}

// ArtifactPinMetrics exports Prometheus collectors tracking artifact pin health.
type ArtifactPinMetrics struct {
	stateGauge *prometheus.GaugeVec
	retryTotal *prometheus.CounterVec
}

// NewArtifactPinMetrics registers pin-state Prometheus collectors.
func NewArtifactPinMetrics(reg prometheus.Registerer) (*ArtifactPinMetrics, error) {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	stateGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "ploy",
		Subsystem: "artifacts",
		Name:      "pin_state",
		Help:      "Count of artifacts observed per pin state.",
	}, []string{"state"})

	retryTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "ploy",
		Subsystem: "artifacts",
		Name:      "retry_total",
		Help:      "Count of artifact pin retries triggered by the reconciler.",
	}, []string{"kind"})

	var err error
	stateGauge, err = registerGaugeVec(reg, stateGauge)
	if err != nil {
		return nil, err
	}
	retryTotal, err = registerCounterVec(reg, retryTotal)
	if err != nil {
		return nil, err
	}

	return &ArtifactPinMetrics{stateGauge: stateGauge, retryTotal: retryTotal}, nil
}

// UpdateState sets the gauge value for each observed pin state.
func (m *ArtifactPinMetrics) UpdateState(counts map[string]int) {
	if m == nil {
		return
	}
	for state, value := range counts {
		m.stateGauge.WithLabelValues(state).Set(float64(value))
	}
}

// ObserveRetry increments the retry counter for the provided artifact kind.
func (m *ArtifactPinMetrics) ObserveRetry(kind string) {
	if m == nil {
		return
	}
	if kind == "" {
		kind = "unknown"
	}
	m.retryTotal.WithLabelValues(kind).Inc()
}

// UpdateState satisfies ArtifactPinRecorder.
var _ ArtifactPinRecorder = (*ArtifactPinMetrics)(nil)
