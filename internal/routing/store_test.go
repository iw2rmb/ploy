package routing_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	natstest "github.com/nats-io/nats-server/v2/test"
	nats "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	"github.com/iw2rmb/ploy/internal/routing"
)

const (
	defaultBucket        = "routing_maps"
	defaultStream        = "routing_events"
	defaultSubjectPrefix = "routing.app"
)

func startJetStreamServer(t *testing.T) (string, func()) {
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

func newTestStore(t *testing.T, conn *nats.Conn) *routing.Store {
	t.Helper()
	store, err := routing.NewStore(context.Background(), routing.StoreConfig{
		Conn:          conn,
		Bucket:        defaultBucket,
		Stream:        defaultStream,
		SubjectPrefix: defaultSubjectPrefix,
	})
	require.NoError(t, err)
	return store
}

func TestStoreSaveAppRoutePersistsAndPublishes(t *testing.T) {
	url, shutdown := startJetStreamServer(t)
	defer shutdown()

	conn, err := nats.Connect(url)
	require.NoError(t, err)
	defer conn.Close()

	ctx := context.Background()
	store := newTestStore(t, conn)

	js, err := conn.JetStream()
	require.NoError(t, err)

	sub, err := js.PullSubscribe(fmt.Sprintf("%s.%s", defaultSubjectPrefix, "demo"), "test-router", nats.BindStream(defaultStream))
	require.NoError(t, err)

	route := routing.DomainRoute{
		App:        "demo",
		Domain:     "demo.example",
		Port:       8080,
		AllocID:    "alloc-1",
		AllocIP:    "10.0.0.1",
		HealthPath: "/healthz",
		CreatedAt:  time.Now().UTC().Round(time.Second),
	}

	err = store.SaveAppRoute(ctx, route)
	require.NoError(t, err)

	objStore, err := js.ObjectStore(defaultBucket)
	require.NoError(t, err)

	data, err := objStore.GetBytes("apps/demo/routes.json")
	require.NoError(t, err)

	var stored map[string]routing.DomainRoute
	require.NoError(t, json.Unmarshal(data, &stored))
	require.Contains(t, stored, "demo.example")
	require.Equal(t, route.AllocID, stored["demo.example"].AllocID)

	msgs, err := sub.Fetch(1, nats.MaxWait(2*time.Second))
	require.NoError(t, err)
	require.Len(t, msgs, 1)

	msg := msgs[0]
	require.Equal(t, fmt.Sprintf("%s.%s", defaultSubjectPrefix, "demo"), msg.Subject)
	revision := msg.Header.Get("X-Ploy-Revision")
	require.NotEmpty(t, revision)
	checksum := msg.Header.Get("X-Ploy-Checksum")
	require.NotEmpty(t, checksum)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(msg.Data, &payload))
	require.Equal(t, "demo", payload["app"])
	require.Equal(t, "upsert", payload["change"])
	require.NotEmpty(t, payload["updated_at"])

	require.NoError(t, msg.Ack())
}

func TestStoreDeleteAppRouteRemovesRouteAndPublishes(t *testing.T) {
	url, shutdown := startJetStreamServer(t)
	defer shutdown()

	conn, err := nats.Connect(url)
	require.NoError(t, err)
	defer conn.Close()

	ctx := context.Background()
	store := newTestStore(t, conn)

	js, err := conn.JetStream()
	require.NoError(t, err)

	sub, err := js.PullSubscribe(fmt.Sprintf("%s.%s", defaultSubjectPrefix, "demo"), "delete-router", nats.BindStream(defaultStream))
	require.NoError(t, err)

	initialRoute := routing.DomainRoute{
		App:    "demo",
		Domain: "demo.example",
		Port:   8080,
	}
	require.NoError(t, store.SaveAppRoute(ctx, initialRoute))

	_, err = sub.Fetch(1, nats.MaxWait(2*time.Second))
	require.NoError(t, err)

	require.NoError(t, store.DeleteAppRoute(ctx, "demo", "demo.example"))

	msgs, err := sub.Fetch(1, nats.MaxWait(2*time.Second))
	require.NoError(t, err)
	require.Len(t, msgs, 1)

	msg := msgs[0]
	require.Equal(t, "delete", msg.Header.Get("X-Ploy-Change"))
	require.NoError(t, msg.Ack())

	objStore, err := js.ObjectStore(defaultBucket)
	require.NoError(t, err)

	data, err := objStore.GetBytes("apps/demo/routes.json")
	require.NoError(t, err)

	var stored map[string]routing.DomainRoute
	require.NoError(t, json.Unmarshal(data, &stored))
	require.NotContains(t, stored, "demo.example")
}

func TestStoreDomainListOperations(t *testing.T) {
	url, shutdown := startJetStreamServer(t)
	defer shutdown()

	conn, err := nats.Connect(url)
	require.NoError(t, err)
	defer conn.Close()

	ctx := context.Background()
	store := newTestStore(t, conn)

	require.NoError(t, store.ReplaceDomains(ctx, "demo", nil))
	require.NoError(t, store.AppendDomain(ctx, "demo", "demo.example"))
	require.NoError(t, store.AppendDomain(ctx, "demo", "www.demo.example"))

	domains, err := store.GetDomains(ctx, "demo")
	require.NoError(t, err)
	require.Equal(t, []string{"demo.example", "www.demo.example"}, domains)

	require.NoError(t, store.RemoveDomain(ctx, "demo", "demo.example"))
	domains, err = store.GetDomains(ctx, "demo")
	require.NoError(t, err)
	require.Equal(t, []string{"www.demo.example"}, domains)

	require.NoError(t, store.ReplaceDomains(ctx, "demo", []string{"a.demo", "b.demo"}))
	domains, err = store.GetDomains(ctx, "demo")
	require.NoError(t, err)
	require.Equal(t, []string{"a.demo", "b.demo"}, domains)
}

func TestRebroadcastAppPublishesEvent(t *testing.T) {
	url, shutdown := startJetStreamServer(t)
	defer shutdown()

	conn, err := nats.Connect(url)
	require.NoError(t, err)
	defer conn.Close()

	ctx := context.Background()
	store := newTestStore(t, conn)

	js, err := conn.JetStream()
	require.NoError(t, err)

	sub, err := js.PullSubscribe(fmt.Sprintf("%s.%s", defaultSubjectPrefix, "demo"), "rebroadcast-router", nats.BindStream(defaultStream))
	require.NoError(t, err)

	route := routing.DomainRoute{
		App:       "demo",
		Domain:    "demo.example",
		Port:      8080,
		AllocID:   "alloc-1",
		AllocIP:   "10.1.1.1",
		CreatedAt: time.Now().UTC(),
	}

	require.NoError(t, store.SaveAppRoute(ctx, route))

	msgs, err := sub.Fetch(1, nats.MaxWait(2*time.Second))
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.NoError(t, msgs[0].Ack())

	require.NoError(t, store.RebroadcastApp(ctx, "demo"))

	msgs, err = sub.Fetch(1, nats.MaxWait(2*time.Second))
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Equal(t, "rebroadcast", msgs[0].Header.Get("X-Ploy-Change"))
	require.NoError(t, msgs[0].Ack())
}
