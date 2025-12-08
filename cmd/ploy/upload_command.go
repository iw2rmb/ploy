package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func handleUpload(args []string, stderr io.Writer) error {
	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("upload", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	// Flags
	// Flag help uses neutral "identifier" wording since run/build IDs are KSUID strings,
	// not UUIDs. Treat them as opaque string identifiers.
	runID := fs.String("run-id", "", "Run identifier to attach the artifact bundle to")
	buildID := fs.String("build-id", "", "Optional build identifier")
	name := fs.String("name", "", "Optional artifact name override (defaults to filename)")
	if err := fs.Parse(args); err != nil {
		printUploadUsage(stderr)
		return err
	}
	remaining := fs.Args()
	if len(remaining) == 0 {
		printUploadUsage(stderr)
		return errors.New("upload path required")
	}
	trimmedRun := strings.TrimSpace(*runID)
	if trimmedRun == "" {
		printUploadUsage(stderr)
		return errors.New("--run-id is required")
	}
	payloadPath := remaining[0]
	info, err := os.Stat(payloadPath)
	if err != nil {
		return fmt.Errorf("stat payload %s: %w", payloadPath, err)
	}
	// Prepare gzipped bundle according to server contract; enforce ≤1 MiB bundle size.
	gz, err := gzipFile(payloadPath)
	if err != nil {
		return err
	}
	if len(gz) > 1<<20 { // 1 MiB
		return fmt.Errorf("gzipped bundle exceeds 1 MiB cap: %d bytes", len(gz))
	}

	ctx := context.Background()
	baseURL, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	endpoint, err := url.JoinPath(baseURL.String(), "v1", "runs", trimmedRun, "artifact_bundles")
	if err != nil {
		return err
	}

	var reqBody struct {
		BuildID *string `json:"build_id,omitempty"`
		Name    *string `json:"name,omitempty"`
		Bundle  []byte  `json:"bundle"`
	}
	if v := strings.TrimSpace(*buildID); v != "" {
		reqBody.BuildID = &v
	}
	n := strings.TrimSpace(*name)
	if n == "" {
		n = filepath.Base(payloadPath)
	}
	reqBody.Name = &n
	reqBody.Bundle = gz

	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(&reqBody); err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		return controlPlaneHTTPError(resp)
	}
	// Best-effort parse of response to surface the created ID.
	var created struct {
		ArtifactBundleID string `json:"artifact_bundle_id"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&created)
	digest, _ := fileDigest(payloadPath)
	if created.ArtifactBundleID != "" {
		_, _ = fmt.Fprintf(stderr, "Uploaded %s (%d bytes raw, %d bytes gz) to run %s (artifact_bundle_id %s, digest %s)\n", filepath.Base(payloadPath), info.Size(), len(gz), trimmedRun, created.ArtifactBundleID, digest)
	} else {
		_, _ = fmt.Fprintf(stderr, "Uploaded %s (%d bytes raw, %d bytes gz) to run %s (digest %s)\n", filepath.Base(payloadPath), info.Size(), len(gz), trimmedRun, digest)
	}
	return nil
}

// printUploadUsage renders help for the upload command. Placeholders use neutral
// <run-id> / <build-id> wording since IDs are KSUID strings (not UUIDs).
func printUploadUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy upload --run-id <run-id> [--build-id <build-id>] [--name <string>] <path>")
}

// report command removed: server provides no GET route for artifact bundles.

func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func gzipFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	if _, err := io.Copy(gz, f); err != nil {
		_ = gz.Close()
		return nil, fmt.Errorf("gzip %s: %w", path, err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("gzip close %s: %w", path, err)
	}
	return b.Bytes(), nil
}
