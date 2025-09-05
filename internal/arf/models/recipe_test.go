package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestUnmarshalRecipeSummaryShape(t *testing.T) {
	// Simulate the API list response shape used by internal/cli/arf tests
	payload := []byte(`{
        "recipes": [
          {
            "id": "test-recipe-1",
            "metadata": {
              "name": "Test Recipe 1",
              "description": "A test recipe",
              "author": "unit",
              "languages": ["java"],
              "categories": ["cleanup"],
              "tags": ["demo"]
            },
            "created_at": "2025-09-04T08:00:00Z",
            "steps": [ {"name": "s1", "type": "shell"} ]
          }
        ],
        "count": 1,
        "total": 1
    }`)

	var resp struct {
		Recipes []Recipe `json:"recipes"`
		Count   int      `json:"count"`
		Total   int      `json:"total"`
	}
	if err := json.Unmarshal(payload, &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(resp.Recipes) != 1 {
		t.Fatalf("expected 1 recipe, got %d", len(resp.Recipes))
	}
	r := resp.Recipes[0]
	if r.ID != "test-recipe-1" {
		t.Fatalf("id mismatch: %s", r.ID)
	}
	if r.Metadata.Name != "Test Recipe 1" || r.Metadata.Description == "" || r.Metadata.Author == "" {
		t.Fatalf("metadata not populated: %+v", r.Metadata)
	}
	if len(r.Steps) != 1 {
		t.Fatalf("expected steps length 1, got %d", len(r.Steps))
	}
	if r.CreatedAt.IsZero() {
		t.Fatalf("created_at not parsed")
	}
}

func (t Time) IsZero() bool {
	return time.Time(t).IsZero()
}
