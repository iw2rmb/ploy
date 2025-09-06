// Package storage provides SeaweedFS-backed persistence for KB data.
//
// This package implements the storage layer for the Knowledge Base system,
// using SeaweedFS as the distributed storage backend. It provides CRUD
// operations for Error patterns, learning Cases, and Summary statistics.
//
// Key components:
//   - KBStorage: Main storage client with HTTP-based SeaweedFS integration
//   - Config: Storage configuration including endpoints and timeouts
//
// Storage schema:
//   - kb/errors/{signature}: Error patterns by canonical signature
//   - kb/cases/{uuid}: Individual learning cases
//   - kb/summaries/{error_id}: Aggregated error statistics
//
// Integration points:
//   - Uses internal/kb/models for data structures
//   - Used by internal/kb/learning for persistence operations
//
// See Also:
//   - internal/kb/models: For data structure definitions
//   - internal/kb/learning: For learning pipeline integration
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/kb/models"
)

// KBStorage provides storage operations for KB data using SeaweedFS
type KBStorage struct {
	config     *Config
	httpClient *http.Client
}

// NewKBStorage creates a new KB storage instance
func NewKBStorage(config *Config) *KBStorage {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	return &KBStorage{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// StoreError stores an error in the KB
func (s *KBStorage) StoreError(ctx context.Context, err *models.Error) error {
	key := fmt.Sprintf("kb/errors/%s", err.Signature)
	return s.storeJSON(ctx, key, err)
}

// RetrieveError retrieves an error from the KB
func (s *KBStorage) RetrieveError(ctx context.Context, signature string) (*models.Error, error) {
	key := fmt.Sprintf("kb/errors/%s", signature)
	var err models.Error
	if retrieveErr := s.retrieveJSON(ctx, key, &err); retrieveErr != nil {
		return nil, retrieveErr
	}
	return &err, nil
}

// StoreCase stores a case in the KB
func (s *KBStorage) StoreCase(ctx context.Context, c *models.Case) error {
	key := fmt.Sprintf("kb/cases/%s", c.ID)
	return s.storeJSON(ctx, key, c)
}

// RetrieveCase retrieves a case from the KB
func (s *KBStorage) RetrieveCase(ctx context.Context, caseID string) (*models.Case, error) {
	key := fmt.Sprintf("kb/cases/%s", caseID)
	var c models.Case
	if err := s.retrieveJSON(ctx, key, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// ListCasesByError lists all cases for a specific error
func (s *KBStorage) ListCasesByError(ctx context.Context, errorID string) ([]*models.Case, error) {
	// For simplicity, we'll scan all cases and filter by ErrorID
	// In production, we might use a better indexing strategy
	prefix := "kb/cases/"
	keys, err := s.listKeys(ctx, prefix)
	if err != nil {
		return nil, err
	}

	var cases []*models.Case
	for _, key := range keys {
		var c models.Case
		if err := s.retrieveJSON(ctx, key, &c); err != nil {
			continue // Skip invalid cases
		}
		if c.ErrorID == errorID {
			cases = append(cases, &c)
		}
	}

	return cases, nil
}

// StoreSummary stores a summary in the KB
func (s *KBStorage) StoreSummary(ctx context.Context, summary *models.Summary) error {
	key := fmt.Sprintf("kb/summaries/%s", summary.ErrorID)
	return s.storeJSON(ctx, key, summary)
}

// RetrieveSummary retrieves a summary from the KB
func (s *KBStorage) RetrieveSummary(ctx context.Context, errorID string) (*models.Summary, error) {
	key := fmt.Sprintf("kb/summaries/%s", errorID)
	var summary models.Summary
	if err := s.retrieveJSON(ctx, key, &summary); err != nil {
		return nil, err
	}
	return &summary, nil
}

// storeJSON stores a JSON object at the given key
func (s *KBStorage) storeJSON(ctx context.Context, key string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	url := fmt.Sprintf("%s/%s", s.config.StorageURL, key)

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("timeout storing data: %w", ctx.Err())
		}
		return fmt.Errorf("failed to store data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("storage failed with status %d", resp.StatusCode)
	}

	return nil
}

// retrieveJSON retrieves and unmarshals a JSON object from the given key
func (s *KBStorage) retrieveJSON(ctx context.Context, key string, target interface{}) error {
	url := fmt.Sprintf("%s/%s", s.config.StorageURL, key)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("timeout retrieving data: %w", ctx.Err())
		}
		return fmt.Errorf("failed to retrieve data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return fmt.Errorf("not found")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("retrieval failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return nil
}

// listKeys lists all keys with the given prefix
func (s *KBStorage) listKeys(ctx context.Context, prefix string) ([]string, error) {
	// Simple implementation - in production we might use SeaweedFS directory listing
	// For now, this is a placeholder that would need real implementation
	// based on SeaweedFS's directory listing capabilities
	return []string{}, nil
}
