package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/transfer"
	"github.com/iw2rmb/ploy/internal/controlplane/tunnel"
	"github.com/iw2rmb/ploy/pkg/sshtransport"
)

func handleUpload(args []string, stderr io.Writer) error {
	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("upload", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jobID := fs.String("job-id", "", "Job or ticket identifier associated with the payload")
	kind := fs.String("kind", "repo", "Transfer kind (repo|logs|report)")
	nodeOverride := fs.String("node-id", "", "Override node identifier for the transfer")
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
	transferClient := transfer.Client{BaseURL: baseURL, HTTPClient: httpClient}
	nodeID := strings.TrimSpace(*nodeOverride)
	if nodeID == "" {
		if nodeID, err = defaultNodeID(); err != nil {
			return err
		}
	}
	slot, err := transferClient.UploadSlot(ctx, transfer.UploadSlotRequest{
		JobID:  trimmedJob,
		Kind:   strings.TrimSpace(*kind),
		NodeID: nodeID,
		Size:   info.Size(),
		Digest: digest,
	})
	if err != nil {
		return fmt.Errorf("request upload slot: %w", err)
	}
	manager, err := tunnel.Manager()
	if err != nil {
		return err
	}
	copyErr := manager.CopyTo(ctx, slot.NodeID, sshtransport.CopyToOptions{
		LocalPath:  payloadPath,
		RemotePath: slot.RemotePath,
	})
	if copyErr != nil {
		_ = transferClient.Abort(ctx, slot.ID)
		return fmt.Errorf("upload payload via ssh: %w", copyErr)
	}
	if err := transferClient.Commit(ctx, slot.ID, transfer.CommitRequest{Size: info.Size(), Digest: digest}); err != nil {
		return fmt.Errorf("commit upload slot: %w", err)
	}
	fmt.Fprintf(stderr, "Uploaded %s (%d bytes) to slot %s on node %s\n", filepath.Base(payloadPath), info.Size(), slot.ID, slot.NodeID)
	return nil
}

func printUploadUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: ploy upload --job-id <id> [--kind <kind>] [--node-id <node>] <path>")
}

func handleReport(args []string, stderr io.Writer) error {
	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jobID := fs.String("job-id", "", "Job or ticket identifier to download from")
	artifactID := fs.String("artifact-id", "", "Specific artifact/slot identifier to download")
	nodeOverride := fs.String("node-id", "", "Override node identifier for the transfer")
	output := fs.String("output", "", "Path to write the downloaded file")
	if err := fs.Parse(args); err != nil {
		printReportUsage(stderr)
		return err
	}
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
	if err != nil {
		return err
	}
	transferClient := transfer.Client{BaseURL: baseURL, HTTPClient: httpClient}
	nodeID := strings.TrimSpace(*nodeOverride)
	if nodeID == "" {
		if nodeID, err = defaultNodeID(); err != nil {
			return err
		}
	}
	slot, err := transferClient.DownloadSlot(ctx, transfer.DownloadSlotRequest{
		JobID:      trimmedJob,
		ArtifactID: strings.TrimSpace(*artifactID),
		NodeID:     nodeID,
	})
	if err != nil {
		return fmt.Errorf("request download slot: %w", err)
	}
	manager, err := tunnel.Manager()
	if err != nil {
		return err
	}
	copyErr := manager.CopyFrom(ctx, slot.NodeID, sshtransport.CopyFromOptions{
		RemotePath: slot.RemotePath,
		LocalPath:  trimmedOutput,
	})
	if copyErr != nil {
		_ = transferClient.Abort(ctx, slot.ID)
		return fmt.Errorf("download payload via ssh: %w", copyErr)
	}
	digest, err := fileDigest(trimmedOutput)
	if err != nil {
		_ = transferClient.Abort(ctx, slot.ID)
		return fmt.Errorf("calculate digest: %w", err)
	}
	if slot.Digest != "" && !strings.EqualFold(slot.Digest, digest) {
		_ = transferClient.Abort(ctx, slot.ID)
		return fmt.Errorf("digest mismatch: expected %s got %s", slot.Digest, digest)
	}
	if err := transferClient.Commit(ctx, slot.ID, transfer.CommitRequest{Size: fileSize(trimmedOutput), Digest: digest}); err != nil {
		return fmt.Errorf("commit download slot: %w", err)
	}
	fmt.Fprintf(stderr, "Downloaded artifact to %s (slot %s)\n", trimmedOutput, slot.ID)
	return nil
}

func printReportUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: ploy report --job-id <id> [--artifact-id <slot>] [--node-id <node>] --output <path>")
}

func defaultNodeID() (string, error) {
	nodes := tunnel.Nodes()
	if len(nodes) == 0 {
		return "", errors.New("no SSH nodes configured; run 'ploy cluster add' or set descriptors")
	}
	return strings.TrimSpace(nodes[0].ID), nil
}

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
