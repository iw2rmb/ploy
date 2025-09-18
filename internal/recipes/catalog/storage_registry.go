package catalog

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

func (r *StorageBackedRegistry) List(ctx context.Context, f Filters) ([]Recipe, error) {
	if r.storage == nil {
		return []Recipe{}, nil
	}
	rc, err := r.storage.Get(ctx, catalogKey)
	if err != nil {
		// If missing or error, return empty list (non-fatal for listing)
		return []Recipe{}, nil
	}
	defer func() { _ = rc.Close() }()
	var metas []CatalogEntry
	if err := json.NewDecoder(rc).Decode(&metas); err != nil {
		return []Recipe{}, nil
	}
	out := make([]Recipe, 0, len(metas))
	for _, m := range metas {
		// Filter by language if specified (use tags as language hints)
		if f.Language != "" {
			langMatch := false
			for _, t := range m.Tags {
				if t == f.Language {
					langMatch = true
					break
				}
			}
			if !langMatch {
				continue
			}
		}
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
		// Fill language best-effort from tags
		lang := ""
		for _, t := range m.Tags {
			switch t {
			case "java", "kotlin", "scala", "go", "node", "javascript", "typescript", "python":
				lang = t
			}
		}
		out = append(out, Recipe{
			ID:          m.ID,
			Name:        name,
			Language:    lang,
			Description: m.Description,
			Pack:        m.Pack,
			Version:     m.Version,
			Tags:        m.Tags,
		})
	}
	return out, nil
}

func (r *StorageBackedRegistry) Get(ctx context.Context, id string) (*Recipe, error) {
	if r.storage == nil {
		return nil, nil
	}
	rc, err := r.storage.Get(ctx, catalogKey)
	if err != nil {
		return nil, nil
	}
	defer func() { _ = rc.Close() }()
	var metas []CatalogEntry
	if err := json.NewDecoder(rc).Decode(&metas); err != nil {
		return nil, nil
	}
	for _, m := range metas {
		if m.ID == id {
			name := m.DisplayName
			if name == "" {
				name = m.ID
			}
			// derive language from tags
			lang := ""
			for _, t := range m.Tags {
				switch t {
				case "java", "kotlin", "scala", "go", "node", "javascript", "typescript", "python":
					lang = t
				}
			}
			rec := &Recipe{
				ID:          m.ID,
				Name:        name,
				Language:    lang,
				Description: m.Description,
				Pack:        m.Pack,
				Version:     m.Version,
				Tags:        m.Tags,
			}
			return rec, nil
		}
	}
	return nil, nil
}
