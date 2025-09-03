package arf

import (
    "archive/zip"
    "bytes"
    "context"
    "fmt"
    "io"
    "path/filepath"
    "strings"
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

// RecipesIndexer indexes OpenRewrite packs into a RecipesCatalog and persists a snapshot.
type RecipesIndexer struct {
    fetcher RecipesFetcher
    store   StorageService
}

func NewRecipesIndexer(fetcher RecipesFetcher, store StorageService) *RecipesIndexer {
    return &RecipesIndexer{fetcher: fetcher, store: store}
}

// Refresh downloads the specified packs, extracts META-INF/rewrite/*.yml entries,
// builds a catalog, and persists a JSON snapshot for fast bootstrap.
func (i *RecipesIndexer) Refresh(ctx context.Context, packs []PackSpec) (*RecipesCatalog, error) {
    if i.fetcher == nil {
        return nil, fmt.Errorf("fetcher not configured")
    }
    cat := NewRecipesCatalog()
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

