package snapshots

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestIPFSGatewayPublisherUploadsArtifacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v0/add" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("pin") != "true" {
			t.Fatalf("expected pin=true query param")
		}
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("read multipart file: %v", err)
		}
		defer func() { _ = file.Close() }()
		body, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read file body: %v", err)
		}
		if string(body) != "artifact-data" {
			t.Fatalf("unexpected artifact body: %q", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"Hash":"bafyreexamplecid","Name":"artifact","Size":"12"}`)
	}))
	defer server.Close()

	publisher, err := NewIPFSGatewayPublisher(server.URL, IPFSGatewayOptions{Pin: true})
	if err != nil {
		t.Fatalf("new IPFS gateway publisher: %v", err)
	}
	cid, err := publisher.Publish(context.Background(), []byte("artifact-data"))
	if err != nil {
		t.Fatalf("publish artifact: %v", err)
	}
	if cid != "bafyreexamplecid" {
		t.Fatalf("expected cid bafyreexamplecid, got %s", cid)
	}
}

func TestIPFSGatewayPublisherRejectsNon200Responses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer server.Close()

	publisher, err := NewIPFSGatewayPublisher(server.URL, IPFSGatewayOptions{})
	if err != nil {
		t.Fatalf("new IPFS gateway publisher: %v", err)
	}
	_, err = publisher.Publish(context.Background(), []byte("artifact"))
	if err == nil || !strings.Contains(err.Error(), "unexpected status 502") {
		t.Fatalf("expected error for non-200 response, got %v", err)
	}
}

func TestNewIPFSGatewayPublisherValidatesEndpoint(t *testing.T) {
	if _, err := NewIPFSGatewayPublisher("", IPFSGatewayOptions{}); err == nil {
		t.Fatal("expected error for empty endpoint")
	}
	if _, err := NewIPFSGatewayPublisher("localhost:5001", IPFSGatewayOptions{}); err == nil {
		t.Fatal("expected error for endpoint missing scheme")
	}
}

func TestIPFSGatewayPublisherWithoutPinOmitsQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pin") != "" {
			t.Fatalf("expected pin query param to be omitted, got %q", r.URL.Query().Get("pin"))
		}
		_ = r.ParseMultipartForm(2 << 20)
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"Hash":"bafynominal","Name":"artifact","Size":"12"}`)
	}))
	defer server.Close()

	publisher, err := NewIPFSGatewayPublisher(server.URL, IPFSGatewayOptions{Pin: false})
	if err != nil {
		t.Fatalf("new IPFS gateway publisher: %v", err)
	}
	if _, err := publisher.Publish(context.Background(), []byte("data")); err != nil {
		t.Fatalf("unexpected publish error: %v", err)
	}
}

func TestIPFSGatewayPublisherRequiresPayload(t *testing.T) {
	publisher, err := NewIPFSGatewayPublisher("https://example.com", IPFSGatewayOptions{})
	if err != nil {
		t.Fatalf("new IPFS gateway publisher: %v", err)
	}
	if _, err := publisher.Publish(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "artifact payload empty") {
		t.Fatalf("expected error for empty payload, got %v", err)
	}
}

func TestIPFSGatewayPublisherSurfacesMissingCID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(2 << 20)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"Name":"artifact","Size":"12"}`)
	}))
	defer server.Close()

	publisher, err := NewIPFSGatewayPublisher(server.URL, IPFSGatewayOptions{Pin: true})
	if err != nil {
		t.Fatalf("new IPFS gateway publisher: %v", err)
	}
	_, err = publisher.Publish(context.Background(), []byte("data"))
	if err == nil || !strings.Contains(err.Error(), "response missing cid") {
		t.Fatalf("expected error for missing CID, got %v", err)
	}
}

func TestMaskLast4Variants(t *testing.T) {
	if got := maskLast4(""); got != "last4-" {
		t.Fatalf("expected last4- for empty string, got %s", got)
	}
	if got := maskLast4("1234"); got != "last4-1234" {
		t.Fatalf("expected last4-1234, got %s", got)
	}
	if got := maskLast4("  12345678  "); got != "last4-5678" {
		t.Fatalf("expected last4-5678, got %s", got)
	}
}

func TestApplySyntheticRejectsUnknownStrategy(t *testing.T) {
	data := dataset{"users": {row{"id": "1"}}}
	err := applySynthetic(data, []SyntheticRule{{Table: "users", Column: "token", Strategy: "unknown"}}, DiffSummary{SyntheticColumns: map[string][]string{}})
	if err == nil || !strings.Contains(err.Error(), "synthetic strategy") {
		t.Fatalf("expected error for unknown synthetic strategy, got %v", err)
	}
}

func TestIPFSGatewayPublisherUsesBackgroundWhenContextNil(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(2 << 20)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"Hash":"bafyctx","Name":"artifact"}`)
	}))
	defer server.Close()

	publisher, err := NewIPFSGatewayPublisher(server.URL, IPFSGatewayOptions{Pin: true})
	if err != nil {
		t.Fatalf("new IPFS gateway publisher: %v", err)
	}
	cid, err := publisher.Publish(nil, []byte("data")) //nolint:staticcheck // exercise nil context branch
	if err != nil || cid != "bafyctx" {
		t.Fatalf("expected cid bafyctx, got cid=%s err=%v", cid, err)
	}
}

func TestExtractCIDSupportsCidField(t *testing.T) {
	cid, err := extractCID([]byte(`{"Cid":"bafyCid"}`))
	if err != nil {
		t.Fatalf("extract cid: %v", err)
	}
	if cid != "bafyCid" {
		t.Fatalf("expected bafyCid, got %s", cid)
	}
}

func TestExtractCIDReadsFirstValidLine(t *testing.T) {
	cid, err := extractCID([]byte("\n{\"Name\":\"artifact\"}\n{\"Hash\":\"bafyline\"}\n"))
	if err != nil {
		t.Fatalf("extract cid: %v", err)
	}
	if cid != "bafyline" {
		t.Fatalf("expected bafyline, got %s", cid)
	}
}

func TestExtractCIDReportsEmptyPayload(t *testing.T) {
	if _, err := extractCID([]byte("   \n\t")); err == nil || !strings.Contains(err.Error(), "<empty>") {
		t.Fatalf("expected error noting empty payload, got %v", err)
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

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate go.mod from tests")
		}
		dir = parent
	}
}
