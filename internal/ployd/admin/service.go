package admin

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/iw2rmb/ploy/internal/controlplane/registry"
	"github.com/iw2rmb/ploy/internal/deploy"
)

// Service provides administrative endpoints powered by the deploy package.
type Service struct {
	EtcdEndpoints []string
}

// NodeRegistrationRequest captures node onboarding parameters.
type NodeRegistrationRequest struct {
	ClusterID string                     `json:"cluster_id"`
	WorkerID  string                     `json:"worker_id"`
	Address   string                     `json:"address"`
	Labels    map[string]string          `json:"labels"`
	Probes    []deploy.WorkerHealthProbe `json:"probes"`
	DryRun    bool                       `json:"dry_run"`
}

// NodeRegistrationResponse summarises onboarding results.
type NodeRegistrationResponse struct {
	WorkerID    string                       `json:"worker_id"`
	Certificate deploy.LeafCertificate       `json:"certificate"`
	DryRun      bool                         `json:"dry_run"`
	Health      []registry.WorkerProbeResult `json:"health"`
}

// RegisterNode executes the worker join workflow.
func (s *Service) RegisterNode(ctx context.Context, req NodeRegistrationRequest) (NodeRegistrationResponse, error) {
	if s == nil || len(s.EtcdEndpoints) == 0 {
		return NodeRegistrationResponse{}, errors.New("admin: etcd endpoints not configured")
	}
	clusterID := strings.TrimSpace(req.ClusterID)
	if clusterID == "" {
		return NodeRegistrationResponse{}, &HTTPError{Code: http.StatusBadRequest, Message: "cluster_id is required"}
	}
	address := strings.TrimSpace(req.Address)
	if address == "" {
		return NodeRegistrationResponse{}, &HTTPError{Code: http.StatusBadRequest, Message: "address is required"}
	}
	workerID := strings.TrimSpace(req.WorkerID)
	if workerID == "" {
		var err error
		workerID, err = gonanoid.Generate("abcdefghijklmnopqrstuvwxyz0123456789", 12)
		if err != nil {
			return NodeRegistrationResponse{}, err
		}
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   s.EtcdEndpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return NodeRegistrationResponse{}, err
	}
	defer client.Close()

	opts := deploy.WorkerJoinOptions{
		ClusterID:    clusterID,
		WorkerID:     workerID,
		Address:      address,
		Labels:       req.Labels,
		HealthProbes: req.Probes,
		DryRun:       req.DryRun,
		Clock:        func() time.Time { return time.Now().UTC() },
	}
	opts.HealthChecker = &deploy.HTTPHealthChecker{Client: &http.Client{Timeout: 5 * time.Second}, Clock: opts.Clock}

	result, err := deploy.RunWorkerJoin(ctx, client, opts)
	if err != nil {
		return NodeRegistrationResponse{}, err
	}
	return NodeRegistrationResponse{
		WorkerID:    workerID,
		Certificate: result.Certificate,
		DryRun:      result.DryRun,
		Health:      result.Health,
	}, nil
}

// HTTPError converts service failures into HTTP responses.
type HTTPError struct {
	Code    int
	Message string
}

func (e *HTTPError) Error() string { return e.Message }
