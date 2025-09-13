package mods

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// downloadToFile fetches the url content and writes to dest path
func downloadToFile(url, dest string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// test indirections
var downloadToFileFn = downloadToFile

// validateArtifactKey ensures keys are safe and within the allowed namespace.
// Allowed form:
//   - must start with "transflow/"
//   - must not contain ".." path segments
//   - must not contain backslashes
//   - must not be empty
func validateArtifactKey(key string) error {
	if key == "" {
		return fmt.Errorf("empty artifact key")
	}
	if strings.HasPrefix(key, "/") {
		// normalize away leading slash for validation
		key = strings.TrimLeft(key, "/")
	}
	if strings.Contains(key, "\\") {
		return fmt.Errorf("invalid artifact key: contains backslash")
	}
	parts := strings.Split(key, "/")
	for _, p := range parts {
		if p == ".." {
			return fmt.Errorf("invalid artifact key: path traversal")
		}
	}
	if !strings.HasPrefix(key, "transflow/") {
		return fmt.Errorf("invalid artifact key: must start with 'transflow/'")
	}
	return nil
}

// putFile uploads a local file to SeaweedFS artifacts namespace using PUT
func putFile(seaweedBase, key, srcPath, contentType string) error {
	if err := validateArtifactKey(key); err != nil {
		return err
	}
	url := strings.TrimRight(seaweedBase, "/") + "/artifacts/" + strings.TrimLeft(key, "/")
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()
	req, err := http.NewRequest(http.MethodPut, url, f)
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("put %s: http %d: %s", key, resp.StatusCode, string(b))
	}
	return nil
}

// putJSON uploads JSON bytes to SeaweedFS
func putJSON(seaweedBase, key string, body []byte) error {
	if err := validateArtifactKey(key); err != nil {
		return err
	}
	url := strings.TrimRight(seaweedBase, "/") + "/artifacts/" + strings.TrimLeft(key, "/")
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("put %s: http %d: %s", key, resp.StatusCode, string(b))
	}
	return nil
}

var putJSONFn = putJSON
var putFileFn = putFile

// getJSON fetches a JSON document from SeaweedFS
func getJSON(seaweedBase, key string) ([]byte, int, error) {
	if err := validateArtifactKey(key); err != nil {
		return nil, 0, err
	}
	url := strings.TrimRight(seaweedBase, "/") + "/artifacts/" + strings.TrimLeft(key, "/")
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return b, resp.StatusCode, nil
}

var getJSONFn = getJSON
