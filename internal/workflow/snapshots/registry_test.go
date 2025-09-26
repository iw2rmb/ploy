package snapshots

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type recordingArtifactPublisher struct {
	payload []byte
	cid     string
	err     error
}

func (r *recordingArtifactPublisher) Publish(ctx context.Context, data []byte) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	r.payload = append([]byte(nil), data...)
	if r.cid == "" {
		r.cid = "cid-test"
	}
	return r.cid, nil
}

type recordingMetadataPublisher struct {
	metadata SnapshotMetadata
	calls    int
	err      error
}

func (r *recordingMetadataPublisher) Publish(ctx context.Context, meta SnapshotMetadata) error {
	r.calls++
	r.metadata = meta
	return r.err
}

func TestPlanSummarisesRules(t *testing.T) {
	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "dev-db.json")
	fixture := map[string][]map[string]string{
		"users": {
			{"id": "1", "email": "alice@example.com", "ssn": "123-45-6789"},
			{"id": "2", "email": "bob@example.com", "ssn": "987-65-4321"},
		},
	}
	writeJSON(t, fixturePath, fixture)

	specPath := filepath.Join(dir, "dev-db.toml")
	writeFile(t, specPath, `name = "dev-db"
description = "Development database"
[source]
engine = "postgres"
dsn = "postgres://dev"
fixture = "`+fixturePath+`"

[[strip]]
table = "users"
columns = ["ssn"]

[[mask]]
table = "users"
column = "email"
strategy = "hash"

[[synthetic]]
table = "users"
column = "token"
strategy = "uuid"
`)

	registry, err := LoadDirectory(dir, LoadOptions{})
	if err != nil {
		t.Fatalf("load directory: %v", err)
	}

	report, err := registry.Plan(context.Background(), "dev-db")
	if err != nil {
		t.Fatalf("plan dev-db: %v", err)
	}

	if report.SnapshotName != "dev-db" {
		t.Fatalf("expected snapshot name dev-db, got %s", report.SnapshotName)
	}
	if report.Engine != "postgres" {
		t.Fatalf("expected engine postgres, got %s", report.Engine)
	}
	if report.Stripping.Total != 1 || report.Masking.Total != 1 || report.Synthetic.Total != 1 {
		t.Fatalf("unexpected rule totals: %+v", report)
	}
	if got := report.Masking.Tables["users"]; got != 1 {
		t.Fatalf("expected masking count 1 for users, got %d", got)
	}
	if len(report.Highlights) == 0 {
		t.Fatalf("expected highlights, got none")
	}
}

func TestCaptureAppliesRulesPublishesArtifactAndMetadata(t *testing.T) {
	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "dev-db.json")
	fixture := map[string][]map[string]string{
		"users": {
			{"id": "1", "email": "alice@example.com", "ssn": "123-45-6789"},
			{"id": "2", "email": "bob@example.com", "ssn": "987-65-4321"},
		},
	}
	writeJSON(t, fixturePath, fixture)

	specPath := filepath.Join(dir, "dev-db.toml")
	writeFile(t, specPath, `name = "dev-db"
description = "Development database"
[source]
engine = "postgres"
dsn = "postgres://dev"
fixture = "`+fixturePath+`"

[[strip]]
table = "users"
columns = ["ssn"]

[[mask]]
table = "users"
column = "email"
strategy = "hash"

[[synthetic]]
table = "users"
column = "token"
strategy = "uuid"
`)

	artifact := &recordingArtifactPublisher{cid: "cid-dev"}
	metadata := &recordingMetadataPublisher{}
	fixedTime := time.Date(2025, 9, 26, 12, 0, 0, 0, time.UTC)
	prevNow := nowFunc
	nowFunc = func() time.Time { return fixedTime }
	defer func() { nowFunc = prevNow }()

	registry, err := LoadDirectory(dir, LoadOptions{
		ArtifactPublisher: artifact,
		MetadataPublisher: metadata,
	})
	if err != nil {
		t.Fatalf("load directory: %v", err)
	}

	result, err := registry.Capture(context.Background(), "dev-db", CaptureOptions{
		Tenant:   "acme",
		TicketID: "ticket-42",
	})
	if err != nil {
		t.Fatalf("capture dev-db: %v", err)
	}

	if result.ArtifactCID != "cid-dev" {
		t.Fatalf("expected cid-dev, got %s", result.ArtifactCID)
	}
	if result.Fingerprint == "" {
		t.Fatal("expected fingerprint to be set")
	}
	if result.Metadata.Tenant != "acme" || result.Metadata.TicketID != "ticket-42" {
		t.Fatalf("unexpected metadata tenant/ticket: %+v", result.Metadata)
	}
	if metadata.calls != 1 {
		t.Fatalf("expected metadata publisher to be called once, got %d", metadata.calls)
	}

	var captured struct {
		Tables []struct {
			Name string `json:"name"`
			Rows []struct {
				Fields []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"fields"`
			} `json:"rows"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(artifact.payload, &captured); err != nil {
		t.Fatalf("unmarshal captured dataset: %v", err)
	}
	if len(captured.Tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(captured.Tables))
	}
	for _, row := range captured.Tables[0].Rows {
		for _, field := range row.Fields {
			if field.Name == "ssn" {
				t.Fatalf("expected ssn column stripped: %+v", captured)
			}
			if field.Name == "email" && field.Value == "alice@example.com" {
				t.Fatalf("expected email to be masked")
			}
			if field.Name == "token" && !startsWith(field.Value, "uuid-") {
				t.Fatalf("expected synthetic token, got %s", field.Value)
			}
		}
	}
}

func TestCaptureErrorsForUnknownSnapshot(t *testing.T) {
	registry := &Registry{specs: map[string]Spec{}}
	_, err := registry.Capture(context.Background(), "missing", CaptureOptions{Tenant: "acme", TicketID: "t"})
	if err == nil {
		t.Fatal("expected error for unknown snapshot")
	}
}

func TestMaskingStrategyValidation(t *testing.T) {
	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "dev-db.json")
	fixture := map[string][]map[string]string{"users": {
		{"id": "1", "email": "a"},
	}}
	writeJSON(t, fixturePath, fixture)
	specPath := filepath.Join(dir, "dev-db.toml")
	writeFile(t, specPath, `name = "dev-db"
[source]
engine = "postgres"
dsn = "postgres://dev"
fixture = "`+fixturePath+`"

[[mask]]
table = "users"
column = "email"
strategy = "unknown"
`)

	registry, err := LoadDirectory(dir, LoadOptions{})
	if err != nil {
		t.Fatalf("load directory: %v", err)
	}
	_, err = registry.Capture(context.Background(), "dev-db", CaptureOptions{Tenant: "acme", TicketID: "t"})
	if err == nil {
		t.Fatal("expected error for unknown mask strategy")
	}
	if !errors.Is(err, ErrInvalidRule) {
		t.Fatalf("expected ErrInvalidRule, got %v", err)
	}
}

func TestCaptureSupportsRedactMaskAndStaticSynthetic(t *testing.T) {
	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "fixture.json")
	fixture := map[string][]map[string]string{
		"users": {
			{"id": "1", "email": "alice@example.com"},
		},
		"orders": {
			{"id": "o-1", "user_id": "1"},
		},
	}
	writeJSON(t, fixturePath, fixture)

	specPath := filepath.Join(dir, "snapshot.toml")
	writeFile(t, specPath, `name = "redact-static"
[source]
engine = "postgres"
dsn = "postgres://dev"
fixture = "`+fixturePath+`"

[[mask]]
table = "users"
column = "email"
strategy = "redact"

[[synthetic]]
table = "orders"
column = "tracking_code"
strategy = "static"
`)

	registry, err := LoadDirectory(dir, LoadOptions{})
	if err != nil {
		t.Fatalf("load directory: %v", err)
	}

	result, err := registry.Capture(context.Background(), "redact-static", CaptureOptions{Tenant: "acme", TicketID: "ticket"})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	var payload orderedDataset
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	for _, table := range payload.Tables {
		switch table.Name {
		case "users":
			for _, row := range table.Rows {
				for _, field := range row.Fields {
					if field.Name == "email" && field.Value != "REDACTED" {
						t.Fatalf("expected email to be redacted, got %s", field.Value)
					}
				}
			}
		case "orders":
			for _, row := range table.Rows {
				for _, field := range row.Fields {
					if field.Name == "tracking_code" && field.Value != "STATIC" {
						t.Fatalf("expected static tracking code, got %s", field.Value)
					}
				}
			}
		}
	}
}

func TestCaptureStripMissingColumnFails(t *testing.T) {
	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "fixture.json")
	fixture := map[string][]map[string]string{"users": {
		{"id": "1"},
	}}
	writeJSON(t, fixturePath, fixture)
	specPath := filepath.Join(dir, "snapshot.toml")
	writeFile(t, specPath, `name = "strip-missing"
[source]
engine = "postgres"
dsn = "postgres://dev"
fixture = "`+fixturePath+`"

[[strip]]
table = "users"
columns = ["email"]
`)

	registry, err := LoadDirectory(dir, LoadOptions{})
	if err != nil {
		t.Fatalf("load directory: %v", err)
	}
	_, err = registry.Capture(context.Background(), "strip-missing", CaptureOptions{Tenant: "acme", TicketID: "ticket"})
	if err == nil {
		t.Fatal("expected error when strip columns missing")
	}
	if !errors.Is(err, ErrInvalidRule) {
		t.Fatalf("expected ErrInvalidRule, got %v", err)
	}
}

func TestLoadDirectoryResolvesRelativeFixture(t *testing.T) {
	dir := t.TempDir()
	fixtureName := "fixture.json"
	fixturePath := filepath.Join(dir, fixtureName)
	writeJSON(t, fixturePath, map[string][]map[string]string{"users": {
		{"id": "1"},
	}})
	specPath := filepath.Join(dir, "snapshot.toml")
	writeFile(t, specPath, `name = "relative"
[source]
engine = "postgres"
dsn = "postgres://dev"
fixture = "`+fixtureName+`"
`)

	registry, err := LoadDirectory(dir, LoadOptions{})
	if err != nil {
		t.Fatalf("load directory: %v", err)
	}

	report, err := registry.Plan(context.Background(), "relative")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	if !filepath.IsAbs(report.FixturePath) {
		t.Fatalf("expected absolute fixture path, got %s", report.FixturePath)
	}
}

func TestCaptureUsesDefaultPublishers(t *testing.T) {
	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "fixture.json")
	writeJSON(t, fixturePath, map[string][]map[string]string{"users": {
		{"id": "1", "email": "a"},
	}})
	specPath := filepath.Join(dir, "snapshot.toml")
	writeFile(t, specPath, `name = "defaults"
[source]
engine = "postgres"
dsn = "postgres://dev"
fixture = "`+fixturePath+`"

[[mask]]
table = "users"
column = "email"
strategy = "hash"
`)

	registry, err := LoadDirectory(dir, LoadOptions{})
	if err != nil {
		t.Fatalf("load directory: %v", err)
	}
	result, err := registry.Capture(context.Background(), "defaults", CaptureOptions{Tenant: "acme", TicketID: "ticket"})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if !startsWith(result.ArtifactCID, "ipfs:") {
		t.Fatalf("expected ipfs cid, got %s", result.ArtifactCID)
	}
	if result.Metadata.ArtifactCID != result.ArtifactCID {
		t.Fatalf("metadata cid mismatch: %s vs %s", result.Metadata.ArtifactCID, result.ArtifactCID)
	}
}

func TestValidateSpecRequiresFields(t *testing.T) {
	if err := validateSpec(Spec{}); err == nil {
		t.Fatal("expected error for missing name")
	}
	if err := validateSpec(Spec{Name: "demo"}); err == nil {
		t.Fatal("expected error for missing engine")
	}
	if err := validateSpec(Spec{Name: "demo", Source: Source{Engine: "postgres"}}); err == nil {
		t.Fatal("expected error for missing fixture")
	}
}

func TestLoadDirectoryRejectsDuplicateNames(t *testing.T) {
	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "fixture.json")
	writeJSON(t, fixturePath, map[string][]map[string]string{"users": {
		{"id": "1"},
	}})
	content := `name = "dup"
[source]
engine = "postgres"
dsn = "postgres://dev"
fixture = "` + fixturePath + `"
`
	writeFile(t, filepath.Join(dir, "a.toml"), content)
	writeFile(t, filepath.Join(dir, "b.toml"), content)

	if _, err := LoadDirectory(dir, LoadOptions{}); err == nil {
		t.Fatal("expected error for duplicate snapshot names")
	}
}

func TestPlanErrorsForUnknownSnapshot(t *testing.T) {
	registry := &Registry{specs: map[string]Spec{}}
	if _, err := registry.Plan(context.Background(), "missing"); err == nil {
		t.Fatal("expected error for missing snapshot")
	}
}

func TestCaptureRequiresTenantAndTicket(t *testing.T) {
	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "fixture.json")
	writeJSON(t, fixturePath, map[string][]map[string]string{"users": {
		{"id": "1", "email": "a"},
	}})
	specPath := filepath.Join(dir, "snapshot.toml")
	writeFile(t, specPath, `name = "missing-tenant"
[source]
engine = "postgres"
dsn = "postgres://dev"
fixture = "`+fixturePath+`"

[[mask]]
table = "users"
column = "email"
strategy = "hash"
`)
	registry, err := LoadDirectory(dir, LoadOptions{})
	if err != nil {
		t.Fatalf("load directory: %v", err)
	}
	if _, err := registry.Capture(context.Background(), "missing-tenant", CaptureOptions{TicketID: "ticket"}); err == nil {
		t.Fatal("expected error for missing tenant")
	}
	if _, err := registry.Capture(context.Background(), "missing-tenant", CaptureOptions{Tenant: "acme"}); err == nil {
		t.Fatal("expected error for missing ticket")
	}
}

func TestGetSpecHandlesEmptyAndNilRegistry(t *testing.T) {
	registry := &Registry{specs: map[string]Spec{}}
	if _, err := registry.getSpec(""); !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("expected ErrSnapshotNotFound, got %v", err)
	}
	var nilRegistry *Registry
	if _, err := nilRegistry.getSpec("anything"); !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("expected ErrSnapshotNotFound for nil registry, got %v", err)
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	writeFile(t, path, string(data))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func startsWith(value, prefix string) bool {
	return len(value) >= len(prefix) && value[:len(prefix)] == prefix
}
