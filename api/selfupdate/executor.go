package selfupdate

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/iw2rmb/ploy/internal/distribution"
)

// executeRollingUpdate performs a rolling update strategy
func (h *Handler) executeRollingUpdate(ctx context.Context, deploymentID string, request UpdateRequest, metadata map[string]string) error {
	log.Printf("Executing rolling update to version %s", request.TargetVersion)

	stageMeta := func(stage string) map[string]string {
		return mergeMetadata(metadata, map[string]string{"stage": stage})
	}

	binaryPath, info, err := h.distributor.DownloadBinary(request.TargetVersion, h.platform, h.architecture)
	if err != nil {
		errMeta := mergeMetadata(stageMeta("download"), map[string]string{"error": err.Error()})
		h.updateStatus(ctx, deploymentID, request, "failed", "Failed to download binary: "+err.Error(), 0, errMeta)
		return fmt.Errorf("failed to download binary: %w", err)
	}

	h.updateStatus(ctx, deploymentID, request, "downloading", "Binary downloaded successfully", 25, stageMeta("download"))

	if err := h.validateBinary(binaryPath, info); err != nil {
		errMeta := mergeMetadata(stageMeta("validate"), map[string]string{"error": err.Error()})
		h.updateStatus(ctx, deploymentID, request, "failed", "Binary validation failed: "+err.Error(), 25, errMeta)
		return fmt.Errorf("binary validation failed: %w", err)
	}

	h.updateStatus(ctx, deploymentID, request, "validating", "Binary validation completed", 50, stageMeta("validate"))

	if err := h.deployUpdate(ctx, deploymentID, binaryPath, info, request, stageMeta("deploy")); err != nil {
		errMeta := mergeMetadata(stageMeta("deploy"), map[string]string{"error": err.Error()})
		h.updateStatus(ctx, deploymentID, request, "failed", "Deployment failed: "+err.Error(), 50, errMeta)
		return fmt.Errorf("deployment failed: %w", err)
	}

	h.updateStatus(ctx, deploymentID, request, "completed", "Update completed successfully", 100, stageMeta("completed"))
	log.Printf("Rolling update to version %s completed successfully", request.TargetVersion)
	return nil
}

// executeBlueGreenUpdate performs a blue-green update strategy
func (h *Handler) executeBlueGreenUpdate(ctx context.Context, deploymentID string, request UpdateRequest, metadata map[string]string) error {
	// TODO: Implement blue-green update logic
	// This would involve:
	// 1. Starting new controller instances with new version
	// 2. Health checking new instances
	// 3. Switching traffic to new instances
	// 4. Stopping old instances
	return fmt.Errorf("blue-green update strategy not yet implemented")
}

// executeEmergencyUpdate performs an emergency update (fastest, least safe)
func (h *Handler) executeEmergencyUpdate(ctx context.Context, deploymentID string, request UpdateRequest, metadata map[string]string) error {
	log.Printf("Executing emergency update to version %s", request.TargetVersion)

	stageMeta := func(stage string) map[string]string {
		return mergeMetadata(metadata, map[string]string{"stage": stage})
	}

	binaryPath, info, err := h.distributor.DownloadBinary(request.TargetVersion, h.platform, h.architecture)
	if err != nil {
		errMeta := mergeMetadata(stageMeta("download"), map[string]string{"error": err.Error()})
		h.updateStatus(ctx, deploymentID, request, "failed", "Failed to download binary: "+err.Error(), 0, errMeta)
		return fmt.Errorf("failed to download binary: %w", err)
	}

	if err := h.quickValidateBinary(binaryPath, info); err != nil {
		errMeta := mergeMetadata(stageMeta("validate"), map[string]string{"error": err.Error()})
		h.updateStatus(ctx, deploymentID, request, "failed", "Quick validation failed: "+err.Error(), 25, errMeta)
		return fmt.Errorf("quick validation failed: %w", err)
	}

	if err := h.emergencyDeploy(ctx, binaryPath, info); err != nil {
		errMeta := mergeMetadata(stageMeta("deploy"), map[string]string{"error": err.Error()})
		h.updateStatus(ctx, deploymentID, request, "failed", "Emergency deployment failed: "+err.Error(), 50, errMeta)
		return fmt.Errorf("emergency deployment failed: %w", err)
	}

	h.updateStatus(ctx, deploymentID, request, "completed", "Emergency update completed", 100, stageMeta("completed"))
	log.Printf("Emergency update to version %s completed", request.TargetVersion)
	return nil
}

// deployUpdate performs the actual binary deployment
func (h *Handler) deployUpdate(ctx context.Context, deploymentID string, binaryPath string, info *distribution.BinaryInfo, request UpdateRequest, metadata map[string]string) error {
	// Create backup of current binary
	currentBinary := os.Args[0]
	backupPath := currentBinary + ".backup." + h.currentVersion

	if err := copyFile(currentBinary, backupPath); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Update status - backup created
	h.updateStatus(ctx, deploymentID, request, "deploying", "Backup created, deploying new binary", 75, metadata)

	// Use atomic replacement to avoid "text file busy" error
	if err := h.atomicBinaryReplacement(binaryPath, currentBinary, backupPath); err != nil {
		return fmt.Errorf("atomic binary replacement failed: %w", err)
	}

	// Signal for restart (will be handled by process manager)
	// In Nomad, this will trigger a restart of the allocation
	go func() {
		time.Sleep(2 * time.Second)
		log.Printf("Triggering restart for version %s", request.TargetVersion)
		os.Exit(0) // Graceful exit, Nomad will restart with new binary
	}()

	return nil
}

// emergencyDeploy performs immediate deployment without extensive coordination
func (h *Handler) emergencyDeploy(ctx context.Context, binaryPath string, info *distribution.BinaryInfo) error {
	currentBinary := os.Args[0]

	// Use atomic replacement even for emergency updates
	if err := h.atomicBinaryReplacement(binaryPath, currentBinary, ""); err != nil {
		return fmt.Errorf("emergency binary replacement failed: %w", err)
	}

	// Immediate restart
	os.Exit(0)
	return nil
}

// atomicBinaryReplacement performs atomic binary replacement to avoid "text file busy" errors
func (h *Handler) atomicBinaryReplacement(newBinaryPath, targetPath, backupPath string) error {
	// Strategy 1: Use rename-based atomic replacement
	tempPath := targetPath + ".new." + fmt.Sprintf("%d", time.Now().Unix())

	// Copy new binary to temporary location
	if err := copyFile(newBinaryPath, tempPath); err != nil {
		return fmt.Errorf("failed to create temporary binary: %w", err)
	}

	// Make temporary binary executable
	if err := os.Chmod(tempPath, 0755); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to make temporary binary executable: %w", err)
	}

	// Attempt atomic rename (this should work even if the target is in use)
	if err := os.Rename(tempPath, targetPath); err != nil {
		// If rename fails, try alternative strategies
		log.Printf("Atomic rename failed: %v, trying alternative strategies", err)
		_ = os.Remove(tempPath)
		return h.fallbackBinaryReplacement(newBinaryPath, targetPath, backupPath)
	}

	log.Printf("Atomic binary replacement successful via rename")
	return nil
}

// fallbackBinaryReplacement implements fallback strategies when atomic rename fails
func (h *Handler) fallbackBinaryReplacement(newBinaryPath, targetPath, backupPath string) error {
	// Strategy 2: Create update script and trigger external update
	return h.createUpdateScript(newBinaryPath, targetPath, backupPath)
}

// createUpdateScript creates an external update script to handle the replacement
func (h *Handler) createUpdateScript(newBinaryPath, targetPath, backupPath string) error {
	scriptPath := targetPath + ".update.sh"

	// Create update script content
	scriptContent := fmt.Sprintf(`#!/bin/bash
set -e

NEW_BINARY="%s"
TARGET_BINARY="%s"
BACKUP_BINARY="%s"
PID=%d

# Wait for current process to exit
echo "Waiting for process $PID to exit..."
while kill -0 $PID 2>/dev/null; do
    sleep 1
done

# Perform the update
echo "Replacing binary..."
if [ -n "$BACKUP_BINARY" ] && [ -f "$TARGET_BINARY" ]; then
    cp "$TARGET_BINARY" "$BACKUP_BINARY" || true
fi

cp "$NEW_BINARY" "$TARGET_BINARY"
chmod 755 "$TARGET_BINARY"

echo "Binary update completed"
rm -f "$0"  # Remove this script
`, newBinaryPath, targetPath, backupPath, os.Getpid())

	// Write script to file
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		return fmt.Errorf("failed to create update script: %w", err)
	}

	// Launch update script in background
	go func() {
		time.Sleep(1 * time.Second) // Give some time for response to be sent
		log.Printf("Executing external update script: %s", scriptPath)

		// Execute the script
		_ = exec.Command("/bin/bash", scriptPath).Start()

		// Exit current process to allow script to replace binary
		time.Sleep(2 * time.Second)
		os.Exit(0)
	}()

	log.Printf("Update script created and scheduled: %s", scriptPath)
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

// Consul coordination helpers removed in favour of JetStream work queue.
