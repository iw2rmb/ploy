package selfupdate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	natstest "github.com/nats-io/nats-server/v2/test"
	nats "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

func startJetStream(t *testing.T) (string, func()) {
	t.Helper()
	storeDir := filepath.Join("test-results", "nats", t.Name())
	absStoreDir, err := filepath.Abs(storeDir)
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(absStoreDir))
	require.NoError(t, os.MkdirAll(absStoreDir, 0o755))

	opts := natstest.DefaultTestOptions
	opts.Host = "127.0.0.1"
	opts.Port = -1
	opts.JetStream = true
	opts.StoreDir = absStoreDir
	opts.NoSigs = true

	srv, err := server.NewServer(&opts)
	require.NoError(t, err)

	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Skip("jetstream server not ready")
	}
	cleanup := func() {
		srv.Shutdown()
		srv.WaitForShutdown()
		_ = os.RemoveAll(absStoreDir)
	}
	return srv.ClientURL(), cleanup
}

func TestJetStreamWorkQueueEnqueueAndFetch(t *testing.T) {
	url, shutdown := startJetStream(t)
	defer shutdown()

	conn, err := nats.Connect(url)
	require.NoError(t, err)
	defer conn.Close()

	js, err := conn.JetStream()
	require.NoError(t, err)

	cfg := WorkQueueConfig{
		Stream:        "updates.control-plane",
		SubjectPrefix: "updates.control-plane.tasks",
		DurablePrefix: "updates-control-plane",
		Lane:          "d",
		AckWait:       2 * time.Minute,
		MaxAckPending: 1,
		MaxDeliver:    3,
		Replicas:      1,
	}

	ctx := context.Background()
	queue, err := NewJetStreamWorkQueue(ctx, js, cfg)
	require.NoError(t, err)

	task := WorkQueueTask{
		DeploymentID: "deploy-123",
		SubmittedBy:  "tester@ploy",
		Request: UpdateRequest{
			TargetVersion: "2025.11.0",
			Strategy:      RollingUpdate,
			Metadata: map[string]string{
				"lane": "d",
			},
		},
		Metadata: map[string]string{
			"reason": "unit-test",
		},
	}

	err = queue.Enqueue(ctx, task)
	require.NoError(t, err)

	msg, err := queue.Fetch(ctx, time.Second)
	require.NoError(t, err)
	require.NotNil(t, msg)
	require.Equal(t, task.DeploymentID, msg.DeploymentID)
	require.Equal(t, task.Request.TargetVersion, msg.Request.TargetVersion)
	require.Equal(t, RollingUpdate, msg.Request.Strategy)
	require.Equal(t, "deploy-123", msg.Headers.Get("Nats-Msg-Id"))
}

func TestJetStreamWorkQueueDuplicateTaskReturnsErrDuplicateTask(t *testing.T) {
	url, shutdown := startJetStream(t)
	defer shutdown()

	conn, err := nats.Connect(url)
	require.NoError(t, err)
	defer conn.Close()

	js, err := conn.JetStream()
	require.NoError(t, err)

	cfg := WorkQueueConfig{
		Stream:        "updates.control-plane",
		SubjectPrefix: "updates.control-plane.tasks",
		DurablePrefix: "updates-control-plane",
		Lane:          "d",
	}

	ctx := context.Background()
	queue, err := NewJetStreamWorkQueue(ctx, js, cfg)
	require.NoError(t, err)

	task := WorkQueueTask{
		DeploymentID: "deploy-duplicate",
		Request: UpdateRequest{
			TargetVersion: "2025.11.1",
			Strategy:      RollingUpdate,
		},
	}

	require.NoError(t, queue.Enqueue(ctx, task))

	err = queue.Enqueue(ctx, task)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrDuplicateTask)
}

func TestStatusPublisherEmitsEvents(t *testing.T) {
	url, shutdown := startJetStream(t)
	defer shutdown()

	conn, err := nats.Connect(url)
	require.NoError(t, err)
	defer conn.Close()

	js, err := conn.JetStream()
	require.NoError(t, err)

	cfg := StatusStreamConfig{
		Stream:        "updates.control-plane.status",
		SubjectPrefix: "updates.control-plane.status",
		DurablePrefix: "updates-status",
		Replicas:      1,
		MaxAge:        time.Hour,
	}

	ctx := context.Background()
	sanitized := sanitizeStreamName(cfg.Stream)
	require.NotEmpty(t, sanitized)
	publisher, err := NewStatusPublisher(ctx, js, cfg)
	require.NoError(t, err)

	event := StatusEvent{
		DeploymentID: "deploy-123",
		Phase:        "downloading",
		Progress:     25,
		Message:      "binary downloaded",
		Executor:     "alloc-test",
	}

	err = publisher.Publish(ctx, event)
	require.NoError(t, err)

	subject := cfg.SubjectPrefix + "." + event.DeploymentID
	sub, err := js.PullSubscribe(subject, cfg.DurablePrefix+"-test", nats.BindStream(sanitized))
	require.NoError(t, err)

	msgs, err := sub.Fetch(1, nats.MaxWait(2*time.Second))
	require.NoError(t, err)
	require.Len(t, msgs, 1)

	msg := msgs[0]
	require.Equal(t, subject, msg.Subject)
	require.Equal(t, event.Phase, msg.Header.Get("X-Ploy-Phase"))
	require.Equal(t, "deploy-123", msg.Header.Get("X-Ploy-Deployment"))

	var payload StatusEvent
	require.NoError(t, json.Unmarshal(msg.Data, &payload))
	require.Equal(t, event.Progress, payload.Progress)
	require.Equal(t, event.Executor, payload.Executor)
}

func TestStatusPublisherLastEvent(t *testing.T) {
	url, shutdown := startJetStream(t)
	defer shutdown()

	conn, err := nats.Connect(url)
	require.NoError(t, err)
	defer conn.Close()

	js, err := conn.JetStream()
	require.NoError(t, err)

	cfg := StatusStreamConfig{
		Stream:        "updates.control-plane.status",
		SubjectPrefix: "updates.control-plane.status",
		DurablePrefix: "updates-status",
		Replicas:      1,
		MaxAge:        time.Hour,
	}

	ctx := context.Background()
	publisher, err := NewStatusPublisher(ctx, js, cfg)
	require.NoError(t, err)

	_, err = publisher.LastEvent(ctx, "missing")
	require.ErrorIs(t, err, ErrStatusEventNotFound)

	event := StatusEvent{
		DeploymentID: "deploy-987",
		Phase:        "deploying",
		Progress:     60,
		Message:      "copying binaries",
		Executor:     "alloc-test",
	}

	require.NoError(t, publisher.Publish(ctx, event))

	got, err := publisher.LastEvent(ctx, event.DeploymentID)
	require.NoError(t, err)
	require.Equal(t, event.DeploymentID, got.DeploymentID)
	require.Equal(t, event.Phase, got.Phase)
	require.Equal(t, event.Progress, got.Progress)
	require.Equal(t, event.Executor, got.Executor)
	require.Equal(t, event.Message, got.Message)
}
