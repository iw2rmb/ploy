package selfupdate

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/storage/providers/memory"
)

func TestProcessTaskNaksOnUpdateFailure(t *testing.T) {
	jsURL, shutdown := startJetStream(t)
	defer shutdown()

	conn, err := nats.Connect(jsURL)
	require.NoError(t, err)
	defer conn.Close()

	js, err := conn.JetStream()
	require.NoError(t, err)

	ctx := context.Background()

	queueCfg := WorkQueueConfig{
		Stream:        "updates.control-plane",
		SubjectPrefix: "updates.control-plane.tasks",
		DurablePrefix: "updates-control-plane",
		Lane:          "d",
		AckWait:       time.Second,
	}

	queue, err := NewJetStreamWorkQueue(ctx, js, queueCfg)
	require.NoError(t, err)

	statusCfg := StatusStreamConfig{
		Stream:        "updates.control-plane.status",
		SubjectPrefix: "updates.control-plane.status",
		DurablePrefix: "updates-status",
	}

	statusPublisher, err := NewStatusPublisher(ctx, js, statusCfg)
	require.NoError(t, err)

	memoryStorage := memory.NewMemoryStorage(0)
	storageProvider := storage.NewProviderFromStorage(memoryStorage, "artifacts")

	handler, err := NewHandler(storageProvider, queue, statusPublisher, "2025.10.0")
	require.NoError(t, err)

	handler.updateFn = func(context.Context, string, UpdateRequest, map[string]string) error {
		return fmt.Errorf("boom")
	}

	ackCalled := false
	nakCalled := false
	var nakDelay time.Duration

	message := &WorkQueueMessage{
		DeploymentID: "deploy-fail",
		Request: UpdateRequest{
			TargetVersion: "2025.11.0",
			Strategy:      RollingUpdate,
		},
		Metadata:    map[string]string{},
		SubmittedBy: "tester",
		SubmittedAt: time.Now(),
		Lane:        "d",
		AckWait:     time.Second,
		ackFn: func() error {
			ackCalled = true
			return nil
		},
		nakFn: func(delay time.Duration) error {
			nakCalled = true
			nakDelay = delay
			return nil
		},
	}

	err = handler.processTask(ctx, message)
	require.Error(t, err)
	require.False(t, ackCalled, "expected no ack on failure")
	require.True(t, nakCalled, "expected nak on failure")
	require.Greater(t, nakDelay, time.Duration(0))
}
