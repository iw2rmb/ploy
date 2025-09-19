package storage

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (h *HealthChecker) checkConnectivity(ctx context.Context, result *HealthCheckResult) {
	start := time.Now()
	checkResult := CheckResult{Status: HealthStatusHealthy}

	if seaweedClient, ok := h.client.(*SeaweedFSClient); ok {
		_, err := h.client.ListObjects("", "")
		duration := time.Since(start)
		checkResult.Duration = duration

		if err != nil {
			if _, assignErr := seaweedClient.TestVolumeAssignment(); assignErr != nil {
				checkResult.Status = HealthStatusUnhealthy
				checkResult.Message = "SeaweedFS services unreachable"
				checkResult.Error = err.Error()
				result.Status = HealthStatusUnhealthy
			} else {
				checkResult.Status = HealthStatusDegraded
				checkResult.Message = "Master reachable, filer may have directory issues"
				checkResult.Error = err.Error()
				if result.Status == HealthStatusHealthy {
					result.Status = HealthStatusDegraded
				}
			}
		} else {
			checkResult.Message = fmt.Sprintf("SeaweedFS services responsive (%.2fms)", float64(duration.Nanoseconds())/1e6)
		}
	} else {
		_, err := h.client.ListObjects("", "")
		duration := time.Since(start)
		checkResult.Duration = duration

		if err != nil {
			checkResult.Status = HealthStatusUnhealthy
			checkResult.Message = "Storage service unreachable"
			checkResult.Error = err.Error()
			result.Status = HealthStatusUnhealthy
		} else {
			checkResult.Message = fmt.Sprintf("Storage service responsive (%.2fms)", float64(duration.Nanoseconds())/1e6)
		}
	}

	result.Checks["connectivity"] = checkResult
}

func (h *HealthChecker) checkConfiguration(ctx context.Context, result *HealthCheckResult) {
	checkResult := CheckResult{Status: HealthStatusHealthy}

	providerType := h.client.GetProviderType()
	if providerType == "" {
		checkResult.Status = HealthStatusUnhealthy
		checkResult.Message = "Storage provider type not configured"
	} else {
		bucket := h.client.GetArtifactsBucket()
		if bucket == "" {
			checkResult.Status = HealthStatusDegraded
			checkResult.Message = "Artifacts bucket not configured"
		} else {
			checkResult.Message = fmt.Sprintf("Configuration valid (provider: %s, bucket: %s)", providerType, bucket)
		}
	}

	if checkResult.Status != HealthStatusHealthy && result.Status == HealthStatusHealthy {
		result.Status = checkResult.Status
	}

	result.Checks["configuration"] = checkResult
}

func (h *HealthChecker) checkStorageOperations(ctx context.Context, result *HealthCheckResult) {
	start := time.Now()
	checkResult := CheckResult{Status: HealthStatusHealthy}
	bucket := h.client.GetArtifactsBucket()

	if seaweedClient, ok := h.client.(*SeaweedFSClient); ok {
		assignment, assignErr := seaweedClient.TestVolumeAssignment()
		if assignErr != nil {
			checkResult.Status = HealthStatusUnhealthy
			checkResult.Message = "Volume assignment failed - storage unavailable"
			checkResult.Error = assignErr.Error()
			result.Status = HealthStatusUnhealthy
		} else if fid, ok := assignment["fid"].(string); ok && fid != "" {
			if url, ok := assignment["url"].(string); ok && url != "" {
				checkResult.Message = fmt.Sprintf("Volume assignment successful (FID: %s, URL: %s)", fid, url)
				testKey := fmt.Sprintf("health_%d", time.Now().Unix())
				testData := strings.NewReader("healthcheck")

				if _, uploadErr := h.client.PutObject(bucket, testKey, testData, "text/plain"); uploadErr != nil {
					if strings.Contains(uploadErr.Error(), "409 Conflict") || strings.Contains(uploadErr.Error(), "failed to create directory") {
						checkResult.Status = HealthStatusDegraded
						checkResult.Message = "Volume assignment working, directory creation issues (expected for SeaweedFS)"
					} else {
						checkResult.Status = HealthStatusDegraded
						checkResult.Message = fmt.Sprintf("Volume assignment working, upload failed: %s", uploadErr.Error())
					}
					if result.Status == HealthStatusHealthy {
						result.Status = HealthStatusDegraded
					}
				} else {
					duration := time.Since(start)
					checkResult.Message = fmt.Sprintf("Storage operations fully successful (%.2fms)", float64(duration.Nanoseconds())/1e6)
				}
			} else {
				checkResult.Status = HealthStatusDegraded
				checkResult.Message = "Volume assignment incomplete - missing URL"
				if result.Status == HealthStatusHealthy {
					result.Status = HealthStatusDegraded
				}
			}
		} else {
			checkResult.Status = HealthStatusDegraded
			checkResult.Message = "Volume assignment incomplete - missing File ID"
			if result.Status == HealthStatusHealthy {
				result.Status = HealthStatusDegraded
			}
		}
	} else {
		testKey := fmt.Sprintf("health_%d.txt", time.Now().Unix())
		testData := strings.NewReader(strings.Repeat("A", int(h.config.TestObjectSize)))

		_, uploadErr := h.client.PutObject(bucket, testKey, testData, "text/plain")
		if uploadErr != nil {
			checkResult.Status = HealthStatusUnhealthy
			checkResult.Message = "Upload operation failed"
			checkResult.Error = uploadErr.Error()
			result.Status = HealthStatusUnhealthy
		} else {
			reader, downloadErr := h.client.GetObject(bucket, testKey)
			if downloadErr != nil {
				checkResult.Status = HealthStatusDegraded
				checkResult.Message = "Download operation failed"
				checkResult.Error = downloadErr.Error()
				if result.Status == HealthStatusHealthy {
					result.Status = HealthStatusDegraded
				}
			} else {
				_ = reader.Close()
				duration := time.Since(start)
				checkResult.Message = fmt.Sprintf("Storage operations successful (%.2fms)", float64(duration.Nanoseconds())/1e6)
			}
		}
	}

	checkResult.Duration = time.Since(start)
	result.Checks["storage_operations"] = checkResult
}
