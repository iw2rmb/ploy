package contracts

import (
	"fmt"
	"strings"
)

const SchemaVersion = "2025-09-27.1"

const (
	webhookSubjectFormat       = "webhook.%s.%s.%s"
	webhookSourcePloyWorkflow  = "ploy"
	webhookEventWorkflowTicket = "workflow-ticket"
	checkpointStreamFormat     = "ploy.workflow.%s.checkpoints"
	artifactStreamFormat       = "ploy.artifact.%s"
	statusStreamFormat         = "jobs.%s.events"
)

type SubjectSet struct {
	TicketInbox      string
	CheckpointStream string
	ArtifactStream   string
	StatusStream     string
}

func SubjectsForTenant(tenant, ticketID string) SubjectSet {
	trimmedTenant := strings.TrimSpace(tenant)
	ticketInbox := ""
	if trimmedTenant != "" {
		ticketInbox = fmt.Sprintf(
			webhookSubjectFormat,
			trimmedTenant,
			webhookSourcePloyWorkflow,
			webhookEventWorkflowTicket,
		)
	}

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
		TicketInbox:      ticketInbox,
		CheckpointStream: checkpointStream,
		ArtifactStream:   artifactStream,
		StatusStream:     statusStream,
	}
}
