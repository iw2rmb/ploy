package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

// downloadTicketArtifacts fetches ticket status and downloads referenced artifacts into dir.
// It creates a manifest.json file listing all downloaded artifacts with their metadata.
func downloadTicketArtifacts(ctx context.Context, base *url.URL, httpClient *http.Client, ticketID, dir string, out io.Writer) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create artifact dir %s: %w", dir, err)
	}
	// Fetch ticket status to retrieve artifact CIDs.
	statusURL, err := url.JoinPath(base.String(), "v1", "mods", url.PathEscape(strings.TrimSpace(ticketID)))
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return fmt.Errorf("build status request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch ticket status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return controlPlaneHTTPError(resp)
	}
	var payload modsapi.TicketStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("decode ticket status: %w", err)
	}
	// Collect artifacts via control-plane HTTP endpoint lookups.
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
	for stageID, st := range payload.Ticket.Stages {
		for name, cid := range st.Artifacts {
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
			filename := buildArtifactFilename(stageID, name, cid, art.Digest)
			path := filepath.Join(dir, filename)
			data, _ := io.ReadAll(dresp.Body)
			_ = dresp.Body.Close()
			if err := os.WriteFile(path, data, 0o644); err != nil {
				return fmt.Errorf("write artifact %s: %w", filename, err)
			}
			items = append(items, manifestItem{Stage: stageID, Name: name, CID: cid, Digest: art.Digest, Size: int64(len(data)), Path: path})
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

// fetchMRURL loads the ticket status and extracts the MR URL from metadata when present.
// Returns empty string if the MR URL is not found or an error occurs.
func fetchMRURL(ctx context.Context, base *url.URL, httpClient *http.Client, ticketID string) (string, error) {
	statusURL, err := url.JoinPath(base.String(), "v1", "mods", url.PathEscape(strings.TrimSpace(ticketID)))
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
	var payload modsapi.TicketStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.Ticket.Metadata != nil {
		if v, ok := payload.Ticket.Metadata["mr_url"]; ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v), nil
		}
	}
	return "", nil
}
