package metrics

import "github.com/prometheus/client_golang/prometheus"

// HydrationMetrics exports Prometheus collectors tracking hydration policy usage.
type HydrationMetrics struct {
	policyDecisions *prometheus.CounterVec
	pinnedBytes     *prometheus.GaugeVec
	snapshotCount   *prometheus.GaugeVec
	replicaCount    *prometheus.GaugeVec
}

// NewHydrationMetrics registers hydration Prometheus collectors.
func NewHydrationMetrics(reg prometheus.Registerer) (*HydrationMetrics, error) {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	policyDecisions := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "ploy",
		Subsystem: "hydration",
		Name:      "policy_decisions_total",
		Help:      "Count of hydration policy decisions emitted partitioned by policy, action, and level.",
	}, []string{"policy", "action", "level"})

	pinnedBytes := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "ploy",
		Subsystem: "hydration",
		Name:      "policy_pinned_bytes",
		Help:      "Pinned bytes consumed by hydration snapshots per policy.",
	}, []string{"policy"})

	snapshotCount := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "ploy",
		Subsystem: "hydration",
		Name:      "policy_snapshot_count",
		Help:      "Active hydration snapshots tracked per policy.",
	}, []string{"policy"})

	replicaCount := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "ploy",
		Subsystem: "hydration",
		Name:      "policy_replica_count",
		Help:      "Total replication target count per policy.",
	}, []string{"policy"})

	var err error
	policyDecisions, err = registerCounterVec(reg, policyDecisions)
	if err != nil {
		return nil, err
	}
	pinnedBytes, err = registerGaugeVec(reg, pinnedBytes)
	if err != nil {
		return nil, err
	}
	snapshotCount, err = registerGaugeVec(reg, snapshotCount)
	if err != nil {
		return nil, err
	}
	replicaCount, err = registerGaugeVec(reg, replicaCount)
	if err != nil {
		return nil, err
	}

	return &HydrationMetrics{
		policyDecisions: policyDecisions,
		pinnedBytes:     pinnedBytes,
		snapshotCount:   snapshotCount,
		replicaCount:    replicaCount,
	}, nil
}

// ObserveDecision increments the decision counter for the provided policy/action/level tuple.
func (m *HydrationMetrics) ObserveDecision(policyID, action, level string) {
	if m == nil {
		return
	}
	if policyID == "" {
		policyID = "unknown"
	}
	if action == "" {
		action = "none"
	}
	if level == "" {
		level = "unknown"
	}
	m.policyDecisions.WithLabelValues(policyID, action, level).Inc()
}

// ObserveUsage records the current usage gauges for the provided policy.
func (m *HydrationMetrics) ObserveUsage(policyID string, pinnedBytes int64, snapshots, replicas int) {
	if m == nil {
		return
	}
	if policyID == "" {
		policyID = "unknown"
	}
	m.pinnedBytes.WithLabelValues(policyID).Set(float64(pinnedBytes))
	m.snapshotCount.WithLabelValues(policyID).Set(float64(snapshots))
	m.replicaCount.WithLabelValues(policyID).Set(float64(replicas))
}
