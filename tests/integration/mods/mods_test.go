//go:build integration

package mods_test

import (
    "encoding/json"
    "bytes"
    "context"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "testing"
    "time"
)

const sampleRepo = "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git"

func imagePrefix() string {
    if u := strings.TrimSpace(os.Getenv("DOCKERHUB_USERNAME")); u != "" {
        return "docker.io/" + u
    }
    if p := strings.TrimSpace(os.Getenv("MODS_IMAGE_PREFIX")); p != "" {
        return p
    }
    return "docker.io/iwtormb"
}

func cloneRepo(t *testing.T, branch string) string {
    t.Helper()
    dir := t.TempDir()
    // embed PAT if provided
    url := sampleRepo
    if tok := strings.TrimSpace(os.Getenv("PLOY_GITLAB_PAT")); tok != "" {
        url = strings.Replace(url, "https://", fmt.Sprintf("https://oauth2:%s@", tok), 1)
    }
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()
    args := []string{"clone", "--depth", "1", "--branch", branch, url, dir}
    cmd := exec.CommandContext(ctx, "git", args...)
    cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
    var out, errb bytes.Buffer
    cmd.Stdout = &out
    cmd.Stderr = &errb
    if err := cmd.Run(); err != nil {
        t.Fatalf("git clone failed: %v\nstdout: %s\nstderr: %s", err, out.String(), errb.String())
    }
    return dir
}

func writeReportFile(t *testing.T, name string, data []byte) {
    t.Helper()
    rel := filepath.Join("report")
    if err := os.MkdirAll(rel, 0o755); err != nil {
        t.Fatalf("mkdir report: %v", err)
    }
    dst := filepath.Join(rel, name)
    if err := os.WriteFile(dst, data, 0o644); err != nil {
        t.Fatalf("write report %s: %v", dst, err)
    }
}

func dockerRun(t *testing.T, timeout time.Duration, args ...string) (string, time.Duration) {
    t.Helper()
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()
    cmd := exec.CommandContext(ctx, "docker", args...)
    var out, errb bytes.Buffer
    cmd.Stdout = &out
    cmd.Stderr = &errb
    start := time.Now()
    err := cmd.Run()
    dur := time.Since(start)
    if ctx.Err() == context.DeadlineExceeded {
        t.Fatalf("docker %v timed out after %s", args, timeout)
    }
    if err != nil {
        t.Fatalf("docker %v failed: %v\nstdout: %s\nstderr: %s", args, err, out.String(), errb.String())
    }
    return out.String() + errb.String(), dur
}

func TestModsPlanClusterArgs(t *testing.T) {
    img := imagePrefix() + "/mods-plan:latest"
    outDir := t.TempDir()
    out, dur := dockerRun(t, 1*time.Minute, "run", "--rm", "-v", outDir+":/out", img, "mods-plan", "--run")
    t.Logf("mods-plan finished in %s\n%s", dur, out)
    // Verify plan.json selected_recipes against golden
    data, err := os.ReadFile(filepath.Join(outDir, "plan.json"))
    if err != nil { t.Fatalf("read plan.json: %v", err) }
    writeReportFile(t, "plan.json", data)
    var actual struct{ Selected []string `json:"selected_recipes"` }
    if err := json.Unmarshal(data, &actual); err != nil { t.Fatalf("decode plan: %v", err) }
    golden, err := os.ReadFile(filepath.Join("expected", "plan", "plan.json"))
    if err != nil { t.Fatalf("read golden: %v", err) }
    var want struct{ Selected []string `json:"selected_recipes"` }
    if err := json.Unmarshal(golden, &want); err != nil { t.Fatalf("decode golden: %v", err) }
    if strings.Join(actual.Selected, ",") != strings.Join(want.Selected, ",") {
        t.Fatalf("selected_recipes mismatch: got %v want %v", actual.Selected, want.Selected)
    }
    if dur > 10*time.Second { t.Fatalf("mods-plan took too long: %s", dur) }
}

func TestModsOpenRewriteApply_MainBranch(t *testing.T) {
    // Clone real repo
    ws := cloneRepo(t, "main")
    // Prepare recipe JSON matching cluster docs
    dir := t.TempDir()
    recipe := filepath.Join(dir, "recipe.json")
    recipeJSON := `{"group":"org.openrewrite.recipe","artifact":"rewrite-migrate-java","version":"3.17.0","name":"org.openrewrite.java.migrate.UpgradeToJava17"}`
    if err := os.WriteFile(recipe, []byte(recipeJSON), 0o644); err != nil { t.Fatal(err) }
    outDir := filepath.Join(dir, "out")
    if err := os.MkdirAll(outDir, 0o755); err != nil { t.Fatal(err) }
    m2 := filepath.Join(dir, ".m2")
    if err := os.MkdirAll(m2, 0o755); err != nil { t.Fatal(err) }
    img := imagePrefix() + "/mods-openrewrite:latest"
    out, dur := dockerRun(t, 6*time.Minute,
        "run", "--rm",
        "-e", "MAVEN_OPTS=-Dmaven.repo.local=/workspace/.m2",
        "-v", ws+":/workspace",
        "-v", m2+":/workspace/.m2",
        "-v", outDir+":/out",
        "-v", recipe+":/in/recipe.json:ro",
        img, "mod-orw", "--apply", "--recipe-json", "/in/recipe.json", "--dir", "/workspace", "--out", "/out",
    )
    t.Logf("mods-openrewrite finished in %s\n%s", dur, firstN(out, 2000))
    // Verify report.json equals golden
    report := filepath.Join(outDir, "report.json")
    got, err := os.ReadFile(report)
    if err != nil { t.Fatalf("read report.json: %v", err) }
    writeReportFile(t, "orw-report.json", got)
    want, err := os.ReadFile(filepath.Join("expected", "orw", "report.json"))
    if err != nil { t.Fatalf("read golden: %v", err) }
    if strings.TrimSpace(string(got)) != strings.TrimSpace(string(want)) {
        t.Fatalf("report mismatch\nGot:\n%s\nWant:\n%s", got, want)
    }
    if dur > 6*time.Minute { t.Fatalf("mods-openrewrite exceeded budget: %s", dur) }
}

func TestModsLLMExec_HealsFailingBranch(t *testing.T) {
    ws := cloneRepo(t, "e2e/fail-missing-symbol")
    img := imagePrefix() + "/mods-llm:latest"
    _, dur := dockerRun(t, 1*time.Minute, "run", "--rm", "-v", ws+":/workspace", img, "mod-llm", "--execute", "--input", "/workspace")
    // Verify healed file content equals golden
    healed := filepath.Join(ws, "src/main/java/e2e/UnknownClass.java")
    got, err := os.ReadFile(healed)
    if err != nil { t.Fatalf("healed file: %v", err) }
    writeReportFile(t, "llm-UnknownClass.java", got)
    want, err := os.ReadFile(filepath.Join("expected", "llm", "UnknownClass.java"))
    if err != nil { t.Fatalf("read golden: %v", err) }
    if strings.TrimSpace(string(got)) != strings.TrimSpace(string(want)) {
        t.Fatalf("healed file mismatch")
    }
    t.Logf("mods-llm finished in %s", dur)
}

// Human gate image removed for now; will add scenario when real gate is available.

func firstN(s string, n int) string {
    if len(s) <= n { return s }
    return s[:n]
}
