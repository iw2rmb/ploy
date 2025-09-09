package arf

import (
	"time"
)

// extractActiveAttempts extracts currently active healing attempts from the healing tree
func extractActiveAttempts(attempts []HealingAttempt) []ActiveAttemptDetails {
	var active []ActiveAttemptDetails

	var extract func([]HealingAttempt)
	extract = func(attempts []HealingAttempt) {
		for _, attempt := range attempts {
			if attempt.Status == "in_progress" {
				detail := ActiveAttemptDetails{
					AttemptPath:   attempt.AttemptPath,
					Status:        attempt.Status,
					TriggerReason: attempt.TriggerReason,
					StartTime:     attempt.StartTime,
					ElapsedTime:   time.Since(attempt.StartTime),
					Progress:      attempt.Progress,
				}
				active = append(active, detail)
			}
			// Recurse into children
			extract(attempt.Children)
		}
	}

	extract(attempts)
	return active
}

// calculateAverageDuration calculates the average duration of completed healing attempts
func calculateAverageDuration(attempts []HealingAttempt) time.Duration {
	var totalDuration time.Duration
	var count int

	for _, attempt := range attempts {
		if !attempt.EndTime.IsZero() {
			totalDuration += attempt.EndTime.Sub(attempt.StartTime)
			count++
		}
	}

	if count > 0 {
		return totalDuration / time.Duration(count)
	}
	return 0
}
