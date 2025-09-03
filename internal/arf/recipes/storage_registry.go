package recipes

import (
    "context"
    "encoding/json"

    istorage "github.com/iw2rmb/ploy/internal/storage"
)

const catalogKey = "artifacts/openrewrite/catalog.json"

// StorageBackedRegistry reads a persisted catalog snapshot from unified storage.
type StorageBackedRegistry struct {
    storage istorage.Storage
}

func NewStorageBacked(st istorage.Storage) *StorageBackedRegistry {
    return &StorageBackedRegistry{storage: st}
}

func (r *StorageBackedRegistry) Ping(ctx context.Context) error { return nil }

// recipeMeta mirrors the persisted catalog schema from api/arf/recipes_catalog.go
type recipeMeta struct {
    ID          string   `json:"id"`
    DisplayName string   `json:"display_name"`
    Description string   `json:"description"`
    Tags        []string `json:"tags"`
    Pack        string   `json:"pack"`
    Version     string   `json:"version"`
}

func (r *StorageBackedRegistry) List(ctx context.Context, f Filters) ([]Recipe, error) {
    if r.storage == nil {
        return []Recipe{}, nil
    }
    rc, err := r.storage.Get(ctx, catalogKey)
    if err != nil {
        // If missing or error, return empty list (non-fatal for listing)
        return []Recipe{}, nil
    }
    defer rc.Close()
    var metas []recipeMeta
    if err := json.NewDecoder(rc).Decode(&metas); err != nil {
        return []Recipe{}, nil
    }
    out := make([]Recipe, 0, len(metas))
    for _, m := range metas {
        // Filter by tag if specified
        if f.Tag != "" {
            match := false
            for _, t := range m.Tags {
                if t == f.Tag {
                    match = true
                    break
                }
            }
            if !match {
                continue
            }
        }
        // Language filtering not available in snapshot yet; ignore for now
        name := m.DisplayName
        if name == "" {
            name = m.ID
        }
        out = append(out, Recipe{ID: m.ID, Name: name, Tags: m.Tags})
    }
    return out, nil
}
