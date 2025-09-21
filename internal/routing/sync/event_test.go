package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	natstest "github.com/nats-io/nats-server/v2/test"
	nats "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	"github.com/iw2rmb/ploy/internal/routing"
)

type stubWriter struct {
	last  map[string]map[string]routing.DomainRoute
	count int
}

func (s *stubWriter) Write(_ context.Context, routes map[string]map[string]routing.DomainRoute) error {
	s.last = routes
	s.count++
	return nil
}

func TestSyncerHandleEventFetchesRoutesAndWrites(t *testing.T) {
	url, shutdown := startTestJetStream(t)
	defer shutdown()

	conn, err := nats.Connect(url)
	require.NoError(t, err)
	defer conn.Close()

	js, err := conn.JetStream()
	require.NoError(t, err)

	// Prepare stream and object store to mirror routing store expectations.
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "routing_events",
		Subjects: []string{"routing.app.*"},
	})
	if err != nil && !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
		require.NoError(t, err)
	}

	store, err := js.CreateObjectStore(&nats.ObjectStoreConfig{Bucket: "routing_maps"})
	require.NoError(t, err)

	route := routing.DomainRoute{
		App:        "demo",
		Domain:     "demo.example",
		Port:       8080,
		AllocID:    "alloc-1",
		AllocIP:    "10.0.0.9",
		HealthPath: "/healthz",
		TLSEnabled: true,
		CreatedAt:  time.Now().UTC().Round(time.Second),
	}

	data, err := json.Marshal(map[string]routing.DomainRoute{"demo.example": route})
	require.NoError(t, err)

	_, err = store.Put(&nats.ObjectMeta{Name: "apps/demo/routes.json"}, bytesReader(data))
	require.NoError(t, err)

	writer := &stubWriter{}
	syncer := &Syncer{
		bucket:        store,
		stream:        "routing_events",
		subjectPrefix: "routing.app",
		routes:        make(map[string]map[string]routing.DomainRoute),
		writer:        writer,
	}

	event := routingEvent{
		App:          "demo",
		Domain:       "demo.example",
		Change:       "upsert",
		Checksum:     "abc",
		Revision:     "rev-1",
		ObjectKey:    "apps/demo/routes.json",
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
		PrevRevision: "",
	}

	require.NoError(t, syncer.handleEvent(context.Background(), event))
	require.Equal(t, 1, writer.count)
	require.Contains(t, writer.last, "demo")
	require.Contains(t, writer.last["demo"], "demo.example")
}

func startTestJetStream(t *testing.T) (string, func()) {
	t.Helper()
	opts := natstest.DefaultTestOptions
	opts.Port = -1
	opts.JetStream = true
	s := natstest.RunServer(&opts)
	return s.ClientURL(), s.Shutdown
}

// bytesReader helps satisfy the io.Reader contract for object store writes without
// importing bytes in the tests that use it later.
func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
