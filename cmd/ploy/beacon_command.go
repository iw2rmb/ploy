package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/iw2rmb/ploy/internal/deploy"
)

type carotator interface {
	Rotate(context.Context, deploy.RotateOptions) (deploy.RotateResult, error)
	Close() error
}

type managedCARotator struct {
	manager *deploy.CARotationManager
	client  *clientv3.Client
}

func (m *managedCARotator) Rotate(ctx context.Context, opts deploy.RotateOptions) (deploy.RotateResult, error) {
	return m.manager.Rotate(ctx, opts)
}

func (m *managedCARotator) Close() error {
	return m.client.Close()
}

var beaconRotationManagerFactory = func(ctx context.Context, clusterID string) (carotator, error) {
	endpoints := etcdEndpointsFromEnv()
	if len(endpoints) == 0 {
		return nil, errors.New("PLOY_ETCD_ENDPOINTS must be set for CA rotation")
	}
	client, err := newEtcdClient(ctx, endpoints)
	if err != nil {
		return nil, err
	}
	manager, err := deploy.NewCARotationManager(client, clusterID)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return &managedCARotator{manager: manager, client: client}, nil
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
