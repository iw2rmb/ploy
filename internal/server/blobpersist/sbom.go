package blobpersist

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/sbom"
	"github.com/iw2rmb/ploy/internal/store"
)

// ExtractSBOMRowsForJob reads persisted artifact bundles for a specific
// (run_id, job_id), parses supported SBOM documents from those bundles, and
// returns normalized package rows tagged with job/repo provenance.
func (s *Service) ExtractSBOMRowsForJob(
	ctx context.Context,
	runID types.RunID,
	jobID types.JobID,
	repoID types.RepoID,
) ([]sbom.Row, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}

	bundles, err := s.store.ListArtifactBundlesByRunAndJob(ctx, store.ListArtifactBundlesByRunAndJobParams{
		RunID: runID,
		JobID: &jobID,
	})
	if err != nil {
		return nil, fmt.Errorf("list artifact bundles: %w", err)
	}

	seen := map[string]struct{}{}
	rows := make([]sbom.Row, 0)
	for _, bundle := range bundles {
		if bundle.ObjectKey == nil || strings.TrimSpace(*bundle.ObjectKey) == "" {
			continue
		}
		rc, _, getErr := s.blobstore.Get(ctx, *bundle.ObjectKey)
		if getErr != nil {
			return nil, fmt.Errorf("get artifact bundle %q: %w", *bundle.ObjectKey, getErr)
		}

		raw, readErr := io.ReadAll(rc)
		_ = rc.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read artifact bundle %q: %w", *bundle.ObjectKey, readErr)
		}

		parsedRows, parseErr := sbom.ExtractRowsFromBundle(raw, jobID, repoID)
		if parseErr != nil {
			return nil, fmt.Errorf("parse sbom bundle %q: %w", *bundle.ObjectKey, parseErr)
		}
		for _, row := range parsedRows {
			key := row.Lib + "\x00" + row.Ver
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			rows = append(rows, row)
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Lib == rows[j].Lib {
			return rows[i].Ver < rows[j].Ver
		}
		return rows[i].Lib < rows[j].Lib
	})
	return rows, nil
}
