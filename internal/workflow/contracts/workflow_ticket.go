package contracts

import "fmt"

type WorkflowTicket struct {
	SchemaVersion string            `json:"schema_version"`
	TicketID      string            `json:"ticket_id"`
	Tenant        string            `json:"tenant"`
	Manifest      ManifestReference `json:"manifest"`
}

func (t WorkflowTicket) Validate() error {
	if t.SchemaVersion == "" {
		return fmt.Errorf("schema_version is required")
	}
	if t.TicketID == "" {
		return fmt.Errorf("ticket_id is required")
	}
	if t.Tenant == "" {
		return fmt.Errorf("tenant is required")
	}
	if err := t.Manifest.Validate(); err != nil {
		return fmt.Errorf("manifest invalid: %w", err)
	}
	return nil
}
