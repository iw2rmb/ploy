package build

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/config"
	"github.com/iw2rmb/ploy/internal/security"
	"github.com/iw2rmb/ploy/internal/utils"
)

// makeBuildResponse assembles the JSON response for a build/deploy request.
func makeBuildResponse(lane, imagePath, dockerImage, namespace string, appType config.AppType, buildStart time.Time, sizeInfo *utils.ImageSizeInfo, imageSizeMB float64, builderJobName string, appName, sha string, registryEndpoint, registryProject string, scanResult *security.ScanResult, scanner *security.VulnerabilityScanner) fiber.Map {
	resp := fiber.Map{
		"status":      "deployed",
		"lane":        lane,
		"image":       imagePath,
		"dockerImage": dockerImage,
		"namespace":   namespace,
		"appType":     string(appType),
		"build": fiber.Map{
			"start":       buildStart.Format(time.RFC3339),
			"end":         time.Now().Format(time.RFC3339),
			"duration_ms": time.Since(buildStart).Milliseconds(),
		},
	}
	var sizeBytes int64
	if sizeInfo != nil {
		sizeBytes = sizeInfo.SizeBytes
	}
	resp["imageSize"] = fiber.Map{
		"mb":    imageSizeMB,
		"bytes": sizeBytes,
	}
	// Builder info
	if lane == "D" && builderJobName != "" {
		logsKey := fmt.Sprintf("build-logs/%s.log", builderJobName)
		resp["builder"] = fiber.Map{
			"job":      builderJobName,
			"logs_key": logsKey,
			"logs_url": buildLogsURL(logsKey),
			"log_path": fmt.Sprintf("/opt/ploy/build-logs/%s.log", builderJobName),
		}
	}
	// Verify container push
	if dockerImage != "" {
		vr := verifyOCIPush(dockerImage)
		resp["pushVerification"] = fiber.Map{
			"ok":      vr.OK,
			"status":  vr.Status,
			"digest":  vr.Digest,
			"message": vr.Message,
		}
		resp["registry"] = fiber.Map{
			"endpoint": registryEndpoint,
			"project":  registryProject,
			"imageTag": dockerImage,
		}
	}
	// Security summary
	if scanResult != nil && scanner != nil {
		resp["security"] = fiber.Map{
			"vulnScanPassed":     scanResult.Passed,
			"vulnerabilityCount": scanResult.VulnCount,
			"criticalCount":      scanResult.CriticalCount,
			"highCount":          scanResult.HighCount,
			"severityThreshold":  scanner.GetSeverityThreshold(),
		}
	}
	return resp
}
