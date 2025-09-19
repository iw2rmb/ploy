package storage

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
)

func (c *SeaweedFSClient) ListObjects(bucket, prefix string) ([]ObjectInfo, error) {
	url := fmt.Sprintf("%s/%s/%s", c.filerURL, bucket, prefix)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return []ObjectInfo{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list objects: %s", resp.Status)
	}

	var result struct {
		Entries []struct {
			FullPath string `json:"FullPath"`
			FileSize int64  `json:"FileSize"`
			Mode     int64  `json:"Mode"`
			Mtime    string `json:"Mtime"`
		} `json:"Entries"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var objects []ObjectInfo
	for _, entry := range result.Entries {
		name := filepath.Base(entry.FullPath)
		objects = append(objects, ObjectInfo{
			Key:          name,
			Size:         entry.FileSize,
			LastModified: entry.Mtime,
			ContentType:  "application/octet-stream",
		})
	}

	return objects, nil
}
