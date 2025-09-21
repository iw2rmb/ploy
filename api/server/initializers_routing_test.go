package server

import (
	"bytes"
	"log"
	"testing"

	natstest "github.com/nats-io/nats-server/v2/test"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/iw2rmb/ploy/api/metrics"
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

func TestInitializeTraefikRouterRecordsObjectStoreBootstrapMetrics(t *testing.T) {
	url, shutdown := startTestJetStream(t)
	defer shutdown()

	metricsInstance := metrics.NewMetrics()
	cfg := &ControllerConfig{
		ConsulAddr: "http://127.0.0.1:8500",
		JetStreamRouting: JetStreamRoutingConfig{
			Enabled:       true,
			URL:           url,
			Bucket:        "routing_maps",
			Stream:        "routing_events",
			SubjectPrefix: "routing.app",
			ChunkSize:     128 * 1024,
			Replicas:      1,
		},
	}

	before := testutil.ToFloat64(metricsInstance.RoutingObjectStoreCreateTotal.WithLabelValues("success"))

	router, err := initializeTraefikRouter(cfg, metricsInstance)
	require.NoError(t, err)
	require.NotNil(t, router)

	after := testutil.ToFloat64(metricsInstance.RoutingObjectStoreCreateTotal.WithLabelValues("success"))
	require.Equal(t, before+1, after)
}

func TestInitializeTraefikRouterRecordsObjectStoreBootstrapErrors(t *testing.T) {
	var buf bytes.Buffer
	original := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(original)

	metricsInstance := metrics.NewMetrics()
	cfg := &ControllerConfig{
		ConsulAddr: "http://127.0.0.1:8500",
		JetStreamRouting: JetStreamRoutingConfig{
			Enabled:       true,
			URL:           "nats://127.0.0.1:4223",
			Bucket:        "routing_maps",
			Stream:        "routing_events",
			SubjectPrefix: "routing.app",
			ChunkSize:     128 * 1024,
			Replicas:      1,
		},
	}

	before := testutil.ToFloat64(metricsInstance.RoutingObjectStoreCreateTotal.WithLabelValues("error"))

	router, err := initializeTraefikRouter(cfg, metricsInstance)
	require.NoError(t, err)
	require.NotNil(t, router)

	after := testutil.ToFloat64(metricsInstance.RoutingObjectStoreCreateTotal.WithLabelValues("error"))
	require.Equal(t, before+1, after)
	require.Contains(t, buf.String(), "routing object store bootstrap")
}

func TestInitializeTraefikRouterSkipsObjectStoreWhenDisabled(t *testing.T) {
	metricsInstance := metrics.NewMetrics()
	cfg := &ControllerConfig{
		ConsulAddr:       "http://127.0.0.1:8500",
		JetStreamRouting: JetStreamRoutingConfig{},
	}

	beforeSuccess := testutil.ToFloat64(metricsInstance.RoutingObjectStoreCreateTotal.WithLabelValues("success"))
	beforeError := testutil.ToFloat64(metricsInstance.RoutingObjectStoreCreateTotal.WithLabelValues("error"))

	router, err := initializeTraefikRouter(cfg, metricsInstance)
	require.NoError(t, err)
	require.NotNil(t, router)

	afterSuccess := testutil.ToFloat64(metricsInstance.RoutingObjectStoreCreateTotal.WithLabelValues("success"))
	afterError := testutil.ToFloat64(metricsInstance.RoutingObjectStoreCreateTotal.WithLabelValues("error"))

	require.Equal(t, beforeSuccess, afterSuccess)
	require.Equal(t, beforeError, afterError)
}
