package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	// WorkerPhaseRegistering indicates the worker is in the process of joining the cluster.
	WorkerPhaseRegistering = "registering"
	// WorkerPhaseReady indicates the worker completed onboarding and passed health probes.
	WorkerPhaseReady = "ready"
	// WorkerPhaseError indicates the worker failed health probes and requires intervention.
	WorkerPhaseError = "error"
)

var (
	// ErrWorkerExists indicates the worker descriptor already exists.
	ErrWorkerExists = errors.New("registry: worker already exists")
	// ErrWorkerNotFound indicates the requested worker descriptor is missing.
	ErrWorkerNotFound = errors.New("registry: worker not found")
	// ErrWorkerConcurrentUpdate indicates a concurrent modification prevented the update.
	ErrWorkerConcurrentUpdate = errors.New("registry: concurrent worker update detected")
)

// WorkerDescriptor captures cluster metadata for a worker node.
type WorkerDescriptor struct {
	ID                 string            `json:"id"`
	Address            string            `json:"address"`
	Labels             map[string]string `json:"labels,omitempty"`
	RegisteredAt       time.Time         `json:"registered_at"`
	CertificateVersion string            `json:"certificate_version,omitempty"`
	Status             WorkerStatus      `json:"status"`
}

// WorkerStatus tracks worker health and readiness information.
type WorkerStatus struct {
	Phase     string              `json:"phase"`
	CheckedAt time.Time           `json:"checked_at"`
	Message   string              `json:"message,omitempty"`
	Probes    []WorkerProbeResult `json:"probes,omitempty"`
}

// WorkerProbeResult records an individual health probe outcome.
type WorkerProbeResult struct {
	Name       string    `json:"name"`
	Endpoint   string    `json:"endpoint,omitempty"`
	Passed     bool      `json:"passed"`
	StatusCode int       `json:"status_code,omitempty"`
	Message    string    `json:"message,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

// WorkerRecord contains a descriptor alongside its etcd revision for optimistic concurrency.
type WorkerRecord struct {
	Descriptor WorkerDescriptor
	Revision   int64
}

// WorkerRegistry stores and retrieves worker descriptors scoped to a cluster.
type WorkerRegistry struct {
	client *clientv3.Client
	prefix string
}

// NewWorkerRegistry constructs a registry for the provided cluster identifier.
func NewWorkerRegistry(client *clientv3.Client, clusterID string) (*WorkerRegistry, error) {
	if client == nil {
		return nil, errors.New("registry: etcd client required")
	}
	trimmed := strings.TrimSpace(clusterID)
	if trimmed == "" {
		return nil, errors.New("registry: cluster id required")
	}
	prefix := fmt.Sprintf("/ploy/clusters/%s/registry/workers", trimmed)
	return &WorkerRegistry{
		client: client,
		prefix: prefix,
	}, nil
}

// Register stores a new worker descriptor, returning an optimistic concurrency record.
func (r *WorkerRegistry) Register(ctx context.Context, descriptor WorkerDescriptor) (WorkerRecord, error) {
	id := strings.TrimSpace(descriptor.ID)
	if id == "" {
		return WorkerRecord{}, errors.New("registry: worker id required")
	}
	descriptor.ID = id
	if descriptor.Status.Phase == "" {
		descriptor.Status.Phase = WorkerPhaseRegistering
	}
	key := r.workerKey(id)
	payload, err := json.Marshal(descriptor)
	if err != nil {
		return WorkerRecord{}, fmt.Errorf("registry: encode worker descriptor: %w", err)
	}

	txn := r.client.Txn(ctx).If(
		clientv3.Compare(clientv3.CreateRevision(key), "=", 0),
	).Then(
		clientv3.OpPut(key, string(payload)),
	)
	resp, err := txn.Commit()
	if err != nil {
		return WorkerRecord{}, fmt.Errorf("registry: persist worker descriptor: %w", err)
	}
	if !resp.Succeeded {
		return WorkerRecord{}, ErrWorkerExists
	}

	return r.Get(ctx, id)
}

// Get retrieves a worker descriptor and its revision.
func (r *WorkerRegistry) Get(ctx context.Context, workerID string) (WorkerRecord, error) {
	id := strings.TrimSpace(workerID)
	if id == "" {
		return WorkerRecord{}, errors.New("registry: worker id required")
	}
	resp, err := r.client.Get(ctx, r.workerKey(id))
	if err != nil {
		return WorkerRecord{}, fmt.Errorf("registry: get worker descriptor: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return WorkerRecord{}, ErrWorkerNotFound
	}
	var descriptor WorkerDescriptor
	if err := json.Unmarshal(resp.Kvs[0].Value, &descriptor); err != nil {
		return WorkerRecord{}, fmt.Errorf("registry: decode worker descriptor: %w", err)
	}
	return WorkerRecord{
		Descriptor: descriptor,
		Revision:   resp.Kvs[0].ModRevision,
	}, nil
}

// Update replaces an existing worker descriptor using optimistic concurrency.
func (r *WorkerRegistry) Update(ctx context.Context, record WorkerRecord, descriptor WorkerDescriptor) (WorkerRecord, error) {
	id := strings.TrimSpace(record.Descriptor.ID)
	if id == "" {
		return WorkerRecord{}, errors.New("registry: worker id required")
	}
	descriptor.ID = id
	key := r.workerKey(id)
	payload, err := json.Marshal(descriptor)
	if err != nil {
		return WorkerRecord{}, fmt.Errorf("registry: encode worker descriptor: %w", err)
	}

	txn := r.client.Txn(ctx).If(
		clientv3.Compare(clientv3.ModRevision(key), "=", record.Revision),
	).Then(
		clientv3.OpPut(key, string(payload)),
	)
	resp, err := txn.Commit()
	if err != nil {
		return WorkerRecord{}, fmt.Errorf("registry: update worker descriptor: %w", err)
	}
	if !resp.Succeeded {
		return WorkerRecord{}, ErrWorkerConcurrentUpdate
	}
	return r.Get(ctx, id)
}

// Delete removes an existing worker descriptor.
func (r *WorkerRegistry) Delete(ctx context.Context, workerID string) error {
	id := strings.TrimSpace(workerID)
	if id == "" {
		return errors.New("registry: worker id required")
	}
	key := r.workerKey(id)
	txn := r.client.Txn(ctx).If(
		clientv3.Compare(clientv3.CreateRevision(key), ">", 0),
	).Then(
		clientv3.OpDelete(key),
	)
	resp, err := txn.Commit()
	if err != nil {
		return fmt.Errorf("registry: delete worker descriptor: %w", err)
	}
	if !resp.Succeeded {
		return ErrWorkerNotFound
	}
	return nil
}

// Exists reports whether a worker descriptor is present.
func (r *WorkerRegistry) Exists(ctx context.Context, workerID string) (bool, error) {
	_, err := r.Get(ctx, workerID)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrWorkerNotFound) {
		return false, nil
	}
	return false, err
}

// List returns all worker descriptors stored in the registry.
func (r *WorkerRegistry) List(ctx context.Context) ([]WorkerDescriptor, error) {
	resp, err := r.client.Get(ctx, r.prefix+"/", clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("registry: list workers: %w", err)
	}
	descriptors := make([]WorkerDescriptor, 0, resp.Count)
	for _, kv := range resp.Kvs {
		var descriptor WorkerDescriptor
		if err := json.Unmarshal(kv.Value, &descriptor); err != nil {
			return nil, fmt.Errorf("registry: decode worker descriptor: %w", err)
		}
		descriptors = append(descriptors, descriptor)
	}
	sort.Slice(descriptors, func(i, j int) bool {
		return descriptors[i].ID < descriptors[j].ID
	})
	return descriptors, nil
}

func (r *WorkerRegistry) workerKey(workerID string) string {
	return fmt.Sprintf("%s/%s", r.prefix, workerID)
}
