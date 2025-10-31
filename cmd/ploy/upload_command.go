package main

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
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
	jobID := fs.String("job-id", "", "Job or ticket identifier associated with the payload")
	kind := fs.String("kind", "repo", "Transfer kind (repo|logs|report)")
    nodeOverride := fs.String("node-id", "", "Override node identifier for the transfer (optional)")
	if err := fs.Parse(args); err != nil {
		printUploadUsage(stderr)
		return err
	}
	remaining := fs.Args()
	if len(remaining) == 0 {
		printUploadUsage(stderr)
		return errors.New("upload path required")
	}
	trimmedJob := strings.TrimSpace(*jobID)
	if trimmedJob == "" {
		printUploadUsage(stderr)
		return errors.New("--job-id is required")
	}
	payloadPath := remaining[0]
	info, err := os.Stat(payloadPath)
	if err != nil {
		return fmt.Errorf("stat payload %s: %w", payloadPath, err)
	}
	digest, err := fileDigest(payloadPath)
	if err != nil {
		return err
	}
	ctx := context.Background()
    baseURL, httpClient, err := resolveControlPlaneHTTP(ctx)
    if err != nil {
        return err
    }
    // Stream payload directly to /v2/artifacts/upload
    endpoint, err := url.JoinPath(baseURL.String(), "v2", "artifacts", "upload")
    if err != nil { return err }
    f, err := os.Open(payloadPath)
    if err != nil { return fmt.Errorf("open payload: %w", err) }
    defer f.Close()
    q := url.Values{}
    q.Set("job_id", trimmedJob)
    if k := strings.TrimSpace(*kind); k != "" { q.Set("kind", k) }
    if n := strings.TrimSpace(*nodeOverride); n != "" { q.Set("node_id", n) }
    q.Set("digest", digest)
    u, err := url.Parse(endpoint)
    if err != nil { return err }
    u.RawQuery = q.Encode()
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), f)
    if err != nil { return err }
    req.Header.Set("Content-Type", "application/octet-stream")
    resp, err := httpClient.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusCreated {
        return controlPlaneHTTPError(resp)
    }
    fmt.Fprintf(stderr, "Uploaded %s (%d bytes) via HTTPS (digest %s)\n", filepath.Base(payloadPath), info.Size(), digest)
    return nil
}

func printUploadUsage(w io.Writer) {
    fmt.Fprintln(w, "Usage: ploy upload --job-id <id> [--kind <kind>] [--node-id <node>] <path>")
}

func handleReport(args []string, stderr io.Writer) error {
    if stderr == nil { stderr = io.Discard }
    fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
    jobID := fs.String("job-id", "", "Job or ticket identifier to download from")
    artifactID := fs.String("artifact-id", "", "Specific artifact/slot identifier to download")
    nodeOverride := fs.String("node-id", "", "(unused in HTTPS mode)")
	output := fs.String("output", "", "Path to write the downloaded file")
    if err := fs.Parse(args); err != nil {
        printReportUsage(stderr)
        return err
    }
    _ = nodeOverride
	trimmedJob := strings.TrimSpace(*jobID)
	if trimmedJob == "" {
		printReportUsage(stderr)
		return errors.New("--job-id is required")
	}
	trimmedOutput := strings.TrimSpace(*output)
	if trimmedOutput == "" {
		printReportUsage(stderr)
		return errors.New("--output is required")
	}
	if err := os.MkdirAll(filepath.Dir(trimmedOutput), 0o755); err != nil {
		return fmt.Errorf("ensure output dir: %w", err)
	}
    ctx := context.Background()
    baseURL, httpClient, err := resolveControlPlaneHTTP(ctx)
    if err != nil { return err }
    // If an artifact ID is provided, hit /v2/artifacts/{id}?download=true
    if strings.TrimSpace(*artifactID) == "" {
        return errors.New("--artifact-id required for HTTPS report")
    }
    endpoint, err := url.JoinPath(baseURL.String(), "v2", "artifacts", url.PathEscape(strings.TrimSpace(*artifactID)))
    if err != nil { return err }
    u, _ := url.Parse(endpoint)
    q := u.Query(); q.Set("download", "true"); u.RawQuery = q.Encode()
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
    if err != nil { return err }
    resp, err := httpClient.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return controlPlaneHTTPError(resp)
    }
    out, err := os.Create(trimmedOutput)
    if err != nil { return fmt.Errorf("open output: %w", err) }
    if _, err := io.Copy(out, resp.Body); err != nil { _ = out.Close(); return fmt.Errorf("write output: %w", err) }
    if err := out.Close(); err != nil { return err }
    fmt.Fprintf(stderr, "Downloaded artifact to %s\n", trimmedOutput)
    return nil
}

func printReportUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: ploy report --job-id <id> [--artifact-id <slot>] [--node-id <node>] --output <path>")
}

func defaultNodeID() (string, error) { return "", nil }

func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
