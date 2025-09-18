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

type deployCase struct {
	Name        string
	Description string
	Files       map[string]string
}

func TestDeployLaneD(t *testing.T) {
	controller := os.Getenv("PLOY_CONTROLLER")
	if controller == "" {
		t.Skip("PLOY_CONTROLLER is required for E2E")
	}

	cases := []deployCase{
		newGoCase(),
		newNodeCase(),
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			runLaneDDeploy(t, controller, tc)
		})
	}
}

func runLaneDDeploy(t *testing.T, controller string, tc deployCase) {
	t.Helper()

	dir := t.TempDir()
	if err := writeCaseFiles(dir, tc.Files); err != nil {
		t.Fatalf("write files: %v", err)
	}
	initGitRepo(t, dir)

	app := sanitizeAppName(fmt.Sprintf("lane-d-%s-%d", tc.Name, time.Now().Unix()))
	pushBudget, asyncBudget, healthBudget := budgetsForCount(t, 1)

	cmd := exec.Command(bin(), "push", "-a", app, "-lane", "D")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PLOY_CONTROLLER=%s", controller),
		"PLOY_ASYNC=1",
	)
	out, err := runWithTimeout(cmd, pushBudget)
	if err != nil {
		t.Fatalf("ploy push failed: %v\n%s", err, string(out))
	}

	id := parseAcceptedID(out)
	if id == "" {
		t.Fatalf("push did not return async id: %s", string(out))
	}

	metrics := waitAsyncStatus(t, controller, app, id, asyncBudget)
	if status, _ := metrics["status"].(string); status != "deployed" {
		t.Fatalf("unexpected build status: %v", status)
	}

	if err := waitHealthOK(controller, app, healthBudget); err != nil {
		t.Fatalf("health check failed: %v", err)
	}

	cleanupApp(controller, app)
	writeResultEntry(t, "D", tc.Description, app, metrics)
}

func writeCaseFiles(root string, files map[string]string) error {
	for name, contents := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run(t, dir, 10*time.Second, "git", "init")
	run(t, dir, 10*time.Second, "git", "config", "user.email", "e2e@example.com")
	run(t, dir, 10*time.Second, "git", "config", "user.name", "Ploy E2E")
	run(t, dir, 10*time.Second, "git", "add", ".")
	run(t, dir, 10*time.Second, "git", "commit", "-m", "init")
}

func newGoCase() deployCase {
	return deployCase{
		Name:        "go",
		Description: "Go HTTP service with multi-stage Dockerfile",
		Files: map[string]string{
			"Dockerfile": `FROM golang:1.22-alpine AS build
WORKDIR /src
COPY . .
RUN go build -o app ./main.go

FROM alpine:3.19
WORKDIR /app
COPY --from=build /src/app /app/app
EXPOSE 8080
CMD ["/app/app"]
`,
			"main.go": `package main

import (
    "fmt"
    "net/http"
    "os"
)

func main() {
    mux := http.NewServeMux()
    mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
        fmt.Fprintln(w, "hello from go lane d")
    })
    mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
        w.WriteHeader(http.StatusOK)
        fmt.Fprintln(w, "ok")
    })
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }
    _ = http.ListenAndServe(":"+port, mux)
}
`,
			".dockerignore": `build
`,
			"go.mod": `module example.com/ploy/e2e

go 1.22
`,
		},
	}
}

func newNodeCase() deployCase {
	return deployCase{
		Name:        "node",
		Description: "Node.js server with simple Dockerfile",
		Files: map[string]string{
			"Dockerfile": `FROM node:20-alpine
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm install --production
COPY index.js ./
EXPOSE 8080
CMD ["node", "index.js"]
`,
			"package.json": `{
  "name": "lane-d-node",
  "version": "1.0.0",
  "main": "index.js",
  "scripts": {
    "start": "node index.js"
  }
}
`,
			"index.js": `const http = require('http');
const port = process.env.PORT || 8080;
const server = http.createServer((req, res) => {
  if (req.url === '/healthz') {
    res.writeHead(200, { 'Content-Type': 'text/plain' });
    return res.end('ok');
  }
  res.writeHead(200, { 'Content-Type': 'text/plain' });
  res.end('hello from node lane d');
});
server.listen(port, () => console.log('listening on', port));
`,
			"package-lock.json": `{
  "name": "lane-d-node",
  "version": "1.0.0",
  "lockfileVersion": 3,
  "packages": {
    "": {
      "name": "lane-d-node",
      "version": "1.0.0"
    }
  }
}
`,
		},
	}
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

func waitAsyncStatus(t *testing.T, controller, app, id string, dur time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(dur)
	base := strings.TrimRight(controller, "/")
	statusURL := fmt.Sprintf("%s/apps/%s/builds/%s/status", base, app, id)
	var last map[string]any
	for time.Now().Before(deadline) {
		resp, err := http.Get(statusURL)
		if err == nil {
			var body map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&body)
			resp.Body.Close()
			last = body
			if status, _ := body["status"].(string); status == "deployed" {
				return body
			}
		} else {
			time.Sleep(2 * time.Second)
			continue
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("async build did not complete within %s: last=%v", dur, last)
	return last
}

func waitHealthOK(controller, app string, dur time.Duration) error {
	base := strings.TrimRight(controller, "/")
	statusURL := fmt.Sprintf("%s/apps/%s/status", base, app)
	deadline := time.Now().Add(dur)
	for time.Now().Before(deadline) {
		resp, err := http.Get(statusURL)
		if err == nil {
			var body map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&body)
			resp.Body.Close()
			if total, _ := body["running_instances"].(float64); total > 0 {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("application %s did not become healthy in %s", app, dur)
}

func cleanupApp(controller, app string) {
	cmd := exec.Command(bin(), "apps", "destroy", "--name", app, "--force")
	cmd.Env = append(os.Environ(), fmt.Sprintf("PLOY_CONTROLLER=%s", controller))
	_, _ = runWithTimeout(cmd, 20*time.Second)
}

func budgetsForCount(t *testing.T, total int) (time.Duration, time.Duration, time.Duration) {
	dl, ok := t.Deadline()
	if !ok || total <= 0 {
		return 45 * time.Second, 90 * time.Second, 45 * time.Second
	}
	remaining := time.Until(dl)
	if remaining <= 0 {
		return 30 * time.Second, 60 * time.Second, 30 * time.Second
	}
	if remaining > 45*time.Second {
		remaining -= 45 * time.Second
	}
	slice := remaining / time.Duration(total)
	push := time.Duration(float64(slice) * 0.25)
	async := time.Duration(float64(slice) * 0.5)
	health := time.Duration(float64(slice) * 0.2)
	if push < 20*time.Second {
		push = 20 * time.Second
	}
	if async < 40*time.Second {
		async = 40 * time.Second
	}
	if health < 20*time.Second {
		health = 20 * time.Second
	}
	if push > 75*time.Second {
		push = 75 * time.Second
	}
	if async > 3*time.Minute {
		async = 3 * time.Minute
	}
	if health > 75*time.Second {
		health = 75 * time.Second
	}
	return push, async, health
}

func writeResultEntry(t *testing.T, lane, desc, app string, metrics map[string]any) {
	t.Helper()
	type entry struct {
		Lane    string         `json:"lane"`
		App     string         `json:"app"`
		Notes   string         `json:"notes"`
		Metrics map[string]any `json:"metrics,omitempty"`
		Time    string         `json:"time"`
	}
	e := entry{Lane: lane, App: app, Notes: desc, Metrics: metrics, Time: time.Now().Format(time.RFC3339)}
	b, _ := json.Marshal(e)
	path := resolveRepoPath(filepath.Join("tests", "e2e", "deploy", "results.jsonl"))
	if err := appendFile(path, string(b)+"\n"); err != nil {
		t.Logf("write results.jsonl: %v", err)
	}

	row := fmt.Sprintf("| %s | %s | %s |\n", lane, app, desc)
	if err := appendFile(resolveRepoPath(filepath.Join("tests", "e2e", "deploy", "results.md")), row); err != nil {
		t.Logf("write results.md: %v", err)
	}
}

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
	out := strings.Trim(b.String(), "-")
	if out == "" || out[0] < 'a' || out[0] > 'z' {
		out = "a-" + out
	}
	return out
}

func appendFile(path, line string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}

func resolveRepoPath(rel string) string {
	if root := os.Getenv("E2E_REPO_ROOT"); root != "" {
		return filepath.Join(root, rel)
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return rel
	}
	base := filepath.Dir(file)               // .../tests/e2e/deploy
	root := filepath.Dir(filepath.Dir(base)) // .../tests
	root = filepath.Dir(root)                // repo root
	return filepath.Join(root, rel)
}
