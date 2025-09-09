package arf

import (
	"fmt"
	"sort"
	"time"
)

// buildTimeline creates a chronological timeline of transformation events
func buildTimeline(status *TransformationStatus) *TransformationTimeline {
	timeline := &TransformationTimeline{
		TransformationID: status.TransformationID,
		Timeline:         []TimelineEntry{},
	}

	// Add transformation start
	if !status.StartTime.IsZero() {
		timeline.Timeline = append(timeline.Timeline, TimelineEntry{
			Timestamp:   status.StartTime,
			EventType:   "transformation_start",
			Description: "Transformation started",
			Status:      status.Status,
		})
	}

	// Add healing attempt events
	addAttemptEvents(&timeline.Timeline, status.Children)

	// Add transformation end
	if !status.EndTime.IsZero() {
		timeline.Timeline = append(timeline.Timeline, TimelineEntry{
			Timestamp:   status.EndTime,
			EventType:   "transformation_end",
			Description: "Transformation completed",
			Status:      status.Status,
		})
		timeline.TotalDuration = status.EndTime.Sub(status.StartTime)
	}

	// Sort timeline by timestamp
	sort.Slice(timeline.Timeline, func(i, j int) bool {
		return timeline.Timeline[i].Timestamp.Before(timeline.Timeline[j].Timestamp)
	})

	// Analyze gaps and parallel periods
	timeline.GapAnalysis = analyzeGaps(timeline.Timeline)
	timeline.ParallelPeriods = analyzeParallelPeriods(timeline.Timeline)

	return timeline
}

// addAttemptEvents recursively adds healing attempt events to the timeline
func addAttemptEvents(timeline *[]TimelineEntry, attempts []HealingAttempt) {
	for _, attempt := range attempts {
		// Add start event
		if !attempt.StartTime.IsZero() {
			*timeline = append(*timeline, TimelineEntry{
				Timestamp:   attempt.StartTime,
				EventType:   "attempt_start",
				AttemptPath: attempt.AttemptPath,
				Description: fmt.Sprintf("Healing attempt %s started (%s)", attempt.AttemptPath, attempt.TriggerReason),
				Status:      attempt.Status,
			})
		}

		// Add end event
		if !attempt.EndTime.IsZero() {
			duration := attempt.EndTime.Sub(attempt.StartTime)
			*timeline = append(*timeline, TimelineEntry{
				Timestamp:   attempt.EndTime,
				EventType:   "attempt_end",
				AttemptPath: attempt.AttemptPath,
				Description: fmt.Sprintf("Healing attempt %s completed with result: %s", attempt.AttemptPath, attempt.Result),
				Status:      attempt.Status,
				Duration:    &duration,
			})
		}

		// Recurse for children
		addAttemptEvents(timeline, attempt.Children)
	}
}

// analyzeGaps identifies periods of inactivity in the timeline
func analyzeGaps(timeline []TimelineEntry) []GapPeriod {
	var gaps []GapPeriod

	for i := 1; i < len(timeline); i++ {
		gap := timeline[i].Timestamp.Sub(timeline[i-1].Timestamp)
		if gap > 5*time.Minute { // Consider gaps longer than 5 minutes significant
			gaps = append(gaps, GapPeriod{
				Start:    timeline[i-1].Timestamp,
				End:      timeline[i].Timestamp,
				Duration: gap,
				Reason:   "Inactivity detected",
			})
		}
	}

	return gaps
}

// analyzeParallelPeriods identifies periods where multiple attempts ran in parallel
func analyzeParallelPeriods(timeline []TimelineEntry) []ParallelPeriod {
	// Simple implementation - would need more sophisticated logic for real parallel analysis
	return []ParallelPeriod{}
}
