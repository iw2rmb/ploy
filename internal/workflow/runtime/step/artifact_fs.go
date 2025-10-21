package step

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FilesystemArtifactPublisherOptions configures the filesystem-backed artifact publisher.
type FilesystemArtifactPublisherOptions struct {
	// Root is the directory where artifacts are persisted. Defaults to the cache root when empty.
	Root string
}

// FilesystemArtifactPublisher stores artifacts on disk and issues deterministic content-addressed CIDs.
type FilesystemArtifactPublisher struct {
	root string
}

// NewFilesystemArtifactPublisher constructs a filesystem-backed artifact publisher.
func NewFilesystemArtifactPublisher(opts FilesystemArtifactPublisherOptions) (*FilesystemArtifactPublisher, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		defaultRoot, err := defaultArtifactRoot()
		if err != nil {
			return nil, err
		}
		root = defaultRoot
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("step: ensure artifact root: %w", err)
	}
	return &FilesystemArtifactPublisher{root: root}, nil
}

// Publish writes the artifact payload to the filesystem and returns a deterministic CID.
func (p *FilesystemArtifactPublisher) Publish(ctx context.Context, req ArtifactRequest) (PublishedArtifact, error) {
	_ = ctx
	if p == nil {
		return PublishedArtifact{}, errors.New("step: artifact publisher not configured")
	}

	payload, err := p.resolvePayload(req)
	if err != nil {
		return PublishedArtifact{}, err
	}
	defer func() {
		_ = payload.Close()
	}()

	sum := sha256.New()
	tempFile, err := os.CreateTemp(p.root, "ploy-artifact-*")
	if err != nil {
		return PublishedArtifact{}, fmt.Errorf("step: create artifact temp file: %w", err)
	}

	if _, err := io.Copy(io.MultiWriter(tempFile, sum), payload); err != nil {
		_ = tempFile.Close()
		return PublishedArtifact{}, fmt.Errorf("step: write artifact: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return PublishedArtifact{}, err
	}

	cid := fmt.Sprintf("ipfs:%x", sum.Sum(nil))
	finalPath := filepath.Join(p.root, string(req.Kind), cid)
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return PublishedArtifact{}, fmt.Errorf("step: prepare artifact directory: %w", err)
	}
	if err := os.Rename(tempFile.Name(), finalPath); err != nil {
		return PublishedArtifact{}, fmt.Errorf("step: move artifact: %w", err)
	}

	return PublishedArtifact{
		CID:  cid,
		Kind: req.Kind,
	}, nil
}

func (p *FilesystemArtifactPublisher) resolvePayload(req ArtifactRequest) (io.ReadCloser, error) {
	if strings.TrimSpace(req.Path) != "" {
		return os.Open(req.Path)
	}
	if len(req.Buffer) > 0 {
		return io.NopCloser(bytes.NewReader(req.Buffer)), nil
	}
	return nil, errors.New("step: artifact payload required")
}
