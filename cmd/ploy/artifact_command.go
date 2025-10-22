package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/artifacts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// artifactService captures the cluster client behaviour required by the CLI.
type artifactService interface {
	Add(ctx context.Context, req artifacts.AddRequest) (artifacts.AddResponse, error)
	Fetch(ctx context.Context, cid string) (artifacts.FetchResult, error)
	Unpin(ctx context.Context, cid string) error
	Status(ctx context.Context, cid string) (artifacts.StatusResult, error)
}

type artifactFactoryFunc func() (artifactService, error)

var (
	artifactClientFactory artifactFactoryFunc = defaultArtifactClientFactory
)

const (
	ipfsClusterURLEnv      = "PLOY_IPFS_CLUSTER_API"
	ipfsClusterTokenEnv    = "PLOY_IPFS_CLUSTER_TOKEN"
	ipfsClusterUserEnv     = "PLOY_IPFS_CLUSTER_USERNAME"
	ipfsClusterPasswordEnv = "PLOY_IPFS_CLUSTER_PASSWORD"
	ipfsClusterReplMinEnv  = "PLOY_IPFS_CLUSTER_REPL_MIN"
	ipfsClusterReplMaxEnv  = "PLOY_IPFS_CLUSTER_REPL_MAX"
)

func handleArtifact(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printArtifactUsage(stderr)
		return errors.New("artifact subcommand required")
	}
	switch args[0] {
	case "push":
		return handleArtifactPush(args[1:], stderr)
	case "pull":
		return handleArtifactPull(args[1:], stderr)
	case "status":
		return handleArtifactStatus(args[1:], stderr)
	case "rm":
		return handleArtifactRemove(args[1:], stderr)
	default:
		printArtifactUsage(stderr)
		return fmt.Errorf("unknown artifact subcommand %q", args[0])
	}
}

func printArtifactUsage(w io.Writer) {
	printCommandUsage(w, "artifact")
}

func handleArtifactPush(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("artifact push", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	name := fs.String("name", "", "artifact name stored alongside the pin")
	kindFlag := fs.String("kind", string(step.ArtifactKindLogs), "artifact kind (diff|logs|custom)")
	replMin := fs.Int("replication-min", 0, "minimum cluster replication factor")
	replMax := fs.Int("replication-max", 0, "maximum cluster replication factor")
	local := fs.Bool("local", false, "retain blocks on the ingesting peer only")
	if err := fs.Parse(args); err != nil {
		printArtifactPushUsage(stderr)
		return err
	}
	remaining := fs.Args()
	if len(remaining) == 0 {
		printArtifactPushUsage(stderr)
		return errors.New("artifact path required")
	}
	path := remaining[0]
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read artifact %s: %w", path, err)
	}

	client, err := artifactClientFactory()
	if err != nil {
		return err
	}

	kind, err := parseArtifactKind(*kindFlag)
	if err != nil {
		return err
	}

	nameValue := strings.TrimSpace(*name)
	if nameValue == "" {
		nameValue = filepath.Base(path)
	}

	addReq := artifacts.AddRequest{
		Name:    nameValue,
		Kind:    kind,
		Payload: bytes.Clone(data),
		Local:   *local,
	}
	if *replMin > 0 {
		addReq.ReplicationFactorMin = *replMin
	}
	if *replMax > 0 {
		addReq.ReplicationFactorMax = *replMax
	}

	result, err := client.Add(context.Background(), addReq)
	if err != nil {
		return err
	}

	printArtifactPushResult(stderr, result)
	return nil
}

func parseArtifactKind(value string) (step.ArtifactKind, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "logs":
		return step.ArtifactKindLogs, nil
	case "diff":
		return step.ArtifactKindDiff, nil
	default:
		return "", fmt.Errorf("unknown artifact kind %q", value)
	}
}

func printArtifactPushUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy artifact push [--name <name>] [--kind <kind>] [--replication-min <n>] [--replication-max <n>] <path>")
}

func printArtifactPushResult(w io.Writer, result artifacts.AddResponse) {
	_, _ = fmt.Fprintf(w, "CID: %s\n", result.CID)
	if result.Name != "" {
		_, _ = fmt.Fprintf(w, "Name: %s\n", result.Name)
	}
	if result.Digest != "" {
		_, _ = fmt.Fprintf(w, "Digest: %s\n", result.Digest)
	}
	if result.Size > 0 {
		_, _ = fmt.Fprintf(w, "Size: %d bytes\n", result.Size)
	}
	if result.ReplicationFactorMin != 0 || result.ReplicationFactorMax != 0 {
		_, _ = fmt.Fprintf(w, "Replication: min=%d max=%d\n", result.ReplicationFactorMin, result.ReplicationFactorMax)
	}
}

func handleArtifactPull(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("artifact pull", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	outputPath := fs.String("output", "", "write artifact to a file instead of stdout")
	if err := fs.Parse(args); err != nil {
		printArtifactPullUsage(stderr)
		return err
	}
	remaining := fs.Args()
	if len(remaining) == 0 {
		printArtifactPullUsage(stderr)
		return errors.New("artifact cid required")
	}
	cid := remaining[0]

	client, err := artifactClientFactory()
	if err != nil {
		return err
	}

	result, err := client.Fetch(context.Background(), cid)
	if err != nil {
		return err
	}

	if strings.TrimSpace(*outputPath) != "" {
		if err := os.WriteFile(*outputPath, result.Data, 0o644); err != nil {
			return fmt.Errorf("write artifact to %s: %w", *outputPath, err)
		}
		_, _ = fmt.Fprintf(stderr, "Wrote artifact to %s (%d bytes)\n", *outputPath, len(result.Data))
		if result.Digest != "" {
			_, _ = fmt.Fprintf(stderr, "Digest: %s\n", result.Digest)
		}
		return nil
	}

	if _, err := stderr.Write(result.Data); err != nil {
		return fmt.Errorf("stream artifact to output: %w", err)
	}
	return nil
}

func printArtifactPullUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy artifact pull [--output <path>] <cid>")
}

func handleArtifactStatus(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printArtifactStatusUsage(stderr)
		return errors.New("artifact cid required")
	}
	cid := strings.TrimSpace(args[0])
	client, err := artifactClientFactory()
	if err != nil {
		return err
	}
	status, err := client.Status(context.Background(), cid)
	if err != nil {
		return err
	}
	printArtifactStatus(stderr, status)
	return nil
}

func printArtifactStatusUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy artifact status <cid>")
}

func printArtifactStatus(w io.Writer, status artifacts.StatusResult) {
	_, _ = fmt.Fprintf(w, "CID: %s\n", status.CID)
	if status.Name != "" {
		_, _ = fmt.Fprintf(w, "Name: %s\n", status.Name)
	}
	if status.ReplicationFactorMin != 0 || status.ReplicationFactorMax != 0 {
		_, _ = fmt.Fprintf(w, "Replication: min=%d max=%d\n", status.ReplicationFactorMin, status.ReplicationFactorMax)
	}
	if status.Summary != "" {
		_, _ = fmt.Fprintf(w, "Summary: %s\n", status.Summary)
	}
	if len(status.Peers) > 0 {
		_, _ = fmt.Fprintln(w, "Peers:")
		for _, peer := range status.Peers {
			_, _ = fmt.Fprintf(w, "  - %s: %s\n", peer.PeerID, peer.Status)
		}
	}
}

func handleArtifactRemove(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printArtifactRemoveUsage(stderr)
		return errors.New("artifact cid required")
	}
	cid := strings.TrimSpace(args[0])
	client, err := artifactClientFactory()
	if err != nil {
		return err
	}
	if err := client.Unpin(context.Background(), cid); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stderr, "Unpinned %s from cluster\n", cid)
	return nil
}

func printArtifactRemoveUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy artifact rm <cid>")
}

func defaultArtifactClientFactory() (artifactService, error) {
	baseURL := strings.TrimSpace(os.Getenv(ipfsClusterURLEnv))
	if baseURL == "" {
		return nil, errors.New("configure artifact client: PLOY_IPFS_CLUSTER_API required")
	}
	opts := artifacts.ClusterClientOptions{
		BaseURL:              baseURL,
		AuthToken:            strings.TrimSpace(os.Getenv(ipfsClusterTokenEnv)),
		BasicAuthUsername:    strings.TrimSpace(os.Getenv(ipfsClusterUserEnv)),
		BasicAuthPassword:    strings.TrimSpace(os.Getenv(ipfsClusterPasswordEnv)),
		ReplicationFactorMin: parseOptionalInt(os.Getenv(ipfsClusterReplMinEnv)),
		ReplicationFactorMax: parseOptionalInt(os.Getenv(ipfsClusterReplMaxEnv)),
	}
	client, err := artifacts.NewClusterClient(opts)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func parseOptionalInt(value string) int {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	num, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return num
}
