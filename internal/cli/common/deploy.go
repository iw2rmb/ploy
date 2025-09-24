package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
	UseMultipart  bool
	Environment   string // dev, staging, prod
	ControllerURL string
	Metadata      map[string]string
	Timeout       time.Duration
	BuildOnly     bool   // when true, API should run build gate and tear down app (no long-lived service)
	WorkingDir    string // optional: directory to tar instead of current working directory
	TarExtras     map[string][]byte
	Deps          *SharedPushDeps
}

// DeployResult contains deployment outcome information
type DeployResult struct {
	Success        bool
	Version        string
	DeploymentID   string
	URL            string
	Message        string
	ErrorCode      string
	ErrorDetails   string
	BuilderJob     string
	BuilderLogs    string
	BuilderLogsKey string
	BuilderLogsURL string
}

// SharedPush handles deployment for both ploy and ployman
func SharedPush(config DeployConfig) (*DeployResult, error) {
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	deps := resolveSharedPushDeps(config.Deps)

	if config.SHA == "" {
		if v := utils.GitSHA(); v != "" {
			config.SHA = v
		} else {
			config.SHA = time.Now().Format("20060102-150405")
		}
	}

	wd := config.WorkingDir
	if wd == "" {
		wd = "."
	}
	if v := os.Getenv("PLOY_AUTOGEN_DOCKERFILE"); v == "1" || v == "true" || v == "TRUE" || v == "on" || v == "ON" {
		_ = deps.autogenDockerfile(wd)
	}
	ign, _ := deps.readGitignore(wd)

	tarReader, tarWriter := io.Pipe()
	go func() {
		opts := utils.TarOptions{}
		if len(config.TarExtras) > 0 {
			extras := make(map[string]utils.TarExtra, len(config.TarExtras))
			for name, data := range config.TarExtras {
				extras[name] = utils.TarExtra{Data: data}
			}
			opts.Extras = extras
		}
		if err := deps.tarBuilder(wd, tarWriter, ign, opts); err != nil {
			_ = tarWriter.CloseWithError(err)
			return
		}
		_ = tarWriter.Close()
	}()

	url := buildDeployURL(config)

	var bodyReader io.Reader = tarReader
	var contentType string

	if config.UseMultipart {
		reqReader, reqWriter := io.Pipe()
		mw := multipart.NewWriter(reqWriter)
		go func() {
			defer func() { _ = reqWriter.Close() }()
			defer func() { _ = mw.Close() }()
			part, err := mw.CreateFormFile("file", "src.tar")
			if err != nil {
				_ = reqWriter.CloseWithError(err)
				return
			}
			if _, err = io.Copy(part, tarReader); err != nil {
				_ = reqWriter.CloseWithError(err)
				return
			}
		}()
		bodyReader = reqReader
		contentType = mw.FormDataContentType()
	} else {
		contentType = "application/x-tar"
	}

	req, err := http.NewRequest("POST", url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	if config.IsPlatform {
		req.Header.Set("X-Platform-Service", "true")
		req.Header.Set("X-Target-Domain", "ployman.app")
	} else {
		req.Header.Set("X-Target-Domain", "ployd.app")
	}

	if config.Environment != "" {
		req.Header.Set("X-Environment", config.Environment)
	}

	if client, ok := deps.httpClient.(*http.Client); ok && config.Timeout > 0 {
		client.Timeout = config.Timeout
	}

	log.Printf("[SharedPush] POST %s app=%s lane=%s env=%s", url, config.App, config.Lane, config.Environment)

	resp, err := deps.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deployment request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var respBody []byte
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		if b, rerr := io.ReadAll(resp.Body); rerr == nil {
			log.Printf("[SharedPush] Non-OK response status=%d body=%s", resp.StatusCode, string(b))
			respBody = b
			resp.Body = io.NopCloser(bytes.NewReader(b))
		}
	}
	result, err := parseDeployResponse(resp, respBody, config)
	if err != nil {
		return nil, err
	}

	if len(respBody) > 0 {
		_, _ = deps.stdout.Write(respBody)
	} else {
		_, _ = io.Copy(deps.stdout, resp.Body)
	}

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
	base := fmt.Sprintf("%s/apps/%s/builds?sha=%s", config.ControllerURL, config.App, config.SHA)

	if config.MainClass != "" {
		base += "&main=" + utils.URLQueryEsc(config.MainClass)
	}

	if config.Lane != "" {
		base += "&lane=" + config.Lane
	}

	if config.IsPlatform {
		base += "&platform=true"
	}

	if config.BlueGreen {
		base += "&blue_green=true"
	}

	if config.Environment != "" {
		base += "&env=" + config.Environment
	}

	if config.BuildOnly {
		base += "&build_only=true"
	}

	if len(config.Metadata) > 0 {
		keys := make([]string, 0, len(config.Metadata))
		for k := range config.Metadata {
			if strings.TrimSpace(k) == "" {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			base += "&" + k + "=" + url.QueryEscape(config.Metadata[k])
		}
	}

	return base
}

// parseDeployResponse parses the HTTP response into a DeployResult
func parseDeployResponse(resp *http.Response, rawBody []byte, config DeployConfig) (*DeployResult, error) {
	// Get the target domain
	domain := getTargetDomain(config)

	// Construct the result
	success := resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted
	result := &DeployResult{
		Success:      success,
		Version:      config.SHA,
		DeploymentID: resp.Header.Get("X-Deployment-ID"),
		URL:          fmt.Sprintf("https://%s.%s", config.App, domain),
		Message:      "Deployment completed",
	}

	// Add error message if not successful
	if !result.Success {
		if len(rawBody) > 0 {
			trimmed := strings.TrimSpace(string(rawBody))
			if trimmed != "" {
				result.Message = trimmed
			} else {
				result.Message = fmt.Sprintf("Deployment failed with status %d", resp.StatusCode)
			}
		} else {
			result.Message = fmt.Sprintf("Deployment failed with status %d", resp.StatusCode)
		}

		// Attempt to parse structured error details from JSON payloads
		var payload struct {
			Error struct {
				Code    string      `json:"code"`
				Message string      `json:"message"`
				Details interface{} `json:"details"`
			} `json:"error"`
			Builder struct {
				Job     string `json:"job"`
				Logs    string `json:"logs"`
				LogsKey string `json:"logs_key"`
				LogsURL string `json:"logs_url"`
			} `json:"builder"`
			Logs string `json:"logs"`
		}
		if err := json.Unmarshal(rawBody, &payload); err == nil {
			if msg := strings.TrimSpace(payload.Error.Message); msg != "" {
				result.Message = msg
			}
			result.ErrorCode = strings.TrimSpace(payload.Error.Code)
			if payload.Error.Details != nil {
				if detail := strings.TrimSpace(fmt.Sprint(payload.Error.Details)); detail != "" {
					result.ErrorDetails = detail
				}
			}
			if payload.Builder.Job != "" {
				result.BuilderJob = strings.TrimSpace(payload.Builder.Job)
			}
			if logs := strings.TrimSpace(payload.Builder.Logs); logs != "" {
				result.BuilderLogs = logs
			} else if logs := strings.TrimSpace(payload.Logs); logs != "" {
				result.BuilderLogs = logs
			}
			if key := strings.TrimSpace(payload.Builder.LogsKey); key != "" {
				result.BuilderLogsKey = key
			}
			if url := strings.TrimSpace(payload.Builder.LogsURL); url != "" {
				result.BuilderLogsURL = url
			}
		}
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
ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1
ENV PYTHONPATH=/app
ENV PORT=8080
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
