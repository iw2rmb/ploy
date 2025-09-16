//go:build e2e

package deploy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type Case struct {
	Lane    string
	Lang    string
	Version string
}

func (c Case) RepoURL(user string) string {
	return fmt.Sprintf("https://github.com/%s/ploy-lane-%s-%s-%s.git", user, strings.ToLower(c.Lane), strings.ToLower(c.Lang), c.Version)
}

var matrix = []Case{
	// Focus on Lane E first while stabilizing Dev
	{Lane: "E", Lang: "node", Version: "20"},
	{Lane: "E", Lang: "go", Version: "1.22"},
	{Lane: "E", Lang: "python", Version: "3.12"},
}

func TestDeployMatrix(t *testing.T) {
	controller := os.Getenv("PLOY_CONTROLLER")
	if controller == "" {
		t.Skip("PLOY_CONTROLLER is required for E2E")
	}
	user := os.Getenv("GITHUB_PLOY_DEV_USERNAME")
	if user == "" {
		t.Skip("GITHUB_PLOY_DEV_USERNAME required")
	}
	total := len(matrix)
	for i, tc := range matrix {
		t.Run(fmt.Sprintf("%s-%s-%s", tc.Lane, tc.Lang, tc.Version), func(t *testing.T) {
			repo := tc.RepoURL(user)
			app := sanitizeAppName(filepath.Base(strings.TrimSuffix(repo, ".git")))
			pushBudget, asyncBudget, healthBudget := budgetsForCase(t, i, total)
			runDeploy(t, controller, tc.Lane, repo, app, pushBudget, asyncBudget, healthBudget)
		})
	}
}

// Explicit case: Java (Gradle) without Jib to validate Lane E autogen
func TestDeploy_Java17_NoJib(t *testing.T) {
	controller := os.Getenv("PLOY_CONTROLLER")
	if controller == "" {
		t.Skip("PLOY_CONTROLLER is required for E2E")
	}
	user := os.Getenv("GITHUB_PLOY_DEV_USERNAME")
	if user == "" {
		t.Skip("GITHUB_PLOY_DEV_USERNAME required")
	}
	lane := "E"
	repo := fmt.Sprintf("https://github.com/%s/ploy-lane-e-java-17-nojib.git", user)
	app := sanitizeAppName("ploy-lane-e-java-17-nojib")
	pushBudget, asyncBudget, healthBudget := budgetsForCase(t, 0, 1)
	runDeploy(t, controller, lane, repo, app, pushBudget, asyncBudget, healthBudget)
}

func runDeploy(t *testing.T, controller, lane, repo, app string, pushBudget, asyncBudget, healthBudget time.Duration) {
	t.Helper()
	// Always attempt cleanup at the end to free resources
	defer func() {
		cmd := exec.Command(bin(), "apps", "destroy", "--name", app, "--force")
		cmd.Env = append(os.Environ(), fmt.Sprintf("PLOY_CONTROLLER=%s", controller))
		_, _ = runWithTimeout(cmd, 20*time.Second)
	}()
	// Clone shallow
	work, err := os.MkdirTemp("", "ploy-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	run(t, work, 20*time.Second, "git", "clone", "--depth", "1", "--branch", "main", repo, "app")
	// Push with ploy CLI; for Lane E, opt-in Dockerfile autogen via env
	cmd := exec.Command(bin(), "push", "-a", app, "-lane", lane)
	cmd.Dir = filepath.Join(work, "app")
	// E2E helper: for Python minimal apps without Dockerfile, write a basic Dockerfile client-side
	if strings.ToUpper(lane) == "E" {
		// quick heuristic: app.py at repo root
		if _, err := os.Stat(filepath.Join(cmd.Dir, "app.py")); err == nil {
			df := filepath.Join(cmd.Dir, "Dockerfile")
			if _, err := os.Stat(df); os.IsNotExist(err) {
				_ = os.WriteFile(df, []byte("FROM python:3.12-slim\nWORKDIR /app\nENV PYTHONDONTWRITEBYTECODE=1 \\\n+    PYTHONUNBUFFERED=1 \\\n+    PYTHONPATH=/app \\\n+    PORT=8080\nCOPY . .\nRUN if [ -f requirements.txt ] && [ -s requirements.txt ]; then pip install --no-cache-dir -r requirements.txt; fi || true\nEXPOSE 8080\nCMD [\"python\",\"app.py\"]\n"), 0644)
			}
		}
	}
	env := append(os.Environ(), fmt.Sprintf("PLOY_CONTROLLER=%s", controller))
	// Prefer async to retrieve build metrics via status endpoint
	env = append(env, "PLOY_ASYNC=1")
	if strings.ToUpper(lane) == "E" {
		env = append(env, "PLOY_AUTOGEN_DOCKERFILE=1")
	}
	cmd.Env = env
	out, err := runWithTimeout(cmd, pushBudget)
	if err != nil {
		// Best-effort logs on push failure
		sha := gitHead(t, filepath.Join(work, "app"))
		collectDeployLogs(t, controller, app, lane, sha, "")
		t.Fatalf("ploy push failed: %v\n%s", err, string(out))
	}
	// Parse async id if present
	id := parseAcceptedID(out)
	var metrics map[string]any
	if id != "" {
		metrics = waitAsync(t, controller, app, lane, id, asyncBudget)
	}
	// Compute image size from registry if metrics missing or imageSize zero
	sha := gitHead(t, filepath.Join(work, "app"))
	if (metrics == nil || !hasPositiveImageSize(metrics)) && sha != "" {
		if tag := deriveImageTagFromMetricsOrGuess(metrics, app, sha); tag != "" {
			if bytes, mb, ok := fetchImageSizeFromRegistry(tag); ok {
				if metrics == nil {
					metrics = map[string]any{}
				}
				metrics["imageSize"] = map[string]any{"bytes": float64(bytes), "mb": mb}
			}
			// Try to fetch uncompressed size via local Docker, then remote Docker via SSH (TARGET_HOST)
			if ub, umb, ok := fetchUncompressedSizeDocker(tag); ok || func() bool {
				if ok {
					return true
				}
				ub, umb, ok = fetchUncompressedSizeRemoteDocker(tag)
				return ok
			}() {
				if metrics == nil {
					metrics = map[string]any{}
				}
				im, _ := metrics["imageSize"].(map[string]any)
				if im == nil {
					im = map[string]any{}
				}
				im["uncompressed_bytes"] = float64(ub)
				im["uncompressed_mb"] = umb
				metrics["imageSize"] = im
			}
		}
	}
	// Optional: collect logs proactively when requested
	if os.Getenv("E2E_LOG_CONFIG") == "1" {
		collectDeployLogs(t, controller, app, lane, sha, id)
	}

	// Health check (fetch logs on failure), with controller-proxied fallback
	if err := waitHealth(controller, app, healthBudget); err != nil {
		collectDeployLogs(t, controller, app, lane, sha, id)
		t.Fatalf("%v", err)
	}
	// Capture builder caps (CPU/memory) for recording if available
	if strings.ToUpper(lane) == "E" && sha != "" {
		if cpu, mem, ok := fetchBuilderCaps(app, sha); ok {
			if metrics == nil {
				metrics = map[string]any{}
			}
			metrics["builder"] = map[string]any{"cpu": float64(cpu), "memory_mb": float64(mem)}
		}
	}
	// Cleanup
	run(t, work, 15*time.Second, bin(), "apps", "destroy", "--name", app, "--force")
	// Write result entry
	writeResult(t, lane, repo, app, metrics)
}

func bin() string {
	if p := os.Getenv("PLOY_CMD"); p != "" {
		return p
	}
	if _, err := os.Stat("./bin/ploy"); err == nil {
		return "./bin/ploy"
	}
	return "ploy"
}

func run(t *testing.T, dir string, timeout time.Duration, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := runWithTimeout(cmd, timeout)
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, string(out))
	}
}

// runWithTimeout runs a command with a timeout and returns (stdout+stderr, error)
func runWithTimeout(cmd *exec.Cmd, timeout time.Duration) ([]byte, error) {
	if timeout <= 0 {
		return cmd.CombinedOutput()
	}
	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := cmd.CombinedOutput()
		done <- result{out: out, err: err}
	}()
	select {
	case r := <-done:
		return r.out, r.err
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return []byte(fmt.Sprintf("command timed out after %s", timeout)), fmt.Errorf("timeout")
	}
}

func parseAcceptedID(out []byte) string {
	// extract {"accepted":true,"id":"..."}
	i := bytes.IndexByte(out, '{')
	if i < 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(out[i:], &m); err != nil {
		return ""
	}
	if v, ok := m["accepted"].(bool); !ok || !v {
		return ""
	}
	if id, ok := m["id"].(string); ok {
		return id
	}
	return ""
}

func hasPositiveImageSize(m map[string]any) bool {
	if m == nil {
		return false
	}
	if im, ok := m["imageSize"].(map[string]any); ok {
		if b, ok := im["bytes"].(float64); ok && b > 0 {
			return true
		}
		if mb, ok := im["mb"].(float64); ok && mb > 0 {
			return true
		}
	}
	return false
}

func deriveImageTagFromMetricsOrGuess(metrics map[string]any, app, sha string) string {
	// Prefer server-supplied imageTag when available
	if metrics != nil {
		if reg, ok := metrics["registry"].(map[string]any); ok {
			if it, ok := reg["imageTag"].(string); ok && it != "" {
				return it
			}
			if ep, ok := reg["endpoint"].(string); ok && ep != "" {
				return ep + "/" + app + ":" + sha
			}
		}
	}
	// Default Dev registry guess
	return "registry.dev.ployman.app/" + app + ":" + sha
}

func fetchImageSizeFromRegistry(tag string) (bytes int64, mb float64, ok bool) {
	// tag format: host/repo:ref
	slash := strings.Index(tag, "/")
	if slash <= 0 || slash >= len(tag)-1 {
		return 0, 0, false
	}
	host := tag[:slash]
	remainder := tag[slash+1:]
	name := remainder
	ref := "latest"
	if at := strings.Index(remainder, "@"); at != -1 {
		name = remainder[:at]
		ref = remainder[at+1:]
	} else if colon := strings.LastIndex(remainder, ":"); colon != -1 {
		name = remainder[:colon]
		ref = remainder[colon+1:]
	}
	// HTTPS then HTTP fallback
	for _, scheme := range []string{"https", "http"} {
		url := scheme + "://" + host + "/v2/" + name + "/manifests/" + ref
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Accept", strings.Join([]string{
			"application/vnd.oci.image.manifest.v1+json",
			"application/vnd.docker.distribution.manifest.v2+json",
		}, ", "))
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			continue
		}
		if layers, ok := body["layers"].([]any); ok {
			var total int64
			for _, l := range layers {
				if m, ok := l.(map[string]any); ok {
					if sz, ok := m["size"].(float64); ok {
						total += int64(sz)
					}
				}
			}
			if total > 0 {
				return total, float64(total) / (1024 * 1024), true
			}
		}
	}
	return 0, 0, false
}

func fetchUncompressedSizeDocker(tag string) (bytes int64, mb float64, ok bool) {
	// Check docker binary presence quickly
	if _, err := exec.LookPath("docker"); err != nil {
		return 0, 0, false
	}
	// docker inspect requires daemon reachable; best-effort
	cmd := exec.Command("docker", "inspect", "--format", "{{.Size}}", tag)
	out, err := runWithTimeout(cmd, 10*time.Second)
	if err != nil {
		return 0, 0, false
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0, 0, false
	}
	// Parse integer bytes
	var n int64
	if _, e := fmt.Sscanf(s, "%d", &n); e != nil {
		return 0, 0, false
	}
	if n <= 0 {
		return 0, 0, false
	}
	return n, float64(n) / (1024 * 1024), true
}

func fetchUncompressedSizeRemoteDocker(tag string) (bytes int64, mb float64, ok bool) {
	host := os.Getenv("TARGET_HOST")
	if host == "" {
		return 0, 0, false
	}
	// ssh to VPS and run docker inspect
	cmd := exec.Command("ssh", "-o", "ConnectTimeout=10", "root@"+host, "docker", "inspect", "--format", "{{.Size}}", tag)
	out, err := runWithTimeout(cmd, 10*time.Second)
	if err != nil {
		return 0, 0, false
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0, 0, false
	}
	var n int64
	if _, e := fmt.Sscanf(s, "%d", &n); e != nil {
		return 0, 0, false
	}
	if n <= 0 {
		return 0, 0, false
	}
	return n, float64(n) / (1024 * 1024), true
}

func waitAsync(t *testing.T, controller, app, lane, id string, dur time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(dur)
	url := fmt.Sprintf("%s/apps/%s/builds/%s/status", strings.TrimRight(controller, "/"), app, id)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == 200 {
			var m map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&m)
			resp.Body.Close()
			status, _ := m["status"].(string)
			if status == "failed" {
				// Collect logs on async failure before failing the test
				collectDeployLogs(t, controller, app, lane, "", id)
				t.Fatalf("async failed: %v", m["message"])
			}
			if status == "completed" {
				// Parse response JSON embedded in message
				if msg, _ := m["message"].(string); msg != "" {
					var rm map[string]any
					_ = json.Unmarshal([]byte(msg), &rm)
					return rm
				}
				return nil
			}
		}
		time.Sleep(1 * time.Second)
	}
	return nil
}

func waitHealth(controller, app string, dur time.Duration) error {
	base := fmt.Sprintf("https://%s.dev.ployd.app/healthz", app)
	deadline := time.Now().Add(dur)
	statusURL := ""
	if controller != "" {
		statusURL = fmt.Sprintf("%s/apps/%s/status", strings.TrimRight(controller, "/"), app)
	}
	for time.Now().Before(deadline) {
		// Prefer event-driven status when available
		if statusURL != "" {
			if r, e := http.Get(statusURL); e == nil && r.StatusCode == 200 {
				var s map[string]any
				_ = json.NewDecoder(r.Body).Decode(&s)
				r.Body.Close()
				if allocs, ok := s["allocations"].([]any); ok && len(allocs) > 0 {
					// Look for healthy or failing signals
					healthy := false
					failing := false
					for _, a := range allocs {
						if m, ok := a.(map[string]any); ok {
							if cs, _ := m["client_status"].(string); cs == "running" {
								healthy = true
							}
							if tasks, ok := m["tasks"].([]any); ok {
								for _, t := range tasks {
									if tm, ok := t.(map[string]any); ok {
										if fv, _ := tm["failed"].(bool); fv {
											failing = true
										}
										if evs, ok := tm["events"].([]any); ok {
											for _, ev := range evs {
												if evm, ok := ev.(map[string]any); ok {
													// Heuristic: "Alloc Unhealthy", "Killing", "Failed" → failing
													if typ, _ := evm["type"].(string); typ == "Alloc Unhealthy" || typ == "Killing" || typ == "Failed" {
														failing = true
													}
												}
											}
										}
									}
								}
							}
						}
					}
					if failing {
						return fmt.Errorf("health failed by alloc events")
					}
					if healthy {
						// Optionally confirm via controller-proxied probe once
						probe := fmt.Sprintf("%s/apps/%s/probe", strings.TrimRight(controller, "/"), app)
						if pr, pe := http.Get(probe); pe == nil {
							var pm map[string]any
							_ = json.NewDecoder(pr.Body).Decode(&pm)
							pr.Body.Close()
							if code, ok := pm["code"].(float64); ok && int(code) == 200 {
								return nil
							}
						}
					}
				}
			}
		}
		// External healthz as a fallback
		if resp, err := http.Get(base); err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("health check failed: %s", base)
}

// timeBudgets derives phase budgets from the test deadline to honor -timeout.
// It reserves 60s for teardown and log collection.
func budgetsForCase(t *testing.T, idx, total int) (time.Duration, time.Duration, time.Duration) {
	dl, ok := t.Deadline()
	if !ok || total <= 0 {
		return 30 * time.Second, 45 * time.Second, 30 * time.Second
	}
	remaining := time.Until(dl)
	if remaining <= 0 {
		return 15 * time.Second, 20 * time.Second, 10 * time.Second
	}
	if remaining > 30*time.Second {
		remaining -= 30 * time.Second
	}
	casesLeft := total - idx
	if casesLeft < 1 {
		casesLeft = 1
	}
	slice := remaining / time.Duration(casesLeft)
	push := time.Duration(float64(slice) * 0.25)
	async := time.Duration(float64(slice) * 0.55)
	health := time.Duration(float64(slice) * 0.15)
	// clamps
	if push > 60*time.Second {
		push = 60 * time.Second
	}
	if async > 240*time.Second {
		async = 240 * time.Second
	}
	if health > 60*time.Second {
		health = 60 * time.Second
	}
	if push < 15*time.Second {
		push = 15 * time.Second
	}
	if async < 20*time.Second {
		async = 20 * time.Second
	}
	if health < 15*time.Second {
		health = 15 * time.Second
	}
	return push, async, health
}

func writeResult(t *testing.T, lane, repo, app string, metrics map[string]any) {
	t.Helper()
	type entry struct {
		Lane    string         `json:"lane"`
		Repo    string         `json:"repo"`
		App     string         `json:"app"`
		Metrics map[string]any `json:"metrics,omitempty"`
		Time    string         `json:"time"`
	}
	e := entry{Lane: lane, Repo: repo, App: app, Metrics: metrics, Time: time.Now().Format(time.RFC3339)}
	b, _ := json.Marshal(e)
	// Write to repo-root-relative path to work regardless of package CWD
	path := resolveRepoPath(filepath.Join("tests", "e2e", "deploy", "results.jsonl"))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Logf("failed to write results: %v", err)
		return
	}
	defer f.Close()
	b = append(b, '\n')
	_, _ = f.Write(b)

	// Also append a human-friendly Markdown row to results.md with size/time if available
	md := resolveRepoPath(filepath.Join("tests", "e2e", "deploy", "results.md"))
	var szMB, uncompressedMB, durMS string
	var builderCPU, builderMem string
	if metrics != nil {
		if im, ok := metrics["imageSize"].(map[string]any); ok {
			if v, ok := im["mb"].(float64); ok {
				szMB = fmt.Sprintf("%.1fMB", v)
			}
			if v, ok := im["uncompressed_mb"].(float64); ok {
				uncompressedMB = fmt.Sprintf("%.1fMB", v)
			}
		}
		if bm, ok := metrics["build"].(map[string]any); ok {
			if v, ok := bm["duration_ms"].(float64); ok {
				durMS = fmt.Sprintf("%.1fs", v/1000.0)
			}
		}
		if b, ok := metrics["builder"].(map[string]any); ok {
			if v, ok := b["cpu"].(float64); ok && v > 0 {
				// Nomad CPU unit is MHz (shares); show as integer
				builderCPU = fmt.Sprintf("%d", int(v))
			}
			if v, ok := b["memory_mb"].(float64); ok && v > 0 {
				builderMem = fmt.Sprintf("%dMB", int(v))
			}
		}
	}
	if szMB == "" {
		szMB = "—"
	}
	if uncompressedMB == "" {
		uncompressedMB = "—"
	}
	if durMS == "" {
		durMS = "—"
	}
	if builderCPU == "" {
		builderCPU = "—"
	}
	if builderMem == "" {
		builderMem = "—"
	}
	row := fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n", lane, inferStack(repo), inferVersion(repo), repo, szMB, uncompressedMB, durMS, builderCPU, builderMem)
	_ = appendFile(md, row)
}

// fetchBuilderCaps attempts to retrieve the Kaniko builder CPU and memory caps for the given app build (by git sha)
// Returns (cpu, memoryMB, ok).
func fetchBuilderCaps(app, sha string) (int, int, bool) {
	host := os.Getenv("TARGET_HOST")
	if host == "" {
		return 0, 0, false
	}
	// Find latest debug job file for this build
	findCmd := exec.Command("ssh", "-o", "ConnectTimeout=10", "root@"+host,
		"ls -1 /opt/ploy/debug/jobs | grep "+app+"-e-build-"+sha+" | sort | tail -n 1")
	out, err := runWithTimeout(findCmd, 8*time.Second)
	if err != nil {
		return 0, 0, false
	}
	file := strings.TrimSpace(string(out))
	if file == "" {
		return 0, 0, false
	}
	cat := exec.Command("ssh", "-o", "ConnectTimeout=10", "root@"+host, "sed", "-n", "1,200p", "/opt/ploy/debug/jobs/"+file)
	body, err := runWithTimeout(cat, 8*time.Second)
	if err != nil {
		return 0, 0, false
	}
	s := string(body)
	// Parse lines for resources block
	var cpu, mem int
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "cpu") {
			var v int
			if _, e := fmt.Sscanf(line, "cpu = %d", &v); e == nil {
				cpu = v
			}
		}
		if strings.HasPrefix(line, "memory") {
			var v int
			if _, e := fmt.Sscanf(line, "memory = %d", &v); e == nil {
				mem = v
			}
		}
	}
	if cpu > 0 || mem > 0 {
		return cpu, mem, true
	}
	return 0, 0, false
}

func inferStack(repo string) string {
	// repo name: ploy-lane-<lane>-<lang>-<ver>
	base := filepath.Base(strings.TrimSuffix(repo, ".git"))
	parts := strings.Split(base, "-")
	if len(parts) >= 4 {
		return parts[3]
	}
	return ""
}

func inferVersion(repo string) string {
	base := filepath.Base(strings.TrimSuffix(repo, ".git"))
	parts := strings.Split(base, "-")
	if len(parts) >= 5 {
		return parts[4]
	}
	return ""
}

// sanitizeAppName makes a best-effort to comply with app name policy:
// start with letter, end with letter/number, only [a-z0-9-].
func sanitizeAppName(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, ".", "-")
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	out := b.String()
	out = strings.Trim(out, "-")
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = strings.TrimRight(out, "-")
	}
	if out == "" || !(out[0] >= 'a' && out[0] <= 'z') {
		out = "a-" + out
	}
	return out
}

func appendFile(path, line string) error {
	// Ensure parent directory exists when path points into repo root
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}

func gitHead(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--short=12", "HEAD")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func fetchLogs(t *testing.T, app, lane, sha string) {
	t.Helper()
	script := resolveRepoPath("tests/e2e/deploy/fetch-logs.sh")
	if _, err := os.Stat(script); err != nil {
		t.Logf("fetch-logs.sh not found: %v", err)
		return
	}
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(),
		"APP_NAME="+app,
		"LANE="+lane,
		"SHA="+sha,
		"LINES=200",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("fetch-logs.sh error: %v", err)
	}
	if len(out) > 0 {
		t.Logf("\n%s\n", string(out))
	}
}

// resolveRepoPath returns an absolute path to a repo-relative path, working under `go test` temp dirs.
func resolveRepoPath(rel string) string {
	// Allow override to set repo root explicitly
	if root := os.Getenv("E2E_REPO_ROOT"); root != "" {
		return filepath.Join(root, rel)
	}
	// Use the path of this source file to derive repo root
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return rel
	}
	// file is .../tests/e2e/deploy/deploy_matrix_test.go
	base := filepath.Dir(file)               // .../tests/e2e/deploy
	root := filepath.Dir(filepath.Dir(base)) // .../tests
	root = filepath.Dir(root)                // repo root
	return filepath.Join(root, rel)
}

func fetchBuilderLogsAPI(t *testing.T, controller, app, id string) {
	t.Helper()
	if controller == "" || id == "" {
		return
	}
	url := fmt.Sprintf("%s/apps/%s/builds/%s/logs?lines=200", strings.TrimRight(controller, "/"), app, id)
	resp, err := http.Get(url)
	if err != nil {
		t.Logf("builder logs API error: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Logf("builder logs API status: %d", resp.StatusCode)
		return
	}
	var m map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&m)
	b, _ := json.MarshalIndent(m, "", "  ")
	t.Logf("\n== Builder logs API (%s)\n%s\n", id, string(b))
}

// collectDeployLogs aggregates builder logs (via API) and app/platform logs via fetch-logs.sh.
// Pass sha when available (for SSH builder logs); id may be empty if push failed before acceptance.
func collectDeployLogs(t *testing.T, controller, app, lane, sha, id string) {
	t.Helper()
	if id != "" {
		fetchBuilderLogsAPI(t, controller, app, id)
	}
	// Pass BUILD_ID to fetch-logs.sh so it can pull builder logs via API
	old := os.Getenv("BUILD_ID")
	if id != "" {
		_ = os.Setenv("BUILD_ID", id)
	}
	fetchLogs(t, app, lane, sha)
	if id != "" {
		_ = os.Setenv("BUILD_ID", old)
	}
}
