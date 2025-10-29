package hydration

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/iw2rmb/ploy/internal/metrics"
)

// PolicyDecisionLevel indicates whether a decision is advisory or enforced.
type PolicyDecisionLevel string

const (
	PolicyDecisionLevelWarn    PolicyDecisionLevel = "warn"
	PolicyDecisionLevelEnforce PolicyDecisionLevel = "enforce"
)

// PolicyDecisionAction enumerates controller operations required for enforcement.
type PolicyDecisionAction string

const (
	PolicyActionWarn              PolicyDecisionAction = "warn"
	PolicyActionReduceReplication PolicyDecisionAction = "reduce_replication"
	PolicyActionEvictSnapshot     PolicyDecisionAction = "evict_snapshot"
)

// PolicyDecision describes a policy breach and the corrective action to perform.
type PolicyDecision struct {
	PolicyID          string
	Fingerprint       string
	Level             PolicyDecisionLevel
	Action            PolicyDecisionAction
	TargetReplication int
	Reason            string
	Usage             PolicyUsage
	Snapshot          SnapshotEntry
}

// PolicyEngineOptions configure the engine.
type PolicyEngineOptions struct {
	Store   *PolicyStore
	Index   *Index
	Clock   func() time.Time
	Metrics *metrics.HydrationMetrics
}

// PolicyEngine evaluates global policies against hydration snapshots and emits decisions.
type PolicyEngine struct {
	store     *PolicyStore
	index     *Index
	clock     func() time.Time
	metrics   *metrics.HydrationMetrics
	decisions chan PolicyDecision
	triggerCh chan struct{}
	once      sync.Once
	started   bool
	mu        sync.Mutex
}

// NewPolicyEngine constructs a new policy engine.
func NewPolicyEngine(opts PolicyEngineOptions) (*PolicyEngine, error) {
	if opts.Store == nil {
		return nil, fmt.Errorf("hydration: policy store required")
	}
	if opts.Index == nil {
		return nil, fmt.Errorf("hydration: hydration index required")
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	engine := &PolicyEngine{
		store:     opts.Store,
		index:     opts.Index,
		clock:     clock,
		metrics:   opts.Metrics,
		decisions: make(chan PolicyDecision, 32),
		triggerCh: make(chan struct{}, 1),
	}
	return engine, nil
}

// Start begins watching policy and snapshot changes.
func (e *PolicyEngine) Start(ctx context.Context) error {
	e.mu.Lock()
	if e.started {
		e.mu.Unlock()
		return fmt.Errorf("hydration: policy engine already started")
	}
	e.started = true
	e.mu.Unlock()

	go e.run(ctx)
	e.enqueueTrigger()
	return nil
}

// Decisions returns a channel of policy decisions.
func (e *PolicyEngine) Decisions() <-chan PolicyDecision {
	return e.decisions
}

// Trigger requests an immediate evaluation cycle.
func (e *PolicyEngine) Trigger() {
	e.enqueueTrigger()
}

func (e *PolicyEngine) run(ctx context.Context) {
	defer close(e.decisions)

	indexWatch := e.watchPrefix(ctx, e.index.client, e.index.prefix)
	policyWatch := e.watchPrefix(ctx, e.store.client, e.store.prefix)

	for {
		select {
		case <-ctx.Done():
			return
		case <-indexWatch:
			e.evaluate(ctx)
		case <-policyWatch:
			e.evaluate(ctx)
		case <-e.triggerCh:
			e.evaluate(ctx)
		}
	}
}

func (e *PolicyEngine) watchPrefix(ctx context.Context, client *clientv3.Client, prefix string) <-chan struct{} {
	ch := make(chan struct{}, 1)
	if client == nil {
		close(ch)
		return ch
	}
	watchCh := client.Watch(ctx, prefix, clientv3.WithPrefix())
	go func() {
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-watchCh:
				if !ok {
					return
				}
				select {
				case ch <- struct{}{}:
				default:
				}
			}
		}
	}()
	return ch
}

func (e *PolicyEngine) enqueueTrigger() {
	select {
	case e.triggerCh <- struct{}{}:
	default:
	}
}

func (e *PolicyEngine) evaluate(ctx context.Context) {
	policies, err := e.store.ListPolicies(ctx)
	if err != nil {
		log.Printf("policy engine: list policies: %v", err)
		return
	}
	if len(policies) == 0 {
		return
	}

	snapshots, err := e.index.ListSnapshots(ctx)
	if err != nil {
		log.Printf("policy engine: list snapshots: %v", err)
		return
	}

	usageByPolicy := make(map[string]*PolicyUsage, len(policies))
	matches := make(map[string][]SnapshotEntry, len(policies))
	for _, policy := range policies {
		usageByPolicy[policy.ID] = &PolicyUsage{PolicyID: policy.ID}
		matches[policy.ID] = nil
	}

	for _, snapshot := range snapshots {
		policy := matchPolicyForEntry(policies, snapshot)
		if policy == nil {
			continue
		}
		usage := usageByPolicy[policy.ID]
		usage.PinnedBytes += snapshot.Bundle.Size
		usage.SnapshotCount++
		usage.ReplicaCount += replicationCount(snapshot)
		usage.ActiveFingerprints = append(usage.ActiveFingerprints, snapshot.Fingerprint)
		matches[policy.ID] = append(matches[policy.ID], snapshot)
	}

	now := e.clock().UTC()
	for _, policy := range policies {
		usage := usageByPolicy[policy.ID]
		usage.UpdatedAt = now
		normalized := usageNormalized(*usage)
		stored := usageNormalized(policy.Usage)
		if !usageEqual(normalized, stored) {
			if _, err := e.store.RecordUsage(ctx, policy.ID, normalized); err != nil {
				log.Printf("policy engine: record usage %s: %v", policy.ID, err)
			}
		}
		e.metrics.ObserveUsage(policy.ID, normalized.PinnedBytes, normalized.SnapshotCount, normalized.ReplicaCount)
		e.emitDecisions(policy, matches[policy.ID], normalized)
	}
}

func replicationCount(snapshot SnapshotEntry) int {
	if snapshot.Replication.Max > 0 {
		return snapshot.Replication.Max
	}
	if snapshot.Replication.Min > 0 {
		return snapshot.Replication.Min
	}
	return 1
}

func usageNormalized(usage PolicyUsage) PolicyUsage {
	out := usage
	if len(out.ActiveFingerprints) > 0 {
		out.ActiveFingerprints = normalizeStrings(out.ActiveFingerprints)
	}
	return out
}

func usageEqual(a, b PolicyUsage) bool {
	if a.PolicyID != b.PolicyID {
		return false
	}
	if a.PinnedBytes != b.PinnedBytes {
		return false
	}
	if a.SnapshotCount != b.SnapshotCount {
		return false
	}
	if a.ReplicaCount != b.ReplicaCount {
		return false
	}
	if len(a.ActiveFingerprints) != len(b.ActiveFingerprints) {
		return false
	}
	for i := range a.ActiveFingerprints {
		if a.ActiveFingerprints[i] != b.ActiveFingerprints[i] {
			return false
		}
	}
	return true
}

func (e *PolicyEngine) emitDecisions(policy GlobalPolicy, snapshots []SnapshotEntry, usage PolicyUsage) {
	e.checkCountLimit(policy, usage, policy.Window.Snapshots, usage.SnapshotCount, "snapshots", snapshots)
	e.checkBytesLimit(policy, usage, policy.Window.PinnedBytes, usage.PinnedBytes, "pinned_bytes", snapshots)
	e.checkReplicaLimit(policy, usage, policy.Window.Replicas, usage.ReplicaCount, "replicas", snapshots)
}

func (e *PolicyEngine) checkBytesLimit(policy GlobalPolicy, usage PolicyUsage, limit LimitBytes, value int64, name string, snapshots []SnapshotEntry) {
	soft, hard := bytesExceeded(value, limit)
	if soft && !hard {
		e.publish(PolicyDecision{
			PolicyID: policy.ID,
			Level:    PolicyDecisionLevelWarn,
			Action:   PolicyActionWarn,
			Reason:   fmt.Sprintf("%s soft limit exceeded (%d > %d)", name, value, limit.Soft),
			Usage:    usage,
		})
		return
	}
	if !hard {
		return
	}

	excess := value - limit.Hard
	if excess <= 0 {
		return
	}

	sorted := append([]SnapshotEntry(nil), snapshots...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].UpdatedAt.Before(sorted[j].UpdatedAt)
	})

	for _, snapshot := range sorted {
		e.publish(PolicyDecision{
			PolicyID:    policy.ID,
			Fingerprint: snapshot.Fingerprint,
			Level:       PolicyDecisionLevelEnforce,
			Action:      PolicyActionEvictSnapshot,
			Reason:      fmt.Sprintf("%s hard limit exceeded (%d > %d)", name, value, limit.Hard),
			Usage:       usage,
			Snapshot:    snapshot,
		})
		excess -= snapshot.Bundle.Size
		if excess <= 0 {
			break
		}
	}
}

func (e *PolicyEngine) checkCountLimit(policy GlobalPolicy, usage PolicyUsage, limit LimitCount, value int, name string, snapshots []SnapshotEntry) {
	soft, hard := countExceeded(value, limit)
	if soft && !hard {
		e.publish(PolicyDecision{
			PolicyID: policy.ID,
			Level:    PolicyDecisionLevelWarn,
			Action:   PolicyActionWarn,
			Reason:   fmt.Sprintf("%s soft limit exceeded (%d > %d)", name, value, limit.Soft),
			Usage:    usage,
		})
		return
	}
	if !hard {
		return
	}

	need := value - limit.Hard
	if need <= 0 {
		return
	}

	sorted := append([]SnapshotEntry(nil), snapshots...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].UpdatedAt.Before(sorted[j].UpdatedAt)
	})

	for _, snapshot := range sorted {
		e.publish(PolicyDecision{
			PolicyID:    policy.ID,
			Fingerprint: snapshot.Fingerprint,
			Level:       PolicyDecisionLevelEnforce,
			Action:      PolicyActionEvictSnapshot,
			Reason:      fmt.Sprintf("%s hard limit exceeded (%d > %d)", name, value, limit.Hard),
			Usage:       usage,
			Snapshot:    snapshot,
		})
		need--
		if need <= 0 {
			break
		}
	}
}

func (e *PolicyEngine) checkReplicaLimit(policy GlobalPolicy, usage PolicyUsage, limit LimitCount, value int, name string, snapshots []SnapshotEntry) {
	soft, hard := countExceeded(value, limit)
	if soft && !hard {
		e.publish(PolicyDecision{
			PolicyID: policy.ID,
			Level:    PolicyDecisionLevelWarn,
			Action:   PolicyActionWarn,
			Reason:   fmt.Sprintf("%s soft limit exceeded (%d > %d)", name, value, limit.Soft),
			Usage:    usage,
		})
		return
	}
	if !hard {
		return
	}

	excess := value - limit.Hard
	if excess <= 0 {
		return
	}

	sorted := append([]SnapshotEntry(nil), snapshots...)
	sort.Slice(sorted, func(i, j int) bool {
		return replicationCount(sorted[i]) > replicationCount(sorted[j])
	})

	for _, snapshot := range sorted {
		current := replicationCount(snapshot)
		target := limit.Hard
		if snapshot.Replication.Min > 0 && target < snapshot.Replication.Min {
			target = snapshot.Replication.Min
		}
		if current <= target {
			continue
		}
		delta := current - target
		e.publish(PolicyDecision{
			PolicyID:          policy.ID,
			Fingerprint:       snapshot.Fingerprint,
			Level:             PolicyDecisionLevelEnforce,
			Action:            PolicyActionReduceReplication,
			TargetReplication: target,
			Reason:            fmt.Sprintf("%s hard limit exceeded (%d > %d)", name, value, limit.Hard),
			Usage:             usage,
			Snapshot:          snapshot,
		})
		excess -= delta
		if excess <= 0 {
			break
		}
	}
}

func bytesExceeded(value int64, limit LimitBytes) (bool, bool) {
	soft := limit.Soft > 0 && value > limit.Soft
	hard := limit.Hard > 0 && value > limit.Hard
	if hard {
		soft = true
	}
	return soft, hard
}

func countExceeded(value int, limit LimitCount) (bool, bool) {
	soft := limit.Soft > 0 && value > limit.Soft
	hard := limit.Hard > 0 && value > limit.Hard
	if hard {
		soft = true
	}
	return soft, hard
}

func (e *PolicyEngine) publish(decision PolicyDecision) {
	select {
	case e.decisions <- decision:
		if e.metrics != nil {
			e.metrics.ObserveDecision(decision.PolicyID, string(decision.Action), string(decision.Level))
		}
	default:
		log.Printf("policy engine: dropping decision for policy %s", decision.PolicyID)
	}
}
