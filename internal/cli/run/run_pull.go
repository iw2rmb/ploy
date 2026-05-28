package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/common"
)

type ArtifactPullOptions struct {
	RunID         string
	ArtifactsPath string
	Output        io.Writer
}

func RunArtifactPull(ctx context.Context, opts ArtifactPullOptions) error {
	runID := strings.TrimSpace(opts.RunID)
	if runID == "" {
		return errors.New("run-id required")
	}
	out := opts.Output
	if out == nil {
		out = io.Discard
	}

	dir, err := resolveArtifactOutputDir(opts.ArtifactsPath)
	if err != nil {
		return err
	}
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	return DownloadRunArtifacts(ctx, base, httpClient, runID, dir, out)
}

func resolveArtifactOutputDir(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == osTempArtifactDirSentinel {
		dir, err := os.MkdirTemp("", "ploy-run-artifacts-*")
		if err != nil {
			return "", fmt.Errorf("create artifact temp dir: %w", err)
		}
		return dir, nil
	}
	return path, nil
}
