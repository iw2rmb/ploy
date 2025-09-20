package seaweedfs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
)

// Storage interface implementation

func (p *Provider) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/%s/%s", p.filerURL, p.collection, key)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("get object failed: %s", resp.Status)
	}
	return resp.Body, nil
}

func (p *Provider) Put(ctx context.Context, key string, reader io.Reader, opts ...storage.PutOption) error {
	// Resolve Put options for content-type and metadata. Metadata is not persisted in SeaweedFS.
	resolved := storage.ResolvePutOptions(opts...)
	contentType := resolved.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	// Construct URL and ensure no accidental double-leading slashes which some proxies mishandle
	cleanedKey := path.Clean(key)
	if strings.HasPrefix(cleanedKey, "/") {
		cleanedKey = strings.TrimLeft(cleanedKey, "/")
	}
	url := fmt.Sprintf("%s/%s/%s", p.filerURL, p.collection, cleanedKey)
	req, err := http.NewRequestWithContext(ctx, "PUT", url, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 0 {
			return fmt.Errorf("put failed: %s url=%s: %s", resp.Status, url, string(body))
		}
		return fmt.Errorf("put failed: %s url=%s", resp.Status, url)
	}
	return nil
}

func (p *Provider) Delete(ctx context.Context, key string) error {
	url := fmt.Sprintf("%s/%s/%s", p.filerURL, p.collection, key)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("delete failed: %s", resp.Status)
	}
	return nil
}

func (p *Provider) Exists(ctx context.Context, key string) (bool, error) {
	if err := p.VerifyUpload(key); err != nil {
		return false, nil
	}
	return true, nil
}

func (p *Provider) List(ctx context.Context, opts storage.ListOptions) ([]storage.Object, error) {
	// Fetch JSON listing from filer
	url := fmt.Sprintf("%s/%s/%s", p.filerURL, p.collection, opts.Prefix)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return []storage.Object{}, nil
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
	var objects []storage.Object
	for _, entry := range result.Entries {
		relKey := strings.TrimPrefix(entry.FullPath, "/")
		if p.collection != "" {
			collectionPrefix := p.collection + "/"
			relKey = strings.TrimPrefix(relKey, collectionPrefix)
		}
		relKey = strings.TrimPrefix(relKey, "/")
		if relKey == "" || relKey == "." {
			continue
		}
		var lastModified time.Time
		if entry.Mtime != "" {
			if parsed, err := time.Parse(time.RFC3339, entry.Mtime); err == nil {
				lastModified = parsed
			}
		}
		objects = append(objects, storage.Object{Key: relKey, Size: entry.FileSize, LastModified: lastModified, ContentType: "application/octet-stream", Metadata: make(map[string]string)})
	}
	return objects, nil
}

func (p *Provider) DeleteBatch(ctx context.Context, keys []string) error {
	for _, key := range keys {
		if err := p.Delete(ctx, key); err != nil {
			return fmt.Errorf("failed to delete key %s: %w", key, err)
		}
	}
	return nil
}

func (p *Provider) Head(ctx context.Context, key string) (*storage.Object, error) {
	url := fmt.Sprintf("%s/%s/%s", p.filerURL, p.collection, key)
	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, storage.NewStorageError("head", fmt.Errorf("object not found"), storage.ErrorContext{Key: key})
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("head request failed: %s", resp.Status)
	}
	contentLength := resp.ContentLength
	contentType := resp.Header.Get("Content-Type")
	etag := resp.Header.Get("ETag")
	lastModifiedStr := resp.Header.Get("Last-Modified")
	var lastModified time.Time
	if lastModifiedStr != "" {
		if parsed, err := time.Parse(http.TimeFormat, lastModifiedStr); err == nil {
			lastModified = parsed
		}
	}
	return &storage.Object{Key: key, Size: contentLength, ContentType: contentType, ETag: etag, LastModified: lastModified, Metadata: make(map[string]string)}, nil
}

func (p *Provider) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	// SeaweedFS filer API does not support rich per-object metadata updates in a
	// portable way. Treat as a no-op to satisfy the unified storage interface.
	// Callers that require metadata should persist it alongside the object.
	_ = ctx
	_ = key
	_ = metadata
	return nil
}

func (p *Provider) Copy(ctx context.Context, src, dst string) error {
	reader, err := p.Get(ctx, src)
	if err != nil {
		return fmt.Errorf("failed to read source: %w", err)
	}
	defer func() { _ = reader.Close() }()
	return p.Put(ctx, dst, reader)
}

func (p *Provider) Move(ctx context.Context, src, dst string) error {
	if err := p.Copy(ctx, src, dst); err != nil {
		return err
	}
	return p.Delete(ctx, src)
}

func (p *Provider) Health(ctx context.Context) error {
	if _, err := p.TestVolumeAssignment(); err != nil {
		return fmt.Errorf("seaweedfs health check failed: %w", err)
	}
	return nil
}

func (p *Provider) Metrics() *storage.StorageMetrics {
	return storage.NewStorageMetrics()
}
