package distribution

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
)

// BuildPipeline handles automated controller builds and distribution
type BuildPipeline struct {
	distributor *BinaryDistributor
	buildDir    string
	gitRepo     string
}

// NewBuildPipeline creates a new build pipeline
func NewBuildPipeline(storageProvider storage.StorageProvider, collection, cacheDir, buildDir, gitRepo string) *BuildPipeline {
	return &BuildPipeline{
		distributor: NewBinaryDistributor(storageProvider, collection, cacheDir),
		buildDir:    buildDir,
		gitRepo:     gitRepo,
	}
}

// BuildAndDistribute builds the controller and distributes it
func (bp *BuildPipeline) BuildAndDistribute(version string, platforms []string, metadata map[string]string) error {
	// Get git commit hash
	gitCommit, err := bp.getGitCommit()
	if err != nil {
		return fmt.Errorf("failed to get git commit: %w", err)
	}

	// Build for each platform
	for _, platform := range platforms {
		parts := strings.Split(platform, "/")
		if len(parts) != 2 {
			return fmt.Errorf("invalid platform format: %s (expected os/arch)", platform)
		}

		goos, goarch := parts[0], parts[1]

		// Build binary
		binaryPath, err := bp.buildBinary(goos, goarch, version)
		if err != nil {
			return fmt.Errorf("failed to build binary for %s: %w", platform, err)
		}

		// Create binary info
		info := CreateBinaryInfo(version, gitCommit, metadata)
		info.Platform = goos
		info.Architecture = goarch
		info.Path = binaryPath

		// Upload to storage
		if err := bp.distributor.UploadBinary(binaryPath, info); err != nil {
			return fmt.Errorf("failed to upload binary for %s: %w", platform, err)
		}

		// Clean up local binary
		_ = os.Remove(binaryPath)

		fmt.Printf("Successfully built and distributed controller v%s for %s\n", version, platform)
	}

	return nil
}

// buildBinary builds the controller binary for specified platform
func (bp *BuildPipeline) buildBinary(goos, goarch, version string) (string, error) {
	// Create build directory
	if err := os.MkdirAll(bp.buildDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create build directory: %w", err)
	}

	// Binary output path
	binaryName := "controller"
	if goos == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(bp.buildDir, fmt.Sprintf("controller-%s-%s-%s", version, goos, goarch))

	// Build command
	cmd := exec.Command("go", "build", "-o", binaryPath, "./controller")
	cmd.Dir = bp.gitRepo
	cmd.Env = append(os.Environ(),
		"GOOS="+goos,
		"GOARCH="+goarch,
		"CGO_ENABLED=0",
	)

	// Add build-time variables
	ldflags := fmt.Sprintf("-ldflags=-X main.Version=%s -X main.BuildTime=%s -X main.GitCommit=%s",
		version,
		time.Now().Format(time.RFC3339),
		bp.getCurrentGitCommit(),
	)

	cmd.Args = append(cmd.Args[:2], ldflags)
	cmd.Args = append(cmd.Args, cmd.Args[2:]...)

	// Execute build
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build failed: %w\nOutput: %s", err, string(output))
	}

	return binaryPath, nil
}

// getGitCommit gets the current git commit hash
func (bp *BuildPipeline) getGitCommit() (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = bp.gitRepo

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

// getCurrentGitCommit gets current git commit (helper for ldflags)
func (bp *BuildPipeline) getCurrentGitCommit() string {
	commit, err := bp.getGitCommit()
	if err != nil {
		return "unknown"
	}
	return commit
}

// GetCommonPlatforms returns common deployment platforms
func GetCommonPlatforms() []string {
	return []string{
		"linux/amd64",
		"linux/arm64",
		"darwin/amd64",
		"darwin/arm64",
		"windows/amd64",
	}
}

// GetLinuxPlatforms returns Linux-specific platforms for server deployment
func GetLinuxPlatforms() []string {
	return []string{
		"linux/amd64",
		"linux/arm64",
	}
}
