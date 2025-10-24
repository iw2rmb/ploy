package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/deploy"
)

type carotator interface {
	Rotate(context.Context, deploy.RotateOptions) (deploy.RotateResult, error)
	Close() error
}

type httpCARotator struct {
	base      *url.URL
	http      *http.Client
	clusterID string
}

func newHTTPCARotator(base *url.URL, client *http.Client, clusterID string) *httpCARotator {
	clone := *base
	return &httpCARotator{
		base:      &clone,
		http:      client,
		clusterID: clusterID,
	}
}

func (r *httpCARotator) Rotate(ctx context.Context, opts deploy.RotateOptions) (deploy.RotateResult, error) {
	payload := map[string]any{
		"cluster_id": r.clusterID,
		"dry_run":    opts.DryRun,
		"operator":   strings.TrimSpace(opts.Operator),
		"reason":     strings.TrimSpace(opts.Reason),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return deploy.RotateResult{}, fmt.Errorf("marshal rotation request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint("/v2/beacon/rotate-ca"), bytes.NewReader(body))
	if err != nil {
		return deploy.RotateResult{}, fmt.Errorf("build rotation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.http.Do(req)
	if err != nil {
		return deploy.RotateResult{}, fmt.Errorf("invoke rotation: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return deploy.RotateResult{}, fmt.Errorf("rotate beacon CA: %w", controlPlaneHTTPError(resp))
	}

	var response struct {
		DryRun                    bool                     `json:"dry_run"`
		OldVersion                string                   `json:"old_version"`
		NewVersion                string                   `json:"new_version"`
		UpdatedBeaconCertificates []deploy.LeafCertificate `json:"updated_beacon_certificates"`
		UpdatedWorkerCertificates []deploy.LeafCertificate `json:"updated_worker_certificates"`
		Revoked                   deploy.RevokedRecord     `json:"revoked"`
		Operator                  string                   `json:"operator"`
		Reason                    string                   `json:"reason"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return deploy.RotateResult{}, fmt.Errorf("decode rotation response: %w", err)
	}

	return deploy.RotateResult{
		DryRun:                    response.DryRun,
		OldVersion:                response.OldVersion,
		NewVersion:                response.NewVersion,
		UpdatedBeaconCertificates: response.UpdatedBeaconCertificates,
		UpdatedWorkerCertificates: response.UpdatedWorkerCertificates,
		Revoked:                   response.Revoked,
		Operator:                  strings.TrimSpace(response.Operator),
		Reason:                    strings.TrimSpace(response.Reason),
	}, nil
}

func (r *httpCARotator) Close() error { return nil }

func (r *httpCARotator) endpoint(path string) string {
	u := *r.base
	u.Path = strings.TrimSuffix(u.Path, "/") + path
	u.RawQuery = ""
	return u.String()
}

var beaconRotationManagerFactory = func(ctx context.Context, clusterID string) (carotator, error) {
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(clusterID)
	if trimmed == "" {
		return nil, errors.New("cluster id required")
	}
	return newHTTPCARotator(base, httpClient, trimmed), nil
}

func handleBeacon(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printBeaconUsage(stderr)
		return errors.New("beacon subcommand required")
	}
	switch args[0] {
	case "rotate-ca":
		return runBeaconRotateCA(args[1:], stderr)
	default:
		printBeaconUsage(stderr)
		return fmt.Errorf("unknown beacon subcommand %q", args[0])
	}
}

func runBeaconRotateCA(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("beacon rotate-ca", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		clusterID stringValue
		operator  stringValue
		reason    stringValue
	)
	dryRun := fs.Bool("dry-run", false, "Preview the CA rotation without persisting changes")
	fs.Var(&clusterID, "cluster-id", "Cluster identifier (required)")
	fs.Var(&operator, "operator", "Operator name recorded in audit trail")
	fs.Var(&reason, "reason", "Rotation reason or reference ticket")

	if err := fs.Parse(args); err != nil {
		printBeaconRotateCAUsage(stderr)
		return err
	}
	if fs.NArg() > 0 {
		printBeaconRotateCAUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !clusterID.set || strings.TrimSpace(clusterID.value) == "" {
		printBeaconRotateCAUsage(stderr)
		return errors.New("cluster-id is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	rotator, err := beaconRotationManagerFactory(ctx, strings.TrimSpace(clusterID.value))
	if err != nil {
		return err
	}
	defer func() {
		_ = rotator.Close()
	}()

	opts := deploy.RotateOptions{
		DryRun:      *dryRun,
		RequestedAt: time.Now().UTC(),
		Operator:    strings.TrimSpace(operator.value),
		Reason:      strings.TrimSpace(reason.value),
	}
	result, err := rotator.Rotate(ctx, opts)
	if err != nil {
		return err
	}

	if result.DryRun {
		if err := writef(stderr, "[DRY RUN] Cluster %s CA rotation preview\n", clusterID.value); err != nil {
			return err
		}
		if err := writef(stderr, "Current CA version: %s\n", result.OldVersion); err != nil {
			return err
		}
		if err := writef(stderr, "Proposed CA version: %s\n", result.NewVersion); err != nil {
			return err
		}
		for _, cert := range result.UpdatedBeaconCertificates {
			if err := writef(stderr, "  Beacon %s -> parent %s\n", cert.NodeID, cert.ParentVersion); err != nil {
				return err
			}
		}
		for _, cert := range result.UpdatedWorkerCertificates {
			if err := writef(stderr, "  Worker %s -> parent %s\n", cert.NodeID, cert.ParentVersion); err != nil {
				return err
			}
		}
		return nil
	}

	if err := writef(stderr, "Rotation complete for cluster %s\n", clusterID.value); err != nil {
		return err
	}
	if err := writef(stderr, "New CA version: %s (replaced %s)\n", result.NewVersion, result.OldVersion); err != nil {
		return err
	}
	if strings.TrimSpace(result.Reason) != "" {
		if err := writef(stderr, "Reason: %s\n", result.Reason); err != nil {
			return err
		}
	}
	if strings.TrimSpace(result.Operator) != "" {
		if err := writef(stderr, "Operator: %s\n", result.Operator); err != nil {
			return err
		}
	}
	if result.Revoked.Version != "" {
		if err := writef(stderr, "Revoked CA version %s at %s\n", result.Revoked.Version, result.Revoked.RevokedAt.UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}
	if len(result.UpdatedBeaconCertificates) > 0 {
		if err := writef(stderr, "Beacon certificates updated (%d):\n", len(result.UpdatedBeaconCertificates)); err != nil {
			return err
		}
		for _, cert := range result.UpdatedBeaconCertificates {
			if err := writef(stderr, "  %s -> %s\n", cert.NodeID, cert.ParentVersion); err != nil {
				return err
			}
		}
	}
	if len(result.UpdatedWorkerCertificates) > 0 {
		if err := writef(stderr, "Worker certificates updated (%d):\n", len(result.UpdatedWorkerCertificates)); err != nil {
			return err
		}
		for _, cert := range result.UpdatedWorkerCertificates {
			if err := writef(stderr, "  %s -> %s (previous %s)\n", cert.NodeID, cert.ParentVersion, cert.PreviousVersion); err != nil {
				return err
			}
		}
	}
	return nil
}

func printBeaconUsage(w io.Writer) {
	printCommandUsage(w, "beacon")
}

func printBeaconRotateCAUsage(w io.Writer) {
	printCommandUsage(w, "beacon", "rotate-ca")
}

func writef(w io.Writer, format string, args ...any) error {
	_, err := fmt.Fprintf(w, format, args...)
	return err
}
