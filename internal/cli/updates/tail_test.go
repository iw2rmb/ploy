package updates

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	natstest "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/iw2rmb/ploy/api/selfupdate"
)

func startTestJetStream(t *testing.T) (string, func()) {
	t.Helper()
	opts := natstest.DefaultTestOptions
	opts.Port = -1
	opts.JetStream = true
	opts.Host = "127.0.0.1"
	opts.StoreDir = t.TempDir()
	opts.NoSigs = true

	srv, err := server.NewServer(&opts)
	require.NoError(t, err)

	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatalf("jetstream server not ready")
	}

	cleanup := func() {
		srv.Shutdown()
		srv.WaitForShutdown()
	}

	return srv.ClientURL(), cleanup
}

func TestConsumeStatusEventsReturnsTerminalEvent(t *testing.T) {
	url, shutdown := startTestJetStream(t)
	defer shutdown()

	conn, err := nats.Connect(url)
	require.NoError(t, err)
	defer conn.Close()

	js, err := conn.JetStream()
	require.NoError(t, err)

	ctx := context.Background()
	statusCfg := selfupdate.StatusStreamConfig{
		Stream:        "updates.control-plane.status",
		SubjectPrefix: "updates.control-plane.status",
		DurablePrefix: "updates-status",
	}

	publisher, err := selfupdate.NewStatusPublisher(ctx, js, statusCfg)
	require.NoError(t, err)

	events := []selfupdate.StatusEvent{
		{
			DeploymentID: "deploy-abc",
			Phase:        "preparing",
			Progress:     0,
			Message:      "claiming update",
			Metadata:     map[string]string{"lane": "d"},
			Timestamp:    time.Now().UTC().Round(time.Millisecond),
		},
		{
			DeploymentID: "deploy-abc",
			Phase:        "completed",
			Progress:     100,
			Message:      "update finished",
			Metadata:     map[string]string{"lane": "d"},
			Timestamp:    time.Now().UTC().Round(time.Millisecond),
		},
	}

	for _, event := range events {
		require.NoError(t, publisher.Publish(ctx, event))
	}

	var received []selfupdate.StatusEvent
	tailCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = ConsumeStatusEvents(tailCtx, js, TailOptions{
		Stream:        statusCfg.Stream,
		SubjectPrefix: statusCfg.SubjectPrefix,
		DurablePrefix: "updates-status-cli",
		DeploymentID:  "deploy-abc",
		AckWait:       time.Second,
		FetchTimeout:  200 * time.Millisecond,
		Follow:        false,
	}, func(event selfupdate.StatusEvent) {
		received = append(received, event)
	})
	require.NoError(t, err)
	require.Len(t, received, 2)
	require.Equal(t, "preparing", received[0].Phase)
	require.Equal(t, "completed", received[1].Phase)
}

func TestConsumeStatusEventsUpdatesLagGauge(t *testing.T) {
	url, shutdown := startTestJetStream(t)
	defer shutdown()

	conn, err := nats.Connect(url)
	require.NoError(t, err)
	defer conn.Close()

	js, err := conn.JetStream()
	require.NoError(t, err)

	ctx := context.Background()
	statusCfg := selfupdate.StatusStreamConfig{
		Stream:        "updates.control-plane.status",
		SubjectPrefix: "updates.control-plane.status",
		DurablePrefix: "updates-status",
	}

	publisher, err := selfupdate.NewStatusPublisher(ctx, js, statusCfg)
	require.NoError(t, err)

	event := selfupdate.StatusEvent{
		DeploymentID: "deploy-lag",
		Phase:        "completed",
		Progress:     100,
		Message:      "done",
		Metadata:     map[string]string{"lane": "d"},
		Timestamp:    time.Now().UTC().Add(-5 * time.Second).Round(time.Millisecond),
	}
	require.NoError(t, publisher.Publish(ctx, event))

	tailCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = ConsumeStatusEvents(tailCtx, js, TailOptions{
		Stream:        statusCfg.Stream,
		SubjectPrefix: statusCfg.SubjectPrefix,
		DurablePrefix: "updates-status-cli",
		DeploymentID:  "deploy-lag",
		AckWait:       time.Second,
		FetchTimeout:  200 * time.Millisecond,
	}, func(selfupdate.StatusEvent) {})
	require.NoError(t, err)

	metrics, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	var observed bool
	for _, mf := range metrics {
		if mf.GetName() != "ploy_updates_status_consumer_lag_seconds" {
			continue
		}
		for _, metric := range mf.GetMetric() {
			if len(metric.GetLabel()) == 0 {
				continue
			}
			labels := metric.GetLabel()
			consumer := ""
			lane := ""
			for _, label := range labels {
				switch label.GetName() {
				case "consumer":
					consumer = label.GetValue()
				case "lane":
					lane = label.GetValue()
				}
			}
			if consumer == "cli" && lane == "d" {
				if metric.GetGauge() != nil && metric.GetGauge().GetValue() >= 0 {
					observed = true
					break
				}
			}
		}
	}
	require.True(t, observed, "expected CLI lag gauge sample")
}
