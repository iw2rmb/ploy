package common

import (
    "fmt"
    "io"
    "net/http"
    "os"
    "time"

    "bytes"
    "log"
    "path/filepath"

    utils "github.com/iw2rmb/ploy/internal/cli/utils"
)

// DeployConfig contains all deployment parameters
type DeployConfig struct {
	App           string
	Lane          string
	MainClass     string
	SHA           string
	IsPlatform    bool // true for ployman, false for ploy
	BlueGreen     bool
	Environment   string // dev, staging, prod
	ControllerURL string
	Metadata      map[string]string
	Timeout       time.Duration
	BuildOnly     bool   // when true, API should run build gate and tear down app (no long-lived service)
	WorkingDir    string // optional: directory to tar instead of current working directory
}

// DeployResult contains deployment outcome information
type DeployResult struct {
	Success      bool
	Version      string
	DeploymentID string
	URL          string
	Message      string
}

// SharedPush handles deployment for both ploy and ployman
func SharedPush(config DeployConfig) (*DeployResult, error) {
	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Generate SHA if not provided
	if config.SHA == "" {
		if v := utils.GitSHA(); v != "" {
			config.SHA = v
		} else {
			config.SHA = time.Now().Format("20060102-150405")
		}
	}

    // Create tar archive (optionally autogenerating a minimal Dockerfile for known stacks)
    wd := config.WorkingDir
    if wd == "" {
        wd = "."
    }
    if v := os.Getenv("PLOY_AUTOGEN_DOCKERFILE"); v == "1" || v == "true" || v == "TRUE" || v == "on" || v == "ON" {
        _ = tryAutogenDockerfile(wd)
    }
    ign, _ := utils.ReadGitignore(wd)
    pr, pw := io.Pipe()
    go func() {
        defer func() { _ = pw.Close() }()
        _ = utils.TarDir(wd, pw, ign)
    }()

	// Build deployment URL
	url := buildDeployURL(config)

	// Create HTTP request
	req, _ := http.NewRequest("POST", url, pr)
	req.Header.Set("Content-Type", "application/x-tar")

	// Add platform-specific headers
	if config.IsPlatform {
		req.Header.Set("X-Platform-Service", "true")
		req.Header.Set("X-Target-Domain", "ployman.app")
	} else {
		req.Header.Set("X-Target-Domain", "ployd.app")
	}

	// Add environment header
	if config.Environment != "" {
		req.Header.Set("X-Environment", config.Environment)
	}

	// Execute request with optional timeout
	client := &http.Client{}
	if config.Timeout > 0 {
		client.Timeout = config.Timeout
	}
	log.Printf("[SharedPush] POST %s app=%s lane=%s env=%s", url, config.App, config.Lane, config.Environment)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deployment request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Parse response
	if resp.StatusCode != http.StatusOK {
		// Read and log response body for diagnostics, then restore body for downstream readers
		if b, rerr := io.ReadAll(resp.Body); rerr == nil {
			log.Printf("[SharedPush] Non-200 response status=%d body=%s", resp.StatusCode, string(b))
			resp.Body = io.NopCloser(bytes.NewReader(b))
		}
	}
	result, err := parseDeployResponse(resp, config)
	if err != nil {
		return nil, err
	}

	// Output to console
	_, _ = io.Copy(os.Stdout, resp.Body)

	return result, nil
}

// validateConfig validates the deployment configuration
func validateConfig(config DeployConfig) error {
	if config.App == "" {
		return fmt.Errorf("app name is required")
	}
	if config.ControllerURL == "" {
		return fmt.Errorf("controller URL is required")
	}
	return nil
}

// buildDeployURL constructs the deployment URL with query parameters
func buildDeployURL(config DeployConfig) string {
	url := fmt.Sprintf("%s/apps/%s/builds?sha=%s",
		config.ControllerURL, config.App, config.SHA)

	if config.MainClass != "" {
		url += "&main=" + utils.URLQueryEsc(config.MainClass)
	}

	if config.Lane != "" {
		url += "&lane=" + config.Lane
	}

	if config.IsPlatform {
		url += "&platform=true"
	}

	if config.BlueGreen {
		url += "&blue_green=true"
	}

	if config.Environment != "" {
		url += "&env=" + config.Environment
	}

	// Signal build-only mode so API can clean up sandboxed app after gate
	if config.BuildOnly {
		url += "&build_only=true"
	}

	return url
}

// parseDeployResponse parses the HTTP response into a DeployResult
func parseDeployResponse(resp *http.Response, config DeployConfig) (*DeployResult, error) {
	// Get the target domain
	domain := getTargetDomain(config)

	// Construct the result
	result := &DeployResult{
		Success:      resp.StatusCode == http.StatusOK,
		Version:      config.SHA,
		DeploymentID: resp.Header.Get("X-Deployment-ID"),
		URL:          fmt.Sprintf("https://%s.%s", config.App, domain),
		Message:      "Deployment completed",
	}

	// Add error message if not successful
	if !result.Success {
		result.Message = fmt.Sprintf("Deployment failed with status %d", resp.StatusCode)
	}

	return result, nil
}

// tryAutogenDockerfile writes a minimal Dockerfile for known stacks when missing.
// Currently supports Python minimal apps with app.py.
func tryAutogenDockerfile(dir string) error {
    df := filepath.Join(dir, "Dockerfile")
    if _, err := os.Stat(df); err == nil {
        return nil
    }
    // Python minimal app: app.py at repo root
    if _, err := os.Stat(filepath.Join(dir, "app.py")); err == nil {
        content := `FROM python:3.12-slim
WORKDIR /app
ENV PYTHONDONTWRITEBYTECODE=1 \\
    PYTHONUNBUFFERED=1 \\
    PYTHONPATH=/app \\
    PORT=8080
COPY . .
RUN if [ -f requirements.txt ] && [ -s requirements.txt ]; then pip install --no-cache-dir -r requirements.txt; fi || true
EXPOSE 8080
CMD ["python","app.py"]
`
        return os.WriteFile(df, []byte(content), 0644)
    }
    return nil
}

// getTargetDomain returns the appropriate domain based on platform and environment
func getTargetDomain(config DeployConfig) string {
	if config.IsPlatform {
		if config.Environment == "dev" {
			return "dev.ployman.app"
		}
		return "ployman.app"
	}

	if config.Environment == "dev" {
		return "dev.ployd.app"
	}
	return "ployd.app"
}
