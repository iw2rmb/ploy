package arf

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
	"time"
)

// PackSpec identifies an OpenRewrite recipe pack to index.
type PackSpec struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// RecipesFetcher abstracts obtaining a pack JAR by name/version.
type RecipesFetcher interface {
	Fetch(pack, version string) ([]byte, error)
}

// MetricsCollector interface for collecting metrics
type MetricsCollector interface {
	RecordDuration(name string, duration time.Duration)
	RecordCount(name string, count int)
}

// RecipesIndexer indexes OpenRewrite packs into a RecipesCatalog and persists a snapshot.
type RecipesIndexer struct {
	fetcher RecipesFetcher
	store   StorageService
	logger  io.Writer
	metrics MetricsCollector
}

func NewRecipesIndexer(fetcher RecipesFetcher, store StorageService) *RecipesIndexer {
	return &RecipesIndexer{fetcher: fetcher, store: store}
}

// SetLogger sets a custom logger for the indexer
func (i *RecipesIndexer) SetLogger(logger io.Writer) {
	i.logger = logger
}

// SetMetricsCollector sets a metrics collector for the indexer
func (i *RecipesIndexer) SetMetricsCollector(metrics MetricsCollector) {
	i.metrics = metrics
}

func (i *RecipesIndexer) logf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if i.logger != nil {
		fmt.Fprintln(i.logger, msg)
	} else {
		log.Println(msg)
	}
}

// Refresh downloads the specified packs, extracts META-INF/rewrite/*.yml entries,
// builds a catalog, and persists a JSON snapshot for fast bootstrap.
func (i *RecipesIndexer) Refresh(ctx context.Context, packs []PackSpec) (*RecipesCatalog, error) {
	startTime := time.Now()

	if i.fetcher == nil {
		return nil, fmt.Errorf("fetcher not configured")
	}
	cat := NewRecipesCatalog()
	recipeCount := 0
	for _, p := range packs {
		if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Version) == "" {
			continue
		}
		jar, err := i.fetcher.Fetch(p.Name, p.Version)
		if err != nil {
			return nil, fmt.Errorf("fetch %s:%s: %w", p.Name, p.Version, err)
		}
		yamls, err := extractRewriteYAMLs(bytes.NewReader(jar), int64(len(jar)))
		if err != nil {
			return nil, fmt.Errorf("parse %s:%s: %w", p.Name, p.Version, err)
		}
		if err := cat.BuildFromYAMLs(yamls, p.Name, p.Version); err != nil {
			return nil, err
		}
		// Count recipes added from this pack
		packRecipes := len(yamls)
		recipeCount += packRecipes
		i.logf("Indexed pack %s:%s with %d recipes", p.Name, p.Version, packRecipes)
	}

	// Calculate metrics
	indexTime := time.Since(startTime)

	// Log catalog metrics
	i.logf("Catalog indexing complete: catalog_size=%d, index_time=%v, pack_count=%d",
		recipeCount, indexTime, len(packs))

	// Record metrics if collector is available
	if i.metrics != nil {
		i.metrics.RecordDuration("catalog.refresh.duration", indexTime)
		i.metrics.RecordCount("catalog.recipe.count", recipeCount)
		i.metrics.RecordCount("catalog.pack.count", len(packs))
	}

	if i.store != nil {
		if data, err := cat.Serialize(); err == nil {
			// Persist to a known key for bootstrap
			_ = i.store.Put(ctx, "artifacts/openrewrite/catalog.json", data)
		}
	}
	return cat, nil
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
