package contracts

import (
	"fmt"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// SchemaVersion identifies the JSON envelope version used by workflow
// run envelopes, checkpoints, and artifact messages. It must be included in all
// published envelopes so consumers can validate and evolve parsers safely.
const SchemaVersion = "2025-09-27.1"

const (
	checkpointStreamFormat = "ploy.workflow.%s.checkpoints"
	artifactStreamFormat   = "ploy.artifact.%s"
	statusStreamFormat     = "jobs.%s.events"
)

// SubjectSet contains the per‑run subjects used for publishing workflow
// events. Empty strings are returned when the run ID is blank to signal
// that publishing should be skipped by callers.
type SubjectSet struct {
	CheckpointStream string
	ArtifactStream   string
	StatusStream     string
}

// SubjectsForRun derives the subjects used to publish checkpoints,
// artifacts, and status events for a given run ID. When the provided
// run ID is empty or whitespace, all fields in the returned set are empty.
func SubjectsForRun(runID types.RunID) SubjectSet {
	trimmedRunID := strings.TrimSpace(runID.String())
	checkpointStream := ""
	artifactStream := ""
	statusStream := ""
	if trimmedRunID != "" {
		checkpointStream = fmt.Sprintf(checkpointStreamFormat, trimmedRunID)
		artifactStream = fmt.Sprintf(artifactStreamFormat, trimmedRunID)
		statusStream = fmt.Sprintf(statusStreamFormat, trimmedRunID)
	}

	return SubjectSet{
		CheckpointStream: checkpointStream,
		ArtifactStream:   artifactStream,
		StatusStream:     statusStream,
	}
}
