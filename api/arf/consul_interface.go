package arf

import (
	"context"
	"time"
)

// ConsulStoreInterface defines the interface for Consul-based transformation storage
type ConsulStoreInterface interface {
	// Core transformation status operations
	StoreTransformationStatus(ctx context.Context, id string, status *TransformationStatus) error
	GetTransformationStatus(ctx context.Context, id string) (*TransformationStatus, error)
	UpdateWorkflowStage(ctx context.Context, id string, stage string) error

	// Cleanup operations
	CleanupCompletedTransformations(ctx context.Context, maxAge time.Duration) error
	SetTransformationTTL(ctx context.Context, id string, ttl time.Duration) error
}
