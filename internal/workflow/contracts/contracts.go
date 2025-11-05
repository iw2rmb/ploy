package contracts

import (
	"fmt"
	"strings"
)

// SchemaVersion identifies the JSON envelope version used by workflow
// tickets, checkpoints, and artifact messages. It must be included in all
// published envelopes so consumers can validate and evolve parsers safely.
const SchemaVersion = "2025-09-27.1"

const (
	checkpointStreamFormat = "ploy.workflow.%s.checkpoints"
	artifactStreamFormat   = "ploy.artifact.%s"
	statusStreamFormat     = "jobs.%s.events"
)

// SubjectSet contains the per‑ticket subjects used for publishing workflow
// events. Empty strings are returned when the ticket ID is blank to signal
// that publishing should be skipped by callers.
type SubjectSet struct {
	CheckpointStream string
	ArtifactStream   string
	StatusStream     string
}

// SubjectsForTicket derives the subjects used to publish checkpoints,
// artifacts, and status events for a given ticket ID. When the provided
// ticket is empty or whitespace, all fields in the returned set are empty.
func SubjectsForTicket(ticketID string) SubjectSet {
	trimmedTicket := strings.TrimSpace(ticketID)
	checkpointStream := ""
	artifactStream := ""
	statusStream := ""
	if trimmedTicket != "" {
		checkpointStream = fmt.Sprintf(checkpointStreamFormat, trimmedTicket)
		artifactStream = fmt.Sprintf(artifactStreamFormat, trimmedTicket)
		statusStream = fmt.Sprintf(statusStreamFormat, trimmedTicket)
	}

	return SubjectSet{
		CheckpointStream: checkpointStream,
		ArtifactStream:   artifactStream,
		StatusStream:     statusStream,
	}
}
