package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/deploy"
)

type fakeCARotator struct {
	rotateCalled bool
	opts         deploy.RotateOptions
	result       deploy.RotateResult
	err          error
}

func (f *fakeCARotator) Rotate(ctx context.Context, opts deploy.RotateOptions) (deploy.RotateResult, error) {
	f.rotateCalled = true
	f.opts = opts
	return f.result, f.err
}

func (f *fakeCARotator) Close() error {
	return nil
}

func TestHandleBeaconRotateCADryRun(t *testing.T) {
	origFactory := beaconRotationManagerFactory
	defer func() { beaconRotationManagerFactory = origFactory }()

	fake := &fakeCARotator{
		result: deploy.RotateResult{
			DryRun:     true,
			OldVersion: "2025-10-01T00:00:00Z-aaaa",
			NewVersion: "2025-10-02T00:00:00Z-bbbb",
			UpdatedBeaconCertificates: []deploy.LeafCertificate{
				{NodeID: "beacon-main", ParentVersion: "2025-10-02T00:00:00Z-bbbb"},
			},
			UpdatedWorkerCertificates: []deploy.LeafCertificate{
				{NodeID: "worker-a", ParentVersion: "2025-10-02T00:00:00Z-bbbb"},
			},
		},
	}
	beaconRotationManagerFactory = func(_ context.Context, _ string) (carotator, error) {
		return fake, nil
	}

	var buf bytes.Buffer
	err := handleBeacon([]string{"rotate-ca", "--cluster-id", "cluster-alpha", "--dry-run"}, &buf)
	if err != nil {
		t.Fatalf("handleBeacon rotate-ca returned error: %v", err)
	}
	if !fake.rotateCalled {
		t.Fatalf("expected rotate to be invoked")
	}
	if !fake.opts.DryRun {
		t.Fatalf("expected dry-run flag to propagate to rotator")
	}
	output := buf.String()
	if !strings.Contains(output, "DRY RUN") {
		t.Fatalf("expected dry-run output marker, got:\n%s", output)
	}
	if !strings.Contains(output, "2025-10-02T00:00:00Z-bbbb") {
		t.Fatalf("expected output to include new CA version, got:\n%s", output)
	}
}

func TestHandleBeaconRotateCAMissingCluster(t *testing.T) {
	var buf bytes.Buffer
	err := handleBeacon([]string{"rotate-ca"}, &buf)
	if err == nil {
		t.Fatalf("expected error when cluster id missing")
	}
	if !strings.Contains(buf.String(), "Usage") {
		t.Fatalf("expected usage to be printed on error")
	}
}

func TestHandleBeaconRotateCAErrorBubble(t *testing.T) {
	origFactory := beaconRotationManagerFactory
	defer func() { beaconRotationManagerFactory = origFactory }()

	fake := &fakeCARotator{err: errors.New("rotate failed")}
	beaconRotationManagerFactory = func(_ context.Context, _ string) (carotator, error) {
		return fake, nil
	}
	var buf bytes.Buffer
	err := handleBeacon([]string{"rotate-ca", "--cluster-id", "cluster-alpha"}, &buf)
	if err == nil || !strings.Contains(err.Error(), "rotate failed") {
		t.Fatalf("expected rotate error to bubble, got %v", err)
	}
}

func TestHandleBeaconRotateCASuccess(t *testing.T) {
	origFactory := beaconRotationManagerFactory
	defer func() { beaconRotationManagerFactory = origFactory }()

	now := time.Date(2025, 10, 22, 22, 34, 7, 0, time.UTC)
	fake := &fakeCARotator{
		result: deploy.RotateResult{
			DryRun:     false,
			OldVersion: "2025-10-21T00:00:00Z-aaaa",
			NewVersion: "2025-10-22T22:34:07Z-bbbb",
			UpdatedBeaconCertificates: []deploy.LeafCertificate{
				{NodeID: "beacon-main", ParentVersion: "2025-10-22T22:34:07Z-bbbb"},
			},
			UpdatedWorkerCertificates: []deploy.LeafCertificate{
				{NodeID: "worker-a", ParentVersion: "2025-10-22T22:34:07Z-bbbb"},
				{NodeID: "worker-b", ParentVersion: "2025-10-22T22:34:07Z-bbbb"},
			},
			Revoked: deploy.RevokedRecord{
				Version:   "2025-10-21T00:00:00Z-aaaa",
				RevokedAt: now,
			},
		},
	}
	beaconRotationManagerFactory = func(_ context.Context, _ string) (carotator, error) {
		return fake, nil
	}

	var buf bytes.Buffer
	err := handleBeacon([]string{"rotate-ca", "--cluster-id", "cluster-alpha"}, &buf)
	if err != nil {
		t.Fatalf("handleBeacon rotate-ca returned error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Rotation complete") {
		t.Fatalf("expected success message, got:\n%s", output)
	}
	if !strings.Contains(output, "worker-b") {
		t.Fatalf("expected worker updates in output, got:\n%s", output)
	}
	if fake.opts.DryRun {
		t.Fatalf("expected dry-run false for success case")
	}
}
