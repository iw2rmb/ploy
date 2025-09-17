package arf

import "time"

// SandboxStatus represents the current state of a sandbox.
type SandboxStatus string

const (
	SandboxStatusCreating SandboxStatus = "creating"
	SandboxStatusReady    SandboxStatus = "ready"
	SandboxStatusRunning  SandboxStatus = "running"
	SandboxStatusStopped  SandboxStatus = "stopped"
	SandboxStatusError    SandboxStatus = "error"
	SandboxStatusExpired  SandboxStatus = "expired"
)

// Sandbox captures high-level metadata about an isolated environment.
type Sandbox struct {
	ID         string            `json:"id"`
	JailName   string            `json:"jail_name"`
	RootPath   string            `json:"root_path"`
	WorkingDir string            `json:"working_dir"`
	CreatedAt  time.Time         `json:"created_at"`
	ExpiresAt  time.Time         `json:"expires_at"`
	Status     SandboxStatus     `json:"status"`
	Config     SandboxConfig     `json:"config"`
	Metadata   map[string]string `json:"metadata"`
}

// SandboxConfig defines sandbox execution parameters preserved for legacy data structures.
type SandboxConfig struct {
	Repository    string        `json:"repository"`
	Branch        string        `json:"branch"`
	LocalPath     string        `json:"local_path"`
	Language      string        `json:"language"`
	BuildTool     string        `json:"build_tool"`
	TTL           time.Duration `json:"ttl"`
	MemoryLimit   string        `json:"memory_limit"`
	CPULimit      string        `json:"cpu_limit"`
	NetworkAccess bool          `json:"network_access"`
	TempSpace     string        `json:"temp_space"`
}

// SandboxInfo provides summary information about a sandbox.
type SandboxInfo struct {
	ID         string        `json:"id"`
	JailName   string        `json:"jail_name"`
	Status     SandboxStatus `json:"status"`
	CreatedAt  time.Time     `json:"created_at"`
	ExpiresAt  time.Time     `json:"expires_at"`
	Repository string        `json:"repository"`
}
