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
    {Lane: "C", Lang: "scala", Version: "21"},
    {Lane: "C", Lang: "java", Version: "8"},
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
    if user == "" { t.Skip("GITHUB_PLOY_DEV_USERNAME required") }
    for _, tc := range matrix {
        t.Run(fmt.Sprintf("%s-%s-%s", tc.Lane, tc.Lang, tc.Version), func(t *testing.T) {
            repo := tc.RepoURL(user)
            app := filepath.Base(strings.TrimSuffix(repo, ".git"))
            runDeploy(t, controller, tc.Lane, repo, app)
        })
    }
}

func runDeploy(t *testing.T, controller, lane, repo, app string) {
    t.Helper()
    // Clone shallow
    work, err := os.MkdirTemp("", "ploy-e2e-")
    if err != nil { t.Fatal(err) }
    defer os.RemoveAll(work)
    run(t, work, "git", "clone", "--depth", "1", "--branch", "main", repo, "app")
    // Push with ploy CLI
    cmd := exec.Command(bin(), "push", "-a", app, "-lane", lane)
    cmd.Dir = filepath.Join(work, "app")
    cmd.Env = append(os.Environ(), fmt.Sprintf("PLOY_CONTROLLER=%s", controller))
    out, err := cmd.CombinedOutput()
    if err != nil { t.Fatalf("ploy push failed: %v\n%s", err, string(out)) }
    // Parse async id if present
    id := parseAcceptedID(out)
    if id != "" { waitAsync(t, controller, app, id, 180*time.Second) }
    // Health check
    waitHealth(t, app, 300*time.Second)
    // Cleanup
    run(t, work, bin(), "apps", "destroy", "--name", app, "--force")
}

func bin() string {
    if p := os.Getenv("PLOY_CMD"); p != "" { return p }
    if _, err := os.Stat("./bin/ploy"); err == nil { return "./bin/ploy" }
    return "ploy"
}

func run(t *testing.T, dir string, name string, args ...string) {
    t.Helper()
    cmd := exec.Command(name, args...)
    cmd.Dir = dir
    out, err := cmd.CombinedOutput()
    if err != nil { t.Fatalf("%s %v failed: %v\n%s", name, args, err, string(out)) }
}

func parseAcceptedID(out []byte) string {
    // extract {"accepted":true,"id":"..."}
    i := bytes.IndexByte(out, '{')
    if i < 0 { return "" }
    var m map[string]any
    if err := json.Unmarshal(out[i:], &m); err != nil { return "" }
    if v, ok := m["accepted"].(bool); !ok || !v { return "" }
    if id, ok := m["id"].(string); ok { return id }
    return "" }

func waitAsync(t *testing.T, controller, app, id string, dur time.Duration) {
    t.Helper()
    deadline := time.Now().Add(dur)
    url := fmt.Sprintf("%s/apps/%s/builds/%s/status", strings.TrimRight(controller, "/"), app, id)
    for time.Now().Before(deadline) {
        resp, err := http.Get(url)
        if err == nil && resp.StatusCode == 200 {
            var m map[string]any
            _ = json.NewDecoder(resp.Body).Decode(&m); resp.Body.Close()
            if s, _ := m["status"].(string); s == "failed" { t.Fatalf("async failed: %v", m["message"]) }
            if s == "completed" { return }
        }
        time.Sleep(3 * time.Second)
    }
}

func waitHealth(t *testing.T, app string, dur time.Duration) {
    t.Helper()
    // preview URL or fallback
    // Since HEAD commit may vary, try the fallback app host first
    base := fmt.Sprintf("https://%s.dev.ployd.app/healthz", app)
    deadline := time.Now().Add(dur)
    for time.Now().Before(deadline) {
        resp, err := http.Get(base)
        if err == nil { resp.Body.Close(); if resp.StatusCode == 200 { return } }
        time.Sleep(3 * time.Second)
    }
    t.Fatalf("health check failed: %s", base)
}

