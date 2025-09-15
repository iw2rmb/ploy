package server

import (
    "encoding/json"
    "fmt"
    "regexp"
    "os"
    "os/exec"
    "strconv"
    "time"

	"github.com/gofiber/fiber/v2"
)

type buildLogsResponse struct {
    ID      string `json:"id"`
    App     string `json:"app"`
    Job     string `json:"job,omitempty"`
    AllocID string `json:"alloc_id,omitempty"`
    AllocStatus string `json:"alloc_status,omitempty"`
    Lines   int    `json:"lines"`
    Logs    string `json:"logs"`
    Message string `json:"message,omitempty"`
    Status  string `json:"status,omitempty"`
    Started string `json:"started_at,omitempty"`
    Ended   string `json:"ended_at,omitempty"`
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
	// Determine builder job name
	job := deriveBuilderJob(id, st, meta)
	lines := 200
	if v := c.Query("lines", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			lines = n
		}
	}
	resp := buildLogsResponse{ID: id, App: app, Job: job, Lines: lines, Status: st.Status, Message: st.Message, Started: st.StartedAt, Ended: st.EndedAt}
	if job == "" {
		return c.JSON(resp)
	}
	// Resolve running allocation for the builder job via the wrapper
    alloc := runJobMgr("running-alloc", job)
    if alloc == "" {
        // Best-effort: return allocs short list for visibility
        allocs := runJobMgr("allocs-human", job)
        resp.Logs = allocs
        return c.JSON(resp)
    }
    resp.AllocID = alloc
    resp.Logs = runJobMgrLogs(alloc, lines)
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
	switch lane {
	case "E", "e":
		return fmt.Sprintf("%s-e-build-%s", meta.App, meta.Sha)
	case "C", "c":
		return fmt.Sprintf("%s-c-build-%s", meta.App, meta.Sha)
	default:
		return ""
	}
}

func runJobMgr(cmd string, job string) string {
    wrapper := "/opt/hashicorp/bin/nomad-job-manager.sh"
    var c *exec.Cmd
    switch cmd {
    case "running-alloc":
        c = exec.Command(wrapper, "running-alloc", "--job", job)
    case "allocs-human":
        c = exec.Command(wrapper, "allocs", "--job", job, "--format", "human")
    default:
        return ""
    }
    out, _ := runCmdTimeout(c, 8*time.Second)
    if cmd == "running-alloc" {
        // Extract first UUID from noisy wrapper output
        // Wrapper logs to stderr; CombinedOutput mixes both; pick the allocation ID via regex.
        uuid := extractUUID(out)
        return uuid
    }
    return out
}

func runJobMgrLogs(alloc string, lines int) string {
    wrapper := "/opt/hashicorp/bin/nomad-job-manager.sh"
    c := exec.Command(wrapper, "logs", "--alloc-id", alloc, "--lines", fmt.Sprintf("%d", lines))
    out, _ := runCmdTimeout(c, 8*time.Second)
    return out
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

func extractUUID(s string) string {
    // match RFC4122-like UUIDs
    re := regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
    return re.FindString(s)
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
