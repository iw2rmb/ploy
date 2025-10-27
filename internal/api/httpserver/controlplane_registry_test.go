package httpserver_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryBlobUploadLifecycle(t *testing.T) {
	t.Parallel()

	fixture := newRegistryHTTPFixture(t)
	repo := "acme/widgets"
	payload := []byte("layer-payload")
	digest := sha256Digest(payload)

	uploadURL := fmt.Sprintf("%s/v1/registry/%s/blobs/uploads", fixture.server.URL, repo)
	status, startResp := postJSONStatus(t, uploadURL, map[string]any{
		"media_type": "application/vnd.oci.image.layer.v1.tar",
		"size":       len(payload),
	})
	if status != http.StatusAccepted {
		t.Fatalf("expected 202 for upload start, got %d", status)
	}
	slotID := requireString(t, startResp["upload_id"])
	remotePath := requireString(t, startResp["remote_path"])
	localPath := fixture.localPath(remotePath)
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("prepare slot dir: %v", err)
	}
	if err := os.WriteFile(localPath, payload, 0o644); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	patchURL := fmt.Sprintf("%s/v1/registry/%s/blobs/uploads/%s", fixture.server.URL, repo, slotID)
	if patchStatus, _ := sendJSONStatus(t, http.MethodPatch, patchURL, map[string]any{"size": len(payload)}); patchStatus != http.StatusAccepted {
		t.Fatalf("expected 202 for upload patch, got %d", patchStatus)
	}
	values := url.Values{}
	values.Set("digest", digest)
	finalizeURL := fmt.Sprintf("%s?%s", patchURL, values.Encode())
	status, commitResp := sendJSONStatus(t, http.MethodPut, finalizeURL, map[string]any{
		"media_type": "application/vnd.oci.image.layer.v1.tar",
		"size":       len(payload),
	})
	if status != http.StatusCreated {
		t.Fatalf("expected 201 for upload commit, got %d", status)
	}
	if requireString(t, commitResp["digest"]) != digest {
		t.Fatalf("commit response digest mismatch")
	}
	blobURL := fmt.Sprintf("%s/v1/registry/%s/blobs/%s", fixture.server.URL, repo, digest)
	resp, err := http.Get(blobURL)
	if err != nil {
		t.Fatalf("get blob: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for blob get, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read blob body: %v", err)
	}
	if !bytes.Equal(body, payload) {
		t.Fatalf("unexpected blob payload: %q", string(body))
	}
}

func TestRegistryManifestLifecycle(t *testing.T) {
	t.Parallel()

	fixture := newRegistryHTTPFixture(t)
	repo := "acme/widgets"
	configDigest := uploadRegistryBlob(t, fixture, repo, []byte("config-json"), "application/vnd.oci.image.config.v1+json")
	layerDigest := uploadRegistryBlob(t, fixture, repo, []byte("layer-data"), "application/vnd.oci.image.layer.v1.tar")
	manifestPayload, err := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.manifest.v1+json",
		"config": map[string]any{
			"mediaType": "application/vnd.oci.image.config.v1+json",
			"digest":    configDigest,
			"size":      12,
		},
		"layers": []map[string]any{
			{
				"mediaType": "application/vnd.oci.image.layer.v1.tar",
				"digest":    layerDigest,
				"size":      10,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	manifestURL := fmt.Sprintf("%s/v1/registry/%s/manifests/latest", fixture.server.URL, repo)
	putReq, err := http.NewRequest(http.MethodPut, manifestURL, bytes.NewReader(manifestPayload))
	if err != nil {
		t.Fatalf("build manifest put: %v", err)
	}
	putReq.Header.Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil {
		t.Fatalf("manifest put: %v", err)
	}
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for manifest put, got %d", putResp.StatusCode)
	}
	manifestDigest := putResp.Header.Get("Docker-Content-Digest")
	if manifestDigest == "" {
		t.Fatalf("expected docker content digest header")
	}
	getURL := fmt.Sprintf("%s/v1/registry/%s/manifests/latest", fixture.server.URL, repo)
	resp, err := http.Get(getURL)
	if err != nil {
		t.Fatalf("manifest get: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read manifest body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for manifest get, got %d", resp.StatusCode)
	}
	if !bytes.Equal(body, manifestPayload) {
		t.Fatalf("unexpected manifest payload")
	}
	tagsURL := fmt.Sprintf("%s/v1/registry/%s/tags/list", fixture.server.URL, repo)
	status, tagList := getJSONStatus(t, tagsURL)
	if status != http.StatusOK {
		t.Fatalf("expected 200 for tags list, got %d", status)
	}
	tags, _ := tagList["tags"].([]any)
	if len(tags) != 1 || tags[0].(string) != "latest" {
		t.Fatalf("unexpected tags response: %#v", tagList)
	}
	deleteURL := fmt.Sprintf("%s/v1/registry/%s/manifests/%s", fixture.server.URL, repo, manifestDigest)
	status, _ = deleteJSONStatus(t, deleteURL, map[string]any{})
	if status != http.StatusAccepted {
		t.Fatalf("expected 202 for manifest delete, got %d", status)
	}
	status, tagList = getJSONStatus(t, tagsURL)
	if status != http.StatusOK {
		t.Fatalf("tags list after delete status %d", status)
	}
	if list, _ := tagList["tags"].([]any); len(list) != 0 {
		t.Fatalf("expected tags cleared after delete, got %#v", tagList)
	}
}
