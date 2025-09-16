package mods

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// DebugNomad returns recent Nomad job diagnostics (allocs and evaluation summary) for troubleshooting
func (h *Handler) DebugNomad(c *fiber.Ctx) error {
	if os.Getenv("PLOY_DEBUG") != "1" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": fiber.Map{"code": "forbidden", "message": "debug endpoint disabled"}})
	}
	// Use the job manager wrapper to list recent jobs known to mod (prefix heuristics)
	// Fallback: scan logs for runID is non-trivial here; instead, list recent jobs by prefix
	type JobInfo struct {
		Name        string                 `json:"name"`
		AllocCount  int                    `json:"alloc_count"`
		AllocStates map[string]int         `json:"alloc_states"`
		LastEval    map[string]interface{} `json:"last_eval,omitempty"`
		Error       string                 `json:"error,omitempty"`
	}
	jobs := []JobInfo{}

	// Helper to run job-manager wrapper and parse JSON
	run := func(args ...string) ([]byte, error) {
		mgr := os.Getenv("NOMAD_JOB_MANAGER")
		if mgr == "" {
			mgr = "/opt/hashicorp/bin/nomad-job-manager.sh"
		}
		if _, err := os.Stat(mgr); err != nil {
			return nil, fmt.Errorf("job manager not available")
		}
		cmd := exec.Command(mgr, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("%v: %s", err, string(out))
		}
		return out, nil
	}

	// Heuristic: scan last 50 jobs via job-manager 'jobs --format json' then filter
	out, err := run("jobs", "--format", "json")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fiber.Map{"code": "internal_error", "message": "jobs listing failed", "details": err.Error()}})
	}
	var jmJobs []map[string]interface{}
	_ = json.Unmarshal(out, &jmJobs)
	candidates := []string{}
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, j := range jmJobs {
		name, _ := j["Name"].(string)
		submitTimeAny := j["SubmitTime"]
		recent := true
		switch t := submitTimeAny.(type) {
		case float64:
			// milliseconds
			ts := time.UnixMilli(int64(t))
			recent = ts.After(cutoff)
		}
		if recent && (strings.HasPrefix(name, "orw-apply-") || strings.Contains(strings.ToLower(name), "mod") || strings.HasPrefix(name, "mod-llm-exec") || strings.HasPrefix(name, "mod-planner") || strings.HasPrefix(name, "mod-reducer")) {
			candidates = append(candidates, name)
		}
	}
	// Limit candidates
	if len(candidates) > 30 {
		candidates = candidates[len(candidates)-30:]
	}

	for _, jobName := range candidates {
		ji := JobInfo{Name: jobName, AllocStates: map[string]int{}}
		// Get allocs for job
		aout, aerr := run("allocs", "--job", jobName, "--format", "json")
		if aerr == nil {
			var allocs []map[string]interface{}
			_ = json.Unmarshal(aout, &allocs)
			ji.AllocCount = len(allocs)
			for _, a := range allocs {
				st, _ := a["ClientStatus"].(string)
				ji.AllocStates[st] = ji.AllocStates[st] + 1
			}
		} else {
			ji.Error = aerr.Error()
		}
		// Evaluations via Nomad HTTP API
		// Best effort: http://127.0.0.1:4646/v1/job/<job>/evaluations
		func() {
			client := &http.Client{Timeout: 5 * time.Second}
			req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:4646/v1/job/%s/evaluations", jobName), nil)
			resp, err := client.Do(req)
			if err != nil {
				return
			}
			defer func() { _ = resp.Body.Close() }()
			var evals []map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&evals); err != nil {
				return
			}
			if len(evals) == 0 {
				return
			}
			last := evals[len(evals)-1]
			ji.LastEval = map[string]interface{}{
				"Status":         last["Status"],
				"TriggeredBy":    last["TriggeredBy"],
				"Class":          last["Class"],
				"NodesEvaluated": last["NodesEvaluated"],
				"NodesFiltered":  last["NodesFiltered"],
				"FailedTGAllocs": last["FailedTGAllocs"],
			}
		}()

		jobs = append(jobs, ji)
	}

	return c.JSON(fiber.Map{"recent_jobs": jobs, "count": len(jobs)})
}
