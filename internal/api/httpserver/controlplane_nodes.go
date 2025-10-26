package httpserver

import (
	"context"
	"errors"
	"fmt"
	"github.com/iw2rmb/ploy/internal/controlplane/registry"
	"github.com/iw2rmb/ploy/internal/deploy"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"log"
	"net/http"
	"strings"
	"time"
)

func (s *controlPlaneServer) handleNodes(w http.ResponseWriter, r *http.Request) {
	if !s.ensureEtcd(w) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		s.handleNodeJoin(w, r)
	case http.MethodGet:
		s.handleNodeList(w, r)
	case http.MethodDelete:
		s.handleNodeDelete(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *controlPlaneServer) handleNodeJoin(w http.ResponseWriter, r *http.Request) {
	var req nodeJoinRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	clusterID := strings.TrimSpace(req.ClusterID)
	if clusterID == "" {
		writeErrorMessage(w, http.StatusBadRequest, "cluster_id is required")
		return
	}
	address := strings.TrimSpace(req.Address)
	if address == "" {
		writeErrorMessage(w, http.StatusBadRequest, "address is required")
		return
	}
	workerID := strings.TrimSpace(req.WorkerID)
	if workerID == "" {
		generated, err := gonanoid.Generate("abcdefghijklmnopqrstuvwxyz0123456789", 12)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		workerID = generated
	}

	created, err := deploy.EnsureClusterPKI(r.Context(), s.etcd, clusterID, deploy.EnsurePKIOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if created {
		log.Printf("auto-bootstrapped cluster PKI for %s", clusterID)
	}

	probes := make([]deploy.WorkerHealthProbe, 0, len(req.Probes))
	for _, probe := range req.Probes {
		probes = append(probes, deploy.WorkerHealthProbe{
			Name:         strings.TrimSpace(probe.Name),
			Endpoint:     strings.TrimSpace(probe.Endpoint),
			ExpectStatus: probe.ExpectStatus,
		})
	}

	opts := deploy.WorkerJoinOptions{
		ClusterID:    clusterID,
		WorkerID:     workerID,
		Address:      address,
		Labels:       req.Labels,
		HealthProbes: probes,
		DryRun:       req.DryRun,
		Clock:        func() time.Time { return time.Now().UTC() },
	}

	result, err := deploy.RunWorkerJoin(r.Context(), s.etcd, opts)
	if err != nil {
		switch {
		case errors.Is(err, registry.ErrWorkerExists), errors.Is(err, deploy.ErrWorkerExists):
			writeErrorMessage(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err)
		}
		return
	}

	resp := map[string]any{
		"worker_id":   result.Descriptor.ID,
		"descriptor":  descriptorDTOFrom(result.Descriptor),
		"certificate": result.Certificate,
		"health":      result.Health,
		"dry_run":     result.DryRun,
		"ca_bundle":   result.CABundle,
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *controlPlaneServer) handleNodeList(w http.ResponseWriter, r *http.Request) {
	clusterID := strings.TrimSpace(r.URL.Query().Get("cluster_id"))
	if clusterID == "" {
		writeErrorMessage(w, http.StatusBadRequest, "cluster_id query parameter required")
		return
	}
	reg, err := registry.NewWorkerRegistry(s.etcd, clusterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	descriptors, err := reg.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	nodes := make([]workerDescriptorDTO, 0, len(descriptors))
	for _, descriptor := range descriptors {
		nodes = append(nodes, descriptorDTOFrom(descriptor))
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes})
}

func (s *controlPlaneServer) handleNodeDelete(w http.ResponseWriter, r *http.Request) {
	if !s.ensureScheduler(w) {
		return
	}
	var req nodeDeleteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	clusterID := strings.TrimSpace(req.ClusterID)
	workerID := strings.TrimSpace(req.WorkerID)
	if clusterID == "" {
		writeErrorMessage(w, http.StatusBadRequest, "cluster_id is required")
		return
	}
	if workerID == "" {
		writeErrorMessage(w, http.StatusBadRequest, "worker_id is required")
		return
	}
	if strings.TrimSpace(req.Confirm) != workerID {
		writeErrorMessage(w, http.StatusBadRequest, "confirm must match worker_id")
		return
	}
	if req.DrainTimeoutSeconds < 0 {
		writeErrorMessage(w, http.StatusBadRequest, "drain_timeout_seconds must be non-negative")
		return
	}
	timeout := time.Duration(req.DrainTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	if err := s.waitForNodeDrain(r.Context(), workerID, timeout); err != nil {
		var drainErr nodeDrainError
		switch {
		case errors.As(err, &drainErr):
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":        fmt.Sprintf("node %s still has running jobs", workerID),
				"running_jobs": drainErr.Remaining,
			})
		case errors.Is(err, context.DeadlineExceeded):
			writeError(w, http.StatusRequestTimeout, err)
		case errors.Is(err, context.Canceled):
			writeError(w, http.StatusRequestTimeout, err)
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	manager, err := deploy.NewCARotationManager(s.etcd, clusterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	reg, err := registry.NewWorkerRegistry(s.etcd, clusterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := reg.Get(r.Context(), workerID); err != nil {
		if errors.Is(err, registry.ErrWorkerNotFound) {
			writeErrorMessage(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := manager.RemoveWorker(r.Context(), workerID); err != nil {
		switch {
		case errors.Is(err, deploy.ErrWorkerNotFound):
			writeErrorMessage(w, http.StatusNotFound, err.Error())
		case errors.Is(err, deploy.ErrConcurrentWorkerUpdate):
			writeErrorMessage(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}
	if err := reg.Delete(r.Context(), workerID); err != nil {
		if errors.Is(err, registry.ErrWorkerNotFound) {
			writeErrorMessage(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type nodeJoinRequest struct {
	ClusterID string             `json:"cluster_id"`
	WorkerID  string             `json:"worker_id"`
	Address   string             `json:"address"`
	Labels    map[string]string  `json:"labels"`
	Probes    []nodeProbeRequest `json:"probes"`
	DryRun    bool               `json:"dry_run"`
}

type nodeProbeRequest struct {
	Name         string `json:"name"`
	Endpoint     string `json:"endpoint"`
	ExpectStatus int    `json:"expect_status"`
}

type nodeDeleteRequest struct {
	ClusterID           string `json:"cluster_id"`
	WorkerID            string `json:"worker_id"`
	Confirm             string `json:"confirm"`
	DrainTimeoutSeconds int    `json:"drain_timeout_seconds"`
}

type workerDescriptorDTO struct {
	ID                 string            `json:"id"`
	Address            string            `json:"address"`
	Labels             map[string]string `json:"labels,omitempty"`
	RegisteredAt       string            `json:"registered_at"`
	CertificateVersion string            `json:"certificate_version,omitempty"`
	Status             workerStatusDTO   `json:"status"`
}

type workerStatusDTO struct {
	Phase     string                       `json:"phase"`
	CheckedAt string                       `json:"checked_at"`
	Message   string                       `json:"message,omitempty"`
	Probes    []registry.WorkerProbeResult `json:"probes,omitempty"`
}

type nodeDrainError struct {
	Remaining int
}

func (e nodeDrainError) Error() string {
	if e.Remaining <= 0 {
		return "no running jobs"
	}
	return fmt.Sprintf("%d jobs still running", e.Remaining)
}

func descriptorDTOFrom(desc registry.WorkerDescriptor) workerDescriptorDTO {
	labels := copyMap(desc.Labels)
	if len(labels) == 0 {
		labels = nil
	}
	statusProbes := make([]registry.WorkerProbeResult, 0, len(desc.Status.Probes))
	if len(desc.Status.Probes) > 0 {
		statusProbes = append(statusProbes, desc.Status.Probes...)
	}
	dto := workerDescriptorDTO{
		ID:                 desc.ID,
		Address:            desc.Address,
		Labels:             labels,
		RegisteredAt:       formatTime(desc.RegisteredAt),
		CertificateVersion: desc.CertificateVersion,
		Status: workerStatusDTO{
			Phase:     desc.Status.Phase,
			CheckedAt: formatTime(desc.Status.CheckedAt),
			Message:   desc.Status.Message,
			Probes:    statusProbes,
		},
	}
	if strings.TrimSpace(dto.Status.Message) == "" {
		dto.Status.Message = ""
	}
	if len(dto.Status.Probes) == 0 {
		dto.Status.Probes = nil
	}
	return dto
}

func (s *controlPlaneServer) waitForNodeDrain(ctx context.Context, nodeID string, timeout time.Duration) error {
	if s.scheduler == nil {
		return errors.New("scheduler unavailable")
	}
	deadline := time.Time{}
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		jobs, err := s.scheduler.RunningJobsForNode(ctx, nodeID)
		if err != nil {
			return err
		}
		remaining := len(jobs)
		if remaining == 0 {
			return nil
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return nodeDrainError{Remaining: remaining}
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nodeDrainError{Remaining: remaining}
			}
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
