package hydration

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
	"github.com/iw2rmb/ploy/internal/metrics"
	"github.com/iw2rmb/ploy/internal/workflow/artifacts"
)

// PinOptions aliases artifacts.PinOptions for cluster interactions.
type PinOptions = artifacts.PinOptions

// Cluster defines the minimal IPFS Cluster client capabilities required by the controller.
type Cluster interface {
	Pin(ctx context.Context, cid string, opts PinOptions) error
	Unpin(ctx context.Context, cid string) error
}

// ControllerOptions configures hydration controller behaviour.
type ControllerOptions struct {
	Index         *Index
	Cluster       Cluster
	DefaultPolicy ReplicationPolicy
	Clock         func() time.Time
	PolicyStore   *PolicyStore
	PolicyEngine  *PolicyEngine
	Metrics       *metrics.HydrationMetrics
}

// Controller coordinates snapshot replication and index updates.
type Controller struct {
	index   *Index
	cluster Cluster
	policy  ReplicationPolicy
	clock   func() time.Time
	engine  *PolicyEngine
	metrics *metrics.HydrationMetrics
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// CompletionEvent captures hydration metadata emitted when a job finishes.
type CompletionEvent struct {
	TicketID    string
	StageID     string
	RepoURL     string
	Revision    string
	Bundle      scheduler.BundleRecord
	Replication ReplicationPolicy
	Sharing     SharingPolicy
}

// NewController constructs a controller from the provided options.
func NewController(client *clientv3.Client, opts ControllerOptions) (*Controller, error) {
	if opts.Index == nil {
		if client == nil {
			return nil, errors.New("hydration: index or client required")
		}
		index, err := NewIndex(client, IndexOptions{})
		if err != nil {
			return nil, err
		}
		opts.Index = index
	}
	if opts.PolicyStore == nil && client != nil {
		store, err := NewPolicyStore(client, PolicyStoreOptions{})
		if err == nil {
			opts.PolicyStore = store
		}
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	controller := &Controller{
		index:   opts.Index,
		cluster: opts.Cluster,
		policy:  opts.DefaultPolicy,
		clock:   clock,
		metrics: opts.Metrics,
	}

	if controller.metrics == nil {
		hydrationMetrics, err := metrics.NewHydrationMetrics(nil)
		if err != nil {
			return nil, err
		}
		controller.metrics = hydrationMetrics
	}

	if opts.PolicyEngine != nil {
		controller.engine = opts.PolicyEngine
	} else if opts.PolicyStore != nil {
		engine, err := NewPolicyEngine(PolicyEngineOptions{
			Store:   opts.PolicyStore,
			Index:   opts.Index,
			Clock:   clock,
			Metrics: controller.metrics,
		})
		if err != nil {
			return nil, err
		}
		controller.engine = engine
	}

	if controller.engine != nil {
		ctx, cancel := context.WithCancel(context.Background())
		if err := controller.engine.Start(ctx); err != nil {
			cancel()
			return nil, err
		}
		controller.cancel = cancel
		controller.wg.Add(1)
		go controller.consumeDecisions(ctx)
	}

	return controller, nil
}

// HandleCompletion records snapshot metadata and ensures replication policies are enforced.
func (c *Controller) HandleCompletion(ctx context.Context, event CompletionEvent) error {
	if c == nil || c.index == nil {
		return errors.New("hydration: controller not configured")
	}
	ticket := strings.TrimSpace(event.TicketID)
	repo := strings.TrimSpace(event.RepoURL)
	revision := strings.TrimSpace(event.Revision)
	cid := strings.TrimSpace(event.Bundle.CID)
	if ticket == "" {
		return errors.New("hydration: completion missing ticket id")
	}
	if repo == "" || revision == "" {
		return errors.New("hydration: completion missing repository metadata")
	}
	if cid == "" {
		return errors.New("hydration: completion missing snapshot cid")
	}

	replication := event.Replication
	if replication.empty() {
		replication = c.policy
	}

	_, err := c.index.UpsertSnapshot(ctx, SnapshotRecord{
		RepoURL:     repo,
		Revision:    revision,
		TicketID:    ticket,
		Bundle:      event.Bundle,
		Replication: replication,
		Sharing:     event.Sharing,
	})
	if err != nil {
		return err
	}

	if c.cluster != nil {
		opts := PinOptions{
			ReplicationFactorMin: replication.Min,
			ReplicationFactorMax: replication.Max,
		}
		if err := c.cluster.Pin(ctx, cid, opts); err != nil {
			return fmt.Errorf("hydration: cluster pin: %w", err)
		}
	}
	return nil
}

// Close stops background policy evaluation loops.
func (c *Controller) Close() {
	if c == nil {
		return
	}
	if c.cancel != nil {
		c.cancel()
		c.wg.Wait()
		c.cancel = nil
	}
}

func (c *Controller) consumeDecisions(ctx context.Context) {
	defer c.wg.Done()
	decisions := c.engine.Decisions()
	for {
		select {
		case <-ctx.Done():
			return
		case decision, ok := <-decisions:
			if !ok {
				return
			}
			if err := c.applyDecision(ctx, decision); err != nil {
				log.Printf("hydration controller: apply decision: %v", err)
			}
		}
	}
}

func (c *Controller) applyDecision(ctx context.Context, decision PolicyDecision) error {
	switch decision.Action {
	case PolicyActionReduceReplication:
		return c.applyReplicationDecision(ctx, decision)
	case PolicyActionEvictSnapshot:
		return c.applyEvictionDecision(ctx, decision)
	default:
		return nil
	}
}

func (c *Controller) applyReplicationDecision(ctx context.Context, decision PolicyDecision) error {
	if c.index == nil {
		return errors.New("hydration: index not configured")
	}
	repl := decision.Snapshot.Replication
	if decision.TargetReplication > 0 {
		repl.Max = decision.TargetReplication
		if repl.Min > repl.Max {
			repl.Min = repl.Max
		}
		if repl.Min == 0 {
			repl.Min = repl.Max
		}
	}
	entry, err := c.index.UpdateReplication(ctx, decision.Fingerprint, repl)
	if err != nil {
		return err
	}
	if c.cluster != nil && strings.TrimSpace(entry.Bundle.CID) != "" {
		opts := PinOptions{ReplicationFactorMin: repl.Min, ReplicationFactorMax: repl.Max}
		if err := c.cluster.Pin(ctx, entry.Bundle.CID, opts); err != nil {
			return fmt.Errorf("hydration: cluster pin (decision): %w", err)
		}
	}
	return nil
}

func (c *Controller) applyEvictionDecision(ctx context.Context, decision PolicyDecision) error {
	if c.index == nil {
		return errors.New("hydration: index not configured")
	}
	if c.cluster != nil && strings.TrimSpace(decision.Snapshot.Bundle.CID) != "" {
		if err := c.cluster.Unpin(ctx, decision.Snapshot.Bundle.CID); err != nil {
			return fmt.Errorf("hydration: cluster unpin: %w", err)
		}
	}
	return c.index.DeleteSnapshot(ctx, decision.Fingerprint)
}
