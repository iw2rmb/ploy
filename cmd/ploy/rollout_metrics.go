package main

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// RolloutMetrics tracks in-memory counters for rollout operations.
type RolloutMetrics struct {
	mu            sync.Mutex
	startTime     time.Time
	stepsTotal    map[string]int
	stepsSuccess  map[string]int
	stepsFailed   map[string]int
	nodesTotal    int
	nodesSuccess  int
	nodesFailed   int
	attemptsTotal map[string]int
}

// NewRolloutMetrics creates a new metrics tracker.
func NewRolloutMetrics() *RolloutMetrics {
	return &RolloutMetrics{
		startTime:     time.Now(),
		stepsTotal:    make(map[string]int),
		stepsSuccess:  make(map[string]int),
		stepsFailed:   make(map[string]int),
		attemptsTotal: make(map[string]int),
	}
}

// RecordStep records a step execution with its outcome.
func (m *RolloutMetrics) RecordStep(step, status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stepsTotal[step]++
	switch status {
	case "completed", "success":
		m.stepsSuccess[step]++
	case "failed":
		m.stepsFailed[step]++
	}
}

// RecordAttempt records a retry attempt for a step.
func (m *RolloutMetrics) RecordAttempt(step string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attemptsTotal[step]++
}

// RecordNode records a node rollout outcome.
func (m *RolloutMetrics) RecordNode(success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodesTotal++
	if success {
		m.nodesSuccess++
	} else {
		m.nodesFailed++
	}
}

// PrintSummary prints a human-readable summary to the given writer.
func (m *RolloutMetrics) PrintSummary(w io.Writer) {
	m.mu.Lock()
	defer m.mu.Unlock()

	duration := time.Since(m.startTime)

	_, _ = fmt.Fprintf(w, "\nRollout Summary:\n")

	if m.nodesTotal > 0 {
		_, _ = fmt.Fprintf(w, "  Nodes: %d total, %d succeeded, %d failed\n",
			m.nodesTotal, m.nodesSuccess, m.nodesFailed)
	}

	totalSteps := 0
	totalSuccess := 0
	totalFailed := 0
	for _, count := range m.stepsTotal {
		totalSteps += count
	}
	for _, count := range m.stepsSuccess {
		totalSuccess += count
	}
	for _, count := range m.stepsFailed {
		totalFailed += count
	}

	if totalSteps > 0 {
		_, _ = fmt.Fprintf(w, "  Steps: %d total, %d succeeded, %d failed\n",
			totalSteps, totalSuccess, totalFailed)
	}

	if len(m.attemptsTotal) > 0 {
		totalAttempts := 0
		for _, count := range m.attemptsTotal {
			totalAttempts += count
		}
		_, _ = fmt.Fprintf(w, "  Attempts: %d total\n", totalAttempts)
	}

	_, _ = fmt.Fprintf(w, "  Duration: %s\n", duration.Round(time.Millisecond))
}
