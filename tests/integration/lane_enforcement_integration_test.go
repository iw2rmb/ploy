//go:build integration

package integration

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/iw2rmb/ploy/internal/testing/helpers"
)

func TestLaneEnforcementDockerOnly(t *testing.T) {
	controller := helpers.GetEnvOrDefault("PLOY_CONTROLLER", "")
	if controller == "" {
		t.Skip("PLOY_CONTROLLER not configured")
	}

	if !isControllerReachable(controller) {
		t.Skipf("controller %s not reachable", controller)
	}

	dockerTar := buildMinimalDockerTar(t)
	appName := fmt.Sprintf("lane-enforcement-%d", time.Now().UnixNano())
	sha := fmt.Sprintf("sha%x", time.Now().UnixNano())

	t.Run("accepts build and records docker lane", func(t *testing.T) {
		id, headers, status := triggerAsyncBuild(t, controller, appName, sha, dockerTar, map[string]string{
			"build_only": "true",
		})
		require.Equal(t, http.StatusAccepted, status, "expected async build to be accepted")
		require.NotEmpty(t, id, "deployment id should be returned")
		assert.Equal(t, id, headers.Get("X-Deployment-ID"))

		require.Eventually(t, func() bool {
			job, logs, err := fetchBuildLogs(controller, appName, id)
			if err != nil {
				return false
			}
			if !strings.Contains(job, "-d-") {
				return false
			}
			if logs == "" {
				return false
			}
			return strings.Contains(strings.ToLower(logs), "docker")
		}, 2*time.Minute, 5*time.Second, "expected docker lane logs to appear")
	})

	t.Run("rejects non docker lane override", func(t *testing.T) {
		_, _, status := triggerAsyncBuild(t, controller, appName+"-bad", sha, dockerTar, map[string]string{
			"lane": "g",
		})
		require.Equal(t, http.StatusBadRequest, status, "non-D lane override should fail")
	})
}

func triggerAsyncBuild(t *testing.T, controller, app, sha string, tarPayload []byte, query map[string]string) (string, http.Header, int) {
	t.Helper()

	values := make([]string, 0, len(query)+2)
	values = append(values, "async=true", fmt.Sprintf("sha=%s", sha))
	for k, v := range query {
		values = append(values, fmt.Sprintf("%s=%s", k, v))
	}
	url := fmt.Sprintf("%s/v1/apps/%s/builds?%s", strings.TrimSuffix(controller, "/"), app, strings.Join(values, "&"))

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(tarPayload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-tar")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", http.Header{}, 0
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusAccepted {
		return "", resp.Header, resp.StatusCode
	}

	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("failed to decode response: %v (body: %s)", err, string(body))
	}

	return payload.ID, resp.Header, resp.StatusCode
}

func fetchBuildLogs(controller, app, id string) (string, string, error) {
	url := fmt.Sprintf("%s/v1/apps/%s/builds/%s/logs", strings.TrimSuffix(controller, "/"), app, id)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	var payload struct {
		Job  string `json:"job"`
		Logs string `json:"logs"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", err
	}
	return payload.Job, payload.Logs, nil
}

func buildMinimalDockerTar(t *testing.T) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	tw := tar.NewWriter(buf)

	files := map[string]string{
		"Dockerfile": "FROM busybox:1.36\nCMD [\"/bin/sh\"]\n",
		"README.md":  "lane enforcement smoke build\n",
	}

	for name, content := range files {
		hdr := &tar.Header{
			Name:    name,
			Mode:    0644,
			Size:    int64(len(content)),
			ModTime: time.Now(),
		}
		require.NoError(t, tw.WriteHeader(hdr))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}

	require.NoError(t, tw.Close())
	return buf.Bytes()
}

func isControllerReachable(base string) bool {
	base = strings.TrimSuffix(base, "/")
	url := base + "/health"
	resp, err := http.Get(url)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode >= 200 && resp.StatusCode < 500
}
