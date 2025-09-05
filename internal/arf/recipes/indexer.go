package recipes

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"time"

	istorage "github.com/iw2rmb/ploy/internal/storage"
)

// PackSpec identifies an OpenRewrite recipe pack to index.
type PackSpec struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Fetcher abstracts obtaining a pack JAR by name/version.
type Fetcher interface {
	Fetch(pack, version string) ([]byte, error)
}

// MetricsCollector for indexer metrics.
type MetricsCollector interface {
	RecordDuration(name string, d time.Duration)
	RecordCount(name string, n int)
}

// Indexer builds a catalog snapshot and persists it to unified storage.
type Indexer struct {
	fetcher Fetcher
	store   istorage.Storage
	metrics MetricsCollector
}

func NewIndexer(fetcher Fetcher, store istorage.Storage) *Indexer {
	return &Indexer{fetcher: fetcher, store: store}
}

func (i *Indexer) SetMetricsCollector(m MetricsCollector) { i.metrics = m }

// Refresh downloads packs, extracts META-INF/rewrite/*.yml entries, and writes catalog snapshot.
func (i *Indexer) Refresh(ctx context.Context, packs []PackSpec) ([]CatalogEntry, error) {
	start := time.Now()
	if i.fetcher == nil {
		return nil, nil
	}
	var entries []CatalogEntry
	for _, p := range packs {
		if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Version) == "" {
			continue
		}
		jar, err := i.fetcher.Fetch(p.Name, p.Version)
		if err != nil {
			return nil, err
		}
		yamls, err := extractRewriteYAMLs(bytes.NewReader(jar), int64(len(jar)))
		if err != nil {
			return nil, err
		}
		// Minimal: treat each YAML as a recipe entry with pack/version
		for _, _ = range yamls {
			entries = append(entries, CatalogEntry{
				ID:          "", // ID not parsed in minimal slice
				DisplayName: "",
				Description: "",
				Tags:        nil,
				Pack:        p.Name,
				Version:     p.Version,
			})
		}
	}
	// Persist snapshot (even if empty) for bootstrap compatibility
	if i.store != nil {
		data, _ := json.Marshal(entries)
		_ = i.store.Put(ctx, catalogKey, bytes.NewReader(data))
	}
	if i.metrics != nil {
		i.metrics.RecordDuration("catalog.refresh.duration", time.Since(start))
		i.metrics.RecordCount("catalog.recipe.count", len(entries))
		i.metrics.RecordCount("catalog.pack.count", len(packs))
	}
	return entries, nil
}

func extractRewriteYAMLs(r io.ReaderAt, size int64) ([][]byte, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, err
	}
	var out [][]byte
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, "META-INF/rewrite/") {
			continue
		}
		if ext := strings.ToLower(filepath.Ext(f.Name)); ext != ".yml" && ext != ".yaml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		if len(b) > 0 {
			out = append(out, b)
		}
	}
	return out, nil
}
