package knowledgebase

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
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
