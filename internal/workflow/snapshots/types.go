package snapshots

import (
	"context"
	"time"
)

type ArtifactPublisher interface {
	Publish(ctx context.Context, data []byte) (string, error)
}

type MetadataPublisher interface {
	Publish(ctx context.Context, meta SnapshotMetadata) error
}

type LoadOptions struct {
	ArtifactPublisher ArtifactPublisher
	MetadataPublisher MetadataPublisher
}

type Registry struct {
	specs    map[string]Spec
	artifact ArtifactPublisher
	metadata MetadataPublisher
}

type Spec struct {
	Name        string          `toml:"name"`
	Description string          `toml:"description"`
	Source      Source          `toml:"source"`
	Strip       []StripRule     `toml:"strip"`
	Mask        []MaskRule      `toml:"mask"`
	Synthetic   []SyntheticRule `toml:"synthetic"`
}

type Source struct {
	Engine  string `toml:"engine"`
	DSN     string `toml:"dsn"`
	Fixture string `toml:"fixture"`
}

type StripRule struct {
	Table   string   `toml:"table"`
	Columns []string `toml:"columns"`
}

type MaskRule struct {
	Table    string `toml:"table"`
	Column   string `toml:"column"`
	Strategy string `toml:"strategy"`
}

type SyntheticRule struct {
	Table    string `toml:"table"`
	Column   string `toml:"column"`
	Strategy string `toml:"strategy"`
}

type RuleSummary struct {
	Total  int
	Tables map[string]int
}

type PlanReport struct {
	SnapshotName string
	Description  string
	Engine       string
	FixturePath  string
	Stripping    RuleSummary
	Masking      RuleSummary
	Synthetic    RuleSummary
	Highlights   []string
}

type CaptureOptions struct {
    TicketID string
}

type DiffSummary struct {
	StrippedColumns  map[string][]string
	MaskedColumns    map[string][]string
	SyntheticColumns map[string][]string
}

type RuleCounts struct {
	Strip     int `json:"strip"`
	Mask      int `json:"mask"`
	Synthetic int `json:"synthetic"`
}

type SnapshotMetadata struct {
    SnapshotName string     `json:"snapshot_name"`
    Description  string     `json:"description"`
    TicketID     string     `json:"ticket_id"`
    Engine       string     `json:"engine"`
    DSN          string     `json:"dsn"`
    Fingerprint  string     `json:"fingerprint"`
    ArtifactCID  string     `json:"artifact_cid"`
    CapturedAt   time.Time  `json:"captured_at"`
    RuleCounts   RuleCounts `json:"rule_counts"`
}

type CaptureResult struct {
	ArtifactCID string
	Fingerprint string
	Metadata    SnapshotMetadata
	Diff        DiffSummary
	Payload     []byte
}
