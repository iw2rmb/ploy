package storage

import (
	"fmt"
	"net/http"
	"os"
)

func (c *SeaweedFSClient) UploadArtifactBundle(keyPrefix, artifactPath string) error {
	if err := c.uploadFile(keyPrefix, artifactPath, "application/octet-stream"); err != nil {
		return fmt.Errorf("failed to upload artifact %s: %w", artifactPath, err)
	}

	sbomPath := artifactPath + ".sbom.json"
	if _, err := os.Stat(sbomPath); err == nil {
		if err := c.uploadFile(keyPrefix, sbomPath, "application/json"); err != nil {
			return fmt.Errorf("failed to upload SBOM %s: %w", sbomPath, err)
		}
	}

	sigPath := artifactPath + ".sig"
	if _, err := os.Stat(sigPath); err == nil {
		if err := c.uploadFile(keyPrefix, sigPath, "application/octet-stream"); err != nil {
			return fmt.Errorf("failed to upload signature %s: %w", sigPath, err)
		}
	}

	crtPath := artifactPath + ".crt"
	if _, err := os.Stat(crtPath); err == nil {
		if err := c.uploadFile(keyPrefix, crtPath, "application/x-pem-file"); err != nil {
			return fmt.Errorf("failed to upload certificate %s: %w", crtPath, err)
		}
	}

	return nil
}

func (c *SeaweedFSClient) UploadArtifactBundleWithVerification(keyPrefix, artifactPath string) (*BundleIntegrityResult, error) {
	if err := c.UploadArtifactBundle(keyPrefix, artifactPath); err != nil {
		return nil, fmt.Errorf("failed to upload artifact bundle: %w", err)
	}

	verifier := NewIntegrityVerifier(c)
	result, err := verifier.VerifyArtifactBundle(keyPrefix, artifactPath)
	if err != nil {
		return result, fmt.Errorf("integrity verification failed: %w", err)
	}

	return result, nil
}

func (c *SeaweedFSClient) VerifyUpload(key string) error {
	bucket := c.GetArtifactsBucket()
	url := fmt.Sprintf("%s/%s/%s", c.filerURL, bucket, key)
	fmt.Printf("[SeaweedFS VerifyUpload] Checking upload at URL: %s (bucket: %s)\n", url, bucket)

	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		fmt.Printf("[SeaweedFS VerifyUpload] ERROR: Failed to create HEAD request: %v\n", err)
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		fmt.Printf("[SeaweedFS VerifyUpload] ERROR: HEAD request failed: %v\n", err)
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	fmt.Printf("[SeaweedFS VerifyUpload] HEAD response status: %s (%d)\n", resp.Status, resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("[SeaweedFS VerifyUpload] ERROR: Object not found at %s, status: %s\n", url, resp.Status)
		return fmt.Errorf("object not found: %s", resp.Status)
	}

	fmt.Printf("[SeaweedFS VerifyUpload] SUCCESS: Upload verified at %s\n", url)
	return nil
}
