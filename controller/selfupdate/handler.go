package selfupdate

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/hashicorp/consul/api"
	"github.com/iw2rmb/ploy/internal/distribution"
	"github.com/iw2rmb/ploy/internal/storage"
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
	distributor    *distribution.BinaryDistributor
	consulClient   *api.Client
	currentVersion string
	gitCommit      string
	gitBranch      string
	buildTime      string
	platform       string
	architecture   string
	leaderPrefix   string
	sessionTTL     time.Duration
}

// NewHandler creates a new self-update handler with Git integration
func NewHandler(storageProvider storage.StorageProvider, consulAddr, currentVersion string) (*Handler, error) {
	// Initialize binary distributor
	distributor := distribution.NewBinaryDistributor(storageProvider, "ploy-artifacts", "/tmp/ploy-updates")

	// Initialize Consul client
	config := api.DefaultConfig()
	config.Address = consulAddr
	consulClient, err := api.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create consul client: %w", err)
	}

	// Get Git metadata from environment or build variables
	gitCommit := os.Getenv("GIT_COMMIT")
	if gitCommit == "" {
		gitCommit = GitCommit // Build-time variable
	}

	gitBranch := os.Getenv("GIT_BRANCH")
	if gitBranch == "" {
		gitBranch = GitBranch // Build-time variable
	}

	buildTime := os.Getenv("BUILD_TIME")
	if buildTime == "" {
		buildTime = BuildTime // Build-time variable
	}

	return &Handler{
		distributor:    distributor,
		consulClient:   consulClient,
		currentVersion: currentVersion,
		gitCommit:      gitCommit,
		gitBranch:      gitBranch,
		buildTime:      buildTime,
		platform:       "linux",    // TODO: detect from runtime
		architecture:   "amd64",    // TODO: detect from runtime
		leaderPrefix:   "ploy/controller/update-coordination",
		sessionTTL:     30 * time.Second,
	}, nil
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
	api := app.Group("/v1/controller")

	// Self-update endpoints
	api.Post("/update", handler.HandleUpdate)
	api.Get("/update/status", handler.HandleGetUpdateStatus)
	api.Post("/update/validate", handler.HandleValidateUpdate)
	api.Post("/rollback", handler.HandleRollback)
	api.Get("/version", handler.HandleGetVersion)
	api.Get("/versions", handler.HandleListVersions)

	// Git-based update endpoints
	api.Post("/update/latest", handler.HandleUpdateToLatest)
	api.Post("/update/branch/:branch", handler.HandleUpdateToBranch)
	api.Get("/update/available", handler.HandleListAvailableVersions)
	api.Post("/deploy/git", handler.HandleGitDeploy)
}

// HandleUpdate initiates a controller update
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

	// Default strategy
	if request.Strategy == "" {
		request.Strategy = RollingUpdate
	}

	// Validate update request
	if err := h.ValidateUpdate(request.TargetVersion); err != nil && !request.Force {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("Update validation failed: %v", err),
		})
	}

	// Start update in background
	go func() {
		ctx := context.Background()
		if err := h.StartUpdate(ctx, request); err != nil {
			log.Printf("Update failed: %v", err)
		}
	}()

	return c.JSON(fiber.Map{
		"message":        "Update initiated",
		"target_version": request.TargetVersion,
		"strategy":       request.Strategy,
	})
}

// HandleGetUpdateStatus returns current update status
func (h *Handler) HandleGetUpdateStatus(c *fiber.Ctx) error {
	status, err := h.GetUpdateStatus()
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
		"valid":          true,
		"target_version": request.TargetVersion,
		"current_version": h.currentVersion,
	})
}

// HandleRollback initiates a controller rollback
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

	// Start rollback in background
	go func() {
		ctx := context.Background()
		if err := h.RollbackUpdate(ctx, request); err != nil {
			log.Printf("Rollback failed: %v", err)
		}
	}()

	return c.JSON(fiber.Map{
		"message": "Rollback initiated",
		"reason":  request.Reason,
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

// StartUpdate initiates a coordinated controller update
func (h *Handler) StartUpdate(ctx context.Context, request UpdateRequest) error {
	log.Printf("Starting controller update to version %s with strategy %s", request.TargetVersion, request.Strategy)

	// Create update coordination session
	sessionID, err := h.createUpdateSession(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to create update session: %w", err)
	}
	defer h.cleanupSession(sessionID)

	// Execute update based on strategy
	switch request.Strategy {
	case RollingUpdate:
		return h.executeRollingUpdate(ctx, sessionID, request)
	case BlueGreenUpdate:
		return h.executeBlueGreenUpdate(ctx, sessionID, request)
	case EmergencyUpdate:
		return h.executeEmergencyUpdate(ctx, sessionID, request)
	default:
		return h.executeRollingUpdate(ctx, sessionID, request)
	}
}

// GetUpdateStatus returns the current update status
func (h *Handler) GetUpdateStatus() (*UpdateStatus, error) {
	kv := h.consulClient.KV()

	// Check for active update session
	kvPair, _, err := kv.Get(h.leaderPrefix+"/status", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get update status: %w", err)
	}

	if kvPair == nil {
		// No active update
		return &UpdateStatus{
			Status:         "idle",
			CurrentVersion: h.currentVersion,
			Progress:       100,
		}, nil
	}

	// Parse status from Consul
	var status UpdateStatus
	if err := parseJSON(kvPair.Value, &status); err != nil {
		return nil, fmt.Errorf("failed to parse update status: %w", err)
	}

	return &status, nil
}

// RollbackUpdate performs rollback to previous version
func (h *Handler) RollbackUpdate(ctx context.Context, request RollbackRequest) error {
	log.Printf("Starting controller rollback: %s", request.Reason)

	var targetVersion string
	if request.TargetVersion != "" {
		targetVersion = request.TargetVersion
	} else {
		// Get last known good version
		rollbackManager := distribution.NewRollbackManager(h.distributor, h.platform, h.architecture)
		lastGood, err := rollbackManager.GetLastKnownGood(h.currentVersion)
		if err != nil {
			return fmt.Errorf("failed to find rollback target: %w", err)
		}
		targetVersion = lastGood
	}

	// Execute emergency rollback
	rollbackRequest := UpdateRequest{
		TargetVersion: targetVersion,
		Force:         request.Force,
		Strategy:      EmergencyUpdate,
		Metadata: map[string]string{
			"rollback_reason":    request.Reason,
			"rollback_from":      h.currentVersion,
			"rollback_initiated": time.Now().Format(time.RFC3339),
		},
	}

	if request.Metadata != nil {
		for k, v := range request.Metadata {
			rollbackRequest.Metadata[k] = v
		}
	}

	return h.StartUpdate(ctx, rollbackRequest)
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
	// Check Consul health
	agent := h.consulClient.Agent()
	_, err := agent.Self()
	if err != nil {
		return fmt.Errorf("consul health check failed: %w", err)
	}

	// TODO: Add more comprehensive cluster health checks
	// - Check Nomad connectivity
	// - Verify service registrations
	// - Check load balancer status

	return nil
}

// GitUpdateRequest represents a Git-based update request
type GitUpdateRequest struct {
	Branch    string            `json:"branch,omitempty"`
	Tag       string            `json:"tag,omitempty"`
	Commit    string            `json:"commit,omitempty"`
	Force     bool              `json:"force,omitempty"`
	Strategy  UpdateStrategy    `json:"strategy,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// GitVersionInfo represents version information with Git metadata
type GitVersionInfo struct {
	Version       string `json:"version"`
	GitCommit     string `json:"git_commit"`
	GitBranch     string `json:"git_branch"`
	BuildTime     string `json:"build_time"`
	Available     bool   `json:"available"`
	ArtifactURL   string `json:"artifact_url,omitempty"`
	Checksum      string `json:"checksum,omitempty"`
}

// HandleUpdateToLatest updates controller to latest Git tag
func (h *Handler) HandleUpdateToLatest(c *fiber.Ctx) error {
	var request GitUpdateRequest
	if err := c.BodyParser(&request); err != nil {
		// Use defaults for latest update
		request = GitUpdateRequest{}
	}

	// Set default strategy
	if request.Strategy == "" {
		request.Strategy = RollingUpdate
	}

	// Get latest version from distributor
	versions, err := h.distributor.ListVersions()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to get available versions: %v", err),
		})
	}

	if len(versions) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "No versions available",
		})
	}

	// Find latest version (assuming versions are sorted)
	latestVersion := versions[len(versions)-1]

	// Create update request
	updateRequest := UpdateRequest{
		TargetVersion: latestVersion,
		Force:         request.Force,
		Strategy:      request.Strategy,
		Metadata: map[string]string{
			"update_type": "git_latest",
			"requested_via": "api_latest_endpoint",
			"timestamp": time.Now().Format(time.RFC3339),
		},
	}

	if request.Metadata != nil {
		for k, v := range request.Metadata {
			updateRequest.Metadata[k] = v
		}
	}

	// Validate and start update
	if err := h.ValidateUpdate(updateRequest.TargetVersion); err != nil && !request.Force {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("Update validation failed: %v", err),
		})
	}

	// Start update in background
	go func() {
		ctx := context.Background()
		if err := h.StartUpdate(ctx, updateRequest); err != nil {
			log.Printf("Latest update failed: %v", err)
		}
	}()

	return c.JSON(fiber.Map{
		"message": "Update to latest version initiated",
		"target_version": latestVersion,
		"strategy": request.Strategy,
		"update_type": "git_latest",
	})
}

// HandleUpdateToBranch updates controller to specific Git branch HEAD
func (h *Handler) HandleUpdateToBranch(c *fiber.Ctx) error {
	branch := c.Params("branch")
	if branch == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "branch parameter is required",
		})
	}

	var request GitUpdateRequest
	if err := c.BodyParser(&request); err != nil {
		// Use defaults
		request = GitUpdateRequest{}
	}

	request.Branch = branch

	// Set default strategy
	if request.Strategy == "" {
		request.Strategy = RollingUpdate
	}

	// Find versions matching the branch pattern
	versions, err := h.distributor.ListVersions()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to get available versions: %v", err),
		})
	}

	// Find latest version for the specified branch
	var targetVersion string
	for i := len(versions) - 1; i >= 0; i-- {
		version := versions[i]
		if strings.Contains(version, branch+"-") {
			targetVersion = version
			break
		}
	}

	if targetVersion == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fmt.Sprintf("No versions found for branch: %s", branch),
		})
	}

	// Create update request
	updateRequest := UpdateRequest{
		TargetVersion: targetVersion,
		Force:         request.Force,
		Strategy:      request.Strategy,
		Metadata: map[string]string{
			"update_type": "git_branch",
			"target_branch": branch,
			"requested_via": "api_branch_endpoint",
			"timestamp": time.Now().Format(time.RFC3339),
		},
	}

	if request.Metadata != nil {
		for k, v := range request.Metadata {
			updateRequest.Metadata[k] = v
		}
	}

	// Validate and start update
	if err := h.ValidateUpdate(updateRequest.TargetVersion); err != nil && !request.Force {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("Update validation failed: %v", err),
		})
	}

	// Start update in background
	go func() {
		ctx := context.Background()
		if err := h.StartUpdate(ctx, updateRequest); err != nil {
			log.Printf("Branch update failed: %v", err)
		}
	}()

	return c.JSON(fiber.Map{
		"message": "Update to branch HEAD initiated",
		"target_branch": branch,
		"target_version": targetVersion,
		"strategy": request.Strategy,
		"update_type": "git_branch",
	})
}

// HandleListAvailableVersions lists available versions with Git metadata
func (h *Handler) HandleListAvailableVersions(c *fiber.Ctx) error {
	versions, err := h.distributor.ListVersions()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to list versions: %v", err),
		})
	}

	// Convert to Git version info format
	gitVersions := make([]GitVersionInfo, 0, len(versions))
	for _, version := range versions {
		gitInfo := GitVersionInfo{
			Version:   version,
			Available: true,
		}

		// Parse Git metadata from version string if available
		if parts := strings.Split(version, "-"); len(parts) >= 3 {
			gitInfo.GitBranch = parts[0]
			if len(parts) >= 4 {
				gitInfo.GitCommit = parts[1]
				gitInfo.BuildTime = strings.Join(parts[2:], "-")
			}
		}

		// Try to get artifact URL and checksum
		if artifactURL, binaryInfo, err := h.distributor.DownloadBinary(version, h.platform, h.architecture); err == nil {
			gitInfo.ArtifactURL = artifactURL
			if binaryInfo != nil {
				gitInfo.Checksum = binaryInfo.SHA256Hash
			}
		}

		gitVersions = append(gitVersions, gitInfo)
	}

	return c.JSON(fiber.Map{
		"versions": gitVersions,
		"current": GitVersionInfo{
			Version:   h.currentVersion,
			GitCommit: h.gitCommit,
			GitBranch: h.gitBranch,
			BuildTime: h.buildTime,
			Available: true,
		},
		"count": len(gitVersions),
	})
}

// HandleGitDeploy triggers a Git-based deployment (convenience endpoint)
func (h *Handler) HandleGitDeploy(c *fiber.Ctx) error {
	var request GitUpdateRequest
	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request format",
		})
	}

	// Set default strategy for deployment
	if request.Strategy == "" {
		request.Strategy = RollingUpdate
	}

	// Determine target based on request
	var targetVersion string
	var updateType string

	if request.Tag != "" {
		// Deploy specific tag
		targetVersion = request.Tag
		updateType = "git_tag"
	} else if request.Branch != "" {
		// Deploy specific branch HEAD
		versions, err := h.distributor.ListVersions()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": fmt.Sprintf("Failed to get available versions: %v", err),
			})
		}

		// Find latest version for branch
		for i := len(versions) - 1; i >= 0; i-- {
			version := versions[i]
			if strings.Contains(version, request.Branch+"-") {
				targetVersion = version
				break
			}
		}

		if targetVersion == "" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": fmt.Sprintf("No versions found for branch: %s", request.Branch),
			})
		}
		updateType = "git_branch"
	} else {
		// Deploy latest available
		versions, err := h.distributor.ListVersions()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": fmt.Sprintf("Failed to get available versions: %v", err),
			})
		}

		if len(versions) == 0 {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "No versions available",
			})
		}

		targetVersion = versions[len(versions)-1]
		updateType = "git_latest"
	}

	// Create update request
	updateRequest := UpdateRequest{
		TargetVersion: targetVersion,
		Force:         request.Force,
		Strategy:      request.Strategy,
		Metadata: map[string]string{
			"update_type": updateType,
			"requested_via": "api_git_deploy_endpoint",
			"timestamp": time.Now().Format(time.RFC3339),
			"deployment_trigger": "api",
		},
	}

	if request.Branch != "" {
		updateRequest.Metadata["target_branch"] = request.Branch
	}
	if request.Tag != "" {
		updateRequest.Metadata["target_tag"] = request.Tag
	}
	if request.Commit != "" {
		updateRequest.Metadata["target_commit"] = request.Commit
	}

	if request.Metadata != nil {
		for k, v := range request.Metadata {
			updateRequest.Metadata[k] = v
		}
	}

	// Validate and start deployment
	if err := h.ValidateUpdate(updateRequest.TargetVersion); err != nil && !request.Force {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("Deployment validation failed: %v", err),
			"target_version": targetVersion,
		})
	}

	// Start deployment in background
	go func() {
		ctx := context.Background()
		if err := h.StartUpdate(ctx, updateRequest); err != nil {
			log.Printf("Git deployment failed: %v", err)
		}
	}()

	return c.JSON(fiber.Map{
		"message": "Git-based deployment initiated",
		"target_version": targetVersion,
		"update_type": updateType,
		"strategy": request.Strategy,
		"deployment_id": fmt.Sprintf("%s-%d", updateType, time.Now().Unix()),
	})
}