package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ImageSizeInfo contains detailed size information for an artifact
type ImageSizeInfo struct {
	FilePath         string  `json:"file_path,omitempty"`         // For file-based artifacts
	DockerImage      string  `json:"docker_image,omitempty"`      // For container images
	SizeBytes        int64   `json:"size_bytes"`                  // Size in bytes
	SizeMB           float64 `json:"size_mb"`                     // Size in megabytes for easier reading
	CompressedSize   int64   `json:"compressed_size,omitempty"`   // For Docker images
	UncompressedSize int64   `json:"uncompressed_size,omitempty"` // For Docker images
	Lane             string  `json:"lane"`                        // Deployment lane
	MeasurementType  string  `json:"measurement_type"`            // "file" or "docker"
}

// GetImageSize measures the size of an artifact based on its type
func GetImageSize(imagePath, dockerImage, lane string) (*ImageSizeInfo, error) {
	if imagePath != "" {
		return getFileSize(imagePath, lane)
	} else if dockerImage != "" {
		return getDockerImageSize(dockerImage, lane)
	}

	return nil, fmt.Errorf("no artifact specified for size measurement")
}

// getFileSize measures the size of a file-based artifact
func getFileSize(filePath, lane string) (*ImageSizeInfo, error) {
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("artifact file not found: %s", filePath)
	}

	// Get file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	sizeBytes := fileInfo.Size()
	sizeMB := float64(sizeBytes) / (1024 * 1024)

	return &ImageSizeInfo{
		FilePath:        filePath,
		SizeBytes:       sizeBytes,
		SizeMB:          sizeMB,
		Lane:            lane,
		MeasurementType: "file",
	}, nil
}

// getDockerImageSize measures the size of a Docker container image
func getDockerImageSize(dockerImage, lane string) (*ImageSizeInfo, error) {
	// First try registry manifest (works in VPS without local Docker)
	if info, err := getDockerImageSizeFromRegistry(dockerImage); err == nil && info != nil {
		info.Lane = lane
		info.MeasurementType = "docker"
		info.DockerImage = dockerImage
		return info, nil
	}
	// Fallback to local docker CLI if available
	cmd := exec.Command("docker", "images", "--format", "{{.Size}}", dockerImage)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get docker image size: %w", err)
	}

	sizeStr := strings.TrimSpace(string(output))
	if sizeStr == "" {
		return nil, fmt.Errorf("docker image not found: %s", dockerImage)
	}

	// Parse size (format could be "123MB", "1.5GB", etc.)
	sizeBytes, err := parseDockerSize(sizeStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse docker image size: %w", err)
	}

	sizeMB := float64(sizeBytes) / (1024 * 1024)

	// For Docker images, try to get more detailed size information
	compressedSize, uncompressedSize := getDetailedDockerSize(dockerImage)

	return &ImageSizeInfo{
		DockerImage:      dockerImage,
		SizeBytes:        sizeBytes,
		SizeMB:           sizeMB,
		CompressedSize:   compressedSize,
		UncompressedSize: uncompressedSize,
		Lane:             lane,
		MeasurementType:  "docker",
	}, nil
}

// Minimal schema for Docker/OCI manifest v2
type manifestV2 struct {
	MediaType string `json:"mediaType"`
	Layers    []struct {
		Size      int64  `json:"size"`
		MediaType string `json:"mediaType"`
	} `json:"layers"`
}

// Parses dockerImage host/repo:tag and sums layer sizes from registry manifest
func getDockerImageSizeFromRegistry(dockerImage string) (*ImageSizeInfo, error) {
	slash := strings.Index(dockerImage, "/")
	if slash <= 0 || slash >= len(dockerImage)-1 {
		return nil, fmt.Errorf("unverifiable image tag: %s", dockerImage)
	}
	host := dockerImage[:slash]
	remainder := dockerImage[slash+1:]
	name := remainder
	ref := "latest"
	if at := strings.Index(remainder, "@"); at != -1 {
		name = remainder[:at]
		ref = remainder[at+1:]
	} else if colon := strings.LastIndex(remainder, ":"); colon != -1 {
		name = remainder[:colon]
		ref = remainder[colon+1:]
	}
	u := url.URL{Scheme: "https", Host: host, Path: "/v2/" + name + "/manifests/" + ref}
	req, _ := http.NewRequest("GET", u.String(), nil)
	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", "))
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Fallback to http (useful for tests or non-TLS registries)
		u.Scheme = "http"
		req.URL = &u
		resp, err = client.Do(req)
		if err != nil {
			return nil, err
		}
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("registry responded %d", resp.StatusCode)
	}
	var m manifestV2
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, err
	}
	var total int64
	for _, l := range m.Layers {
		if l.Size > 0 {
			total += l.Size
		}
	}
	if total <= 0 {
		return nil, fmt.Errorf("no layer sizes found")
	}
	return &ImageSizeInfo{
		SizeBytes: total,
		SizeMB:    float64(total) / (1024 * 1024),
	}, nil
}

// parseDockerSize converts Docker size format (e.g., "123MB", "1.5GB") to bytes
func parseDockerSize(sizeStr string) (int64, error) {
	sizeStr = strings.ToUpper(strings.TrimSpace(sizeStr))

	// Extract number and unit
	var num float64
	var unit string

	if strings.HasSuffix(sizeStr, "GB") {
		unit = "GB"
		numStr := strings.TrimSuffix(sizeStr, "GB")
		var err error
		num, err = strconv.ParseFloat(numStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid size format: %s", sizeStr)
		}
	} else if strings.HasSuffix(sizeStr, "MB") {
		unit = "MB"
		numStr := strings.TrimSuffix(sizeStr, "MB")
		var err error
		num, err = strconv.ParseFloat(numStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid size format: %s", sizeStr)
		}
	} else if strings.HasSuffix(sizeStr, "KB") {
		unit = "KB"
		numStr := strings.TrimSuffix(sizeStr, "KB")
		var err error
		num, err = strconv.ParseFloat(numStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid size format: %s", sizeStr)
		}
	} else if strings.HasSuffix(sizeStr, "B") {
		unit = "B"
		numStr := strings.TrimSuffix(sizeStr, "B")
		var err error
		num, err = strconv.ParseFloat(numStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid size format: %s", sizeStr)
		}
	} else {
		// Try parsing as bytes
		var err error
		num, err = strconv.ParseFloat(sizeStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid size format: %s", sizeStr)
		}
		unit = "B"
	}

	// Validate that the number is non-negative
	if num < 0 {
		return 0, fmt.Errorf("size cannot be negative: %s", sizeStr)
	}

	// Convert to bytes
	switch unit {
	case "GB":
		return int64(num * 1024 * 1024 * 1024), nil
	case "MB":
		return int64(num * 1024 * 1024), nil
	case "KB":
		return int64(num * 1024), nil
	case "B":
		return int64(num), nil
	default:
		return 0, fmt.Errorf("unknown size unit: %s", unit)
	}
}

// getDetailedDockerSize attempts to get more detailed size information for Docker images
func getDetailedDockerSize(dockerImage string) (compressedSize, uncompressedSize int64) {
	// Try to get detailed size using docker inspect
	cmd := exec.Command("docker", "inspect", "--format", "{{.Size}}", dockerImage)
	if output, err := cmd.Output(); err == nil {
		if size, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64); err == nil {
			uncompressedSize = size
		}
	}

	// For compressed size, we'd need registry API access, so for now return 0
	// In a production environment, this could query the registry
	compressedSize = 0

	return compressedSize, uncompressedSize
}

// LaneSizeLimit defines the size limits for each deployment lane
type LaneSizeLimit struct {
	Lane        string `json:"lane"`
	MaxSizeMB   int64  `json:"max_size_mb"`
	Description string `json:"description"`
}

// GetLaneSizeLimits returns the size limits for all deployment lanes
func GetLaneSizeLimits() []LaneSizeLimit {
	return []LaneSizeLimit{
		{
			Lane:        "A",
			MaxSizeMB:   50,
			Description: "Unikernel minimal - optimized for microsecond boot performance",
		},
		{
			Lane:        "B",
			MaxSizeMB:   100,
			Description: "Unikernel POSIX - enhanced runtime compatibility",
		},
		{
			Lane:        "C",
			MaxSizeMB:   500,
			Description: "OSv/JVM - accommodates Java runtime requirements",
		},
		{
			Lane:        "D",
			MaxSizeMB:   200,
			Description: "FreeBSD jail - efficient containerization",
		},
		{
			Lane:        "E",
			MaxSizeMB:   1024, // 1GB
			Description: "OCI container - standard container deployment",
		},
		{
			Lane:        "F",
			MaxSizeMB:   5120, // 5GB
			Description: "Full VM - balanced functionality and storage efficiency",
		},
	}
}

// GetLaneSizeLimit returns the size limit for a specific lane
func GetLaneSizeLimit(lane string) (LaneSizeLimit, error) {
	limits := GetLaneSizeLimits()

	for _, limit := range limits {
		if strings.EqualFold(limit.Lane, lane) {
			return limit, nil
		}
	}

	return LaneSizeLimit{}, fmt.Errorf("no size limit defined for lane: %s", lane)
}

// FormatSize formats a size in bytes to a human-readable string
func FormatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
