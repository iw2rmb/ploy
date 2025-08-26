package seaweedfs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// SeaweedFSStorage provides SeaweedFS storage for OpenRewrite diffs
type SeaweedFSStorage struct {
	masterURL         string
	httpClient        *http.Client
	volumeURLOverride string // For testing purposes
}

// NewSeaweedFSStorage creates a new SeaweedFS storage instance
func NewSeaweedFSStorage(masterURL string) (*SeaweedFSStorage, error) {
	if masterURL == "" {
		return nil, fmt.Errorf("SeaweedFS master URL is required")
	}
	
	// Ensure URL has scheme
	if !strings.HasPrefix(masterURL, "http://") && !strings.HasPrefix(masterURL, "https://") {
		masterURL = "http://" + masterURL
	}
	
	return &SeaweedFSStorage{
		masterURL: masterURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// StoreDiff uploads diff to SeaweedFS and returns file ID
func (s *SeaweedFSStorage) StoreDiff(jobID string, diff []byte) (string, error) {
	// Step 1: Assign volume from master
	assignURL := fmt.Sprintf("%s/dir/assign", s.masterURL)
	resp, err := s.httpClient.Post(assignURL, "application/json", nil)
	if err != nil {
		return "", fmt.Errorf("failed to assign volume: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to assign volume: status %d, body: %s", resp.StatusCode, body)
	}
	
	var assign AssignResponse
	if err := json.NewDecoder(resp.Body).Decode(&assign); err != nil {
		return "", fmt.Errorf("failed to decode assign response: %w", err)
	}
	
	if assign.Error != "" {
		return "", fmt.Errorf("assign error: %s", assign.Error)
	}
	
	// Step 2: Upload diff to volume server
	volumeURL := assign.URL
	if s.volumeURLOverride != "" {
		// For testing - override the volume URL
		volumeURL = s.volumeURLOverride
	}
	
	uploadURL := fmt.Sprintf("http://%s/%s", volumeURL, assign.FID)
	
	// Create multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	
	// Add file part
	part, err := writer.CreateFormFile("file", fmt.Sprintf("%s.diff", jobID))
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}
	
	if _, err := io.Copy(part, bytes.NewReader(diff)); err != nil {
		return "", fmt.Errorf("failed to write diff to form: %w", err)
	}
	
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close multipart writer: %w", err)
	}
	
	// Create upload request
	req, err := http.NewRequest("POST", uploadURL, body)
	if err != nil {
		return "", fmt.Errorf("failed to create upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	
	// Execute upload
	resp, err = s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to upload diff: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed: status %d, body: %s", resp.StatusCode, body)
	}
	
	return assign.FID, nil
}

// RetrieveDiff downloads diff from SeaweedFS
func (s *SeaweedFSStorage) RetrieveDiff(fileID string) ([]byte, error) {
	// Extract volume ID from file ID
	volumeID, err := s.extractVolumeID(fileID)
	if err != nil {
		return nil, fmt.Errorf("failed to extract volume ID: %w", err)
	}
	
	// Step 1: Lookup volume location
	lookupURL := fmt.Sprintf("%s/dir/lookup?volumeId=%s", s.masterURL, volumeID)
	resp, err := s.httpClient.Get(lookupURL)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup volume: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to lookup volume: status %d, body: %s", resp.StatusCode, body)
	}
	
	var lookup LookupResponse
	if err := json.NewDecoder(resp.Body).Decode(&lookup); err != nil {
		return nil, fmt.Errorf("failed to decode lookup response: %w", err)
	}
	
	if lookup.Error != "" {
		return nil, fmt.Errorf("lookup error: %s", lookup.Error)
	}
	
	if len(lookup.Locations) == 0 {
		return nil, fmt.Errorf("no locations found for file %s", fileID)
	}
	
	// Step 2: Download from first available location
	volumeURL := lookup.Locations[0].URL
	if s.volumeURLOverride != "" {
		// For testing - override the volume URL
		volumeURL = s.volumeURLOverride
	}
	
	downloadURL := fmt.Sprintf("http://%s/%s", volumeURL, fileID)
	resp, err = s.httpClient.Get(downloadURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download diff: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download failed: status %d, body: %s", resp.StatusCode, body)
	}
	
	return io.ReadAll(resp.Body)
}

// extractVolumeID extracts the volume ID from a file ID
// File ID format: "volumeId,fileKey" (e.g., "3,01234567890123")
func (s *SeaweedFSStorage) extractVolumeID(fileID string) (string, error) {
	if fileID == "" {
		return "", fmt.Errorf("empty file ID")
	}
	
	parts := strings.Split(fileID, ",")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid file ID format: %s", fileID)
	}
	
	return parts[0], nil
}

// DeleteDiff removes a diff from SeaweedFS (optional, for cleanup)
func (s *SeaweedFSStorage) DeleteDiff(fileID string) error {
	// Extract volume ID
	volumeID, err := s.extractVolumeID(fileID)
	if err != nil {
		return fmt.Errorf("failed to extract volume ID: %w", err)
	}
	
	// Lookup volume location
	lookupURL := fmt.Sprintf("%s/dir/lookup?volumeId=%s", s.masterURL, volumeID)
	resp, err := s.httpClient.Get(lookupURL)
	if err != nil {
		return fmt.Errorf("failed to lookup volume: %w", err)
	}
	defer resp.Body.Close()
	
	var lookup LookupResponse
	if err := json.NewDecoder(resp.Body).Decode(&lookup); err != nil {
		return fmt.Errorf("failed to decode lookup response: %w", err)
	}
	
	if len(lookup.Locations) == 0 {
		return fmt.Errorf("no locations found for file %s", fileID)
	}
	
	// Delete from volume server
	volumeURL := lookup.Locations[0].URL
	if s.volumeURLOverride != "" {
		volumeURL = s.volumeURLOverride
	}
	
	deleteURL := fmt.Sprintf("http://%s/%s", volumeURL, fileID)
	req, err := http.NewRequest("DELETE", deleteURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}
	
	resp, err = s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete diff: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed: status %d, body: %s", resp.StatusCode, body)
	}
	
	return nil
}