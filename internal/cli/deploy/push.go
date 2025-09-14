package deploy

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	utils "github.com/iw2rmb/ploy/internal/cli/utils"
)

// DeployResult contains app deployment outcome information
type DeployResult struct {
	Success      bool
	Version      string
	DeploymentID string
	URL          string
	Message      string
}

// DeployApp handles deployment for regular applications (ploy)
func DeployApp(appName, lane, mainClass, sha string, blueGreen bool) (*DeployResult, error) {
	// Validate inputs
	if appName == "" {
		return nil, fmt.Errorf("app name is required")
	}

	// Get controller URL (regular apps always use ployd.app domain)
	controllerURL := os.Getenv("PLOY_CONTROLLER")
	if controllerURL == "" {
		controllerURL = "http://localhost:8081/v1"
	}

	// Generate SHA if not provided
	if sha == "" {
		if v := utils.GitSHA(); v != "" {
			sha = v
		} else {
			sha = time.Now().Format("20060102-150405")
		}
	}

	// Create tar archive into a temp file so we can set Content-Length
	ign, _ := utils.ReadGitignore(".")
	tmpf, err := os.CreateTemp("", "ploy-push-*.tar")
	if err != nil {
		return nil, fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmpf.Name()
	if err := utils.TarDir(".", tmpf, ign); err != nil {
		_ = tmpf.Close()
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("tar dir: %w", err)
	}
	if err := tmpf.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("close temp: %w", err)
	}
	rf, err := os.Open(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("open temp: %w", err)
	}
	stat, _ := rf.Stat()

	// Build app-specific URL
	url := fmt.Sprintf("%s/apps/%s/builds?sha=%s",
		controllerURL, appName, sha)

	if mainClass != "" {
		url += "&main=" + utils.URLQueryEsc(mainClass)
	}

	if lane != "" {
		url += "&lane=" + lane
	}

	if blueGreen {
		url += "&blue_green=true"
	}

	// Prefer async mode to avoid long-lived client connections through ingress, unless disabled via env
	if v := os.Getenv("PLOY_ASYNC"); !(v == "0" || v == "false" || v == "off" || v == "FALSE") {
		url += "&async=true"
	}
	// Propagate autogen flag to server so the async inner call can honor it
	if v := os.Getenv("PLOY_AUTOGEN_DOCKERFILE"); v == "1" || v == "true" || v == "on" || v == "TRUE" {
		url += "&autogen_dockerfile=true"
	}

	var (
		req         *http.Request
		clientBody  io.Reader
		contentType string
	)
	if os.Getenv("PLOY_PUSH_MULTIPART") == "1" {
		// Multipart upload: stream tar as a file part to avoid proxy buffering issues
		pr, pw := io.Pipe()
		mw := multipart.NewWriter(pw)
		go func() {
			defer pw.Close()
			defer mw.Close()
			part, err := mw.CreateFormFile("file", "src.tar")
			if err != nil {
				_ = pw.CloseWithError(err)
				return
			}
			if _, err := io.Copy(part, rf); err != nil {
				_ = pw.CloseWithError(err)
				return
			}
		}()
		clientBody = pr
		contentType = mw.FormDataContentType()
	} else {
		clientBody = rf
		contentType = "application/x-tar"
	}
	// Create HTTP request (multipart or raw tar)
	req, _ = http.NewRequest("POST", url, clientBody)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Target-Domain", "ployd.app")
	if stat != nil && os.Getenv("PLOY_PUSH_MULTIPART") != "1" {
		req.ContentLength = stat.Size()
	}

	// Execute request with a generous timeout
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("app deployment request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body for messaging
	body, _ := io.ReadAll(resp.Body)

	// Parse response
	result := &DeployResult{
		Success:      resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted,
		Version:      sha,
		DeploymentID: resp.Header.Get("X-Deployment-ID"),
		URL:          fmt.Sprintf("https://%s.ployd.app", appName),
		Message:      string(body),
	}

	// Output response body to console and clean up temp
	_, _ = os.Stdout.Write(body)
	_ = rf.Close()
	_ = os.Remove(tmpPath)

	return result, nil
}
