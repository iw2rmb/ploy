package artifacts

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// addPublisher is the subset of the cluster client required by the step publisher.
type addPublisher interface {
	Add(ctx context.Context, req AddRequest) (AddResponse, error)
}

// ClusterPublisherOptions configure the IPFS-backed step artifact publisher.
type ClusterPublisherOptions struct {
	Client addPublisher

	ReplicationFactorMin int
	ReplicationFactorMax int
	Local                bool

	// NameBuilder optionally customises the artifact file name using the artifact kind.
	NameBuilder func(kind step.ArtifactKind) string
}

// ClusterPublisher implements the step ArtifactPublisher backed by IPFS Cluster.
type ClusterPublisher struct {
	client      addPublisher
	replMin     int
	replMax     int
	local       bool
	nameBuilder func(kind step.ArtifactKind) string
}

// NewClusterPublisher constructs a publisher that uploads diff/log artifacts to IPFS Cluster.
func NewClusterPublisher(opts ClusterPublisherOptions) (*ClusterPublisher, error) {
	if opts.Client == nil {
		return nil, errors.New("artifacts: cluster publisher client required")
	}
	nameBuilder := opts.NameBuilder
	if nameBuilder == nil {
		nameBuilder = defaultNameBuilder
	}
	return &ClusterPublisher{
		client:      opts.Client,
		replMin:     opts.ReplicationFactorMin,
		replMax:     opts.ReplicationFactorMax,
		local:       opts.Local,
		nameBuilder: nameBuilder,
	}, nil
}

// Publish uploads the artifact payload to IPFS Cluster and returns the resulting reference.
func (p *ClusterPublisher) Publish(ctx context.Context, req step.ArtifactRequest) (step.PublishedArtifact, error) {
	if p == nil || p.client == nil {
		return step.PublishedArtifact{}, errors.New("artifacts: cluster publisher not configured")
	}
	payload, name, err := p.resolvePayload(req)
	if err != nil {
		return step.PublishedArtifact{}, err
	}

	addRequest := AddRequest{
		Name:                 name,
		Kind:                 req.Kind,
		Payload:              payload,
		ReplicationFactorMin: p.replMin,
		ReplicationFactorMax: p.replMax,
		Local:                p.local,
	}

	result, err := p.client.Add(ctx, addRequest)
	if err != nil {
		return step.PublishedArtifact{}, err
	}
	return step.PublishedArtifact{
		CID:    result.CID,
		Kind:   req.Kind,
		Digest: result.Digest,
	}, nil
}

func (p *ClusterPublisher) resolvePayload(req step.ArtifactRequest) ([]byte, string, error) {
	if path := filepath.Clean(req.Path); path != "." && path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, "", fmt.Errorf("artifacts: read artifact %s: %w", path, err)
		}
		return data, p.deriveName(req.Kind, filepath.Base(path)), nil
	}
	if len(req.Buffer) > 0 {
		return req.Buffer, p.deriveName(req.Kind, ""), nil
	}
	return nil, "", errors.New("artifacts: artifact payload required")
}

func (p *ClusterPublisher) deriveName(kind step.ArtifactKind, fallback string) string {
	if fallback != "" {
		return fallback
	}
	return p.nameBuilder(kind)
}

func defaultNameBuilder(kind step.ArtifactKind) string {
	timestamp := time.Now().UTC().Format("20060102-150405")
	switch kind {
	case step.ArtifactKindDiff:
		return fmt.Sprintf("diff-%s.tar", timestamp)
	case step.ArtifactKindLogs:
		return fmt.Sprintf("logs-%s.txt", timestamp)
	default:
		return fmt.Sprintf("artifact-%s.bin", timestamp)
	}
}
