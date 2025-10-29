package httpserver_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"

	"github.com/iw2rmb/ploy/internal/api/httpserver"
	"github.com/iw2rmb/ploy/internal/controlplane/hydration"
	controlplanemods "github.com/iw2rmb/ploy/internal/controlplane/mods"
	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
	"github.com/iw2rmb/ploy/internal/node/logstream"
)

type modsServerFixture struct {
	server         *httptest.Server
	etcd           *embed.Etcd
	client         *clientv3.Client
	scheduler      *scheduler.Scheduler
	mods           *controlplanemods.Service
	streams        *logstream.Hub
	ctx            context.Context
	cancel         context.CancelFunc
	hydrationIndex *hydration.Index
	policyStore    *hydration.PolicyStore
}

func newModsServerFixture(t *testing.T) *modsServerFixture {
	t.Helper()
	etcd, client := startTestEtcd(t)
	ctx, cancel := context.WithCancel(context.Background())

	sched, err := scheduler.New(client, scheduler.Options{LeaseTTL: 3 * time.Second})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}

	hydIndex, err := hydration.NewIndex(client, hydration.IndexOptions{})
	if err != nil {
		t.Fatalf("new hydration index: %v", err)
	}

	policyStore, err := hydration.NewPolicyStore(client, hydration.PolicyStoreOptions{})
	if err != nil {
		t.Fatalf("new hydration policy store: %v", err)
	}
	if _, err := policyStore.SavePolicy(ctx, hydration.GlobalPolicy{
		ID: "default",
		Scope: hydration.PolicyScope{
			RepoPrefixes: []string{"https://git.example.com/org/"},
		},
		Window: hydration.QuotaWindow{
			PinnedBytes: hydration.LimitBytes{Hard: 50 << 20},
			Snapshots:   hydration.LimitCount{Hard: 10},
			Replicas:    hydration.LimitCount{Hard: 3},
		},
	}); err != nil {
		t.Fatalf("seed global policy: %v", err)
	}

	service, err := controlplanemods.NewService(client, controlplanemods.Options{
		Scheduler: controlplanemods.NewSchedulerBridge(sched),
		Clock:     func() time.Time { return time.Date(2025, 10, 24, 10, 0, 0, 0, time.UTC) },
		Hydration: controlplanemods.HydrationOptions{
			Index: hydIndex,
		},
	})
	if err != nil {
		t.Fatalf("new mods service: %v", err)
	}

	streams := logstream.NewHub(logstream.Options{BufferSize: 8, HistorySize: 32})

	handler := newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Scheduler:         sched,
		Etcd:              client,
		Mods:              service,
		Streams:           streams,
		HydrationIndex:    hydIndex,
		HydrationPolicies: policyStore,
	})
	server := httptest.NewServer(handler)

	fixture := &modsServerFixture{
		server:         server,
		etcd:           etcd,
		client:         client,
		scheduler:      sched,
		mods:           service,
		streams:        streams,
		ctx:            ctx,
		cancel:         cancel,
		hydrationIndex: hydIndex,
		policyStore:    policyStore,
	}

	t.Cleanup(func() {
		server.Close()
		_ = service.Close()
		_ = sched.Close()
		_ = client.Close()
		etcd.Close()
		cancel()
	})

	return fixture
}
