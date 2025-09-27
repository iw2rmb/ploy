package knowledgebase

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrDuplicateIncident indicates an ingest attempted to add an incident whose ID already exists.
var ErrDuplicateIncident = errors.New("knowledge base incident already exists")

var (
	createTemp = os.CreateTemp
	renameFile = os.Rename
	chmodFile  = os.Chmod
)

// Catalog enumerates classified incidents used to provide Mods guidance.
type Catalog struct {
	SchemaVersion string     `json:"schema_version"`
	Incidents     []Incident `json:"incidents"`
}

// Incident records a single knowledge base incident with remediation payloads.
type Incident struct {
	ID              string           `json:"id"`
	Errors          []string         `json:"errors"`
	Recipes         []string         `json:"recipes"`
	Summary         string           `json:"summary"`
	HumanGate       bool             `json:"human_gate"`
	Playbooks       []string         `json:"playbooks"`
	Recommendations []Recommendation `json:"recommendations"`
}

// Recommendation captures a suggested remediation surfaced to the planner.
type Recommendation struct {
	Source      string   `json:"source"`
	Message     string   `json:"message"`
	Confidence  float64  `json:"confidence"`
	Recipes     []string `json:"recipes"`
	ArtifactCID string   `json:"artifact_cid"`
}

// LoadCatalogFile parses a JSON catalog from the provided filesystem path.
func LoadCatalogFile(path string) (Catalog, error) {
	if strings.TrimSpace(path) == "" {
		return Catalog{}, fmt.Errorf("catalog path required")
	}
	f, err := os.Open(path)
	if err != nil {
		return Catalog{}, fmt.Errorf("open catalog: %w", err)
	}
	catalog, loadErr := LoadCatalog(f)
	closeErr := f.Close()
	if loadErr != nil {
		return Catalog{}, loadErr
	}
	if closeErr != nil {
		return Catalog{}, fmt.Errorf("close catalog: %w", closeErr)
	}
	return catalog, nil
}

// LoadCatalog decodes a catalog from the supplied reader.
func LoadCatalog(r io.Reader) (Catalog, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	var catalog Catalog
	if err := decoder.Decode(&catalog); err != nil {
		return Catalog{}, fmt.Errorf("decode catalog: %w", err)
	}
	catalog.SchemaVersion = strings.TrimSpace(catalog.SchemaVersion)
	for i := range catalog.Incidents {
		catalog.Incidents[i] = normaliseIncident(catalog.Incidents[i])
	}
	return catalog, nil
}

func normaliseIncident(incident Incident) Incident {
	incident.ID = strings.TrimSpace(incident.ID)
	incident.Summary = strings.TrimSpace(incident.Summary)
	incident.Errors = trimStrings(incident.Errors)
	incident.Recipes = trimStrings(incident.Recipes)
	incident.Playbooks = trimStrings(incident.Playbooks)
	for i := range incident.Recommendations {
		rec := incident.Recommendations[i]
		rec.Source = strings.TrimSpace(rec.Source)
		rec.Message = strings.TrimSpace(rec.Message)
		rec.Recipes = trimStrings(rec.Recipes)
		rec.ArtifactCID = strings.TrimSpace(rec.ArtifactCID)
		if rec.Confidence < 0 {
			rec.Confidence = 0
		}
		if rec.Confidence > 1 {
			rec.Confidence = 1
		}
		incident.Recommendations[i] = rec
	}
	return incident
}

func trimStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	clean := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			clean = append(clean, trimmed)
		}
	}
	if len(clean) == 0 {
		return nil
	}
	return clean
}

// MergeCatalog combines the base catalog with incidents from the additions catalog.
func MergeCatalog(base Catalog, additions Catalog) (Catalog, error) {
	merged := Catalog{
		SchemaVersion: strings.TrimSpace(base.SchemaVersion),
		Incidents:     append([]Incident(nil), base.Incidents...),
	}
	if merged.SchemaVersion == "" {
		merged.SchemaVersion = strings.TrimSpace(additions.SchemaVersion)
	}
	existing := make(map[string]struct{}, len(base.Incidents))
	for _, incident := range base.Incidents {
		if incident.ID != "" {
			existing[incident.ID] = struct{}{}
		}
	}
	for _, incident := range additions.Incidents {
		normalised := normaliseIncident(incident)
		if strings.TrimSpace(normalised.ID) == "" {
			return Catalog{}, fmt.Errorf("incident id required")
		}
		if _, ok := existing[normalised.ID]; ok {
			return Catalog{}, fmt.Errorf("%w: %s", ErrDuplicateIncident, normalised.ID)
		}
		merged.Incidents = append(merged.Incidents, normalised)
		existing[normalised.ID] = struct{}{}
	}
	sort.SliceStable(merged.Incidents, func(i, j int) bool {
		return merged.Incidents[i].ID < merged.Incidents[j].ID
	})
	return merged, nil
}

// SaveCatalogFile writes the provided catalog to disk atomically.
func SaveCatalogFile(path string, catalog Catalog) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("catalog path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("ensure catalog directory: %w", err)
	}
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal catalog: %w", err)
	}
	tmp, err := createTemp(filepath.Dir(path), "catalog-*.json")
	if err != nil {
		return fmt.Errorf("create temp catalog: %w", err)
	}
	tmpPath := tmp.Name()
	writeErr := func(err error) error {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return writeErr(fmt.Errorf("write temp catalog: %w", err))
	}
	if err := tmp.Close(); err != nil {
		return writeErr(fmt.Errorf("close temp catalog: %w", err))
	}
	if err := chmodFile(tmpPath, 0o600); err != nil {
		return writeErr(fmt.Errorf("chmod temp catalog: %w", err))
	}
	if err := renameFile(tmpPath, path); err != nil {
		return writeErr(fmt.Errorf("replace catalog: %w", err))
	}
	return nil
}
