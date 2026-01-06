// job_status.go defines typed constants for job status and mod_type values.
//
// These constants centralize string literals used across the nodeagent package
// to reduce drift between components and ensure consistency with the control plane
// API contract. The v1 API uses capitalized job status values (Success, Fail, Cancelled).
//
// Usage:
//   - Import these constants instead of using string literals directly.
//   - This prevents typos and enables IDE refactoring across all usages.
//   - Changes to status values propagate automatically to all call sites.
package nodeagent

// JobStatus represents the terminal status of a job execution.
// The v1 API uses capitalized job status values.
type JobStatus string

const (
	// JobStatusSuccess indicates the job completed successfully.
	// Used when exit code is 0 and no runtime errors occurred.
	JobStatusSuccess JobStatus = "Success"

	// JobStatusFail indicates the job failed.
	// Used when exit code is non-zero, runtime errors occurred,
	// or other failure conditions are detected (e.g., healing produced no changes).
	JobStatusFail JobStatus = "Fail"

	// JobStatusCancelled indicates the job was cancelled.
	// Used when the context is cancelled before job completion.
	JobStatusCancelled JobStatus = "Cancelled"
)

// String returns the string representation of the JobStatus.
// This allows JobStatus to be used directly in APIs that expect strings.
func (s JobStatus) String() string {
	return string(s)
}

// DiffModType represents the mod_type value used to tag diffs.
// This categorizes diffs for filtering during workspace rehydration.
type DiffModType string

const (
	// DiffModTypeMod indicates a diff produced by a mod job.
	// These diffs participate in the rehydration chain for subsequent steps.
	DiffModTypeMod DiffModType = "mod"

	// DiffModTypeHealing indicates a diff produced by inline healing.
	// These diffs are filtered out during rehydration to avoid applying
	// intermediate healing states. Discrete healing jobs use DiffModTypeMod
	// so their changes are included in the rehydration chain.
	DiffModTypeHealing DiffModType = "healing"
)

// String returns the string representation of the DiffModType.
// This allows DiffModType to be used directly in APIs that expect strings.
func (t DiffModType) String() string {
	return string(t)
}
