package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

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

const defaultJobID = "cli-manual"

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

func contextOrBackground(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

// NewControlPlaneService constructs an HTTP-backed artifact service.
func NewControlPlaneService(base *url.URL, client *http.Client) (Service, error) {
	if base == nil {
		return nil, errors.New("artifact service: base url required")
	}
	if client == nil {
		return nil, errors.New("artifact service: http client required")
	}
	return &controlPlaneService{base: base, httpClient: client, jobID: defaultJobID}, nil
}

type controlPlaneService struct {
	base       *url.URL
	httpClient *http.Client
	jobID      string
}

func (s *controlPlaneService) Add(ctx context.Context, req artifacts.AddRequest) (artifacts.AddResponse, error) {
	if len(req.Payload) == 0 {
		return artifacts.AddResponse{}, errors.New("artifact payload required")
	}
	endpoint := s.base.ResolveReference(&url.URL{Path: "/v1/artifacts/upload"})
	query := endpoint.Query()
	query.Set("job_id", s.jobID)
	if req.Kind != "" {
		query.Set("kind", string(req.Kind))
	}
	query.Set("name", strings.TrimSpace(req.Name))
	if req.ReplicationFactorMin > 0 {
		query.Set("replication_min", fmt.Sprintf("%d", req.ReplicationFactorMin))
	}
	if req.ReplicationFactorMax > 0 {
		query.Set("replication_max", fmt.Sprintf("%d", req.ReplicationFactorMax))
	}
	digest := sha256.Sum256(req.Payload)
	query.Set("digest", "sha256:"+hex.EncodeToString(digest[:]))
	endpoint.RawQuery = query.Encode()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(req.Payload))
	if err != nil {
		return artifacts.AddResponse{}, fmt.Errorf("artifact upload: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")

	resp, err := s.do(httpReq)
	if err != nil {
		return artifacts.AddResponse{}, err
	}
	defer resp.Body.Close()
	var payload struct {
		Artifact artifactDTO `json:"artifact"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return artifacts.AddResponse{}, fmt.Errorf("artifact upload: decode response: %w", err)
	}
	return payload.Artifact.toAddResponse(), nil
}

func (s *controlPlaneService) Fetch(ctx context.Context, cid string) (artifacts.FetchResult, error) {
	meta, err := s.resolveByCID(ctx, cid)
	if err != nil {
		return artifacts.FetchResult{}, err
	}
	endpoint := s.base.ResolveReference(&url.URL{Path: path.Join("/v1/artifacts", meta.ID)})
	query := endpoint.Query()
	query.Set("download", "1")
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return artifacts.FetchResult{}, fmt.Errorf("artifact download: build request: %w", err)
	}
	resp, err := s.do(req)
	if err != nil {
		return artifacts.FetchResult{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return artifacts.FetchResult{}, fmt.Errorf("artifact download: read body: %w", err)
	}
	digest := meta.Digest
	if strings.TrimSpace(digest) == "" {
		sum := sha256.Sum256(data)
		digest = "sha256:" + hex.EncodeToString(sum[:])
	}
	return artifacts.FetchResult{CID: meta.CID, Data: data, Size: int64(len(data)), Digest: digest}, nil
}

func (s *controlPlaneService) Unpin(ctx context.Context, cid string) error {
	meta, err := s.resolveByCID(ctx, cid)
	if err != nil {
		return err
	}
	endpoint := s.base.ResolveReference(&url.URL{Path: path.Join("/v1/artifacts", meta.ID)})
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("artifact delete: build request: %w", err)
	}
	resp, err := s.do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (s *controlPlaneService) Status(ctx context.Context, cid string) (artifacts.StatusResult, error) {
	meta, err := s.resolveByCID(ctx, cid)
	if err != nil {
		return artifacts.StatusResult{}, err
	}
	return meta.toStatusResult(), nil
}

func (s *controlPlaneService) resolveByCID(ctx context.Context, cid string) (artifactDTO, error) {
	endpoint := s.base.ResolveReference(&url.URL{Path: "/v1/artifacts"})
	query := endpoint.Query()
	query.Set("cid", strings.TrimSpace(cid))
	query.Set("limit", "1")
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return artifactDTO{}, fmt.Errorf("artifact lookup: build request: %w", err)
	}
	resp, err := s.do(req)
	if err != nil {
		return artifactDTO{}, err
	}
	defer resp.Body.Close()
	var payload struct {
		Artifacts []artifactDTO `json:"artifacts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return artifactDTO{}, fmt.Errorf("artifact lookup: decode response: %w", err)
	}
	if len(payload.Artifacts) == 0 {
		return artifactDTO{}, fmt.Errorf("artifact %s not found", cid)
	}
	return payload.Artifacts[0], nil
}

func (s *controlPlaneService) do(req *http.Request) (*http.Response, error) {
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("artifact request failed: %w", err)
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var apiErr struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		if len(body) > 0 {
			_ = json.Unmarshal(body, &apiErr)
		}
		msg := strings.TrimSpace(apiErr.Message)
		if msg == "" {
			msg = strings.TrimSpace(apiErr.Error)
		}
		if msg == "" {
			msg = strings.TrimSpace(string(body))
		}
		if msg == "" {
			msg = resp.Status
		}
		return nil, errors.New(msg)
	}
	return resp, nil
}

type artifactDTO struct {
	ID                   string `json:"id"`
	CID                  string `json:"cid"`
	Digest               string `json:"digest"`
	Name                 string `json:"name"`
	Size                 int64  `json:"size"`
	ReplicationFactorMin int    `json:"replication_factor_min"`
	ReplicationFactorMax int    `json:"replication_factor_max"`
	PinState             string `json:"pin_state"`
	PinReplicas          int    `json:"pin_replicas"`
	PinError             string `json:"pin_error"`
	PinRetryCount        int    `json:"pin_retry_count"`
	PinNextAttemptAt     string `json:"pin_next_attempt_at"`
}

func (dto artifactDTO) toAddResponse() artifacts.AddResponse {
	return artifacts.AddResponse{
		CID:                  dto.CID,
		Name:                 dto.Name,
		Size:                 dto.Size,
		Digest:               dto.Digest,
		ReplicationFactorMin: dto.ReplicationFactorMin,
		ReplicationFactorMax: dto.ReplicationFactorMax,
	}
}

func (dto artifactDTO) toStatusResult() artifacts.StatusResult {
	return artifacts.StatusResult{
		CID:                  dto.CID,
		Name:                 dto.Name,
		Summary:              dto.PinState,
		ReplicationFactorMin: dto.ReplicationFactorMin,
		ReplicationFactorMax: dto.ReplicationFactorMax,
		PinState:             dto.PinState,
		PinReplicas:          dto.PinReplicas,
		PinError:             dto.PinError,
		PinRetryCount:        dto.PinRetryCount,
		PinNextAttemptAt:     parseAPITime(dto.PinNextAttemptAt),
	}
}

func parseAPITime(value string) time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}
	}
	if ts, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return ts.UTC()
	}
	return time.Time{}
}
