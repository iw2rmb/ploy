package server

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

type buildLogsResponse struct {
	ID          string `json:"id"`
	App         string `json:"app"`
	Job         string `json:"job,omitempty"`
	AllocID     string `json:"alloc_id,omitempty"`
	AllocStatus string `json:"alloc_status,omitempty"`
	Allocs      string `json:"allocs,omitempty"`
	Lines       int    `json:"lines"`
	Logs        string `json:"logs"`
	Message     string `json:"message,omitempty"`
	Status      string `json:"status,omitempty"`
	Started     string `json:"started_at,omitempty"`
	Ended       string `json:"ended_at,omitempty"`
	DockerImage string `json:"docker_image,omitempty"`
	PushVerify  any    `json:"push_verify,omitempty"`
}

// handleBuildLogs returns recent builder logs for a given async build id.
func (s *Server) handleBuildLogs(c *fiber.Ctx) error {
	id := c.Params("id")
	// Load status (if present) to include context/message
	var st buildStatus
	if b, err := os.ReadFile(statusPath(id)); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	app := c.Params("app")
	// Load meta for sha/lane
	var meta struct{ App, Sha, Lane string }
	if b, err := os.ReadFile(metaPath(id)); err == nil {
		_ = json.Unmarshal(b, &meta)
	}
	if meta.App == "" {
		meta.App = app
	}
	// Determine builder job name and enrich metadata from message
	job := deriveBuilderJob(id, st, meta)
	lines := 200
	if v := c.Query("lines", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			lines = n
		}
	}
	resp := buildLogsResponse{ID: id, App: app, Job: job, Lines: lines, Status: st.Status, Message: st.Message, Started: st.StartedAt, Ended: st.EndedAt}
	if st.Message != "" {
		var m map[string]any
		if json.Unmarshal([]byte(st.Message), &m) == nil {
			if di, ok := m["dockerImage"].(string); ok {
				resp.DockerImage = di
			}
			if pv, ok := m["pushVerification"]; ok {
				resp.PushVerify = pv
			}
		}
	}
	if job == "" {
		return c.JSON(resp)
	}
	// Resolve running allocation for the builder job via the wrapper
	alloc := runJobMgr("running-alloc", job)
	if alloc == "" {
		if st.Message != "" {
			if u := extractLastUUID(st.Message); u != "" {
				alloc = u
			}
		}
		// Try to get the most recent alloc IDs and attempt logs for each
		if ids := getAllocIDs(job); len(ids) > 0 {
			for _, cand := range ids {
				if logs := runJobMgrLogsAny(cand, lines); logs != "" {
					alloc = cand
					resp.Logs = logs
					break
				}
			}
		}
		if alloc == "" && resp.Logs == "" {
			// Best-effort: return allocs short list for visibility
			allocs := runJobMgr("allocs-human", job)
			resp.Logs = allocs
			return c.JSON(resp)
		}
	}
	resp.AllocID = alloc
	if aj := runJobMgr("allocs-json", job); aj != "" {
		resp.Allocs = aj
	}
	if resp.Logs == "" {
		resp.Logs = runJobMgrLogs(alloc, lines)
		if resp.Logs == "" {
			resp.Logs = runJobMgrLogsNoTask(alloc, lines)
		}
	}
	// Include allocation status snapshot for quick diagnosis
	if st := runJobMgrAllocStatus(alloc); st != "" {
		// limit size to avoid huge payloads
		if len(st) > 4000 {
			st = st[:4000]
		}
		resp.AllocStatus = st
	}
	return c.JSON(resp)
}

func deriveBuilderJob(id string, st buildStatus, meta struct{ App, Sha, Lane string }) string {
	// If status message embeds a JSON with builder job, prefer it
	if st.Message != "" {
		var m map[string]any
		if json.Unmarshal([]byte(st.Message), &m) == nil {
			if b, ok := m["builder"].(map[string]any); ok {
				if j, ok := b["job"].(string); ok && j != "" {
					return j
				}
			}
		}
	}
	if meta.App == "" || meta.Sha == "" {
		return ""
	}
	lane := meta.Lane
	if lane == "" {
		lane = "E"
	}
	lane = string([]byte{byte(([]rune(lane))[0])})
	// For Lane E, try to find the most recent nonce-suffixed builder job from debug copies
	if lane == "E" || lane == "e" {
		if j := findLatestBuilderJobFromDebug(meta.App, meta.Sha); j != "" {
			return j
		}
	}
	switch lane {
	case "E", "e":
		return fmt.Sprintf("%s-e-build-%s", meta.App, meta.Sha)
	case "C", "c":
		return fmt.Sprintf("%s-c-build-%s", meta.App, meta.Sha)
	default:
		return ""
	}
}

// findLatestBuilderJobFromDebug scans /opt/ploy/debug/jobs for the newest HCL
// matching the Lane E builder pattern for a given app and sha, and returns the job name.
func findLatestBuilderJobFromDebug(app, sha string) string {
	dir := "/opt/ploy/debug/jobs"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	prefix := fmt.Sprintf("%s-e-build-%s-", app, sha)
	newestName := ""
	var newestTime time.Time
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".hcl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		mod := info.ModTime()
		if newestName == "" || mod.After(newestTime) {
			newestName = name
			newestTime = mod
		}
	}
	if newestName == "" {
		return ""
	}
	// Strip .hcl to get the job name
	return strings.TrimSuffix(newestName, ".hcl")
}

func runJobMgr(cmd string, job string) string {
	wrapper := "/opt/hashicorp/bin/nomad-job-manager.sh"
	var c *exec.Cmd
	switch cmd {
	case "running-alloc":
		c = exec.Command(wrapper, "running-alloc", "--job", job)
	case "allocs-human":
		c = exec.Command(wrapper, "allocs", "--job", job, "--format", "human")
	case "allocs-json":
		c = exec.Command(wrapper, "allocs", "--job", job, "--format", "json")
	default:
		return ""
	}
	out, _ := runCmdTimeout(c, 8*time.Second)
	if cmd == "running-alloc" {
		// Wrapper logs + payload; extract the last UUID which should be the alloc ID
		uuid := extractLastUUID(out)
		return uuid
	}
	return out
}

func runJobMgrLogs(alloc string, lines int) string {
	wrapper := "/opt/hashicorp/bin/nomad-job-manager.sh"
	// Prefer explicit task name for known builders to avoid Nomad 400 errors
	c := exec.Command(wrapper, "logs", "--alloc-id", alloc, "--task", "kaniko", "--lines", fmt.Sprintf("%d", lines))
	out, _ := runCmdTimeout(c, 8*time.Second)
	return out
}

func runJobMgrLogsNoTask(alloc string, lines int) string {
	wrapper := "/opt/hashicorp/bin/nomad-job-manager.sh"
	c := exec.Command(wrapper, "logs", "--alloc-id", alloc, "--lines", fmt.Sprintf("%d", lines))
	out, _ := runCmdTimeout(c, 8*time.Second)
	return out
}

// runJobMgrLogsAny tries task-specific logs first, then falls back to no-task logs.
func runJobMgrLogsAny(alloc string, lines int) string {
	if alloc == "" {
		return ""
	}
	if out := runJobMgrLogs(alloc, lines); out != "" {
		return out
	}
	return runJobMgrLogsNoTask(alloc, lines)
}

func runJobMgrAllocStatus(alloc string) string {
	if alloc == "" {
		return ""
	}
	wrapper := "/opt/hashicorp/bin/nomad-job-manager.sh"
	c := exec.Command(wrapper, "alloc-status", "--alloc-id", alloc)
	out, _ := runCmdTimeout(c, 8*time.Second)
	return out
}

func extractLastUUID(s string) string {
	re := regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	all := re.FindAllString(s, -1)
	if len(all) == 0 {
		return ""
	}
	return all[len(all)-1]
}

func getAllocIDs(job string) []string {
	out := runJobMgr("allocs-json", job)
	if out == "" {
		return nil
	}
	type alloc struct {
		ID         string `json:"ID"`
		ModifyTime int64  `json:"ModifyTime"`
	}
	var a []alloc
	if err := json.Unmarshal([]byte(out), &a); err != nil {
		// Fallback: return all UUIDs found (order unknown)
		re := regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
		return re.FindAllString(out, -1)
	}
	// Sort by ModifyTime desc (newest first)
	if len(a) > 1 {
		sort.Slice(a, func(i, j int) bool { return a[i].ModifyTime > a[j].ModifyTime })
	}
	ids := make([]string, 0, len(a))
	for _, e := range a {
		if e.ID != "" {
			ids = append(ids, e.ID)
		}
	}
	return ids
}

func runCmdTimeout(cmd *exec.Cmd, timeout time.Duration) (string, error) {
	type res struct {
		out []byte
		err error
	}
	ch := make(chan res, 1)
	go func() { b, e := cmd.CombinedOutput(); ch <- res{b, e} }()
	select {
	case r := <-ch:
		b := r.out
		if len(b) > 4000 {
			b = b[len(b)-4000:]
		}
		return string(b), r.err
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("timeout")
	}
}
