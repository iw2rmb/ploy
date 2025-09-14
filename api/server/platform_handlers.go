package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/platform"
	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
)

// handlePlatformDeploy handles platform service deployment
func (s *Server) handlePlatformDeploy(c *fiber.Ctx) error {
	// Use factory pattern to get unified storage interface
	storage, err := s.resolveUnifiedStorage()
	if err != nil {
		return c.Status(503).JSON(fiber.Map{
			"error":   "Storage initialization failed",
			"details": err.Error(),
		})
	}

	handler := platform.NewHandlerWithStorage(storage, s.dependencies.EnvStore)
	return handler.DeployPlatformService(c)
}

// handlePlatformStatus handles platform service status requests
func (s *Server) handlePlatformStatus(c *fiber.Ctx) error {
	// Use factory pattern to get unified storage interface
	storage, err := s.resolveUnifiedStorage()
	if err != nil {
		return c.Status(503).JSON(fiber.Map{
			"error":   "Storage initialization failed",
			"details": err.Error(),
		})
	}

	handler := platform.NewHandlerWithStorage(storage, s.dependencies.EnvStore)
	return handler.GetPlatformStatus(c)
}

// handlePlatformRollback handles platform service rollback
func (s *Server) handlePlatformRollback(c *fiber.Ctx) error {
	serviceName := c.Params("service")
	targetVersion := c.Query("version")

	if targetVersion == "" {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Target version is required",
			"details": "Provide version parameter",
		})
	}

	// TODO: Implement platform rollback logic
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Platform service rollback initiated",
		"service": serviceName,
		"version": targetVersion,
	})
}

// handlePlatformRemove handles platform service removal
func (s *Server) handlePlatformRemove(c *fiber.Ctx) error {
	serviceName := c.Params("service")

	// TODO: Implement platform service removal
	// This should:
	// 1. Stop the Nomad job
	// 2. Clean up storage artifacts
	// 3. Remove DNS entries
	// 4. Clean up certificates

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Platform service removal initiated",
		"service": serviceName,
	})
}

// handlePlatformLogs handles platform service log retrieval
func (s *Server) handlePlatformLogs(c *fiber.Ctx) error {
	serviceName := c.Params("service")
	lines := c.QueryInt("lines", 200)
	follow := c.QueryBool("follow", false)

	// Derive Nomad job and task names for platform services
	jobName := serviceName
	taskName := serviceName
	switch serviceName {
	case "api":
		jobName = "ploy-api"
		taskName = "api"
	case "traefik":
		jobName = "traefik-system"
		taskName = "traefik"
	default:
		if !strings.HasPrefix(jobName, "ploy-") {
			jobName = "ploy-" + jobName
		}
	}

	monitor := orchestration.NewHealthMonitor()
	allocs, err := monitor.GetJobAllocations(jobName)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to retrieve allocations",
			"details": err.Error(),
		})
	}
	runningID := ""
	for _, a := range allocs {
		if a.ClientStatus == "running" {
			runningID = a.ID
			break
		}
	}
	// Fallback: use job manager to list allocations if SDK path found none
	if runningID == "" {
		type allocInfo struct {
			ID           string `json:"ID"`
			ClientStatus string `json:"ClientStatus"`
		}
		ctxList, cancelList := context.WithTimeout(c.Context(), 15*time.Second)
		defer cancelList()
		cmdList := exec.CommandContext(ctxList, "/opt/hashicorp/bin/nomad-job-manager.sh", "allocs", "--job", jobName, "--format", "json")
		var outList bytes.Buffer
		cmdList.Stdout = &outList
		cmdList.Stderr = &outList
		if err := cmdList.Run(); err == nil {
			var arr []allocInfo
			if json.Unmarshal(outList.Bytes(), &arr) == nil {
				for _, ai := range arr {
					if ai.ClientStatus == "running" {
						runningID = ai.ID
						break
					}
				}
				if runningID == "" && len(arr) > 0 {
					runningID = arr[0].ID
				}
			}
		}
	}
	if runningID == "" {
		return c.JSON(fiber.Map{
			"service":         serviceName,
			"job_name":        jobName,
			"logs":            "No running allocations found",
			"lines_requested": lines,
		})
	}

	// Prefer the VPS job manager wrapper when available for consistent log retrieval
	wrapper := "/opt/hashicorp/bin/nomad-job-manager.sh"
	ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
	defer cancel()
	// Build command arguments safely
	args := []string{"logs", "--alloc-id", runningID}
	if taskName != "" {
		args = append(args, "--task", taskName)
	}
	if lines > 0 {
		args = append(args, "--lines", fmt.Sprintf("%d", lines))
	}
	if follow {
		args = append(args, "--follow")
	}
	cmd := exec.CommandContext(ctx, wrapper, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		// Fallback: return basic info rather than failing hard
		return c.Status(500).JSON(fiber.Map{
			"error":    "Failed to fetch logs via job manager",
			"details":  err.Error(),
			"output":   out.String(),
			"service":  serviceName,
			"job_name": jobName,
		})
	}
	c.Set("Content-Type", "text/plain; charset=utf-8")
	return c.Send(out.Bytes())
}
