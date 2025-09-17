package seaweedfs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type VolumeAssignment struct {
	FileID    string `json:"fid"`
	URL       string `json:"url"`
	PublicURL string `json:"publicUrl"`
	Count     int    `json:"count"`
}

func (p *Provider) assignVolume() (*VolumeAssignment, error) {
	url := fmt.Sprintf("%s/dir/assign?collection=%s&replication=%s", p.masterURL, p.collection, p.replication)
	resp, err := p.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to assign volume: %s", resp.Status)
	}
	var assignment VolumeAssignment
	if err := json.NewDecoder(resp.Body).Decode(&assignment); err != nil {
		return nil, err
	}
	return &assignment, nil
}

func (p *Provider) createDirectory(bucket, dir string) error {
	var fullPath string
	if bucket != "" {
		fullPath = fmt.Sprintf("%s/%s", bucket, dir)
	} else {
		fullPath = dir
	}
	url := fmt.Sprintf("%s/%s/", p.filerURL, fullPath)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("failed to create directory: %s, body: %s", resp.Status, string(body))
	}
	return nil
}

func (p *Provider) createDirectoryFullPath(fullPath string) error {
	url := fmt.Sprintf("%s/%s/", p.filerURL, fullPath)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("failed to create directory: %s, body: %s", resp.Status, string(body))
	}
	return nil
}

func (p *Provider) uploadFile(keyPrefix, filePath, contentType string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	key := keyPrefix + filepath.Base(filePath)
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if _, err := file.Seek(0, 0); err != nil {
			return fmt.Errorf("failed to reset file pointer: %w", err)
		}
		result, err := p.PutObject(p.collection, key, file, contentType)
		if err != nil {
			lastErr = err
			continue
		}
		if result != nil && result.Size > 0 {
			return nil
		}
		lastErr = fmt.Errorf("upload completed but no valid result received")
	}
	return fmt.Errorf("failed to upload after 3 attempts: %w", lastErr)
}
