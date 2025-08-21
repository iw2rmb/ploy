package selfupdate

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/ploy/ploy/internal/distribution"
)

// createUpdateSession creates a Consul session for update coordination
func (h *Handler) createUpdateSession(ctx context.Context, request UpdateRequest) (string, error) {
	session := h.consulClient.Session()

	// Create session for coordination
	sessionEntry := &api.SessionEntry{
		Name:      "ploy-controller-update",
		TTL:       h.sessionTTL.String(),
		Behavior:  api.SessionBehaviorRelease,
		LockDelay: 5 * time.Second,
	}

	sessionID, _, err := session.Create(sessionEntry, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	// Initialize update status
	status := UpdateStatus{
		Status:         "preparing",
		CurrentVersion: h.currentVersion,
		TargetVersion:  request.TargetVersion,
		Progress:       0,
		StartedAt:      time.Now(),
		Metadata:       request.Metadata,
	}

	kv := h.consulClient.KV()
	statusData, _ := toJSON(status)

	// Store update status in Consul
	kvPair := &api.KVPair{
		Key:     h.leaderPrefix + "/status",
		Value:   statusData,
		Session: sessionID,
	}

	acquired, _, err := kv.Acquire(kvPair, nil)
	if err != nil {
		return "", fmt.Errorf("failed to acquire update lock: %w", err)
	}

	if !acquired {
		return "", fmt.Errorf("another update is already in progress")
	}

	log.Printf("Created update session %s for version %s", sessionID, request.TargetVersion)
	return sessionID, nil
}

// executeRollingUpdate performs a rolling update strategy
func (h *Handler) executeRollingUpdate(ctx context.Context, sessionID string, request UpdateRequest) error {
	log.Printf("Executing rolling update to version %s", request.TargetVersion)

	// Download and validate target binary
	binaryPath, info, err := h.distributor.DownloadBinary(request.TargetVersion, h.platform, h.architecture)
	if err != nil {
		h.updateStatus(sessionID, "failed", "Failed to download binary: "+err.Error(), 0)
		return fmt.Errorf("failed to download binary: %w", err)
	}

	// Update status - binary downloaded
	h.updateStatus(sessionID, "downloading", "Binary downloaded successfully", 25)

	// Validate binary integrity
	if err := h.validateBinary(binaryPath, info); err != nil {
		h.updateStatus(sessionID, "failed", "Binary validation failed: "+err.Error(), 25)
		return fmt.Errorf("binary validation failed: %w", err)
	}

	// Update status - validation complete
	h.updateStatus(sessionID, "validating", "Binary validation completed", 50)

	// Execute staged deployment
	if err := h.deployUpdate(ctx, sessionID, binaryPath, info, request); err != nil {
		h.updateStatus(sessionID, "failed", "Deployment failed: "+err.Error(), 50)
		return fmt.Errorf("deployment failed: %w", err)
	}

	// Update status - deployment complete
	h.updateStatus(sessionID, "completed", "Update completed successfully", 100)

	log.Printf("Rolling update to version %s completed successfully", request.TargetVersion)
	return nil
}

// executeBlueGreenUpdate performs a blue-green update strategy
func (h *Handler) executeBlueGreenUpdate(ctx context.Context, sessionID string, request UpdateRequest) error {
	// TODO: Implement blue-green update logic
	// This would involve:
	// 1. Starting new controller instances with new version
	// 2. Health checking new instances
	// 3. Switching traffic to new instances
	// 4. Stopping old instances
	return fmt.Errorf("blue-green update strategy not yet implemented")
}

// executeEmergencyUpdate performs an emergency update (fastest, least safe)
func (h *Handler) executeEmergencyUpdate(ctx context.Context, sessionID string, request UpdateRequest) error {
	log.Printf("Executing emergency update to version %s", request.TargetVersion)

	// Download binary
	binaryPath, info, err := h.distributor.DownloadBinary(request.TargetVersion, h.platform, h.architecture)
	if err != nil {
		h.updateStatus(sessionID, "failed", "Failed to download binary: "+err.Error(), 0)
		return fmt.Errorf("failed to download binary: %w", err)
	}

	// Minimal validation for emergency updates
	if err := h.quickValidateBinary(binaryPath, info); err != nil {
		h.updateStatus(sessionID, "failed", "Quick validation failed: "+err.Error(), 25)
		return fmt.Errorf("quick validation failed: %w", err)
	}

	// Immediate deployment
	if err := h.emergencyDeploy(ctx, binaryPath, info); err != nil {
		h.updateStatus(sessionID, "failed", "Emergency deployment failed: "+err.Error(), 50)
		return fmt.Errorf("emergency deployment failed: %w", err)
	}

	h.updateStatus(sessionID, "completed", "Emergency update completed", 100)
	log.Printf("Emergency update to version %s completed", request.TargetVersion)
	return nil
}

// deployUpdate performs the actual binary deployment
func (h *Handler) deployUpdate(ctx context.Context, sessionID string, binaryPath string, info *distribution.BinaryInfo, request UpdateRequest) error {
	// Create backup of current binary
	currentBinary := os.Args[0]
	backupPath := currentBinary + ".backup." + h.currentVersion

	if err := copyFile(currentBinary, backupPath); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Update status - backup created
	h.updateStatus(sessionID, "deploying", "Backup created, deploying new binary", 75)

	// Replace current binary
	if err := copyFile(binaryPath, currentBinary); err != nil {
		// Restore backup on failure
		copyFile(backupPath, currentBinary)
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	// Make executable
	if err := os.Chmod(currentBinary, 0755); err != nil {
		return fmt.Errorf("failed to make binary executable: %w", err)
	}

	// Signal for restart (will be handled by process manager)
	// In Nomad, this will trigger a restart of the allocation
	go func() {
		time.Sleep(2 * time.Second)
		log.Printf("Triggering restart for version %s", request.TargetVersion)
		os.Exit(0) // Graceful exit, Nomad will restart
	}()

	return nil
}

// emergencyDeploy performs immediate deployment without extensive coordination
func (h *Handler) emergencyDeploy(ctx context.Context, binaryPath string, info *distribution.BinaryInfo) error {
	currentBinary := os.Args[0]

	// Direct replacement
	if err := copyFile(binaryPath, currentBinary); err != nil {
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	if err := os.Chmod(currentBinary, 0755); err != nil {
		return fmt.Errorf("failed to make binary executable: %w", err)
	}

	// Immediate restart
	os.Exit(0)
	return nil
}

// validateBinary performs comprehensive binary validation
func (h *Handler) validateBinary(binaryPath string, info *distribution.BinaryInfo) error {
	// Check file exists and is executable
	stat, err := os.Stat(binaryPath)
	if err != nil {
		return fmt.Errorf("binary not accessible: %w", err)
	}

	if stat.Mode()&0111 == 0 {
		return fmt.Errorf("binary is not executable")
	}

	// Validate binary info
	if err := info.Validate(); err != nil {
		return fmt.Errorf("binary info validation failed: %w", err)
	}

	// Check platform compatibility
	if !info.IsCompatibleWith(h.platform, h.architecture) {
		return fmt.Errorf("binary not compatible with %s/%s", h.platform, h.architecture)
	}

	return nil
}

// quickValidateBinary performs minimal validation for emergency updates
func (h *Handler) quickValidateBinary(binaryPath string, info *distribution.BinaryInfo) error {
	// Just check if file exists and is executable
	stat, err := os.Stat(binaryPath)
	if err != nil {
		return fmt.Errorf("binary not accessible: %w", err)
	}

	if stat.Mode()&0111 == 0 {
		return fmt.Errorf("binary is not executable")
	}

	return nil
}

// updateStatus updates the update status in Consul
func (h *Handler) updateStatus(sessionID, status, message string, progress int) {
	kv := h.consulClient.KV()

	updateStatus := UpdateStatus{
		Status:         status,
		CurrentVersion: h.currentVersion,
		Progress:       progress,
		Message:        message,
		StartedAt:      time.Now(),
	}

	if status == "completed" || status == "failed" {
		updateStatus.CompletedAt = time.Now()
	}

	statusData, _ := toJSON(updateStatus)
	kvPair := &api.KVPair{
		Key:     h.leaderPrefix + "/status",
		Value:   statusData,
		Session: sessionID,
	}

	kv.Put(kvPair, nil)
}

// cleanupSession cleans up the update session
func (h *Handler) cleanupSession(sessionID string) {
	session := h.consulClient.Session()
	session.Destroy(sessionID, nil)

	kv := h.consulClient.KV()
	kv.Delete(h.leaderPrefix+"/status", nil)

	log.Printf("Cleaned up update session %s", sessionID)
}