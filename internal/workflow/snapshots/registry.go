package snapshots

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"
)

var nowFunc = time.Now

// Plan generates a plan report that summarises how a snapshot will be captured.
func (r *Registry) Plan(ctx context.Context, name string) (PlanReport, error) {
	spec, err := r.getSpec(name)
	if err != nil {
		return PlanReport{}, err
	}

	report := PlanReport{
		SnapshotName: spec.Name,
		Description:  spec.Description,
		Engine:       spec.Source.Engine,
		FixturePath:  spec.Source.Fixture,
		Stripping:    summariseStrip(spec.Strip),
		Masking:      summariseMask(spec.Mask),
		Synthetic:    summariseSynthetic(spec.Synthetic),
		Highlights:   buildHighlights(spec),
	}
	return report, nil
}

// Capture executes the snapshot workflow and emits artifact metadata.
func (r *Registry) Capture(ctx context.Context, name string, opts CaptureOptions) (CaptureResult, error) {
	if strings.TrimSpace(opts.Tenant) == "" {
		return CaptureResult{}, errors.New("tenant is required")
	}
	if strings.TrimSpace(opts.TicketID) == "" {
		return CaptureResult{}, errors.New("ticket id is required")
	}

	spec, err := r.getSpec(name)
	if err != nil {
		return CaptureResult{}, err
	}

	data, err := loadFixture(spec.Source.Fixture)
	if err != nil {
		return CaptureResult{}, err
	}

	diff := DiffSummary{
		StrippedColumns:  make(map[string][]string),
		MaskedColumns:    make(map[string][]string),
		SyntheticColumns: make(map[string][]string),
	}

	if err := applyStrip(data, spec.Strip, diff); err != nil {
		return CaptureResult{}, err
	}
	if err := applyMask(data, spec.Mask, diff); err != nil {
		return CaptureResult{}, err
	}
	if err := applySynthetic(data, spec.Synthetic, diff); err != nil {
		return CaptureResult{}, err
	}

	encoded, err := encodeDataset(data)
	if err != nil {
		return CaptureResult{}, err
	}
	sum := sha256.Sum256(encoded)
	fingerprint := fmt.Sprintf("%x", sum[:])

	cid, err := r.artifact.Publish(ctx, encoded)
	if err != nil {
		return CaptureResult{}, err
	}

	metadata := SnapshotMetadata{
		SnapshotName: spec.Name,
		Description:  spec.Description,
		Tenant:       opts.Tenant,
		TicketID:     opts.TicketID,
		Engine:       spec.Source.Engine,
		DSN:          spec.Source.DSN,
		Fingerprint:  fingerprint,
		ArtifactCID:  cid,
		CapturedAt:   nowFunc(),
		RuleCounts: RuleCounts{
			Strip:     len(spec.Strip),
			Mask:      len(spec.Mask),
			Synthetic: len(spec.Synthetic),
		},
	}

	if err := r.metadata.Publish(ctx, metadata); err != nil {
		return CaptureResult{}, err
	}

	return CaptureResult{
		ArtifactCID: cid,
		Fingerprint: fingerprint,
		Metadata:    metadata,
		Diff:        diff,
		Payload:     encoded,
	}, nil
}

// getSpec retrieves a spec by name, performing basic validation on the request.
func (r *Registry) getSpec(name string) (Spec, error) {
	if r == nil {
		return Spec{}, ErrSnapshotNotFound
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return Spec{}, ErrSnapshotNotFound
	}
	spec, ok := r.specs[trimmed]
	if !ok {
		return Spec{}, ErrSnapshotNotFound
	}
	return spec, nil
}
