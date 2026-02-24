// mod_run_artifact.go isolates artifact download and fetching logic.
//
// This file contains downloadRunArtifacts which fetches run status
// and downloads referenced artifacts to disk. It generates deterministic
// filenames, streams bytes without in-memory buffering, and produces a
// manifest.json for artifact metadata. Artifact download is separated from
// execution flow to enable independent testing of HTTP fetching and retry.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

// downloadRunArtifacts fetches run status and downloads referenced artifacts into dir.
// It streams bytes to disk (no in‑memory buffering), produces deterministic filenames,
// and writes a manifest.json with stable, sorted entries for reproducible output.
func downloadRunArtifacts(ctx context.Context, base *url.URL, httpClient *http.Client, runID, dir string, out io.Writer) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create artifact dir %s: %w", dir, err)
	}
	// Fetch run status to retrieve artifact CIDs.
	statusURL, err := url.JoinPath(base.String(), "v1", "runs", url.PathEscape(strings.TrimSpace(runID)), "status")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return fmt.Errorf("build status request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch run status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return controlPlaneHTTPError(resp)
	}
	// Decode RunSummary directly — the server returns the canonical type (no wrapper).
	var summary modsapi.RunSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		if errors.Is(err, io.EOF) {
			// Treat empty body as an empty status (no artifacts).
			summary = modsapi.RunSummary{}
		} else {
			return fmt.Errorf("decode run status: %w", err)
		}
	}
	// Collect artifacts via control-plane HTTP endpoint lookups.
	// Note: keep in sync with mod_run_artifact_test.go manifest shape.
	type manifestItem struct {
		Stage  string `json:"stage"`
		Name   string `json:"name"`
		CID    string `json:"cid"`
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
		Path   string `json:"path"`
	}
	items := make([]manifestItem, 0)
	var downloaded int

	// Deterministic iteration: sort stages and artifact names for stable manifests.
	stageIDs := make([]domaintypes.JobID, 0, len(summary.Stages))
	for id := range summary.Stages {
		stageIDs = append(stageIDs, id)
	}
	sort.Slice(stageIDs, func(i, j int) bool { return stageIDs[i].String() < stageIDs[j].String() })
	for _, stageID := range stageIDs {
		st := summary.Stages[stageID]
		names := make([]string, 0, len(st.Artifacts))
		for n := range st.Artifacts {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, name := range names {
			cid := st.Artifacts[name]
			// Lookup artifact metadata by CID via /v1/artifacts?cid=<cid>.
			lookupURL, err := url.Parse(base.String())
			if err != nil {
				return err
			}
			lookupURL.Path, err = url.JoinPath(lookupURL.Path, "v1", "artifacts")
			if err != nil {
				return err
			}
			q := lookupURL.Query()
			q.Set("cid", cid)
			lookupURL.RawQuery = q.Encode()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, lookupURL.String(), nil)
			if err != nil {
				return err
			}
			lr, err := httpClient.Do(req)
			if err != nil {
				return err
			}
			var listing struct {
				Artifacts []struct {
					ID, CID, Digest, Name string
					Size                  int64
				} `json:"artifacts"`
			}
			if lr.StatusCode != http.StatusOK {
				_ = lr.Body.Close()
				return controlPlaneHTTPError(lr)
			}
			if err := json.NewDecoder(lr.Body).Decode(&listing); err != nil {
				_ = lr.Body.Close()
				return fmt.Errorf("decode artifact listing: %w", err)
			}
			_ = lr.Body.Close()
			if len(listing.Artifacts) == 0 {
				return fmt.Errorf("no artifact found for CID %s", cid)
			}
			art := listing.Artifacts[0]
			// Download artifact content via /v1/artifacts/:id?download=true.
			dlURL, err := url.Parse(base.String())
			if err != nil {
				return err
			}
			dlURL.Path, err = url.JoinPath(dlURL.Path, "v1", "artifacts", url.PathEscape(art.ID))
			if err != nil {
				return err
			}
			q2 := dlURL.Query()
			q2.Set("download", "true")
			dlURL.RawQuery = q2.Encode()
			dreq, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL.String(), nil)
			if err != nil {
				return err
			}
			dresp, err := httpClient.Do(dreq)
			if err != nil {
				return err
			}
			if dresp.StatusCode != http.StatusOK {
				_ = dresp.Body.Close()
				return controlPlaneHTTPError(dresp)
			}
			filename := buildArtifactFilename(stageID.String(), name, cid, art.Digest)
			path := filepath.Join(dir, filename)

			// Stream download to disk to avoid buffering large artifacts in memory.
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				_ = dresp.Body.Close()
				return fmt.Errorf("open artifact %s: %w", filename, err)
			}
			n, copyErr := io.Copy(f, dresp.Body)
			closeErr := f.Close()
			_ = dresp.Body.Close()
			if copyErr != nil {
				return fmt.Errorf("download artifact %s: %w", filename, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close artifact %s: %w", filename, closeErr)
			}
			items = append(items, manifestItem{Stage: stageID.String(), Name: name, CID: cid, Digest: art.Digest, Size: n, Path: path})
			downloaded++
		}
	}
	// Write manifest.json containing artifact metadata.
	manifestPath := filepath.Join(dir, "manifest.json")
	data, _ := json.MarshalIndent(struct {
		Artifacts []manifestItem `json:"artifacts"`
	}{Artifacts: items}, "", "  ")
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	_, _ = fmt.Fprintf(out, "Downloaded %d artifacts to %s\n", downloaded, dir)
	return nil
}

// buildArtifactFilename generates a safe filename for an artifact using stage, name, cid, and digest.
// The digest (if present) is truncated to 20 characters and prefixed to the filename.
func buildArtifactFilename(stage, name, cid, digest string) string {
	clean := func(s string) string {
		s = strings.TrimSpace(s)
		s = strings.ReplaceAll(s, "/", "_")
		s = strings.ReplaceAll(s, "\\", "_")
		s = strings.ReplaceAll(s, ":", "_")
		return s
	}
	stage = clean(stage)
	name = clean(name)
	cid = clean(cid)
	if d := strings.TrimSpace(digest); d != "" {
		d = strings.ReplaceAll(d, ":", "-")
		if len(d) > 20 {
			d = d[:20]
		}
		return fmt.Sprintf("%s_%s_%s.bin", d, stage, name)
	}
	return fmt.Sprintf("%s_%s_%s.bin", cid, stage, name)
}

// fetchMRURL loads the run status and extracts the MR URL from metadata when present.
// Returns empty string if the MR URL is not found or an error occurs.
func fetchMRURL(ctx context.Context, base *url.URL, httpClient *http.Client, runID string) (string, error) {
	statusURL, err := url.JoinPath(base.String(), "v1", "runs", url.PathEscape(strings.TrimSpace(runID)), "status")
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", controlPlaneHTTPError(resp)
	}
	// Decode RunSummary directly — the server returns the canonical type (no wrapper).
	var summary modsapi.RunSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return "", err
	}
	if summary.Metadata != nil {
		if v, ok := summary.Metadata["mr_url"]; ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v), nil
		}
	}
	return "", nil
}
