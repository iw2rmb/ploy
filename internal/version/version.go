package version

import (
	"fmt"
	"runtime"
	"time"
)

// Build-time variables (set via -ldflags)
var (
	// Version is the semantic version (set at build time)
	Version = "dev"
	
	// GitCommit is the git commit hash (set at build time)
	GitCommit = "unknown"
	
	// GitBranch is the git branch (set at build time)
	GitBranch = "unknown"
	
	// BuildTime is the build timestamp (set at build time)
	BuildTime = "unknown"
	
	// GoVersion is the Go version used to build
	GoVersion = runtime.Version()
	
	// Platform is the target platform
	Platform = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
)

// Info contains all version information
type Info struct {
	Version   string    `json:"version"`
	GitCommit string    `json:"git_commit"`
	GitBranch string    `json:"git_branch"`
	BuildTime string    `json:"build_time"`
	GoVersion string    `json:"go_version"`
	Platform  string    `json:"platform"`
	StartTime time.Time `json:"start_time"`
	Uptime    string    `json:"uptime"`
}

// Get returns the version info struct
func Get() Info {
	return Info{
		Version:   Version,
		GitCommit: GitCommit,
		GitBranch: GitBranch,
		BuildTime: BuildTime,
		GoVersion: GoVersion,
		Platform:  Platform,
		StartTime: startTime,
		Uptime:    time.Since(startTime).String(),
	}
}

// String returns a formatted version string
func String() string {
	return fmt.Sprintf("ploy-controller %s (commit: %s, branch: %s, built: %s)",
		Version, GitCommit, GitBranch, BuildTime)
}

// Short returns just the version
func Short() string {
	return Version
}

var startTime = time.Now()