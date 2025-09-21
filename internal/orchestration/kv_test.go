package orchestration

import (
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/stretchr/testify/require"
)

func TestNewKVDefaultsToConsul(t *testing.T) {
	t.Setenv("PLOY_USE_JETSTREAM_KV", "")
	kv := NewKV()
	_, ok := kv.(*consulKV)
	require.True(t, ok, "expected fallback to consul KV when flag disabled")
}

func TestJetstreamKVOperations(t *testing.T) {
	srv := runJetstreamServer(t)
	t.Cleanup(func() {
		srv.Shutdown()
		srv.WaitForShutdown()
	})

	t.Setenv("PLOY_USE_JETSTREAM_KV", "true")
	t.Setenv("PLOY_JETSTREAM_URL", srv.ClientURL())
	// ensure a dedicated bucket for isolation
	t.Setenv("PLOY_JETSTREAM_KV_BUCKET", "test_kv")

	kv := NewKV()
	jsAdapter, ok := kv.(*jetstreamKV)
	require.True(t, ok, "expected jetstream KV when flag enabled")
	require.NotNil(t, jsAdapter)
	t.Cleanup(func() {
		if jsAdapter.conn != nil {
			if err := jsAdapter.conn.Drain(); err != nil {
				t.Logf("jetstream drain error: %v", err)
			}
			jsAdapter.conn.Close()
		}
	})

	key := "test/payload"
	value := []byte("hello-world")

	require.NoError(t, kv.Put(key, value))
	got, err := kv.Get(key)
	require.NoError(t, err)
	require.Equal(t, value, got)

	keys, err := kv.Keys("test/", "/")
	require.NoError(t, err)
	require.Contains(t, keys, key)

	require.NoError(t, kv.Delete(key))
	missing, err := kv.Get(key)
	require.NoError(t, err)
	require.Nil(t, missing)
}

func runJetstreamServer(t *testing.T) *server.Server {
	t.Helper()
	opts := &server.Options{
		ServerName: "kv-test",
		Port:       -1,
		JetStream:  true,
		StoreDir:   t.TempDir(),
	}
	srv, err := server.NewServer(opts)
	require.NoError(t, err)

	go srv.Start()
	if !srv.ReadyForConnections(10 * time.Second) {
		srv.Shutdown()
		t.Fatalf("jetstream server failed to start")
	}
	return srv
}
