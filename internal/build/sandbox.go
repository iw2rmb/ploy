package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	ibuilders "github.com/iw2rmb/ploy/internal/builders"
)

// SandboxOptions controls optional analysis features for sandbox builds.
type SandboxOptions struct {
	EnableSecurity       bool
	EnableStaticAnalysis bool
}

// SandboxRequest describes a sandbox build invocation.
type SandboxRequest struct {
	RepoPath  string
	AppName   string
	SHA       string
	Lane      string
	MainClass string
	EnvVars   map[string]string
	Timeout   time.Duration
	Options   SandboxOptions
}

// SandboxArtifact represents a build artifact produced during sandbox execution.
type SandboxArtifact struct {
	Path string
	Type string
}

// AnalyzerIssue represents a static or security finding surfaced during sandbox builds.
type AnalyzerIssue struct {
	Tool     string
	Severity string
	Message  string
	Location string
	Metadata map[string]string
}

// SandboxResult captures the outcome of a sandbox build.
type SandboxResult struct {
	Success     bool
	Message     string
	BuildSystem string
	Language    string
	Duration    time.Duration
	Stdout      string
	Stderr      string
	Errors      []ParsedBuildError
	Issues      []AnalyzerIssue
	Artifacts   []SandboxArtifact
}

// SandboxService executes repository builds without deployment side effects.
type SandboxService struct{}

// NewSandboxService constructs a sandbox build service.
func NewSandboxService() *SandboxService {
	return &SandboxService{}
}

// Run executes the sandbox build with the provided request.
func (s *SandboxService) Run(ctx context.Context, req SandboxRequest) (*SandboxResult, error) {
	if req.RepoPath == "" {
		return nil, fmt.Errorf("sandbox build requires repo path")
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	appName := req.AppName
	if strings.TrimSpace(appName) == "" {
		appName = filepath.Base(req.RepoPath)
		if appName == "" || appName == "." {
			appName = "sandbox-app"
		}
	}
	sha := req.SHA
	if strings.TrimSpace(sha) == "" {
		sha = fmt.Sprintf("sandbox-%d", time.Now().Unix())
	}

	laneHint := strings.ToUpper(strings.TrimSpace(req.Lane))
	mainHint := strings.TrimSpace(req.MainClass)

	_, language, _, mainClass, _ := detectBuildContext(req.RepoPath, laneHint, mainHint)
	lane := "D"
	if mainHint != "" {
		mainClass = mainHint
	}

	result := &SandboxResult{BuildSystem: strings.ToUpper(lane), Language: language}

	start := time.Now()
	artifact, buildErr := s.runLaneBuild(ctx, lane, appName, sha, req.RepoPath, mainClass, req.EnvVars)
	result.Duration = time.Since(start)

	if buildErr != nil {
		result.Success = false
		result.Errors = ParseBuildErrors(language, strings.ToUpper(lane), buildErr.Error())
		if len(result.Errors) > 0 {
			first := result.Errors[0]
			msg := first.Message
			if strings.TrimSpace(first.File) != "" && first.Line > 0 {
				msg = fmt.Sprintf("%s (%s:%d)", msg, first.File, first.Line)
			}
			result.Message = msg
		} else {
			result.Message = buildErr.Error()
		}
		return result, nil
	}

	result.Success = true
	result.Message = "build succeeded"
	if artifact != nil {
		result.Artifacts = append(result.Artifacts, *artifact)
	}

	// Future hooks: populate result.Issues when static/security analyzers are enabled.

	return result, nil
}

func (s *SandboxService) runLaneBuild(ctx context.Context, lane, appName, sha, repoPath, mainClass string, envVars map[string]string) (*SandboxArtifact, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	tmpDir, err := os.MkdirTemp("", "sandbox-build-")
	if err != nil {
		return nil, fmt.Errorf("create sandbox workspace: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	imageTag := sha
	if strings.TrimSpace(imageTag) == "" {
		imageTag = fmt.Sprintf("sandbox-%d", time.Now().Unix())
	}
	imageRef, err := ibuilders.BuildOCI(appName, repoPath, imageTag, envVars)
	if err != nil {
		return nil, err
	}
	return &SandboxArtifact{Path: imageRef, Type: "oci"}, nil
}
