package snapshots

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

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

func TestBuiltInSnapshotsCapture(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(repoRoot(t), "configs", "snapshots")
	cases := []struct {
		name   string
		engine string
	}{
		{name: "dev-db", engine: "postgres"},
		{name: "commit-db", engine: "postgres"},
		{name: "commit-cache", engine: "redis"},
		{name: "mysql-orders", engine: "mysql"},
		{name: "doc-events", engine: "document"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			artifact := &recordingArtifactPublisher{cid: "cid-" + tc.name}
			metadata := &recordingMetadataPublisher{}
			registry, err := LoadDirectory(dir, LoadOptions{
				ArtifactPublisher: artifact,
				MetadataPublisher: metadata,
			})
			if err != nil {
				t.Fatalf("load directory: %v", err)
			}

			plan, err := registry.Plan(ctx, tc.name)
			if err != nil {
				t.Fatalf("plan snapshot %s: %v", tc.name, err)
			}
			if plan.Engine != tc.engine {
				t.Fatalf("expected engine %s, got %s", tc.engine, plan.Engine)
			}

			result, err := registry.Capture(ctx, tc.name, CaptureOptions{Tenant: "acme", TicketID: "ticket-123"})
			if err != nil {
				t.Fatalf("capture snapshot %s: %v", tc.name, err)
			}
			if result.Metadata.Engine != tc.engine {
				t.Fatalf("expected metadata engine %s, got %s", tc.engine, result.Metadata.Engine)
			}
			if result.ArtifactCID != artifact.cid {
				t.Fatalf("expected artifact cid %s, got %s", artifact.cid, result.ArtifactCID)
			}
			if metadata.calls == 0 {
				t.Fatalf("expected metadata publisher to be invoked")
			}
		})
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
