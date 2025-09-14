package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

type buildStatus struct {
	ID        string `json:"id"`
	App       string `json:"app"`
	Status    string `json:"status"` // accepted, running, completed, failed
	Code      int    `json:"code"`
	Message   string `json:"message"`
	StartedAt string `json:"started_at"`
	EndedAt   string `json:"ended_at"`
}

func statusPath(id string) string {
	dir := "/opt/ploy/uploads"
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, id+".json")
}

// metaPath stores ancillary info about a build (app, sha, lane) for diagnostics/logs.
func metaPath(id string) string {
	dir := "/opt/ploy/uploads"
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, id+".meta.json")
}

func writeStatus(id string, st buildStatus) {
	b, _ := json.Marshal(st)
	_ = os.WriteFile(statusPath(id), b, 0644)
}

// handleBuildStatus returns status JSON for async builds
func (s *Server) handleBuildStatus(c *fiber.Ctx) error {
	id := c.Params("id")
	data, err := os.ReadFile(statusPath(id))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "status not found"})
	}
	c.Type("json")
	return c.Send(data)
}

// startAsyncBuild saves the tar body and triggers a background self-call to complete the build.
func (s *Server) startAsyncBuild(c *fiber.Ctx, app, sha, lane, main string) (string, error) {
	id := fmt.Sprintf("b-%d", time.Now().UnixNano())
	// Persist body
	upDir := "/opt/ploy/uploads"
	if err := os.MkdirAll(upDir, 0755); err != nil {
		return "", err
	}
	tarPath := filepath.Join(upDir, id+".tar")
	if err := os.WriteFile(tarPath, c.Body(), 0644); err != nil {
		return "", err
	}
	// Initial status
	writeStatus(id, buildStatus{ID: id, App: app, Status: "accepted", StartedAt: time.Now().Format(time.RFC3339)})
	// Persist meta for later log retrieval
	_ = os.WriteFile(metaPath(id), []byte(fmt.Sprintf(`{"app":"%s","sha":"%s","lane":"%s"}`, app, sha, lane)), 0644)

	// Fire background requester against local fiber listener
	go func() {
		defer func() {
			if r := recover(); r != nil {
				writeStatus(id, buildStatus{ID: id, App: app, Status: "failed", Message: fmt.Sprintf("panic: %v", r), EndedAt: time.Now().Format(time.RFC3339)})
			}
		}()
		writeStatus(id, buildStatus{ID: id, App: app, Status: "running", StartedAt: time.Now().Format(time.RFC3339)})
		// Build internal URL (bypass ingress)
		q := []string{fmt.Sprintf("sha=%s", sha), "async=false"}
		if lane != "" {
			q = append(q, "lane="+lane)
		}
		if main != "" {
			q = append(q, "main="+main)
		}
		url := fmt.Sprintf("http://127.0.0.1:%s/v1/apps/%s/builds?%s", s.config.Port, app, strings.Join(q, "&"))
		f, err := os.Open(tarPath)
		if err != nil {
			writeStatus(id, buildStatus{ID: id, App: app, Status: "failed", Message: err.Error(), EndedAt: time.Now().Format(time.RFC3339)})
			return
		}
		defer f.Close()
		req, _ := http.NewRequest("POST", url, f)
		req.Header.Set("Content-Type", "application/x-tar")
		st, _ := f.Stat()
		if st != nil {
			req.ContentLength = st.Size()
		}
		client := &http.Client{Timeout: 15 * time.Minute}
		resp, err := client.Do(req)
		if err != nil {
			writeStatus(id, buildStatus{ID: id, App: app, Status: "failed", Message: err.Error(), EndedAt: time.Now().Format(time.RFC3339)})
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == 200 {
			writeStatus(id, buildStatus{ID: id, App: app, Status: "completed", Code: resp.StatusCode, Message: string(body), EndedAt: time.Now().Format(time.RFC3339)})
		} else {
			writeStatus(id, buildStatus{ID: id, App: app, Status: "failed", Code: resp.StatusCode, Message: string(body), EndedAt: time.Now().Format(time.RFC3339)})
		}
	}()

	return id, nil
}
