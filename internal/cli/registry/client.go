package registry

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "path"
    "strings"
)

// Client wraps HTTP calls to the Ploy control-plane registry endpoints.
type Client struct {
    BaseURL    *url.URL
    HTTPClient *http.Client
}

type UploadStartRequest struct {
    MediaType string `json:"media_type,omitempty"`
    Size      int64  `json:"size,omitempty"`
    NodeID    string `json:"node_id,omitempty"`
}

type UploadStartResponse struct {
    UploadID  string `json:"upload_id"`
    RemotePath string `json:"remote_path"`
    NodeID    string `json:"node_id"`
    Location  string `json:"location"`
}

type UploadProgressRequest struct {
    Size int64 `json:"size,omitempty"`
}

type UploadCommitRequest struct {
    MediaType string `json:"media_type,omitempty"`
    Size      int64  `json:"size,omitempty"`
}

type BlobCommitResponse struct {
    Digest   string `json:"digest"`
    CID      string `json:"cid"`
    Location string `json:"location"`
}

type TagList struct {
    Name string   `json:"name"`
    Tags []string `json:"tags"`
}

func (c Client) StartUpload(ctx context.Context, repo string, req UploadStartRequest) (UploadStartResponse, error) {
    var out UploadStartResponse
    endpoint := c.joinPath("v1", "registry", strings.TrimPrefix(repo, "/"), "blobs", "uploads")
    body, _ := json.Marshal(req)
    httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
    if err != nil { return out, err }
    httpReq.Header.Set("Content-Type", "application/json")
    resp, err := c.HTTPClient.Do(httpReq)
    if err != nil { return out, err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusAccepted {
        data, _ := io.ReadAll(resp.Body)
        return out, fmt.Errorf("start upload: %s", strings.TrimSpace(string(data)))
    }
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return out, err }
    return out, nil
}

func (c Client) PatchUpload(ctx context.Context, repo, uploadID string, req UploadProgressRequest) error {
    endpoint := c.joinPath("v1", "registry", strings.TrimPrefix(repo, "/"), "blobs", "uploads", uploadID)
    body, _ := json.Marshal(req)
    httpReq, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(body))
    if err != nil { return err }
    httpReq.Header.Set("Content-Type", "application/json")
    resp, err := c.HTTPClient.Do(httpReq)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusAccepted {
        data, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("patch upload: %s", strings.TrimSpace(string(data)))
    }
    return nil
}

func (c Client) CommitUpload(ctx context.Context, repo, uploadID, digest string, req UploadCommitRequest) (BlobCommitResponse, error) {
    var out BlobCommitResponse
    endpoint := c.joinPath("v1", "registry", strings.TrimPrefix(repo, "/"), "blobs", "uploads", uploadID)
    // Append digest as query parameter
    u, err := url.Parse(endpoint)
    if err != nil { return out, err }
    q := u.Query()
    q.Set("digest", strings.TrimSpace(digest))
    u.RawQuery = q.Encode()
    body, _ := json.Marshal(req)
    httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), bytes.NewReader(body))
    if err != nil { return out, err }
    httpReq.Header.Set("Content-Type", "application/json")
    resp, err := c.HTTPClient.Do(httpReq)
    if err != nil { return out, err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusCreated {
        data, _ := io.ReadAll(resp.Body)
        return out, fmt.Errorf("commit upload: %s", strings.TrimSpace(string(data)))
    }
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return out, err }
    return out, nil
}

func (c Client) GetBlob(ctx context.Context, repo, digest string) ([]byte, error) {
    endpoint := c.joinPath("v1", "registry", strings.TrimPrefix(repo, "/"), "blobs", strings.TrimSpace(digest))
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
    if err != nil { return nil, err }
    resp, err := c.HTTPClient.Do(req)
    if err != nil { return nil, err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        data, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("get blob: %s", strings.TrimSpace(string(data)))
    }
    return io.ReadAll(resp.Body)
}

func (c Client) DeleteBlob(ctx context.Context, repo, digest string) error {
    endpoint := c.joinPath("v1", "registry", strings.TrimPrefix(repo, "/"), "blobs", strings.TrimSpace(digest))
    req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
    if err != nil { return err }
    resp, err := c.HTTPClient.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusAccepted {
        data, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("delete blob: %s", strings.TrimSpace(string(data)))
    }
    return nil
}

func (c Client) PutManifest(ctx context.Context, repo, reference string, manifest []byte) (string, error) {
    endpoint := c.joinPath("v1", "registry", strings.TrimPrefix(repo, "/"), "manifests", strings.TrimSpace(reference))
    req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(manifest))
    if err != nil { return "", err }
    req.Header.Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
    resp, err := c.HTTPClient.Do(req)
    if err != nil { return "", err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusCreated {
        data, _ := io.ReadAll(resp.Body)
        return "", fmt.Errorf("put manifest: %s", strings.TrimSpace(string(data)))
    }
    var payload struct{ Digest string `json:"digest"` }
    if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil { return "", err }
    return payload.Digest, nil
}

func (c Client) GetManifest(ctx context.Context, repo, reference string) ([]byte, error) {
    endpoint := c.joinPath("v1", "registry", strings.TrimPrefix(repo, "/"), "manifests", strings.TrimSpace(reference))
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
    if err != nil { return nil, err }
    resp, err := c.HTTPClient.Do(req)
    if err != nil { return nil, err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        data, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("get manifest: %s", strings.TrimSpace(string(data)))
    }
    return io.ReadAll(resp.Body)
}

func (c Client) DeleteManifest(ctx context.Context, repo, reference string) error {
    endpoint := c.joinPath("v1", "registry", strings.TrimPrefix(repo, "/"), "manifests", strings.TrimSpace(reference))
    req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
    if err != nil { return err }
    resp, err := c.HTTPClient.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusAccepted {
        data, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("delete manifest: %s", strings.TrimSpace(string(data)))
    }
    return nil
}

func (c Client) ListTags(ctx context.Context, repo string) (TagList, error) {
    var out TagList
    endpoint := c.joinPath("v1", "registry", strings.TrimPrefix(repo, "/"), "tags", "list")
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
    if err != nil { return out, err }
    resp, err := c.HTTPClient.Do(req)
    if err != nil { return out, err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        data, _ := io.ReadAll(resp.Body)
        return out, fmt.Errorf("list tags: %s", strings.TrimSpace(string(data)))
    }
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return out, err }
    return out, nil
}

func (c Client) joinPath(parts ...string) string {
    // Build a URL using url.URL ResolveReference semantics to avoid double slashes
    ref := &url.URL{Path: path.Join(parts...)}
    return c.BaseURL.ResolveReference(ref).String()
}

