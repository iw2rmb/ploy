package health

import (
	"testing"

	natstest "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

func TestHealthStatusReportsJetStreamDependency(t *testing.T) {
	t.Setenv("PLOY_JETSTREAM_URL", "nats://127.0.0.1:49222")
	t.Setenv("PLOY_JETSTREAM_CREDS", "")
	t.Setenv("PLOY_JETSTREAM_USER", "")
	t.Setenv("PLOY_JETSTREAM_PASSWORD", "")
	t.Setenv("PLOY_JETSTREAM_HEALTH_TIMEOUT", "1s")
	t.Setenv("NOMAD_ADDR", "http://127.0.0.1:1")
	t.Setenv("NOMAD_HTTP_MAX_RETRIES", "0")

	checker := NewHealthChecker("", "")
	status := checker.GetHealthStatus()

	dep, ok := status.Dependencies["jetstream"]
	require.True(t, ok, "expected jetstream dependency to be reported")
	require.Equal(t, "unhealthy", dep.Status, "expected unreachable jetstream to mark dependency unhealthy")
	_, hasConsul := status.Dependencies["consul"]
	require.False(t, hasConsul, "consul dependency should be removed after migration")
}

func TestReadinessCriticalDependenciesIncludeJetStream(t *testing.T) {
	t.Setenv("PLOY_JETSTREAM_URL", "nats://127.0.0.1:49223")
	t.Setenv("PLOY_JETSTREAM_HEALTH_TIMEOUT", "1s")
	t.Setenv("NOMAD_ADDR", "http://127.0.0.1:1")
	t.Setenv("NOMAD_HTTP_MAX_RETRIES", "0")

	checker := NewHealthChecker("", "")
	readiness := checker.GetReadinessStatus()

	require.Contains(t, readiness.Dependencies, "jetstream", "jetstream readiness check expected")
	require.NotContains(t, readiness.Dependencies, "consul", "consul readiness check should be removed")

	require.Contains(t, readiness.CriticalDependencies, "jetstream", "jetstream must be a critical dependency")
	for _, dep := range readiness.CriticalDependencies {
		require.NotEqual(t, "consul", dep, "consul should not be marked critical")
	}
}

func TestReadinessReportsHealthyJetStream(t *testing.T) {
	opts := natstest.DefaultTestOptions
	opts.Port = -1
	opts.JetStream = true
	opts.StoreDir = t.TempDir()

	srv := natstest.RunServer(&opts)
	t.Cleanup(func() {
		srv.Shutdown()
	})

	t.Setenv("PLOY_JETSTREAM_URL", srv.ClientURL())
	t.Setenv("PLOY_JETSTREAM_ENV_BUCKET", "health_env")
	t.Setenv("PLOY_JETSTREAM_HEALTH_TIMEOUT", "1s")
	t.Setenv("NOMAD_ADDR", "http://127.0.0.1:1")
	t.Setenv("NOMAD_HTTP_MAX_RETRIES", "0")

	nc, err := nats.Connect(srv.ClientURL())
	require.NoError(t, err)
	js, err := nc.JetStream()
	require.NoError(t, err)
	_, err = js.CreateKeyValue(&nats.KeyValueConfig{Bucket: "health_env"})
	require.NoError(t, err)
	t.Cleanup(func() { nc.Close() })

	checker := NewHealthChecker("", "")
	readiness := checker.GetReadinessStatus()

	dep, ok := readiness.Dependencies["jetstream"]
	require.True(t, ok, "expected jetstream dependency")
	require.Equal(t, "healthy", dep.Status, "expected jetstream readiness to report healthy when available")
}
