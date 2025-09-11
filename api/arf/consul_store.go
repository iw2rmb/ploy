package arf

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/consul/api"
)

// ConsulHealingStore provides persistent storage for transformation status using Consul KV
type ConsulHealingStore struct {
	client    *api.Client
	keyPrefix string
}

// NewConsulHealingStore creates a new Consul-backed healing store
func NewConsulHealingStore(client *api.Client, keyPrefix string) *ConsulHealingStore {
	return &ConsulHealingStore{
		client:    client,
		keyPrefix: keyPrefix,
	}
}

// StoreTransformationStatus stores or updates a transformation status in Consul
func (c *ConsulHealingStore) StoreTransformationStatus(ctx context.Context, id string, status *TransformationStatus) error {
	kv := c.client.KV()

	// Serialize status to JSON
	data, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal transformation status: %w", err)
	}

	// Store in Consul
	key := fmt.Sprintf("%s/%s/status", c.keyPrefix, id)
	kvPair := &api.KVPair{
		Key:   key,
		Value: data,
	}

	_, err = kv.Put(kvPair, nil)
	if err != nil {
		return fmt.Errorf("failed to store transformation status: %w", err)
	}

	return nil
}

// GetTransformationStatus retrieves a transformation status from Consul
func (c *ConsulHealingStore) GetTransformationStatus(ctx context.Context, id string) (*TransformationStatus, error) {
	kv := c.client.KV()

	// Get from Consul
	key := fmt.Sprintf("%s/%s/status", c.keyPrefix, id)
	kvPair, _, err := kv.Get(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get transformation status: %w", err)
	}

	if kvPair == nil {
		return nil, nil // Not found
	}

	// Deserialize
	var status TransformationStatus
	if err := json.Unmarshal(kvPair.Value, &status); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transformation status: %w", err)
	}

	return &status, nil
}

// UpdateWorkflowStage updates only the workflow stage of a transformation
func (c *ConsulHealingStore) UpdateWorkflowStage(ctx context.Context, id string, stage string) error {
	// Get current status
	status, err := c.GetTransformationStatus(ctx, id)
	if err != nil {
		return err
	}
	if status == nil {
		return fmt.Errorf("transformation %s not found", id)
	}

	// Update stage
	status.WorkflowStage = stage

	// Store back
	return c.StoreTransformationStatus(ctx, id, status)
}

// AddHealingAttempt adds a healing attempt to the transformation hierarchy
/*func (c *ConsulHealingStore) AddHealingAttempt(ctx context.Context, rootID, attemptPath string, attempt *HealingAttempt) error {
	// Get current status
	status, err := c.GetTransformationStatus(ctx, rootID)
	if err != nil {
		return err
	}
	if status == nil {
		return fmt.Errorf("transformation %s not found", rootID)
	}

	// Auto-generate path if not provided
	if attemptPath == "" && attempt != nil {
		parentPath := attempt.ParentAttempt
		attemptPath = GenerateAttemptPath(rootID, parentPath, status.Children)
		attempt.AttemptPath = attemptPath
	}

	// Validate the attempt path
	if err := ValidateAttemptPath(attemptPath); err != nil {
		return fmt.Errorf("invalid attempt path: %w", err)
	}

	// Ensure the parent exists if this is a child attempt
	parentPath := GetParentPath(attemptPath)
	if parentPath != "" && !IsValidParent(status.Children, parentPath) {
		return fmt.Errorf("parent path %s does not exist", parentPath)
	}

	// Add attempt to the correct position in the tree
	if err := c.addAttemptToTree(&status.Children, attemptPath, attempt); err != nil {
		return err
	}

	// Update counts
	status.TotalHealingAttempts++
	if attempt.Status == "in_progress" || attempt.Status == "pending" {
		status.ActiveHealingCount++
	}

	// Store back
	return c.StoreTransformationStatus(ctx, rootID, status)
}*/

// addAttemptToTree recursively adds an attempt to the correct position in the tree
/*func (c *ConsulHealingStore) addAttemptToTree(children *[]HealingAttempt, path string, attempt *HealingAttempt) error {
	// Parse path (e.g., "1.2.3")
	parts := strings.Split(path, ".")

	if len(parts) == 1 {
		// Direct child - add to this level
		*children = append(*children, *attempt)
		return nil
	}

	// Find parent attempt
	parentPath := strings.Join(parts[:len(parts)-1], ".")
	for i := range *children {
		if (*children)[i].AttemptPath == parentPath {
			// Found parent - add as child
			(*children)[i].Children = append((*children)[i].Children, *attempt)
			return nil
		}
		// Recursively search in children
		if err := c.addAttemptToTree(&(*children)[i].Children, path, attempt); err == nil {
			return nil
		}
	}

	return fmt.Errorf("parent path %s not found", parentPath)
}*/

// UpdateHealingAttempt updates an existing healing attempt
/*func (c *ConsulHealingStore) UpdateHealingAttempt(ctx context.Context, rootID, attemptPath string, attempt *HealingAttempt) error {
	// Get current status
	status, err := c.GetTransformationStatus(ctx, rootID)
	if err != nil {
		return err
	}
	if status == nil {
		return fmt.Errorf("transformation %s not found", rootID)
	}

	// Update attempt in the tree
	if err := c.updateAttemptInTree(&status.Children, attemptPath, attempt); err != nil {
		return err
	}

	// Update counts if status changed
	if attempt.Status == "completed" || attempt.Status == "failed" {
		status.ActiveHealingCount--
		if attempt.Result == "success" {
			// Count successful heals at any depth
			c.countSuccessfulHeals(status)
		}
	}

	// Store back
	return c.StoreTransformationStatus(ctx, rootID, status)
}*/

// updateAttemptInTree recursively updates an attempt in the tree
/*func (c *ConsulHealingStore) updateAttemptInTree(children *[]HealingAttempt, path string, attempt *HealingAttempt) error {
	for i := range *children {
		if (*children)[i].AttemptPath == path {
			// Found - update
			(*children)[i] = *attempt
			return nil
		}
		// Recursively search in children
		if err := c.updateAttemptInTree(&(*children)[i].Children, path, attempt); err == nil {
			return nil
		}
	}
	return fmt.Errorf("attempt path %s not found", path)
}*/

// GetHealingTree retrieves the complete healing tree for a transformation
/*func (c *ConsulHealingStore) GetHealingTree(ctx context.Context, rootID string) (*HealingTree, error) {
	status, err := c.GetTransformationStatus(ctx, rootID)
	if err != nil {
		return nil, err
	}
	if status == nil {
		return nil, nil
	}

	tree := &HealingTree{
		RootTransformID: rootID,
		Attempts:        status.Children,
		ActiveAttempts:  []string{},
		TotalAttempts:   0,
		SuccessfulHeals: 0,
		FailedHeals:     0,
		MaxDepth:        0,
	}

	// Calculate metrics
	c.calculateTreeMetrics(tree, status.Children, 1)

	return tree, nil
}*/

// calculateTreeMetrics removed

// GetActiveHealingAttempts returns paths of all active healing attempts
/*func (c *ConsulHealingStore) GetActiveHealingAttempts(ctx context.Context, rootID string) ([]string, error) {
    return []string{}, nil
}*/

// CleanupCompletedTransformations removes transformations older than maxAge
func (c *ConsulHealingStore) CleanupCompletedTransformations(ctx context.Context, maxAge time.Duration) error {
	kv := c.client.KV()

	// List all transformations
	keys, _, err := kv.Keys(c.keyPrefix+"/", "/", nil)
	if err != nil {
		return fmt.Errorf("failed to list transformation keys: %w", err)
	}

	now := time.Now()
	for _, key := range keys {
		// Skip non-status keys
		if !strings.HasSuffix(key, "/status") {
			continue
		}

		// Get transformation
		kvPair, _, err := kv.Get(key, nil)
		if err != nil {
			continue // Skip on error
		}
		if kvPair == nil {
			continue
		}

		var status TransformationStatus
		if err := json.Unmarshal(kvPair.Value, &status); err != nil {
			continue // Skip on error
		}

		// Check if completed and old enough
		if status.Status == "completed" && !status.EndTime.IsZero() {
			age := now.Sub(status.EndTime)
			if age > maxAge {
				// Delete this transformation
				transformID := strings.TrimPrefix(strings.TrimSuffix(key, "/status"), c.keyPrefix+"/")
				if err := c.deleteTransformation(ctx, transformID); err != nil {
					// Log error but continue
					fmt.Printf("Failed to delete transformation %s: %v\n", transformID, err)
				}
			}
		}
	}

	return nil
}

// deleteTransformation deletes all data for a transformation
func (c *ConsulHealingStore) deleteTransformation(ctx context.Context, id string) error {
	kv := c.client.KV()

	// Delete all keys for this transformation
	prefix := fmt.Sprintf("%s/%s/", c.keyPrefix, id)
	_, err := kv.DeleteTree(prefix, nil)
	if err != nil {
		return fmt.Errorf("failed to delete transformation %s: %w", id, err)
	}

	return nil
}

// SetTransformationTTL sets a TTL for a transformation (for testing)
func (c *ConsulHealingStore) SetTransformationTTL(ctx context.Context, id string, ttl time.Duration) error {
	// Note: Consul doesn't have built-in TTL for KV entries
	// This is a simplified implementation for testing
	// In production, we'd use Consul sessions with TTL or a background cleanup job

	// For testing, we'll schedule a deletion
	go func() {
		time.Sleep(ttl)
		_ = c.deleteTransformation(context.Background(), id)
	}()

	return nil
}

// countSuccessfulHeals recursively counts successful heals in the tree
/*func (c *ConsulHealingStore) countSuccessfulHeals(status *TransformationStatus) {
	count := 0
	c.countHealsRecursive(status.Children, &count)
	// Update the count in status if needed
}

// countHealsRecursive is a helper to recursively count successful heals
func (c *ConsulHealingStore) countHealsRecursive(attempts []HealingAttempt, count *int) {
	for _, attempt := range attempts {
		if attempt.Status == "completed" && attempt.Result == "success" {
			*count++
		}
		if len(attempt.Children) > 0 {
			c.countHealsRecursive(attempt.Children, count)
		}
	}
}*/

// GenerateNextAttemptPath generates the next available attempt path for a transformation
/*func (c *ConsulHealingStore) GenerateNextAttemptPath(ctx context.Context, rootID string, parentPath string) (string, error) {
	status, err := c.GetTransformationStatus(ctx, rootID)
	if err != nil {
		return "", fmt.Errorf("failed to get transformation status: %w", err)
	}

	if status == nil {
		// No existing transformation, this would be the first attempt
		if parentPath != "" {
			return "", fmt.Errorf("cannot create child attempt for non-existent transformation")
		}
		// For a new transformation, we might want to initialize it first
		return "", fmt.Errorf("transformation %s not found - initialize it first", rootID)
	}

	// Validate parent path exists if specified
	if parentPath != "" && !IsValidParent(status.Children, parentPath) {
		return "", fmt.Errorf("parent path %s does not exist", parentPath)
	}

	return GenerateAttemptPath(rootID, parentPath, status.Children), nil
}*/
