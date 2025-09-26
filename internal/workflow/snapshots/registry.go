package snapshots

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

var nowFunc = time.Now

var (
	ErrSnapshotNotFound = errors.New("snapshot not found")
	ErrInvalidSpec      = errors.New("invalid snapshot spec")
	ErrInvalidRule      = errors.New("invalid snapshot rule")
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
	Tenant   string
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
	Tenant       string     `json:"tenant"`
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

type dataset map[string][]row

type row map[string]string

type orderedField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type orderedRow struct {
	Fields []orderedField `json:"fields"`
}

type orderedTable struct {
	Name string       `json:"name"`
	Rows []orderedRow `json:"rows"`
}

type orderedDataset struct {
	Tables []orderedTable `json:"tables"`
}

func LoadDirectory(dir string, opts LoadOptions) (*Registry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read snapshot directory: %w", err)
	}

	specs := make(map[string]Spec)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".toml" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read snapshot file %s: %w", entry.Name(), err)
		}
		var spec Spec
		if err := toml.Unmarshal(data, &spec); err != nil {
			return nil, fmt.Errorf("decode snapshot %s: %w", entry.Name(), err)
		}
		if err := validateSpec(spec); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSpec, err)
		}
		if !filepath.IsAbs(spec.Source.Fixture) {
			spec.Source.Fixture = filepath.Join(dir, spec.Source.Fixture)
		}
		if _, exists := specs[spec.Name]; exists {
			return nil, fmt.Errorf("%w: duplicate snapshot %q", ErrInvalidSpec, spec.Name)
		}
		specs[spec.Name] = spec
	}

	if opts.ArtifactPublisher == nil {
		opts.ArtifactPublisher = NewInMemoryArtifactPublisher()
	}
	if opts.MetadataPublisher == nil {
		opts.MetadataPublisher = NewNoopMetadataPublisher()
	}

	return &Registry{
		specs:    specs,
		artifact: opts.ArtifactPublisher,
		metadata: opts.MetadataPublisher,
	}, nil
}

func validateSpec(spec Spec) error {
	if strings.TrimSpace(spec.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(spec.Source.Engine) == "" {
		return errors.New("source.engine is required")
	}
	if strings.TrimSpace(spec.Source.Fixture) == "" {
		return errors.New("source.fixture is required")
	}
	return nil
}

func (r *Registry) Plan(ctx context.Context, name string) (PlanReport, error) {
	spec, err := r.getSpec(name)
	if err != nil {
		return PlanReport{}, err
	}

	report := PlanReport{
		SnapshotName: spec.Name,
		Description:  spec.Description,
		Engine:       spec.Source.Engine,
		FixturePath:  spec.Source.Fixture,
		Stripping:    summariseStrip(spec.Strip),
		Masking:      summariseMask(spec.Mask),
		Synthetic:    summariseSynthetic(spec.Synthetic),
		Highlights:   buildHighlights(spec),
	}
	return report, nil
}

func (r *Registry) Capture(ctx context.Context, name string, opts CaptureOptions) (CaptureResult, error) {
	if strings.TrimSpace(opts.Tenant) == "" {
		return CaptureResult{}, errors.New("tenant is required")
	}
	if strings.TrimSpace(opts.TicketID) == "" {
		return CaptureResult{}, errors.New("ticket id is required")
	}

	spec, err := r.getSpec(name)
	if err != nil {
		return CaptureResult{}, err
	}

	data, err := loadFixture(spec.Source.Fixture)
	if err != nil {
		return CaptureResult{}, err
	}

	diff := DiffSummary{
		StrippedColumns:  make(map[string][]string),
		MaskedColumns:    make(map[string][]string),
		SyntheticColumns: make(map[string][]string),
	}

	if err := applyStrip(data, spec.Strip, diff); err != nil {
		return CaptureResult{}, err
	}
	if err := applyMask(data, spec.Mask, diff); err != nil {
		return CaptureResult{}, err
	}
	if err := applySynthetic(data, spec.Synthetic, diff); err != nil {
		return CaptureResult{}, err
	}

	encoded, err := encodeDataset(data)
	if err != nil {
		return CaptureResult{}, err
	}
	sum := sha256.Sum256(encoded)
	fingerprint := fmt.Sprintf("%x", sum[:])

	cid, err := r.artifact.Publish(ctx, encoded)
	if err != nil {
		return CaptureResult{}, err
	}

	metadata := SnapshotMetadata{
		SnapshotName: spec.Name,
		Description:  spec.Description,
		Tenant:       opts.Tenant,
		TicketID:     opts.TicketID,
		Engine:       spec.Source.Engine,
		DSN:          spec.Source.DSN,
		Fingerprint:  fingerprint,
		ArtifactCID:  cid,
		CapturedAt:   nowFunc(),
		RuleCounts: RuleCounts{
			Strip:     len(spec.Strip),
			Mask:      len(spec.Mask),
			Synthetic: len(spec.Synthetic),
		},
	}

	if err := r.metadata.Publish(ctx, metadata); err != nil {
		return CaptureResult{}, err
	}

	return CaptureResult{
		ArtifactCID: cid,
		Fingerprint: fingerprint,
		Metadata:    metadata,
		Diff:        diff,
		Payload:     encoded,
	}, nil
}

func (r *Registry) getSpec(name string) (Spec, error) {
	if r == nil {
		return Spec{}, ErrSnapshotNotFound
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return Spec{}, ErrSnapshotNotFound
	}
	spec, ok := r.specs[trimmed]
	if !ok {
		return Spec{}, ErrSnapshotNotFound
	}
	return spec, nil
}

func summariseStrip(rules []StripRule) RuleSummary {
	summary := RuleSummary{Tables: make(map[string]int)}
	summary.Total = len(rules)
	for _, rule := range rules {
		summary.Tables[rule.Table]++
	}
	return summary
}

func summariseMask(rules []MaskRule) RuleSummary {
	summary := RuleSummary{Tables: make(map[string]int)}
	summary.Total = len(rules)
	for _, rule := range rules {
		summary.Tables[rule.Table]++
	}
	return summary
}

func summariseSynthetic(rules []SyntheticRule) RuleSummary {
	summary := RuleSummary{Tables: make(map[string]int)}
	summary.Total = len(rules)
	for _, rule := range rules {
		summary.Tables[rule.Table]++
	}
	return summary
}

func buildHighlights(spec Spec) []string {
	highlights := make([]string, 0, len(spec.Strip)+len(spec.Mask)+len(spec.Synthetic))
	for _, rule := range spec.Strip {
		for _, column := range rule.Columns {
			highlights = append(highlights, fmt.Sprintf("strip %s.%s", rule.Table, column))
		}
	}
	for _, rule := range spec.Mask {
		highlights = append(highlights, fmt.Sprintf("mask %s.%s -> %s", rule.Table, rule.Column, rule.Strategy))
	}
	for _, rule := range spec.Synthetic {
		highlights = append(highlights, fmt.Sprintf("synth %s.%s -> %s", rule.Table, rule.Column, rule.Strategy))
	}
	sort.Strings(highlights)
	return highlights
}

func loadFixture(path string) (dataset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixture %s: %w", path, err)
	}
	var raw map[string][]map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode fixture %s: %w", path, err)
	}
	result := make(dataset, len(raw))
	for table, rows := range raw {
		converted := make([]row, 0, len(rows))
		for _, r := range rows {
			out := make(row, len(r))
			for key, value := range r {
				out[key] = fmt.Sprint(value)
			}
			converted = append(converted, out)
		}
		result[table] = converted
	}
	return result, nil
}

func applyStrip(data dataset, rules []StripRule, diff DiffSummary) error {
	for _, rule := range rules {
		rows, ok := data[rule.Table]
		if !ok {
			return fmt.Errorf("%w: table %s missing for strip", ErrInvalidRule, rule.Table)
		}
		touched := make(map[string]struct{})
		for i := range rows {
			for _, column := range rule.Columns {
				if _, exists := rows[i][column]; exists {
					delete(rows[i], column)
					touched[column] = struct{}{}
				}
			}
		}
		if len(touched) == 0 {
			return fmt.Errorf("%w: strip columns not found on table %s", ErrInvalidRule, rule.Table)
		}
		diff.StrippedColumns[rule.Table] = mergeAndSort(diff.StrippedColumns[rule.Table], touched)
	}
	return nil
}

func applyMask(data dataset, rules []MaskRule, diff DiffSummary) error {
	for _, rule := range rules {
		rows, ok := data[rule.Table]
		if !ok {
			return fmt.Errorf("%w: table %s missing for mask", ErrInvalidRule, rule.Table)
		}
		found := false
		for idx := range rows {
			value, exists := rows[idx][rule.Column]
			if !exists {
				continue
			}
			masked, err := applyMaskStrategy(rule.Strategy, value, rule.Table, idx)
			if err != nil {
				return err
			}
			rows[idx][rule.Column] = masked
			found = true
		}
		if !found {
			return fmt.Errorf("%w: column %s missing for mask", ErrInvalidRule, rule.Column)
		}
		diff.MaskedColumns[rule.Table] = mergeAndSort(diff.MaskedColumns[rule.Table], map[string]struct{}{rule.Column: struct{}{}})
	}
	return nil
}

func applyMaskStrategy(strategy, value, table string, rowIndex int) (string, error) {
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "hash":
		sum := sha256.Sum256([]byte(value))
		return fmt.Sprintf("hash-%x", sum[:8]), nil
	case "redact":
		return "REDACTED", nil
	case "last4":
		return maskLast4(value), nil
	default:
		return "", fmt.Errorf("%w: mask strategy %s for table %s", ErrInvalidRule, strategy, table)
	}
}

func maskLast4(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "last4-"
	}
	runes := []rune(trimmed)
	if len(runes) > 4 {
		runes = runes[len(runes)-4:]
	}
	return "last4-" + string(runes)
}

func applySynthetic(data dataset, rules []SyntheticRule, diff DiffSummary) error {
	for _, rule := range rules {
		rows, ok := data[rule.Table]
		if !ok {
			return fmt.Errorf("%w: table %s missing for synthetic", ErrInvalidRule, rule.Table)
		}
		switch strings.ToLower(strings.TrimSpace(rule.Strategy)) {
		case "uuid":
			for idx := range rows {
				rows[idx][rule.Column] = fmt.Sprintf("uuid-%s-%d", rule.Table, idx+1)
			}
		case "static":
			for idx := range rows {
				rows[idx][rule.Column] = "STATIC"
			}
		default:
			return fmt.Errorf("%w: synthetic strategy %s for table %s", ErrInvalidRule, rule.Strategy, rule.Table)
		}
		diff.SyntheticColumns[rule.Table] = mergeAndSort(diff.SyntheticColumns[rule.Table], map[string]struct{}{rule.Column: struct{}{}})
	}
	return nil
}

func mergeAndSort(existing []string, additions map[string]struct{}) []string {
	seen := make(map[string]struct{}, len(existing)+len(additions))
	for _, value := range existing {
		seen[value] = struct{}{}
	}
	for value := range additions {
		seen[value] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func encodeDataset(data dataset) ([]byte, error) {
	tableNames := make([]string, 0, len(data))
	for table := range data {
		tableNames = append(tableNames, table)
	}
	sort.Strings(tableNames)

	ordered := orderedDataset{Tables: make([]orderedTable, 0, len(tableNames))}
	for _, name := range tableNames {
		rows := data[name]
		orderedRows := make([]orderedRow, 0, len(rows))
		for _, r := range rows {
			fields := make([]orderedField, 0, len(r))
			keys := make([]string, 0, len(r))
			for key := range r {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				fields = append(fields, orderedField{Name: key, Value: r[key]})
			}
			orderedRows = append(orderedRows, orderedRow{Fields: fields})
		}
		ordered.Tables = append(ordered.Tables, orderedTable{Name: name, Rows: orderedRows})
	}

	dataBytes, err := json.Marshal(ordered)
	if err != nil {
		return nil, fmt.Errorf("encode dataset: %w", err)
	}
	return dataBytes, nil
}

type inMemoryArtifactPublisher struct{}

func NewInMemoryArtifactPublisher() ArtifactPublisher {
	return &inMemoryArtifactPublisher{}
}

func (p *inMemoryArtifactPublisher) Publish(ctx context.Context, data []byte) (string, error) {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("ipfs:%x", sum[:8]), nil
}

type noopMetadataPublisher struct{}

func NewNoopMetadataPublisher() MetadataPublisher {
	return &noopMetadataPublisher{}
}

func (n *noopMetadataPublisher) Publish(ctx context.Context, meta SnapshotMetadata) error {
	return nil
}
