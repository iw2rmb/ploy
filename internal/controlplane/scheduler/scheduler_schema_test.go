package scheduler_test

import (
	"context"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
)

func TestSchedulerJobRecordSchema(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		prepare func(ctx context.Context, t *testing.T, client *clientv3.Client) map[string]any
		assert  func(t *testing.T, doc map[string]any)
	}{
		{
			name: "queued job baseline metadata",
			prepare: func(ctx context.Context, t *testing.T, client *clientv3.Client) map[string]any {
				opts := defaultOptions()
				now := time.Date(2025, 10, 24, 11, 0, 0, 0, time.UTC)
				opts.Now = func() time.Time { return now }

				sched := mustNewScheduler(t, client, opts)
				job, err := sched.SubmitJob(ctx, scheduler.JobSpec{
					Ticket:      "mod-schema",
					StepID:      "plan",
					Priority:    "default",
					MaxAttempts: 1,
				})
				if err != nil {
					t.Fatalf("submit job: %v", err)
				}

				key := opts.JobsPrefix + job.Ticket + "/jobs/" + job.ID
				doc := mustLoadJobDocument(t, ctx, client, key)
				_ = sched.Close()
				return doc
			},
			assert: func(t *testing.T, doc map[string]any) {
				if value, ok := doc["expires_at"]; ok {
					str, ok := value.(string)
					if !ok {
						t.Fatalf("expected expires_at string, got %T", value)
					}
					if str != "" {
						t.Fatalf("expected expires_at empty, got %s", str)
					}
				}
				if _, ok := doc["node_snapshot"]; ok {
					t.Fatalf("did not expect node_snapshot for queued job")
				}
				if _, ok := doc["lease_id"]; ok {
					t.Fatalf("did not expect lease_id for queued job")
				}
			},
		},
		{
			name: "running job captures node snapshot and lease metadata",
			prepare: func(ctx context.Context, t *testing.T, client *clientv3.Client) map[string]any {
				opts := defaultOptions()
				now := time.Date(2025, 10, 24, 11, 30, 0, 0, time.UTC)
				opts.Now = func() time.Time { return now }

				sched := mustNewScheduler(t, client, opts)
				nodeID := "node-schema"
				capacityJSON := `{"cpu_free": 6000, "mem_free": 8192, "heartbeat": "2025-10-24T11:29:30Z", "revision": 10}`
				if _, err := client.Put(ctx, opts.NodesPrefix+nodeID+"/capacity", capacityJSON); err != nil {
					t.Fatalf("put capacity: %v", err)
				}

				job, err := sched.SubmitJob(ctx, scheduler.JobSpec{
					Ticket:      "mod-schema",
					StepID:      "apply",
					Priority:    "default",
					MaxAttempts: 2,
					Metadata:    map[string]string{"lane": "schema"},
				})
				if err != nil {
					t.Fatalf("submit job: %v", err)
				}

				if _, err := sched.ClaimNext(ctx, scheduler.ClaimRequest{NodeID: nodeID}); err != nil {
					t.Fatalf("claim job: %v", err)
				}

				key := opts.JobsPrefix + job.Ticket + "/jobs/" + job.ID
				doc := mustLoadJobDocument(t, ctx, client, key)
				_ = sched.Close()
				return doc
			},
			assert: func(t *testing.T, doc map[string]any) {
				value, ok := doc["node_snapshot"]
				if !ok {
					t.Fatalf("expected node_snapshot present")
				}
				snapshot, ok := value.(map[string]any)
				if !ok {
					t.Fatalf("node_snapshot type %T", value)
				}
				nodeID, ok := snapshot["node_id"].(string)
				if !ok || nodeID != "node-schema" {
					t.Fatalf("unexpected node_id: %v", snapshot["node_id"])
				}
				capacityVal, ok := snapshot["capacity"]
				if !ok {
					t.Fatalf("expected capacity in node_snapshot")
				}
				capacity, ok := capacityVal.(map[string]any)
				if !ok {
					t.Fatalf("capacity type %T", capacityVal)
				}
				if cpu, ok := capacity["cpu_free"].(float64); !ok || cpu != 6000 {
					t.Fatalf("unexpected cpu_free: %v", capacity["cpu_free"])
				}
				if mem, ok := capacity["mem_free"].(float64); !ok || mem != 8192 {
					t.Fatalf("unexpected mem_free: %v", capacity["mem_free"])
				}
				if ts, ok := snapshot["capacity_at"].(string); !ok || ts == "" {
					t.Fatalf("expected capacity_at timestamp")
				}
				leaseID, ok := doc["lease_id"].(float64)
				if !ok || leaseID <= 0 {
					t.Fatalf("expected lease_id > 0, got %v", doc["lease_id"])
				}
				leaseExpiry, ok := doc["lease_expires_at"].(string)
				if !ok || leaseExpiry == "" {
					t.Fatalf("expected lease_expires_at string, got %v", doc["lease_expires_at"])
				}
			},
		},
		{
			name: "completed job records expiry and retention",
			prepare: func(ctx context.Context, t *testing.T, client *clientv3.Client) map[string]any {
				opts := defaultOptions()
				now := time.Date(2025, 10, 24, 12, 0, 0, 0, time.UTC)
				opts.Now = func() time.Time { return now }

				sched := mustNewScheduler(t, client, opts)
				nodeID := "node-schema"
				capacityJSON := `{"cpu_free": 8000, "mem_free": 16384, "heartbeat": "2025-10-24T11:59:30Z", "revision": 15}`
				if _, err := client.Put(ctx, opts.NodesPrefix+nodeID+"/capacity", capacityJSON); err != nil {
					t.Fatalf("put capacity: %v", err)
				}

				job, err := sched.SubmitJob(ctx, scheduler.JobSpec{
					Ticket:      "mod-schema",
					StepID:      "finish",
					Priority:    "default",
					MaxAttempts: 1,
				})
				if err != nil {
					t.Fatalf("submit job: %v", err)
				}

				claim, err := sched.ClaimNext(ctx, scheduler.ClaimRequest{NodeID: nodeID})
				if err != nil {
					t.Fatalf("claim job: %v", err)
				}

				_, err = sched.CompleteJob(ctx, scheduler.CompleteRequest{
					JobID:  claim.Job.ID,
					Ticket: job.Ticket,
					NodeID: nodeID,
					State:  scheduler.JobStateSucceeded,
					Bundles: map[string]scheduler.BundleRecord{
						"logs": {
							CID:      "bafy-doc",
							Digest:   "sha256:doc",
							Size:     512,
							Retained: true,
							TTL:      "72h",
						},
					},
				})
				if err != nil {
					t.Fatalf("complete job: %v", err)
				}

				key := opts.JobsPrefix + job.Ticket + "/jobs/" + job.ID
				doc := mustLoadJobDocument(t, ctx, client, key)
				_ = sched.Close()
				return doc
			},
			assert: func(t *testing.T, doc map[string]any) {
				expiresAt, ok := doc["expires_at"].(string)
				if !ok || expiresAt == "" {
					t.Fatalf("expected expires_at string, got %v", doc["expires_at"])
				}
				retentionVal, ok := doc["retention"]
				if !ok {
					t.Fatalf("expected retention present")
				}
				retention, ok := retentionVal.(map[string]any)
				if !ok {
					t.Fatalf("retention type %T", retentionVal)
				}
				retentionExpiry, ok := retention["expires_at"].(string)
				if !ok || retentionExpiry != expiresAt {
					t.Fatalf("unexpected retention expires_at: %v (want %s)", retention["expires_at"], expiresAt)
				}
				bundlesVal, ok := doc["bundles"]
				if !ok {
					t.Fatalf("expected bundles present")
				}
				bundles, ok := bundlesVal.(map[string]any)
				if !ok {
					t.Fatalf("bundles type %T", bundlesVal)
				}
				logBundleVal, ok := bundles["logs"]
				if !ok {
					t.Fatalf("expected logs bundle entry")
				}
				logBundle, ok := logBundleVal.(map[string]any)
				if !ok {
					t.Fatalf("logs bundle type %T", logBundleVal)
				}
				if bundleExpiry, ok := logBundle["expires_at"].(string); !ok || bundleExpiry != expiresAt {
					t.Fatalf("unexpected bundle expires_at: %v (want %s)", logBundle["expires_at"], expiresAt)
				}
			},
		},
	}

	for _, tc := range cases {
		ct := tc
		t.Run(ct.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			etcd, client := newTestEtcd(t)
			defer etcd.Close()
			defer func() { _ = client.Close() }()

			doc := ct.prepare(ctx, t, client)
			ct.assert(t, doc)
		})
	}
}

func TestSchedulerNodeStatusWatcherUpdatesJob(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	etcd, client := newTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	opts := defaultOptions()
	now := time.Date(2025, 10, 24, 12, 0, 0, 0, time.UTC)
	opts.Now = func() time.Time { return now }

	sched := mustNewScheduler(t, client, opts)
	defer func() { _ = sched.Close() }()

	nodeID := "node-watch"
	capacityJSON := `{"cpu_free": 4000, "mem_free": 16384, "heartbeat": "2025-10-24T11:59:30Z", "revision": 5}`
	if _, err := client.Put(ctx, opts.NodesPrefix+nodeID+"/capacity", capacityJSON); err != nil {
		t.Fatalf("put capacity: %v", err)
	}

	job, err := sched.SubmitJob(ctx, scheduler.JobSpec{
		Ticket:      "mod-watch",
		StepID:      "apply",
		Priority:    "default",
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}

	if _, err := sched.ClaimNext(ctx, scheduler.ClaimRequest{NodeID: nodeID}); err != nil {
		t.Fatalf("claim job: %v", err)
	}

	statusJSON := `{"phase":"degraded","heartbeat":"2025-10-24T12:02:00Z","message":"disk pressure"}`
	if _, err := client.Put(ctx, opts.NodesPrefix+nodeID+"/status", statusJSON); err != nil {
		t.Fatalf("put status: %v", err)
	}

	jobKey := opts.JobsPrefix + job.Ticket + "/jobs/" + job.ID
	waitForCondition(t, 5*time.Second, func() bool {
		doc, err := loadJobDocument(ctx, client, jobKey)
		if err != nil {
			return false
		}
		value, ok := doc["node_snapshot"]
		if !ok {
			return false
		}
		snapshot, ok := value.(map[string]any)
		if !ok {
			return false
		}
		statusVal, ok := snapshot["status"]
		if !ok {
			return false
		}
		status, ok := statusVal.(map[string]any)
		if !ok {
			return false
		}
		phase, ok := status["phase"].(string)
		if !ok || phase != "degraded" {
			return false
		}
		message, _ := status["message"].(string)
		return message == "disk pressure"
	})
}
