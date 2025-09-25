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

	mockMetrics := &mockMetricsRecorder{}
	handler, err := NewHandler(storageProvider, queue, statusPublisher, "2025.10.0", mockMetrics)
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
	require.Len(t, mockMetrics.executorDurations, 1)
	require.Equal(t, "d", mockMetrics.executorDurations[0].lane)
	require.Equal(t, string(RollingUpdate), mockMetrics.executorDurations[0].strategy)
	require.Equal(t, "failed", mockMetrics.executorDurations[0].result)
	require.Len(t, mockMetrics.redeliveries, 1)
	require.Equal(t, "executor_error", mockMetrics.redeliveries[0].reason)
	require.NotEmpty(t, mockMetrics.statusPublished)
}

type mockMetricsRecorder struct {
	bootstrap         []struct{ stream, status string }
	taskSubmissions   []struct{ lane, strategy, result string }
	executorDurations []struct {
		lane, strategy, result string
		duration               time.Duration
	}
	statusPublished []struct{ lane, phase string }
	redeliveries    []struct{ lane, reason string }
	consumerLag     []struct {
		consumer, lane string
		lag            time.Duration
	}
}

func (m *mockMetricsRecorder) RecordSelfUpdateBootstrap(stream, status string) {
	m.bootstrap = append(m.bootstrap, struct{ stream, status string }{stream, status})
}

func (m *mockMetricsRecorder) RecordSelfUpdateTaskSubmission(lane, strategy, result string) {
	m.taskSubmissions = append(m.taskSubmissions, struct{ lane, strategy, result string }{lane, strategy, result})
}

func (m *mockMetricsRecorder) ObserveSelfUpdateExecutorDuration(lane, strategy, result string, duration time.Duration) {
	m.executorDurations = append(m.executorDurations, struct {
		lane, strategy, result string
		duration               time.Duration
	}{lane: lane, strategy: strategy, result: result, duration: duration})
}

func (m *mockMetricsRecorder) RecordSelfUpdateStatusPublished(lane, phase string) {
	m.statusPublished = append(m.statusPublished, struct{ lane, phase string }{lane, phase})
}

func (m *mockMetricsRecorder) RecordSelfUpdateRedelivery(lane, reason string) {
	m.redeliveries = append(m.redeliveries, struct{ lane, reason string }{lane, reason})
}

func (m *mockMetricsRecorder) RecordSelfUpdateStatusConsumerLag(consumer, lane string, lag time.Duration) {
	m.consumerLag = append(m.consumerLag, struct {
		consumer, lane string
		lag            time.Duration
	}{consumer: consumer, lane: lane, lag: lag})
}

func TestEnqueueUpdateRecordsTaskMetric(t *testing.T) {
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

	mockMetrics := &mockMetricsRecorder{}
	handler, err := NewHandler(storageProvider, queue, statusPublisher, "2025.10.0", mockMetrics)
	require.NoError(t, err)

	request := UpdateRequest{TargetVersion: "2025.11.0", Strategy: RollingUpdate}
	deploymentID, err := handler.enqueueUpdate(ctx, request, "tester@example.com")
	require.NoError(t, err)
	require.NotEmpty(t, deploymentID)
	require.Len(t, mockMetrics.taskSubmissions, 1)
	sample := mockMetrics.taskSubmissions[0]
	require.Equal(t, "d", sample.lane)
	require.Equal(t, string(RollingUpdate), sample.strategy)
	require.Equal(t, "accepted", sample.result)
}

func TestProcessTaskRecordsSuccessMetrics(t *testing.T) {
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

	mockMetrics := &mockMetricsRecorder{}
	handler, err := NewHandler(storageProvider, queue, statusPublisher, "2025.10.0", mockMetrics)
	require.NoError(t, err)

	handler.updateFn = func(context.Context, string, UpdateRequest, map[string]string) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	ackCalled := false
	message := &WorkQueueMessage{
		DeploymentID: "deploy-success",
		Request: UpdateRequest{
			TargetVersion: "2025.11.0",
			Strategy:      RollingUpdate,
		},
		Metadata: map[string]string{},
		Lane:     "d",
		AckWait:  time.Second,
		ackFn: func() error {
			ackCalled = true
			return nil
		},
	}

	err = handler.processTask(ctx, message)
	require.NoError(t, err)
	require.True(t, ackCalled, "expected ack on success")
	require.Len(t, mockMetrics.executorDurations, 1)
	sample := mockMetrics.executorDurations[0]
	require.Equal(t, "d", sample.lane)
	require.Equal(t, string(RollingUpdate), sample.strategy)
	require.Equal(t, "success", sample.result)
	require.NotEmpty(t, mockMetrics.statusPublished)
}
