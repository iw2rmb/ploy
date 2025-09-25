package selfupdate

import "time"

// MetricsRecorder captures Prometheus samples for self-update workflows.
// Implementations should be concurrency-safe.
type MetricsRecorder interface {
	RecordSelfUpdateBootstrap(stream, status string)
	RecordSelfUpdateTaskSubmission(lane, strategy, result string)
	ObserveSelfUpdateExecutorDuration(lane, strategy, result string, duration time.Duration)
	RecordSelfUpdateStatusPublished(lane, phase string)
	RecordSelfUpdateRedelivery(lane, reason string)
	RecordSelfUpdateStatusConsumerLag(consumer, lane string, lag time.Duration)
}
