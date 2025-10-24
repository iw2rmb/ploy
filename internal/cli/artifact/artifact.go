package artifact

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/artifacts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

type Service interface {
	Add(ctx context.Context, req artifacts.AddRequest) (artifacts.AddResponse, error)
	Fetch(ctx context.Context, cid string) (artifacts.FetchResult, error)
	Unpin(ctx context.Context, cid string) error
	Status(ctx context.Context, cid string) (artifacts.StatusResult, error)
}

type ClientFactory func() (Service, error)

const (
	IPFSClusterURLEnv      = "PLOY_IPFS_CLUSTER_API"
	IPFSClusterTokenEnv    = "PLOY_IPFS_CLUSTER_TOKEN"
	IPFSClusterUserEnv     = "PLOY_IPFS_CLUSTER_USERNAME"
	IPFSClusterPasswordEnv = "PLOY_IPFS_CLUSTER_PASSWORD"
	IPFSClusterReplMinEnv  = "PLOY_IPFS_CLUSTER_REPL_MIN"
	IPFSClusterReplMaxEnv  = "PLOY_IPFS_CLUSTER_REPL_MAX"
)

var (
	ErrPathRequired = errors.New("artifact path required")
	ErrCIDRequired  = errors.New("artifact cid required")
)

type PushOptions struct {
	Path           string
	Name           string
	Kind           string
	Local          bool
	ReplicationMin int
	ReplicationMax int
}

func Push(ctx context.Context, factory ClientFactory, opts PushOptions) (artifacts.AddResponse, error) {
	if strings.TrimSpace(opts.Path) == "" {
		return artifacts.AddResponse{}, ErrPathRequired
	}
	svc, err := factory()
	if err != nil {
		return artifacts.AddResponse{}, err
	}
	data, err := os.ReadFile(opts.Path)
	if err != nil {
		return artifacts.AddResponse{}, fmt.Errorf("read artifact %s: %w", opts.Path, err)
	}

	kind, err := ParseKind(opts.Kind)
	if err != nil {
		return artifacts.AddResponse{}, err
	}

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = filepath.Base(opts.Path)
	}

	req := artifacts.AddRequest{
		Name:    name,
		Kind:    kind,
		Local:   opts.Local,
		Payload: bytes.Clone(data),
	}
	if opts.ReplicationMin > 0 {
		req.ReplicationFactorMin = opts.ReplicationMin
	}
	if opts.ReplicationMax > 0 {
		req.ReplicationFactorMax = opts.ReplicationMax
	}

	return svc.Add(contextOrBackground(ctx), req)
}

type PullOptions struct {
	CID string
}

func Pull(ctx context.Context, factory ClientFactory, cid string) (artifacts.FetchResult, error) {
	trimmed := strings.TrimSpace(cid)
	if trimmed == "" {
		return artifacts.FetchResult{}, ErrCIDRequired
	}
	svc, err := factory()
	if err != nil {
		return artifacts.FetchResult{}, err
	}
	return svc.Fetch(contextOrBackground(ctx), trimmed)
}

func Status(ctx context.Context, factory ClientFactory, cid string) (artifacts.StatusResult, error) {
	trimmed := strings.TrimSpace(cid)
	if trimmed == "" {
		return artifacts.StatusResult{}, ErrCIDRequired
	}
	svc, err := factory()
	if err != nil {
		return artifacts.StatusResult{}, err
	}
	return svc.Status(contextOrBackground(ctx), trimmed)
}

func Remove(ctx context.Context, factory ClientFactory, cid string) error {
	trimmed := strings.TrimSpace(cid)
	if trimmed == "" {
		return ErrCIDRequired
	}
	svc, err := factory()
	if err != nil {
		return err
	}
	return svc.Unpin(contextOrBackground(ctx), trimmed)
}

func ParseKind(value string) (step.ArtifactKind, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "logs":
		return step.ArtifactKindLogs, nil
	case "diff":
		return step.ArtifactKindDiff, nil
	default:
		return "", fmt.Errorf("unknown artifact kind %q", value)
	}
}

func DefaultClientFactory() (Service, error) {
	baseURL := strings.TrimSpace(os.Getenv(IPFSClusterURLEnv))
	if baseURL == "" {
		return nil, errors.New("configure artifact client: PLOY_IPFS_CLUSTER_API required")
	}
	opts := artifacts.ClusterClientOptions{
		BaseURL:              baseURL,
		AuthToken:            strings.TrimSpace(os.Getenv(IPFSClusterTokenEnv)),
		BasicAuthUsername:    strings.TrimSpace(os.Getenv(IPFSClusterUserEnv)),
		BasicAuthPassword:    strings.TrimSpace(os.Getenv(IPFSClusterPasswordEnv)),
		ReplicationFactorMin: parseOptionalInt(os.Getenv(IPFSClusterReplMinEnv)),
		ReplicationFactorMax: parseOptionalInt(os.Getenv(IPFSClusterReplMaxEnv)),
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

func contextOrBackground(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}
