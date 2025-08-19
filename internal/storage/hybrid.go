package storage

import (
	"fmt"
	"io"
)

// HybridClient implements StorageProvider with dual-write and fallback capabilities
type HybridClient struct {
	primary   StorageProvider
	secondary StorageProvider
	config    HybridConfig
}

// Ensure HybridClient implements StorageProvider
var _ StorageProvider = (*HybridClient)(nil)

// NewHybridClient creates a new hybrid storage client
func NewHybridClient(cfg Config) (*Client, error) {
	if !cfg.Hybrid.Enabled {
		return nil, fmt.Errorf("hybrid storage is not enabled")
	}

	var primary, secondary StorageProvider
	var err error

	// Create primary provider
	switch cfg.Hybrid.PrimaryProvider {
	case "seaweedfs":
		primaryCfg := cfg
		primaryCfg.Provider = "seaweedfs"
		client, err := NewSeaweedFSClient(primaryCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create primary SeaweedFS client: %w", err)
		}
		primary = (*SeaweedFSClient)(client)
	case "minio":
		primaryCfg := cfg
		primaryCfg.Provider = "minio"
		client, err := NewMinIOClient(primaryCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create primary MinIO client: %w", err)
		}
		primary = client
	default:
		return nil, fmt.Errorf("unsupported primary provider: %s", cfg.Hybrid.PrimaryProvider)
	}

	// Create secondary provider (opposite of primary)
	if cfg.Hybrid.DualWrite || cfg.Hybrid.FallbackRead {
		switch cfg.Hybrid.PrimaryProvider {
		case "seaweedfs":
			secondaryCfg := cfg
			secondaryCfg.Provider = "minio"
			client, err := NewMinIOClient(secondaryCfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create secondary MinIO client: %w", err)
			}
			secondary = client
		case "minio":
			secondaryCfg := cfg
			secondaryCfg.Provider = "seaweedfs"
			client, err := NewSeaweedFSClient(secondaryCfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create secondary SeaweedFS client: %w", err)
			}
			secondary = (*SeaweedFSClient)(client)
		}
	}

	hybrid := &HybridClient{
		primary:   primary,
		secondary: secondary,
		config:    cfg.Hybrid,
	}

	// Return as legacy Client type for compatibility
	return (*Client)(hybrid), nil
}

// StorageProvider interface implementation for HybridClient

func (h *HybridClient) PutObject(bucket, key string, body io.ReadSeeker, contentType string) (*PutObjectResult, error) {
	// Always try primary first
	result, err := h.primary.PutObject(bucket, key, body, contentType)
	if err == nil {
		// Primary succeeded
		if h.config.DualWrite && h.secondary != nil {
			// Reset body for secondary write
			if seeker, ok := body.(io.Seeker); ok {
				seeker.Seek(0, io.SeekStart)
			}
			
			// Try secondary write (don't fail if this fails)
			if _, secondaryErr := h.secondary.PutObject(bucket, key, body, contentType); secondaryErr != nil {
				fmt.Printf("Warning: Secondary write failed for %s/%s: %v\n", bucket, key, secondaryErr)
			} else {
				fmt.Printf("Dual-write successful for %s/%s\n", bucket, key)
			}
		}
		return result, nil
	}

	// Primary failed, try secondary if available
	if h.secondary != nil {
		fmt.Printf("Primary upload failed for %s/%s: %v, trying secondary\n", bucket, key, err)
		if seeker, ok := body.(io.Seeker); ok {
			seeker.Seek(0, io.SeekStart)
		}
		return h.secondary.PutObject(bucket, key, body, contentType)
	}

	return nil, err
}

func (h *HybridClient) GetObject(bucket, key string) (io.ReadCloser, error) {
	// Try primary first
	reader, err := h.primary.GetObject(bucket, key)
	if err == nil {
		return reader, nil
	}

	// Primary failed, try secondary if fallback is enabled
	if h.config.FallbackRead && h.secondary != nil {
		fmt.Printf("Primary read failed for %s/%s: %v, trying secondary\n", bucket, key, err)
		return h.secondary.GetObject(bucket, key)
	}

	return nil, err
}

func (h *HybridClient) ListObjects(bucket, prefix string) ([]ObjectInfo, error) {
	// Try primary first
	objects, err := h.primary.ListObjects(bucket, prefix)
	if err == nil {
		return objects, nil
	}

	// Primary failed, try secondary if fallback is enabled
	if h.config.FallbackRead && h.secondary != nil {
		fmt.Printf("Primary list failed for %s/%s: %v, trying secondary\n", bucket, prefix, err)
		return h.secondary.ListObjects(bucket, prefix)
	}

	return nil, err
}

func (h *HybridClient) UploadArtifactBundle(keyPrefix, artifactPath string) error {
	// Try primary first
	err := h.primary.UploadArtifactBundle(keyPrefix, artifactPath)
	if err == nil {
		// Primary succeeded
		if h.config.DualWrite && h.secondary != nil {
			// Try secondary upload (don't fail if this fails)
			if secondaryErr := h.secondary.UploadArtifactBundle(keyPrefix, artifactPath); secondaryErr != nil {
				fmt.Printf("Warning: Secondary artifact bundle upload failed for %s: %v\n", artifactPath, secondaryErr)
			} else {
				fmt.Printf("Dual-write artifact bundle successful for %s\n", artifactPath)
			}
		}
		return nil
	}

	// Primary failed, try secondary if available
	if h.secondary != nil {
		fmt.Printf("Primary artifact bundle upload failed for %s: %v, trying secondary\n", artifactPath, err)
		return h.secondary.UploadArtifactBundle(keyPrefix, artifactPath)
	}

	return err
}

func (h *HybridClient) VerifyUpload(key string) error {
	// Try primary first
	err := h.primary.VerifyUpload(key)
	if err == nil {
		return nil
	}

	// Primary failed, try secondary if fallback is enabled
	if h.config.FallbackRead && h.secondary != nil {
		fmt.Printf("Primary verification failed for %s: %v, trying secondary\n", key, err)
		return h.secondary.VerifyUpload(key)
	}

	return err
}

func (h *HybridClient) GetProviderType() string {
	primaryType := h.primary.GetProviderType()
	if h.secondary != nil {
		secondaryType := h.secondary.GetProviderType()
		return fmt.Sprintf("hybrid(%s+%s)", primaryType, secondaryType)
	}
	return fmt.Sprintf("hybrid(%s)", primaryType)
}

func (h *HybridClient) GetArtifactsBucket() string {
	return h.primary.GetArtifactsBucket()
}

// Hybrid-specific methods

// GetPrimaryProvider returns the primary storage provider
func (h *HybridClient) GetPrimaryProvider() StorageProvider {
	return h.primary
}

// GetSecondaryProvider returns the secondary storage provider
func (h *HybridClient) GetSecondaryProvider() StorageProvider {
	return h.secondary
}

// SwitchPrimary switches the primary and secondary providers
func (h *HybridClient) SwitchPrimary() {
	if h.secondary != nil {
		h.primary, h.secondary = h.secondary, h.primary
		fmt.Printf("Switched primary provider to %s\n", h.primary.GetProviderType())
	}
}

// GetConfig returns the hybrid configuration
func (h *HybridClient) GetConfig() HybridConfig {
	return h.config
}