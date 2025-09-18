package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// handleBuildLogsStream streams builder logs (SSE) for a given async build id.
// Uses the Nomad job-manager wrapper with --follow to relay stdout/stderr.
func (s *Server) handleBuildLogsStream(c *fiber.Ctx) error {
	id := c.Params("id")
	app := c.Params("app")
	// Load status/meta to derive job name
	var st buildStatus
	if b, err := os.ReadFile(statusPath(id)); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	var meta struct{ App, Sha, Lane string }
	if b, err := os.ReadFile(metaPath(id)); err == nil {
		_ = json.Unmarshal(b, &meta)
	}
	if meta.App == "" {
		meta.App = app
	}
	job := deriveBuilderJob(id, st, meta)
	if strings.TrimSpace(job) == "" {
		return c.Status(404).JSON(fiber.Map{"error": "builder job not found for build id"})
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		writeSSE := func(event, data string) {
			if event != "" {
				_, _ = w.WriteString("event: " + event + "\n")
			}
			for _, line := range strings.Split(data, "\n") {
				if line == "" {
					continue
				}
				_, _ = w.WriteString("data: " + line + "\n")
			}
			_, _ = w.WriteString("\n")
			_ = w.Flush()
		}

		writeSSE("init", fmt.Sprintf("job=%s", job))

		// Poll for running alloc
		alloc := ""
		for i := 0; i < 50; i++ { // ~5s at 100ms steps
			a := runJobMgr("running-alloc", job)
			if strings.TrimSpace(a) != "" {
				alloc = a
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if alloc == "" {
			writeSSE("error", "no running allocation")
			writeSSE("end", "")
			return
		}

		// Attach follow to logs for task kaniko (fallback to no task)
		wrapper := "/opt/hashicorp/bin/nomad-job-manager.sh"
		cmd := exec.Command(wrapper, "logs", "--alloc-id", alloc, "--task", "kaniko", "--both", "--follow")
		stdout, err1 := cmd.StdoutPipe()
		stderr, err2 := cmd.StderrPipe()
		if err1 != nil || err2 != nil || cmd.Start() != nil {
			// Fallback: no task
			cmd = exec.Command(wrapper, "logs", "--alloc-id", alloc, "--both", "--follow")
			stdout, _ = cmd.StdoutPipe()
			stderr, _ = cmd.StderrPipe()
			_ = cmd.Start()
		}

		// drain both streams
		done := make(chan struct{}, 2)
		drain := func(rdr *bufio.Reader) {
			for {
				line, err := rdr.ReadString('\n')
				if line != "" {
					writeSSE("", strings.TrimRight(line, "\n"))
				}
				if err != nil {
					break
				}
			}
			done <- struct{}{}
		}
		go drain(bufio.NewReader(stdout))
		go drain(bufio.NewReader(stderr))
		<-done
		<-done
		_ = cmd.Wait()
		writeSSE("end", "")
	})
	return nil
}

// handleBuildLogsDownload proxies the full builder log stored in SeaweedFS for a build id.
func (s *Server) handleBuildLogsDownload(c *fiber.Ctx) error {
	id := c.Params("id")
	app := c.Params("app")
	var st buildStatus
	if b, err := os.ReadFile(statusPath(id)); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	var meta struct{ App, Sha, Lane string }
	if b, err := os.ReadFile(metaPath(id)); err == nil {
		_ = json.Unmarshal(b, &meta)
	}
	if meta.App == "" {
		meta.App = app
	}
	job := deriveBuilderJob(id, st, meta)
	if job == "" {
		return c.Status(404).JSON(fiber.Map{"error": "builder job not found for build id"})
	}
	// Build SeaweedFS URL from env
	base := os.Getenv("PLOY_SEAWEEDFS_URL")
	if strings.TrimSpace(base) == "" {
		base = "http://seaweedfs-filer.storage.ploy.local:8888"
	}
	if !strings.HasPrefix(base, "http") {
		base = "http://" + base
	}
	if !strings.HasSuffix(base, "/artifacts") {
		base = strings.TrimRight(base, "/") + "/artifacts"
	}
	url := strings.TrimRight(base, "/") + "/build-logs/" + job + ".log"
	resp, err := http.Get(url)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": "failed to fetch log", "details": err.Error()})
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return c.Status(resp.StatusCode).JSON(fiber.Map{"error": "log not available", "code": resp.StatusCode})
	}
	c.Response().Header.SetContentType("text/plain")
	c.Response().Header.Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s.log\"", job))
	// stream
	_, _ = bufio.NewReader(resp.Body).WriteTo(c.Response().BodyWriter())
	return nil
}
