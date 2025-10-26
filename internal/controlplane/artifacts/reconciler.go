package artifacts

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/metrics"
	workflowartifacts "github.com/iw2rmb/ploy/internal/workflow/artifacts"
)

type statusClient interface {
	Status(ctx context.Context, cid string) (workflowartifacts.StatusResult, error)
	Pin(ctx context.Context, cid string, opts workflowartifacts.PinOptions) error
}

// ReconcilerOptions configure the artifact pin reconciler.
type ReconcilerOptions struct {
	Store      *Store
	Cluster    statusClient
	Metrics    metrics.ArtifactPinRecorder
	Interval   time.Duration
	RetryDelay time.Duration
	BatchSize  int
	Clock      func() time.Time
	Logger     *log.Logger
}

// Reconciler keeps artifact pin metadata in sync with the IPFS Cluster status.
type Reconciler struct {
	store      *Store
	cluster    statusClient
	metrics    metrics.ArtifactPinRecorder
	interval   time.Duration
	retryDelay time.Duration
	batchSize  int
	now        func() time.Time
	logger     *log.Logger

	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

const (
	defaultReconcileInterval = 30 * time.Second
	defaultRetryDelay        = time.Minute
	defaultBatchSize         = 200
)

// NewReconciler constructs a reconciler instance with sane defaults.
func NewReconciler(opts ReconcilerOptions) *Reconciler {
	interval := opts.Interval
	if interval <= 0 {
		interval = defaultReconcileInterval
	}
	retryDelay := opts.RetryDelay
	if retryDelay <= 0 {
		retryDelay = defaultRetryDelay
	}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	now := opts.Clock
	if now == nil {
		now = time.Now
	}
	return &Reconciler{
		store:      opts.Store,
		cluster:    opts.Cluster,
		metrics:    opts.Metrics,
		interval:   interval,
		retryDelay: retryDelay,
		batchSize:  batchSize,
		now:        now,
		logger:     opts.Logger,
	}
}

// Start begins the background reconcile loop.
func (r *Reconciler) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return errors.New("artifacts: reconciler already running")
	}
	if r.store == nil || r.cluster == nil {
		return errors.New("artifacts: reconciler requires store and cluster client")
	}
	loopCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.loop(loopCtx)
	}()
	return nil
}

// Stop terminates the background loop.
func (r *Reconciler) Stop(ctx context.Context) error {
	r.mu.Lock()
	cancel := r.cancel
	r.cancel = nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	c := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(c)
	}()
	select {
	case <-c:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Reconciler) loop(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		if err := r.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			r.logf("artifact reconciler run failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunOnce processes a single reconciliation pass; exported for tests.
func (r *Reconciler) RunOnce(ctx context.Context) error {
	if r.store == nil || r.cluster == nil {
		return errors.New("artifacts: reconciler not configured")
	}
	counts := make(map[string]int)
	cursor := ""
	now := r.now().UTC()
	for {
		result, err := r.store.List(ctx, ListOptions{Cursor: cursor, Limit: r.batchSize})
		if err != nil {
			return err
		}
		for _, meta := range result.Artifacts {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			state := r.processArtifact(ctx, meta, now)
			counts[string(state)]++
		}
		if strings.TrimSpace(result.NextCursor) == "" {
			break
		}
		cursor = result.NextCursor
	}
	if r.metrics != nil {
		r.metrics.UpdateState(counts)
	}
	return nil
}

func (r *Reconciler) processArtifact(ctx context.Context, meta Metadata, now time.Time) PinState {
	if meta.Deleted {
		return meta.PinState
	}
	if meta.PinState == PinStatePinned && !needsReplication(meta) {
		return PinStatePinned
	}
	if !readyForAttempt(meta, now) {
		return meta.PinState
	}
	status, err := r.cluster.Status(ctx, meta.CID)
	if err != nil {
		r.markFailure(meta, err, now)
		return PinStateFailed
	}
	replicas := countPinnedPeers(status.Peers)
	targetState := deriveState(status, meta, replicas)
	switch targetState {
	case PinStatePinned:
		r.updateState(meta.ID, PinStateUpdate{State: PinStatePinned, Replicas: &replicas})
		return PinStatePinned
	case PinStateFailed:
		if !readyForAttempt(meta, now) {
			return PinStateFailed
		}
		if err := r.cluster.Pin(ctx, meta.CID, workflowartifacts.PinOptions{
			ReplicationFactorMin: meta.ReplicationFactorMin,
			ReplicationFactorMax: meta.ReplicationFactorMax,
		}); err != nil {
			r.markFailure(meta, err, now)
			return PinStateFailed
		}
		if r.metrics != nil {
			r.metrics.ObserveRetry(meta.Kind)
		}
		next := now.Add(r.retryDelay)
		delta := 1
		r.updateState(meta.ID, PinStateUpdate{State: PinStatePinning, Replicas: &replicas, RetryCountDelta: delta, NextAttemptAt: next})
		return PinStatePinning
	default:
		next := now.Add(r.retryDelay)
		r.updateState(meta.ID, PinStateUpdate{State: targetState, Replicas: &replicas, NextAttemptAt: next})
		return targetState
	}
}

func (r *Reconciler) markFailure(meta Metadata, err error, now time.Time) {
	msg := err.Error()
	next := now.Add(r.retryDelay)
	r.updateState(meta.ID, PinStateUpdate{State: PinStateFailed, Error: msg, NextAttemptAt: next})
}

func (r *Reconciler) updateState(id string, update PinStateUpdate) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := r.store.UpdatePinState(ctx, id, update)
	if err != nil {
		r.logf("artifact reconciler: update pin state %s: %v", id, err)
	}
}

func readyForAttempt(meta Metadata, now time.Time) bool {
	if meta.PinNextAttemptAt.IsZero() {
		return true
	}
	return !meta.PinNextAttemptAt.After(now)
}

func needsReplication(meta Metadata) bool {
	if meta.ReplicationFactorMin <= 0 {
		return false
	}
	return meta.PinReplicas < meta.ReplicationFactorMin
}

func deriveState(status workflowartifacts.StatusResult, meta Metadata, replicas int) PinState {
	summary := strings.ToLower(strings.TrimSpace(status.Summary))
	switch summary {
	case "pinned":
		return PinStatePinned
	case "pin_error", "error":
		return PinStateFailed
	case "pinning", "pin_queued", "queued":
		return PinStatePinning
	}
	if meta.ReplicationFactorMin > 0 && replicas >= meta.ReplicationFactorMin {
		return PinStatePinned
	}
	if strings.Contains(summary, "error") {
		return PinStateFailed
	}
	return PinStatePinning
}

func countPinnedPeers(peers []workflowartifacts.StatusPeer) int {
	count := 0
	for _, peer := range peers {
		if strings.EqualFold(strings.TrimSpace(peer.Status), "pinned") {
			count++
		}
	}
	return count
}

func (r *Reconciler) logf(format string, args ...any) {
	logger := r.logger
	if logger != nil {
		logger.Printf(format, args...)
		return
	}
	log.Printf(format, args...)
}
