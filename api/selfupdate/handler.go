package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	consulapi "github.com/hashicorp/consul/api"
	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/iw2rmb/ploy/internal/distribution"
	"github.com/iw2rmb/ploy/internal/orchestration"
	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/utils"
)

// Build-time variables injected via ldflags
var (
	BuildVersion string
	GitCommit    string
	GitBranch    string
	BuildTime    string
)

// Handler handles controller self-update operations
type Handler struct {
	distributor      *distribution.BinaryDistributor
	queue            *JetStreamWorkQueue
	statusPublisher  *StatusPublisher
	currentVersion   string
	gitCommit        string
	gitBranch        string
	buildTime        string
	platform         string
	architecture     string
	executorID       string
	statusMu         sync.RWMutex
	statusCache      map[string]*UpdateStatus
	latestDeployment string
	workerOnce       sync.Once
	updateFn         func(context.Context, string, UpdateRequest, map[string]string) error
}

var (
	checkNomadCluster   = defaultNomadClusterCheck
	checkTraefikCluster = defaultTraefikClusterCheck
)

// NewHandler creates a new self-update handler with Git integration
func NewHandler(storageProvider storage.StorageProvider, queue *JetStreamWorkQueue, statusPublisher *StatusPublisher, currentVersion string) (*Handler, error) {
	if queue == nil {
		return nil, fmt.Errorf("jetstream work queue required")
	}
	if statusPublisher == nil {
		return nil, fmt.Errorf("status publisher required")
	}

	cacheDir := "/var/cache/ploy-api"
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		cacheDir = filepath.Join(os.TempDir(), "ploy-api-cache")
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create cache directory: %w", err)
		}
	}

	distributor := distribution.NewBinaryDistributor(storageProvider, "artifacts", cacheDir)

	gitCommit := os.Getenv("GIT_COMMIT")
	if gitCommit == "" {
		gitCommit = GitCommit
	}

	gitBranch := os.Getenv("GIT_BRANCH")
	if gitBranch == "" {
		gitBranch = GitBranch
	}

	buildTime := os.Getenv("BUILD_TIME")
	if buildTime == "" {
		buildTime = BuildTime
	}

	executorID := os.Getenv("NOMAD_ALLOC_ID")
	if executorID == "" {
		if hostname, err := os.Hostname(); err == nil {
			executorID = hostname
		} else {
			executorID = "controller"
		}
	}

	handler := &Handler{
		distributor:     distributor,
		queue:           queue,
		statusPublisher: statusPublisher,
		currentVersion:  currentVersion,
		gitCommit:       gitCommit,
		gitBranch:       gitBranch,
		buildTime:       buildTime,
		platform:        "linux", // TODO: detect from runtime
		architecture:    "amd64", // TODO: detect from runtime
		executorID:      executorID,
		statusCache:     make(map[string]*UpdateStatus),
	}
	handler.updateFn = handler.StartUpdate
	return handler, nil
}

// UpdateRequest represents a controller update request
type UpdateRequest struct {
	TargetVersion string            `json:"target_version"`
	Force         bool              `json:"force,omitempty"`
	Strategy      UpdateStrategy    `json:"strategy,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// UpdateStrategy defines the update strategy
type UpdateStrategy string

const (
	RollingUpdate   UpdateStrategy = "rolling"
	BlueGreenUpdate UpdateStrategy = "blue_green"
	EmergencyUpdate UpdateStrategy = "emergency"
)

// UpdateStatus represents the current update status
type UpdateStatus struct {
	DeploymentID     string            `json:"deployment_id,omitempty"`
	Status           string            `json:"status"`
	CurrentVersion   string            `json:"current_version"`
	TargetVersion    string            `json:"target_version,omitempty"`
	Progress         int               `json:"progress"`
	Message          string            `json:"message,omitempty"`
	StartedAt        time.Time         `json:"started_at,omitempty"`
	CompletedAt      time.Time         `json:"completed_at,omitempty"`
	UpdatedInstances []string          `json:"updated_instances,omitempty"`
	FailedInstances  []string          `json:"failed_instances,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// RollbackRequest represents a rollback request
type RollbackRequest struct {
	Reason        string            `json:"reason"`
	Force         bool              `json:"force,omitempty"`
	TargetVersion string            `json:"target_version,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// SetupRoutes configures self-update routes
func SetupRoutes(app *fiber.App, handler *Handler) {
	api := app.Group("/v1")

	// Self-update endpoints
	api.Post("/update", handler.HandleUpdate)
	api.Get("/update/status", handler.HandleGetUpdateStatus)
	api.Post("/update/validate", handler.HandleValidateUpdate)
	api.Post("/rollback", handler.HandleRollback)
	api.Get("/version", handler.HandleGetVersion)
	api.Get("/versions", handler.HandleListVersions)

}

// HandleUpdate enqueues a controller update request onto the JetStream work queue
func (h *Handler) HandleUpdate(c *fiber.Ctx) error {
	var request UpdateRequest
	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request format",
		})
	}

	if request.TargetVersion == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "target_version is required",
		})
	}

	if request.Strategy == "" {
		request.Strategy = RollingUpdate
	}

	if request.Metadata == nil {
		request.Metadata = map[string]string{}
	}

	if err := h.ValidateUpdate(request.TargetVersion); err != nil && !request.Force {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("Update validation failed: %v", err),
		})
	}

	submittedBy := c.Get("X-Ploy-User")
	if submittedBy == "" {
		submittedBy = c.Get("X-Authenticated-User")
	}
	if submittedBy == "" {
		submittedBy = c.IP()
	}

	ctx := context.Background()
	deploymentID, err := h.enqueueUpdate(ctx, request, submittedBy)
	if err != nil {
		log.Printf("[selfupdate] enqueue failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to enqueue update task",
		})
	}

	return c.JSON(fiber.Map{
		"message":        "Update queued",
		"deployment_id":  deploymentID,
		"target_version": request.TargetVersion,
		"strategy":       request.Strategy,
	})
}

// HandleGetUpdateStatus returns current update status
func (h *Handler) HandleGetUpdateStatus(c *fiber.Ctx) error {
	deploymentID := c.Query("deployment_id")
	ctx := context.Background()

	status, err := h.GetUpdateStatus(ctx, deploymentID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to get update status: %v", err),
		})
	}

	return c.JSON(status)
}

// HandleValidateUpdate validates an update without executing it
func (h *Handler) HandleValidateUpdate(c *fiber.Ctx) error {
	var request struct {
		TargetVersion string `json:"target_version"`
	}

	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request format",
		})
	}

	if request.TargetVersion == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "target_version is required",
		})
	}

	if err := h.ValidateUpdate(request.TargetVersion); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"valid": false,
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"valid":           true,
		"target_version":  request.TargetVersion,
		"current_version": h.currentVersion,
	})
}

// HandleRollback enqueues a rollback request using the emergency strategy
func (h *Handler) HandleRollback(c *fiber.Ctx) error {
	var request RollbackRequest
	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request format",
		})
	}

	if request.Reason == "" {
		request.Reason = "Manual rollback initiated"
	}

	ctx := context.Background()
	updateRequest, err := h.RollbackUpdate(ctx, request)
	if err != nil {
		log.Printf("[selfupdate] prepare rollback failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to prepare rollback",
		})
	}

	submittedBy := c.Get("X-Ploy-User")
	if submittedBy == "" {
		submittedBy = c.Get("X-Authenticated-User")
	}
	if submittedBy == "" {
		submittedBy = c.IP()
	}

	deploymentID, err := h.enqueueUpdate(ctx, updateRequest, submittedBy)
	if err != nil {
		log.Printf("[selfupdate] enqueue rollback failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to enqueue rollback",
		})
	}

	return c.JSON(fiber.Map{
		"message":        "Rollback queued",
		"deployment_id":  deploymentID,
		"target_version": updateRequest.TargetVersion,
		"reason":         request.Reason,
	})
}

// HandleGetVersion returns current controller version with Git metadata
func (h *Handler) HandleGetVersion(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"version":      h.currentVersion,
		"git_commit":   h.gitCommit,
		"git_branch":   h.gitBranch,
		"build_time":   h.buildTime,
		"platform":     h.platform,
		"architecture": h.architecture,
	})
}

// HandleListVersions lists available controller versions
func (h *Handler) HandleListVersions(c *fiber.Ctx) error {
	versions, err := h.distributor.ListVersions()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to list versions: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"versions":        versions,
		"current_version": h.currentVersion,
	})
}

// ValidateUpdate performs pre-update validation checks
func (h *Handler) ValidateUpdate(targetVersion string) error {
	log.Printf("Validating update from %s to %s", h.currentVersion, targetVersion)

	// Check if target version is available
	_, _, err := h.distributor.DownloadBinary(targetVersion, h.platform, h.architecture)
	if err != nil {
		return fmt.Errorf("target version %s not available: %w", targetVersion, err)
	}

	// Validate version compatibility (prevent downgrades unless forced)
	if !isNewerVersion(targetVersion, h.currentVersion) {
		return fmt.Errorf("target version %s is not newer than current %s", targetVersion, h.currentVersion)
	}

	// Check system resources
	if err := h.checkSystemResources(); err != nil {
		return fmt.Errorf("system resource check failed: %w", err)
	}

	// Validate cluster health
	if err := h.validateClusterHealth(); err != nil {
		return fmt.Errorf("cluster health validation failed: %w", err)
	}

	log.Printf("Update validation passed for version %s", targetVersion)
	return nil
}

// StartUpdate runs the update strategy for a claimed deployment task
func (h *Handler) StartUpdate(ctx context.Context, deploymentID string, request UpdateRequest, metadata map[string]string) error {
	log.Printf("Starting controller update to version %s with strategy %s (deployment=%s)", request.TargetVersion, request.Strategy, deploymentID)

	metadata = mergeMetadata(metadata, map[string]string{
		"deployment_id": deploymentID,
	})

	switch request.Strategy {
	case RollingUpdate:
		return h.executeRollingUpdate(ctx, deploymentID, request, metadata)
	case BlueGreenUpdate:
		return h.executeBlueGreenUpdate(ctx, deploymentID, request, metadata)
	case EmergencyUpdate:
		return h.executeEmergencyUpdate(ctx, deploymentID, request, metadata)
	default:
		return h.executeRollingUpdate(ctx, deploymentID, request, metadata)
	}
}

// GetUpdateStatus returns the most recent status for the given deployment (or the latest overall when empty)
func (h *Handler) GetUpdateStatus(ctx context.Context, deploymentID string) (*UpdateStatus, error) {
	h.statusMu.RLock()
	if deploymentID == "" {
		deploymentID = h.latestDeployment
	}
	if deploymentID != "" {
		if cached, ok := h.statusCache[deploymentID]; ok {
			copy := *cached
			copy.Metadata = cloneMetadata(cached.Metadata)
			h.statusMu.RUnlock()
			return &copy, nil
		}
	}
	h.statusMu.RUnlock()

	if deploymentID == "" {
		return &UpdateStatus{
			Status:         "idle",
			CurrentVersion: h.currentVersion,
			Progress:       100,
		}, nil
	}

	event, err := h.statusPublisher.LastEvent(ctx, deploymentID)
	if err != nil {
		if errors.Is(err, ErrStatusEventNotFound) {
			return &UpdateStatus{
				DeploymentID:   deploymentID,
				Status:         "idle",
				CurrentVersion: h.currentVersion,
				Progress:       100,
			}, nil
		}
		return nil, err
	}

	status := h.statusFromEvent(event)
	h.statusMu.Lock()
	h.statusCache[deploymentID] = status
	h.latestDeployment = deploymentID
	h.statusMu.Unlock()

	copy := *status
	copy.Metadata = cloneMetadata(status.Metadata)
	return &copy, nil
}

// RollbackUpdate constructs an emergency update request targeting the last known good version
func (h *Handler) RollbackUpdate(ctx context.Context, request RollbackRequest) (UpdateRequest, error) {
	log.Printf("Preparing controller rollback: %s", request.Reason)

	var targetVersion string
	if request.TargetVersion != "" {
		targetVersion = request.TargetVersion
	} else {
		rollbackManager := distribution.NewRollbackManager(h.distributor, h.platform, h.architecture)
		lastGood, err := rollbackManager.GetLastKnownGood(h.currentVersion)
		if err != nil {
			return UpdateRequest{}, fmt.Errorf("failed to find rollback target: %w", err)
		}
		targetVersion = lastGood
	}

	metadata := map[string]string{
		"rollback_reason":    request.Reason,
		"rollback_from":      h.currentVersion,
		"rollback_initiated": time.Now().Format(time.RFC3339),
	}
	metadata = mergeMetadata(metadata, request.Metadata)

	rollbackRequest := UpdateRequest{
		TargetVersion: targetVersion,
		Force:         request.Force,
		Strategy:      EmergencyUpdate,
		Metadata:      metadata,
	}

	return rollbackRequest, nil
}

// checkSystemResources validates system resources for update
func (h *Handler) checkSystemResources() error {
	// Check disk space
	currentBinary := os.Args[0]
	stat, err := os.Stat(currentBinary)
	if err != nil {
		return fmt.Errorf("failed to stat current binary: %w", err)
	}

	// Ensure we have at least 2x the binary size available for backup
	required := stat.Size() * 2
	available, err := getDiskSpace(filepath.Dir(currentBinary))
	if err != nil {
		return fmt.Errorf("failed to check disk space: %w", err)
	}

	if available < required {
		return fmt.Errorf("insufficient disk space: need %d, have %d", required, available)
	}

	return nil
}

// validateClusterHealth checks if cluster is healthy enough for update
func (h *Handler) validateClusterHealth() error {
	var errs []string
	if err := checkNomadCluster(); err != nil {
		errs = append(errs, err.Error())
	}
	if err := checkTraefikCluster(); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("cluster health validation failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

func defaultNomadClusterCheck() error {
	monitor := orchestration.NewHealthMonitor()
	jobs := []string{}
	if controllerJob := utils.Getenv("PLOY_CONTROLLER_NOMAD_JOB", utils.Getenv("NOMAD_CONTROLLER_JOB", "")); controllerJob != "" {
		jobs = append(jobs, controllerJob)
	}
	jobs = append(jobs, utils.Getenv("PLOY_TRAEFIK_NOMAD_JOB", "traefik-system"))
	if len(jobs) == 0 {
		jobs = append(jobs, "ploy-api")
	}
	var errs []string
	for _, job := range jobs {
		if job == "" {
			continue
		}
		allocs, err := monitor.GetJobAllocations(job)
		if err != nil {
			errs = append(errs, fmt.Sprintf("nomad job %s allocation query failed: %v", job, err))
			continue
		}
		healthy := false
		for _, alloc := range allocs {
			if alloc.ClientStatus == "running" {
				healthy = true
				break
			}
		}
		if !healthy {
			errs = append(errs, fmt.Sprintf("nomad job %s has no running allocations", job))
		}
	}

	nomadAddr := utils.Getenv("NOMAD_ADDR", "http://127.0.0.1:4646")
	cfg := nomadapi.DefaultConfig()
	cfg.Address = nomadAddr
	client, err := nomadapi.NewClient(cfg)
	if err != nil {
		errs = append(errs, fmt.Sprintf("nomad client init failed: %v", err))
	} else if _, err := client.Status().Leader(); err != nil {
		errs = append(errs, fmt.Sprintf("failed to query nomad leader: %v", err))
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func defaultTraefikClusterCheck() error {
	consulAddr := utils.Getenv("CONSUL_HTTP_ADDR", utils.Getenv("CONSUL_ADDR", "127.0.0.1:8500"))
	config := consulapi.DefaultConfig()
	config.Address = consulAddr
	client, err := consulapi.NewClient(config)
	if err != nil {
		return fmt.Errorf("failed to create consul client: %w", err)
	}

	services, _, err := client.Catalog().Service("traefik", "", nil)
	if err != nil {
		return fmt.Errorf("failed to query traefik service catalog: %w", err)
	}
	if len(services) == 0 {
		return fmt.Errorf("no traefik service registered in consul at %s", consulAddr)
	}

	healthEntries, _, err := client.Health().Service("traefik", "", true, nil)
	if err != nil {
		return fmt.Errorf("failed to fetch traefik health: %w", err)
	}
	if len(healthEntries) == 0 {
		return fmt.Errorf("traefik service has no passing health checks in consul")
	}

	return nil
}

func (h *Handler) StartExecutor(ctx context.Context) {
	h.workerOnce.Do(func() {
		go h.runExecutor(ctx)
	})
}

func (h *Handler) runExecutor(ctx context.Context) {
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		timeout := h.queue.DefaultFetchTimeout()
		msg, err := h.queue.Fetch(ctx, timeout)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				continue
			}
			log.Printf("[selfupdate] work queue fetch error: %v", err)
			time.Sleep(backoff)
			if backoff < 10*time.Second {
				backoff *= 2
			}
			continue
		}

		backoff = time.Second
		if msg == nil {
			continue
		}

		if err := h.processTask(ctx, msg); err != nil {
			log.Printf("[selfupdate] task %s failed: %v", msg.DeploymentID, err)
		}
	}
}

func (h *Handler) processTask(ctx context.Context, msg *WorkQueueMessage) error {
	req := msg.Request
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}

	combined := mergeMetadata(req.Metadata, msg.Metadata)
	if msg.Lane != "" {
		combined["lane"] = msg.Lane
	}
	if msg.SubmittedBy != "" {
		combined["submitted_by"] = msg.SubmittedBy
	}
	combined["executor"] = h.executorID

	h.updateStatus(ctx, msg.DeploymentID, req, "preparing", "Update task claimed", 0, combined)

	updateFn := h.updateFn
	if updateFn == nil {
		updateFn = h.StartUpdate
	}

	if err := updateFn(ctx, msg.DeploymentID, req, combined); err != nil {
		h.updateStatus(ctx, msg.DeploymentID, req, "failed", err.Error(), 0, combined)
		delay := msg.AckWait / 2
		if delay <= 0 {
			delay = time.Second
		}
		if nakErr := msg.NakWithDelay(delay); nakErr != nil {
			log.Printf("[selfupdate] nak failed task %s: %v", msg.DeploymentID, nakErr)
			if ackErr := msg.Ack(); ackErr != nil {
				log.Printf("[selfupdate] fallback ack failed task %s: %v", msg.DeploymentID, ackErr)
			}
		}
		return err
	}

	if err := msg.Ack(); err != nil {
		log.Printf("[selfupdate] ack task %s: %v", msg.DeploymentID, err)
	}

	return nil
}

func mergeMetadata(sets ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, m := range sets {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

func cloneMetadata(meta map[string]string) map[string]string {
	if len(meta) == 0 {
		return nil
	}
	copy := make(map[string]string, len(meta))
	for k, v := range meta {
		copy[k] = v
	}
	return copy
}

func (h *Handler) updateStatus(ctx context.Context, deploymentID string, request UpdateRequest, status, message string, progress int, metadata map[string]string) {
	event := StatusEvent{
		DeploymentID: deploymentID,
		Phase:        status,
		Progress:     progress,
		Message:      message,
		Executor:     h.executorID,
		Metadata:     cloneMetadata(metadata),
	}

	if err := h.statusPublisher.Publish(ctx, event); err != nil {
		log.Printf("[selfupdate] publish status event failed: %v", err)
	}

	h.statusMu.Lock()
	defer h.statusMu.Unlock()

	cached, ok := h.statusCache[deploymentID]
	if !ok {
		cached = &UpdateStatus{
			DeploymentID:   deploymentID,
			CurrentVersion: h.currentVersion,
			TargetVersion:  request.TargetVersion,
			StartedAt:      event.Timestamp,
		}
	}

	cached.Status = status
	cached.Progress = progress
	cached.Message = message
	cached.TargetVersion = request.TargetVersion
	cached.Metadata = mergeMetadata(cached.Metadata, metadata)
	if status == "completed" || status == "failed" {
		cached.CompletedAt = event.Timestamp
	}
	if cached.StartedAt.IsZero() {
		cached.StartedAt = event.Timestamp
	}

	h.statusCache[deploymentID] = cached
	h.latestDeployment = deploymentID
}

func (h *Handler) enqueueUpdate(ctx context.Context, request UpdateRequest, submittedBy string) (string, error) {
	deploymentID := uuid.New().String()

	metadata := mergeMetadata(request.Metadata, map[string]string{
		"target_version": request.TargetVersion,
		"strategy":       string(request.Strategy),
	})
	metadata["lane"] = h.queue.cfg.Lane
	if submittedBy != "" {
		metadata["submitted_by"] = submittedBy
	}

	task := WorkQueueTask{
		DeploymentID: deploymentID,
		SubmittedBy:  submittedBy,
		Request:      request,
		Metadata:     metadata,
	}

	if err := h.queue.Enqueue(ctx, task); err != nil {
		return "", err
	}

	h.updateStatus(ctx, deploymentID, request, "queued", "Update request queued", 0, metadata)
	return deploymentID, nil
}

func (h *Handler) statusFromEvent(event *StatusEvent) *UpdateStatus {
	status := &UpdateStatus{
		DeploymentID:   event.DeploymentID,
		Status:         event.Phase,
		Progress:       event.Progress,
		Message:        event.Message,
		Metadata:       cloneMetadata(event.Metadata),
		CurrentVersion: h.currentVersion,
		StartedAt:      event.Timestamp,
	}

	if status.Metadata != nil {
		if target, ok := status.Metadata["target_version"]; ok {
			status.TargetVersion = target
		}
	}

	if event.Phase == "completed" || event.Phase == "failed" {
		status.CompletedAt = event.Timestamp
	}

	return status
}
