package aster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrBundleNotFound indicates that no bundle metadata was found for the
// requested stage/toggle pair.
var ErrBundleNotFound = errors.New("aster bundle not found")

// Request identifies a specific workflow stage/toggle combination whose bundle
// metadata should be retrieved.
type Request struct {
	Stage  string
	Toggle string
}

// Metadata captures the provenance for an Aster bundle that the runtime can use to
// select accelerator runtimes and cache entries.
type Metadata struct {
	Stage       string
	Toggle      string
	BundleID    string
	Digest      string
	ArtifactCID string
	Source      string
}

// Locator discovers bundle metadata for workflow stages.
type Locator interface {
	Locate(ctx context.Context, req Request) (Metadata, error)
}

// NewFilesystemLocator builds a Locator that reads bundle metadata from JSON
// files rooted at dir. The expected layout is `<dir>/<stage>/<toggle>.json`.
func NewFilesystemLocator(dir string) (Locator, error) {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return nil, errors.New("aster root directory is required")
	}
	info, err := os.Stat(trimmed)
	if err != nil {
		return nil, fmt.Errorf("inspect aster root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("aster root %s is not a directory", trimmed)
	}
	return &filesystemLocator{root: trimmed}, nil
}

type filesystemLocator struct {
	root string
}

type fileMetadata struct {
	BundleID    string `json:"bundle_id"`
	Digest      string `json:"digest"`
	ArtifactCID string `json:"artifact_cid"`
	Source      string `json:"source"`
}

// Locate loads bundle metadata for the requested stage/toggle pair. Stage and
// toggle names are trimmed, lower-cased, and joined to construct the expected
// metadata path.
func (l *filesystemLocator) Locate(ctx context.Context, req Request) (Metadata, error) {
	if l == nil {
		return Metadata{}, errors.New("locator unavailable")
	}
	if err := ctx.Err(); err != nil {
		return Metadata{}, err
	}
	stageName := sanitizeComponent(req.Stage)
	toggleName := sanitizeComponent(req.Toggle)
	if stageName == "" {
		return Metadata{}, errors.New("stage is required")
	}
	if toggleName == "" {
		return Metadata{}, errors.New("toggle is required")
	}
	if strings.Contains(stageName, "..") || strings.Contains(toggleName, "..") {
		return Metadata{}, errors.New("stage or toggle contains invalid traversal segment")
	}
	path := filepath.Join(l.root, stageName, toggleName+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Metadata{}, ErrBundleNotFound
		}
		return Metadata{}, fmt.Errorf("read aster metadata %s: %w", path, err)
	}
	var payload fileMetadata
	if err := json.Unmarshal(data, &payload); err != nil {
		return Metadata{}, fmt.Errorf("decode aster metadata %s: %w", path, err)
	}
	meta := Metadata{
		Stage:       strings.TrimSpace(req.Stage),
		Toggle:      strings.TrimSpace(req.Toggle),
		BundleID:    strings.TrimSpace(payload.BundleID),
		Digest:      strings.TrimSpace(payload.Digest),
		ArtifactCID: strings.TrimSpace(payload.ArtifactCID),
		Source:      strings.TrimSpace(payload.Source),
	}
	if meta.BundleID == "" {
		meta.BundleID = fmt.Sprintf("%s-%s", stageName, toggleName)
	}
	return meta, nil
}

func sanitizeComponent(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return strings.ToLower(trimmed)
}
