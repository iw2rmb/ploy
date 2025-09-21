package routing

import (
	"context"
	"testing"
	"time"

	natstest "github.com/nats-io/nats-server/v2/test"
	nats "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	irouting "github.com/iw2rmb/ploy/internal/routing"
)

func startTestJetStream(t *testing.T) (string, func()) {
	t.Helper()
	opts := natstest.DefaultTestOptions
	opts.Port = -1
	opts.JetStream = true
	srv := natstest.RunServer(&opts)
	cleanup := func() {
		srv.Shutdown()
	}
	return srv.ClientURL(), cleanup
}

func TestGetAppRoutesPrefersJetStreamStore(t *testing.T) {
	url, shutdown := startTestJetStream(t)
	defer shutdown()

	conn, err := nats.Connect(url)
	require.NoError(t, err)
	t.Cleanup(conn.Close)

	store, err := irouting.NewStore(context.Background(), irouting.StoreConfig{
		Conn:          conn,
		Bucket:        "routing_maps",
		Stream:        "routing_events",
		SubjectPrefix: "routing.app",
	})
	require.NoError(t, err)

	router, err := NewTraefikRouter("127.0.0.1:8500", RouterOptions{
		Store:        store,
		StoreTimeout: time.Second,
	})
	require.NoError(t, err)

	route := irouting.DomainRoute{
		App:        "demo",
		Domain:     "demo.example",
		Port:       8080,
		AllocID:    "alloc-1",
		AllocIP:    "10.0.0.5",
		HealthPath: "/healthz",
		Aliases:    []string{"www.demo.example"},
		TLSEnabled: true,
		CreatedAt:  time.Unix(171, 0).UTC(),
	}
	require.NoError(t, store.SaveAppRoute(context.Background(), route))

	routes, err := router.GetAppRoutes("demo")
	require.NoError(t, err)
	require.Len(t, routes, 1)
	got := routes[0]
	require.Equal(t, route.Domain, got.Domain)
	require.Equal(t, route.AllocID, got.AllocID)
	require.Equal(t, route.AllocIP, got.AllocIP)
	require.Equal(t, route.Port, got.Port)
	require.ElementsMatch(t, append([]string(nil), route.Aliases...), got.Aliases)
}
