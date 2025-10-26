package httpserver_test

import (
	"net/http/httptest"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"

	"github.com/iw2rmb/ploy/internal/api/httpserver"
	controlplanemods "github.com/iw2rmb/ploy/internal/controlplane/mods"
	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
	"github.com/iw2rmb/ploy/internal/node/logstream"
)

type modsServerFixture struct {
	server    *httptest.Server
	etcd      *embed.Etcd
	client    *clientv3.Client
	scheduler *scheduler.Scheduler
	mods      *controlplanemods.Service
	streams   *logstream.Hub
}

func newModsServerFixture(t *testing.T) *modsServerFixture {
	t.Helper()
	etcd, client := startTestEtcd(t)

	sched, err := scheduler.New(client, scheduler.Options{LeaseTTL: 3 * time.Second})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}

	service, err := controlplanemods.NewService(client, controlplanemods.Options{
		Scheduler: controlplanemods.NewSchedulerBridge(sched),
		Clock:     func() time.Time { return time.Date(2025, 10, 24, 10, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("new mods service: %v", err)
	}

	streams := logstream.NewHub(logstream.Options{BufferSize: 8, HistorySize: 32})

	handler := newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Etcd:      client,
		Mods:      service,
		Streams:   streams,
	})
	server := httptest.NewServer(handler)

	fixture := &modsServerFixture{
		server:    server,
		etcd:      etcd,
		client:    client,
		scheduler: sched,
		mods:      service,
		streams:   streams,
	}

	t.Cleanup(func() {
		server.Close()
		_ = service.Close()
		_ = sched.Close()
		_ = client.Close()
		etcd.Close()
	})

	return fixture
}
