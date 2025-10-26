package scheduler

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/google/uuid"

	metricsx "github.com/iw2rmb/ploy/internal/metrics"
)

const (
	defaultRetentionWindow     = 24 * time.Hour
	jobRetryReasonLeaseExpired = "lease_expired"
)

// Scheduler coordinates job submission, claims, and lifecycle management backed by etcd.
type Scheduler struct {
	client *clientv3.Client

	jobsPrefix   string
	queuePrefix  string
	leasesPrefix string
	nodesPrefix  string
	gcPrefix     string

	leaseTTL    time.Duration
	clockSkew   time.Duration
	now         func() time.Time
	idGenerator func() string

	ctx     context.Context
	cancel  context.CancelFunc
	metrics metricsx.SchedulerRecorder
	wg      sync.WaitGroup
}

// New constructs a scheduler with the provided etcd client and options.
func New(client *clientv3.Client, opts Options) (*Scheduler, error) {
	if client == nil {
		return nil, errors.New("scheduler: etcd client is required")
	}

	cfg := compileOptions(opts)

	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		client:       client,
		jobsPrefix:   cfg.jobsPrefix,
		queuePrefix:  cfg.queuePrefix,
		leasesPrefix: cfg.leasesPrefix,
		nodesPrefix:  cfg.nodesPrefix,
		gcPrefix:     cfg.gcPrefix,
		leaseTTL:     cfg.leaseTTL,
		clockSkew:    cfg.clockSkew,
		now:          cfg.now,
		idGenerator:  cfg.idGenerator,
		ctx:          ctx,
		cancel:       cancel,
		metrics:      cfg.metrics,
	}

	s.wg.Add(1)
	go s.watchLeases()

	s.wg.Add(1)
	go s.watchGCMarkers()

	s.wg.Add(1)
	go s.watchNodeStatus()

	return s, nil
}

// Close stops background watchers.
func (s *Scheduler) Close() error {
	s.cancel()
	s.wg.Wait()
	return nil
}

type compiledOptions struct {
	jobsPrefix   string
	queuePrefix  string
	leasesPrefix string
	nodesPrefix  string
	gcPrefix     string
	leaseTTL     time.Duration
	clockSkew    time.Duration
	now          func() time.Time
	idGenerator  func() string
	metrics      metricsx.SchedulerRecorder
}

// compileOptions normalises scheduler options with sane defaults.
func compileOptions(opts Options) compiledOptions {
	cfg := compiledOptions{
		jobsPrefix:   normalizePrefix(opts.JobsPrefix, "mods"),
		queuePrefix:  normalizePrefix(opts.QueuePrefix, "queue/mods"),
		leasesPrefix: normalizePrefix(opts.LeasesPrefix, "leases/jobs"),
		nodesPrefix:  normalizePrefix(opts.NodesPrefix, "nodes"),
		gcPrefix:     normalizePrefix(opts.GCPrefix, "gc/jobs"),
		leaseTTL:     opts.LeaseTTL,
		clockSkew:    opts.ClockSkewBuffer,
		now:          opts.Now,
		idGenerator:  opts.IDGenerator,
		metrics:      opts.Metrics,
	}

	if cfg.leaseTTL <= 0 {
		cfg.leaseTTL = 120 * time.Second
	}
	if cfg.clockSkew < 0 {
		cfg.clockSkew = 0
	}
	if cfg.now == nil {
		cfg.now = func() time.Time { return time.Now().UTC() }
	}
	if cfg.idGenerator == nil {
		cfg.idGenerator = func() string { return uuid.NewString() }
	}
	if cfg.metrics == nil {
		cfg.metrics = metricsx.NewNoopSchedulerRecorder()
	}

	return cfg
}

// normalizePrefix sanitises scheduler key prefixes.
func normalizePrefix(prefix, fallback string) string {
	p := strings.Trim(prefix, "/")
	if p == "" {
		p = fallback
	}
	return p
}

// jobKey builds the etcd key for a job record.
func (s *Scheduler) jobKey(ticket, jobID string) string {
	return path.Join(s.jobsPrefix, ticket, "jobs", jobID)
}

// queueKey builds the etcd key for a queued job entry.
func (s *Scheduler) queueKey(priority, jobID string) string {
	return path.Join(s.queuePrefix, priority, jobID)
}

// leaseKey builds the etcd key holding the job lease metadata.
func (s *Scheduler) leaseKey(jobID string) string {
	return path.Join(s.leasesPrefix, jobID)
}

// gcKey builds the etcd key storing job GC markers.
func (s *Scheduler) gcKey(jobID string) string {
	return path.Join(s.gcPrefix, jobID)
}

// lookupJobKey scans job prefixes to locate a ticket/job key pair.
func (s *Scheduler) lookupJobKey(ctx context.Context, jobID string) (string, string, error) {
	resp, err := s.client.Get(ctx, s.jobsPrefix, clientv3.WithPrefix(), clientv3.WithKeysOnly())
	if err != nil {
		return "", "", fmt.Errorf("scheduler: lookup job key: %w", err)
	}
	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		if strings.HasSuffix(key, "/"+jobID) {
			segments := strings.Split(key, "/")
			if len(segments) < 3 {
				continue
			}
			ticket := segments[len(segments)-3]
			return key, ticket, nil
		}
	}
	return "", "", fmt.Errorf("scheduler: job %s not found", jobID)
}
