package nodeagent

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

const maxArtifactSize = 1 << 20 // 1 MiB

// sizeLimitedPublisher wraps an artifact publisher and enforces size caps on gzipped output.
type sizeLimitedPublisher struct {
	delegate step.ArtifactPublisher
	maxSize  int64
}

func newSizeLimitedPublisher(delegate step.ArtifactPublisher, maxSize int64) *sizeLimitedPublisher {
	return &sizeLimitedPublisher{delegate: delegate, maxSize: maxSize}
}

func (p *sizeLimitedPublisher) Publish(ctx context.Context, req step.ArtifactRequest) (step.PublishedArtifact, error) {
	// Compress and measure the artifact.
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)

	var reader io.Reader
	if strings.TrimSpace(req.Path) != "" {
		file, err := os.Open(req.Path)
		if err != nil {
			return step.PublishedArtifact{}, fmt.Errorf("open artifact: %w", err)
		}
		defer func() { _ = file.Close() }()
		reader = file
	} else if len(req.Buffer) > 0 {
		reader = bytes.NewReader(req.Buffer)
	} else {
		return step.PublishedArtifact{}, errors.New("artifact payload required")
	}

	if _, err := io.Copy(gzWriter, reader); err != nil {
		return step.PublishedArtifact{}, fmt.Errorf("compress artifact: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		return step.PublishedArtifact{}, fmt.Errorf("finalize compression: %w", err)
	}

	compressed := buf.Bytes()
	if int64(len(compressed)) > p.maxSize {
		return step.PublishedArtifact{}, fmt.Errorf("artifact exceeds size limit: %d > %d bytes (gzipped)", len(compressed), p.maxSize)
	}

	// Publish the compressed artifact.
	compressedReq := step.ArtifactRequest{Kind: req.Kind, Buffer: compressed}
	return p.delegate.Publish(ctx, compressedReq)
}
