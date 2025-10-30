package javaexec

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "time"

    "github.com/iw2rmb/ploy/internal/workflow/buildgate"
)

// CommandResult captures stdout/stderr emitted by the command invocation.
type CommandResult struct {
    Stdout   string
    Stderr   string
    ExitCode int
}

// CommandRunner executes commands in a workspace or on the host.
type CommandRunner interface {
    Run(ctx context.Context, cmd []string, env map[string]string, dir string) (CommandResult, error)
}

// Options configures the Java sandbox executor.
type Options struct {
    Runner        CommandRunner
    MavenImage    string
}

const defaultMavenImage = "maven:3-eclipse-temurin-17"

// NewExecutor constructs a sandbox executor that runs Java tests using Gradle/Maven
// wrappers when present, otherwise falls back to Dockerised Maven.
func NewExecutor(opts Options) (buildgate.SandboxExecutor, error) {
    runner := opts.Runner
    if runner == nil {
        runner = execRunner{}
    }
    image := strings.TrimSpace(opts.MavenImage)
    if image == "" {
        if env := strings.TrimSpace(os.Getenv("PLOY_BUILDGATE_JAVA_IMAGE")); env != "" {
            image = env
        } else {
            image = defaultMavenImage
        }
    }
    return &executor{runner: runner, mavenImage: image}, nil
}

type executor struct {
    runner     CommandRunner
    mavenImage string
}

func (e *executor) Execute(ctx context.Context, spec buildgate.SandboxSpec) (buildgate.SandboxBuildResult, error) {
    workspace := strings.TrimSpace(spec.Workspace)
    if workspace == "" {
        return buildgate.SandboxBuildResult{}, fmt.Errorf("javaexec: workspace path required")
    }

    // Choose execution strategy
    tool, cmd := e.selectCommand(workspace)
    env := mergeEnv(spec.Env)

    // Respect context timeout via caller; just run.
    start := time.Now()
    result, runErr := e.runner.Run(ctx, cmd, env, workspace)
    duration := time.Since(start)
    logDigest := digest(result.Stdout, result.Stderr)

    // Prepare base payload
    summary := summaryPayload{
        Status:     "success",
        Tool:       tool,
        Orchestrator: "ployd-buildgate",
        ExitCode:   result.ExitCode,
        DurationMs: int64(duration / time.Millisecond),
        Workspace:  workspace,
    }

    success := runErr == nil && result.ExitCode == 0
    if !success {
        summary.Status = "failed"
    }
    report, _ := json.Marshal(summary)

    // Build metadata findings
    findings := []buildgate.LogFinding{}
    sev := "info"
    if !success {
        sev = "error"
    }
    evidence := []string{"tool=" + tool}
    if summary.DurationMs > 0 {
        evidence = append(evidence, fmt.Sprintf("duration=%dms", summary.DurationMs))
    }
    if result.ExitCode != 0 {
        evidence = append(evidence, fmt.Sprintf("exit_code=%d", result.ExitCode))
    }
    findings = append(findings, buildgate.LogFinding{
        Code:     "buildgate.java.summary",
        Severity: sev,
        Message:  fmt.Sprintf("java build gate status %s", summary.Status),
        Evidence: strings.Join(evidence, " "),
    })

    // Construct final result
    buildResult := buildgate.SandboxBuildResult{
        Success:   success,
        CacheHit:  false,
        LogDigest: logDigest,
        Metadata: buildgate.Metadata{
            LogFindings: findings,
        },
        Report: report,
    }

    if !success {
        buildResult.FailureReason = "exit_code"
        // Prefer stderr, then stdout, else generic message.
        detail := firstLine(strings.TrimSpace(result.Stderr))
        if detail == "" {
            detail = firstLine(strings.TrimSpace(result.Stdout))
        }
        if detail == "" {
            detail = fmt.Sprintf("command exited with code %d", result.ExitCode)
        } else {
            detail = fmt.Sprintf("command exited with code %d: %s", result.ExitCode, detail)
        }
        if runErr != nil && !errors.Is(runErr, context.DeadlineExceeded) {
            // Include runner error if it wasn't just a non-zero exit
            buildResult.FailureReason = "execution"
            buildResult.FailureDetail = strings.TrimSpace(runErr.Error())
        } else {
            buildResult.FailureDetail = detail
        }
    }

    return buildResult, nil
}

func (e *executor) selectCommand(workspace string) (tool string, cmd []string) {
    gradlew := filepath.Join(workspace, "gradlew")
    mvnw := filepath.Join(workspace, "mvnw")
    if fileExists(gradlew) {
        // Use Gradle wrapper
        return "gradle-wrapper", []string{"./gradlew", "test", "--no-daemon", "--stacktrace"}
    }
    if fileExists(mvnw) {
        // Use Maven wrapper
        return "maven-wrapper", []string{"./mvnw", "-B", "-DfailIfNoTests=false", "test"}
    }
    // Docker fallback with Maven image
    // Mount workspace at /ws and run tests
    return "docker-maven", []string{
        "docker", "run", "--rm",
        "-v", workspace + ":/ws",
        "-w", "/ws",
        e.mavenImage,
        "mvn", "-B", "-DfailIfNoTests=false", "test",
    }
}

func fileExists(path string) bool {
    info, err := os.Stat(path)
    if err != nil {
        return false
    }
    return !info.IsDir()
}

func mergeEnv(specEnv map[string]string) map[string]string {
    merged := make(map[string]string)
    for _, entry := range os.Environ() {
        parts := strings.SplitN(entry, "=", 2)
        if len(parts) == 2 {
            merged[parts[0]] = parts[1]
        }
    }
    for k, v := range specEnv {
        merged[k] = v
    }
    return merged
}

func digest(stdout, stderr string) string {
    combined := strings.TrimSpace(stdout + stderr)
    if combined == "" {
        return ""
    }
    sum := sha256.Sum256([]byte(stdout + stderr))
    return "sha256:" + hex.EncodeToString(sum[:])
}

func firstLine(text string) string {
    for _, line := range strings.Split(text, "\n") {
        if trimmed := strings.TrimSpace(line); trimmed != "" {
            return trimmed
        }
    }
    return ""
}

type summaryPayload struct {
    Status       string `json:"status"`
    Tool         string `json:"tool"`
    Orchestrator string `json:"orchestrator"`
    ExitCode     int    `json:"exit_code"`
    DurationMs   int64  `json:"duration_ms"`
    Workspace    string `json:"workspace"`
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, cmd []string, env map[string]string, dir string) (CommandResult, error) {
    if len(cmd) == 0 {
        return CommandResult{}, errors.New("javaexec: command missing")
    }
    command := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
    if dir != "" {
        command.Dir = dir
    }
    if len(env) > 0 {
        entries := make([]string, 0, len(env))
        // Merge with current environment
        merged := mergeEnv(env)
        for k, v := range merged {
            entries = append(entries, fmt.Sprintf("%s=%s", k, v))
        }
        command.Env = entries
    }
    var stdout, stderr strings.Builder
    command.Stdout = &stdout
    command.Stderr = &stderr
    err := command.Run()
    result := CommandResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: 0}
    if exitErr, ok := err.(*exec.ExitError); ok {
        result.ExitCode = exitErr.ExitCode()
        // Non-zero exit code is not treated as a transport error here
        return result, nil
    }
    if err != nil {
        return result, err
    }
    return result, nil
}

