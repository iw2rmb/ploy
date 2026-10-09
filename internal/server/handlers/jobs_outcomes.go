package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/iw2rmb/ploy/internal/blobstore"
	types "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/sbom"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

var errOutcomeUnavailable = errors.New("gate outcome unavailable")

func getJobOutcomeHandler(st store.Store, bs blobstore.Store, outcome string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseRequiredPathIDOrWriteError[types.JobID](w, r, "job_id")
		if !ok {
			return
		}
		job, ok := getJobOrFail(w, r, st, id, "get job outcome")
		if !ok {
			return
		}
		if !lifecycle.IsGateJobType(job.JobType) || (outcome == "sbom-diff" && job.JobType != types.JobTypePostGate) {
			writeHTTPError(w, http.StatusBadRequest, "outcome is not supported for this job type")
			return
		}
		var body []byte
		var err error
		filename := "grype.json"
		switch outcome {
		case "sbom":
			filename = "sbom.spdx.json"
			body, err = readGateOutcome(r.Context(), st, bs, job, outcome, filename)
		case "cves":
			body, err = readGateOutcome(r.Context(), st, bs, job, outcome, filename)
		case "sbom-diff":
			filename = "sbom-diff.json"
			body, err = jobSBOMDiff(r.Context(), st, bs, job)
		}
		if err != nil {
			if errors.Is(err, errOutcomeUnavailable) || errors.Is(err, blobstore.ErrNotFound) {
				writeHTTPError(w, http.StatusNotFound, "gate outcome unavailable")
				return
			}
			serverError(w, "get job outcome", "read outcome", err, "job_id", id.String())
			return
		}
		streamBlob(w, bytes.NewReader(body), int64(len(body)), id.String()+"-"+filename, "application/json")
	}
}

func readGateOutcome(ctx context.Context, st store.Store, bs blobstore.Store, job store.Job, name, filename string) ([]byte, error) {
	bundles, err := listArtifactBundlesByJob(ctx, st, job)
	if err != nil {
		return nil, err
	}
	// The store orders newest first; never fall back to an older execution.
	for _, bundle := range bundles {
		if bundle.Name == nil || *bundle.Name != name {
			continue
		}
		if bundle.ObjectKey == nil || bs == nil {
			return nil, errOutcomeUnavailable
		}
		rc, _, err := bs.Get(ctx, *bundle.ObjectKey)
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		zr, err := gzip.NewReader(rc)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		// Leave bounded space for tar metadata in addition to the JSON payload.
		tr := tar.NewReader(io.LimitReader(zr, migsapi.MaxGateOutcomeBytes+(1<<20)))
		header, err := tr.Next()
		if err != nil {
			return nil, fmt.Errorf("read gate outcome entry: %w", err)
		}
		if header.Name != filename || header.Typeflag != tar.TypeReg || header.Size > migsapi.MaxGateOutcomeBytes {
			return nil, fmt.Errorf("invalid gate outcome entry")
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		if !json.Valid(body) {
			return nil, fmt.Errorf("invalid gate outcome JSON")
		}
		return body, nil
	}
	return nil, errOutcomeUnavailable
}

func jobSBOMDiff(ctx context.Context, st store.Store, bs blobstore.Store, post store.Job) ([]byte, error) {
	jobs, err := st.ListJobsByRunAttempt(ctx, store.ListJobsByRunAttemptParams{RunID: post.RunID, Attempt: post.Attempt})
	if err != nil {
		return nil, err
	}
	var pre *store.Job
	for i := range jobs {
		if jobs[i].JobType == types.JobTypePreGate && jobs[i].RepoID == post.RepoID {
			if pre != nil {
				return nil, fmt.Errorf("ambiguous pre-gate baseline")
			}
			pre = &jobs[i]
		}
	}
	if pre == nil {
		return nil, errOutcomeUnavailable
	}
	before, err := gateSBOMPackages(ctx, st, bs, *pre)
	if err != nil {
		return nil, err
	}
	after, err := gateSBOMPackages(ctx, st, bs, post)
	if err != nil {
		return nil, err
	}
	return json.Marshal(migsapi.JobSBOMDiffResponse{JobID: post.ID, BaselineJobID: pre.ID, Packages: diffSBOMPackages(before, after)})
}

func gateSBOMPackages(ctx context.Context, st store.Store, bs blobstore.Store, job store.Job) ([]migsapi.RunSBOMPackage, error) {
	body, err := readGateOutcome(ctx, st, bs, job, "sbom", "sbom.spdx.json")
	if err != nil {
		return nil, err
	}
	rows, err := sbom.ExtractPackagesFromJSON(body)
	if err != nil {
		return nil, err
	}
	packages := make([]migsapi.RunSBOMPackage, 0, len(rows))
	for _, row := range rows {
		packages = append(packages, migsapi.RunSBOMPackage{Package: row.Name, Version: row.Version})
	}
	return packages, nil
}
