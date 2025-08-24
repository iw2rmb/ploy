package arf

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// SandboxManager provides secure isolated environments for transformations
type SandboxManager interface {
	CreateSandbox(ctx context.Context, config SandboxConfig) (*Sandbox, error)
	DestroySandbox(ctx context.Context, sandboxID string) error
	ListSandboxes(ctx context.Context) ([]SandboxInfo, error)
	CleanupExpiredSandboxes(ctx context.Context) error
}

// Sandbox represents an isolated environment for transformations
type Sandbox struct {
	ID          string            `json:"id"`
	JailName    string            `json:"jail_name"`
	RootPath    string            `json:"root_path"`
	WorkingDir  string            `json:"working_dir"`
	CreatedAt   time.Time         `json:"created_at"`
	ExpiresAt   time.Time         `json:"expires_at"`
	Status      SandboxStatus     `json:"status"`
	Config      SandboxConfig     `json:"config"`
	Metadata    map[string]string `json:"metadata"`
}

// SandboxConfig defines sandbox creation parameters
type SandboxConfig struct {
	Repository    string        `json:"repository"`
	Branch        string        `json:"branch"`
	LocalPath     string        `json:"local_path"`     // Path to transformed code (if already cloned/modified)
	Language      string        `json:"language"`
	BuildTool     string        `json:"build_tool"`
	TTL           time.Duration `json:"ttl"`
	MemoryLimit   string        `json:"memory_limit"`   // e.g., "2G"
	CPULimit      string        `json:"cpu_limit"`      // e.g., "2"
	NetworkAccess bool          `json:"network_access"`
	TempSpace     string        `json:"temp_space"`     // e.g., "1G"
}

// SandboxStatus represents the current state of a sandbox
type SandboxStatus string

const (
	SandboxStatusCreating SandboxStatus = "creating"
	SandboxStatusReady    SandboxStatus = "ready"
	SandboxStatusRunning  SandboxStatus = "running"
	SandboxStatusStopped  SandboxStatus = "stopped"
	SandboxStatusError    SandboxStatus = "error"
	SandboxStatusExpired  SandboxStatus = "expired"
)

// SandboxInfo provides summary information about a sandbox
type SandboxInfo struct {
	ID         string        `json:"id"`
	JailName   string        `json:"jail_name"`
	Status     SandboxStatus `json:"status"`
	CreatedAt  time.Time     `json:"created_at"`
	ExpiresAt  time.Time     `json:"expires_at"`
	Repository string        `json:"repository"`
}

// FreeBSDJailManager implements SandboxManager using FreeBSD jails
type FreeBSDJailManager struct {
	jailBaseDir   string
	templateDir   string
	maxSandboxes  int
	defaultTTL    time.Duration
	jailInterface string // Network interface for jails
}

// NewFreeBSDJailManager creates a new FreeBSD jail-based sandbox manager
func NewFreeBSDJailManager(baseDir, templateDir string, maxSandboxes int, defaultTTL time.Duration, jailInterface string) *FreeBSDJailManager {
	return &FreeBSDJailManager{
		jailBaseDir:   baseDir,
		templateDir:   templateDir,
		maxSandboxes:  maxSandboxes,
		defaultTTL:    defaultTTL,
		jailInterface: jailInterface,
	}
}

// CreateSandbox creates a new FreeBSD jail sandbox
func (m *FreeBSDJailManager) CreateSandbox(ctx context.Context, config SandboxConfig) (*Sandbox, error) {
	// Check if jail system is available
	if _, err := exec.LookPath("jail"); err != nil {
		return nil, fmt.Errorf("FreeBSD jail system not available: %w", err)
	}

	// Generate unique sandbox ID
	sandboxID := fmt.Sprintf("arf-%d", time.Now().UnixNano())
	jailName := fmt.Sprintf("arf-sandbox-%s", sandboxID[4:14]) // Use last 10 chars
	
	// Set default TTL if not specified
	ttl := config.TTL
	if ttl == 0 {
		ttl = m.defaultTTL
	}

	sandbox := &Sandbox{
		ID:         sandboxID,
		JailName:   jailName,
		RootPath:   filepath.Join(m.jailBaseDir, jailName),
		WorkingDir: "/workspace",
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(ttl),
		Status:     SandboxStatusCreating,
		Config:     config,
		Metadata:   make(map[string]string),
	}

	// Create jail root directory
	if err := os.MkdirAll(sandbox.RootPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create jail root: %w", err)
	}

	// Copy base template to jail root
	if err := m.copyTemplate(sandbox.RootPath); err != nil {
		os.RemoveAll(sandbox.RootPath)
		return nil, fmt.Errorf("failed to copy jail template: %w", err)
	}

	// Create workspace directory
	workspaceDir := filepath.Join(sandbox.RootPath, "workspace")
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		os.RemoveAll(sandbox.RootPath)
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	// Clone repository into workspace
	if config.Repository != "" {
		if err := m.cloneRepository(ctx, config.Repository, config.Branch, workspaceDir); err != nil {
			os.RemoveAll(sandbox.RootPath)
			return nil, fmt.Errorf("failed to clone repository: %w", err)
		}
	}

	// Create jail configuration
	jailConf := m.generateJailConfig(sandbox, config)
	if err := m.writeJailConfig(jailName, jailConf); err != nil {
		os.RemoveAll(sandbox.RootPath)
		return nil, fmt.Errorf("failed to write jail config: %w", err)
	}

	// Start the jail
	if err := m.startJail(ctx, jailName); err != nil {
		os.RemoveAll(sandbox.RootPath)
		m.removeJailConfig(jailName)
		return nil, fmt.Errorf("failed to start jail: %w", err)
	}

	sandbox.Status = SandboxStatusReady
	return sandbox, nil
}

// DestroySandbox removes a sandbox and its jail
func (m *FreeBSDJailManager) DestroySandbox(ctx context.Context, sandboxID string) error {
	// Find sandbox by ID (in production, this would be stored in a database)
	sandboxes, err := m.ListSandboxes(ctx)
	if err != nil {
		return fmt.Errorf("failed to list sandboxes: %w", err)
	}

	var targetJail string
	for _, sb := range sandboxes {
		if sb.ID == sandboxID {
			targetJail = sb.JailName
			break
		}
	}

	if targetJail == "" {
		return fmt.Errorf("sandbox %s not found", sandboxID)
	}

	// Stop and remove jail
	if err := m.stopJail(ctx, targetJail); err != nil {
		return fmt.Errorf("failed to stop jail: %w", err)
	}

	// Remove jail configuration
	if err := m.removeJailConfig(targetJail); err != nil {
		return fmt.Errorf("failed to remove jail config: %w", err)
	}

	// Remove jail root directory
	jailRoot := filepath.Join(m.jailBaseDir, targetJail)
	if err := os.RemoveAll(jailRoot); err != nil {
		return fmt.Errorf("failed to remove jail root: %w", err)
	}

	return nil
}

// ListSandboxes returns information about all active sandboxes
func (m *FreeBSDJailManager) ListSandboxes(ctx context.Context) ([]SandboxInfo, error) {
	cmd := exec.CommandContext(ctx, "jls", "-h", "jid", "name", "path")
	output, err := cmd.Output()
	if err != nil {
		// If jls command fails, return empty list instead of error
		// This allows the API to work even if jail system is not configured
		return []SandboxInfo{}, nil
	}

	var sandboxes []SandboxInfo
	lines := strings.Split(string(output), "\n")
	
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // Skip header and empty lines
		}

		fields := strings.Fields(line)
		if len(fields) >= 3 && strings.HasPrefix(fields[1], "arf-sandbox-") {
			// Extract sandbox ID from jail name
			sandboxID := "arf-" + fields[1][12:] // Remove "arf-sandbox-" prefix

			// Get creation and expiration times (would be from database in production)
			now := time.Now()
			sandboxes = append(sandboxes, SandboxInfo{
				ID:        sandboxID,
				JailName:  fields[1],
				Status:    SandboxStatusReady, // Would query actual status
				CreatedAt: now.Add(-time.Hour), // Placeholder
				ExpiresAt: now.Add(time.Hour),  // Placeholder
			})
		}
	}

	return sandboxes, nil
}

// CleanupExpiredSandboxes removes sandboxes that have exceeded their TTL
func (m *FreeBSDJailManager) CleanupExpiredSandboxes(ctx context.Context) error {
	sandboxes, err := m.ListSandboxes(ctx)
	if err != nil {
		return fmt.Errorf("failed to list sandboxes: %w", err)
	}

	now := time.Now()
	for _, sandbox := range sandboxes {
		if now.After(sandbox.ExpiresAt) {
			if err := m.DestroySandbox(ctx, sandbox.ID); err != nil {
				// Log error but continue cleanup
				fmt.Printf("Failed to cleanup expired sandbox %s: %v\n", sandbox.ID, err)
			}
		}
	}

	return nil
}

// Helper methods

func (m *FreeBSDJailManager) copyTemplate(destPath string) error {
	cmd := exec.Command("cp", "-R", m.templateDir+"/.", destPath)
	return cmd.Run()
}

func (m *FreeBSDJailManager) cloneRepository(ctx context.Context, repo, branch, destPath string) error {
	args := []string{"clone"}
	if branch != "" {
		args = append(args, "-b", branch)
	}
	args = append(args, repo, destPath)

	cmd := exec.CommandContext(ctx, "git", args...)
	return cmd.Run()
}

func (m *FreeBSDJailManager) generateJailConfig(sandbox *Sandbox, config SandboxConfig) string {
	conf := fmt.Sprintf(`%s {
    path = "%s";
    host.hostname = "%s";
    ip4.addr = "%s";
    interface = "%s";
    exec.start = "/bin/sh /etc/rc";
    exec.stop = "/bin/sh /etc/rc.shutdown";
    exec.clean;
    mount.devfs;
    allow.raw_sockets = 0;
    securelevel = 3;
}`, sandbox.JailName, sandbox.RootPath, sandbox.JailName, "inherit", m.jailInterface)

	return conf
}

func (m *FreeBSDJailManager) writeJailConfig(jailName, config string) error {
	confPath := fmt.Sprintf("/etc/jail.conf.d/%s.conf", jailName)
	return os.WriteFile(confPath, []byte(config), 0644)
}

func (m *FreeBSDJailManager) removeJailConfig(jailName string) error {
	confPath := fmt.Sprintf("/etc/jail.conf.d/%s.conf", jailName)
	return os.Remove(confPath)
}

func (m *FreeBSDJailManager) startJail(ctx context.Context, jailName string) error {
	cmd := exec.CommandContext(ctx, "jail", "-c", jailName)
	return cmd.Run()
}

func (m *FreeBSDJailManager) stopJail(ctx context.Context, jailName string) error {
	cmd := exec.CommandContext(ctx, "jail", "-r", jailName)
	return cmd.Run()
}

// MockSandboxManager provides a mock implementation for non-FreeBSD systems
type MockSandboxManager struct {
	sandboxes map[string]*Sandbox
}

// NewMockSandboxManager creates a new mock sandbox manager for development/testing
func NewMockSandboxManager() *MockSandboxManager {
	return &MockSandboxManager{
		sandboxes: make(map[string]*Sandbox),
	}
}

// CreateSandbox creates a mock sandbox for testing
func (m *MockSandboxManager) CreateSandbox(ctx context.Context, config SandboxConfig) (*Sandbox, error) {
	sandboxID := fmt.Sprintf("mock-%d", time.Now().UnixNano())
	
	ttl := config.TTL
	if ttl == 0 {
		ttl = 30 * time.Minute
	}

	rootPath := fmt.Sprintf("/tmp/mock-sandbox-%s", sandboxID[5:15])
	workspacePath := filepath.Join(rootPath, "workspace")

	sandbox := &Sandbox{
		ID:         sandboxID,
		JailName:   fmt.Sprintf("mock-sandbox-%s", sandboxID[5:15]),
		RootPath:   rootPath,
		WorkingDir: "/workspace",
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(ttl),
		Status:     SandboxStatusReady,
		Config:     config,
		Metadata:   make(map[string]string),
	}

	// Create mock sandbox directory structure
	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create mock sandbox workspace: %w", err)
	}

	m.sandboxes[sandboxID] = sandbox
	return sandbox, nil
}

// DestroySandbox removes a mock sandbox
func (m *MockSandboxManager) DestroySandbox(ctx context.Context, sandboxID string) error {
	sandbox, exists := m.sandboxes[sandboxID]
	if !exists {
		return fmt.Errorf("sandbox %s not found", sandboxID)
	}
	
	// Clean up mock sandbox directory
	if err := os.RemoveAll(sandbox.RootPath); err != nil {
		// Log error but don't fail - this is a mock sandbox
		fmt.Printf("Warning: Failed to clean up mock sandbox directory %s: %v\n", sandbox.RootPath, err)
	}
	
	delete(m.sandboxes, sandboxID)
	return nil
}

// ListSandboxes returns all mock sandboxes
func (m *MockSandboxManager) ListSandboxes(ctx context.Context) ([]SandboxInfo, error) {
	var sandboxes []SandboxInfo
	
	for _, sb := range m.sandboxes {
		sandboxes = append(sandboxes, SandboxInfo{
			ID:         sb.ID,
			JailName:   sb.JailName,
			Status:     sb.Status,
			CreatedAt:  sb.CreatedAt,
			ExpiresAt:  sb.ExpiresAt,
			Repository: sb.Config.Repository,
		})
	}
	
	return sandboxes, nil
}

// CleanupExpiredSandboxes removes expired mock sandboxes
func (m *MockSandboxManager) CleanupExpiredSandboxes(ctx context.Context) error {
	now := time.Now()
	
	for id, sandbox := range m.sandboxes {
		if now.After(sandbox.ExpiresAt) {
			delete(m.sandboxes, id)
		}
	}
	
	return nil
}

// NewSandboxManagerForOS creates appropriate sandbox manager for the current OS
func NewSandboxManagerForOS(jailBaseDir, templateDir string, maxSandboxes int, defaultTTL time.Duration, jailInterface string) SandboxManager {
	// Check if we're running inside the controller itself
	// The controller sets PORT environment variable and also has PLOY_VERSION set by Nomad
	port := os.Getenv("PORT")
	ployVersion := os.Getenv("PLOY_VERSION")
	
	// If we have both PORT and PLOY_VERSION, we're running inside the controller
	if port != "" && ployVersion != "" {
		// We're inside the controller, use localhost to call ourselves
		controllerURL := fmt.Sprintf("http://localhost:%s/v1", port)
		fmt.Printf("Running inside controller (version %s), using self-reference: %s\n", ployVersion, controllerURL)
		return NewDeploymentSandboxManager(controllerURL)
	}
	
	// Check if we have a controller URL configured (for external clients)
	if controllerURL := os.Getenv("PLOY_CONTROLLER"); controllerURL != "" {
		fmt.Printf("Using deployment sandbox manager with controller: %s\n", controllerURL)
		return NewDeploymentSandboxManager(controllerURL)
	}
	
	// Check for default environment (ploy CLI available)
	if _, err := exec.LookPath("ploy"); err == nil {
		fmt.Println("Using deployment sandbox manager with default controller")
		// Use default controller URL
		return NewDeploymentSandboxManager("https://api.dev.ployd.app/v1")
	}
	
	// Check for remote FreeBSD jail host configuration
	freebsdHost := os.Getenv("ARF_FREEBSD_HOST")
	if freebsdHost != "" {
		return NewRemoteFreeBSDJailManager(freebsdHost, jailBaseDir, templateDir, maxSandboxes, defaultTTL, jailInterface)
	}
	
	if runtime.GOOS == "freebsd" {
		// Check if jail command is available
		if _, err := exec.LookPath("jail"); err == nil {
			return NewFreeBSDJailManager(jailBaseDir, templateDir, maxSandboxes, defaultTTL, jailInterface)
		}
	}
	
	// Fallback to mock sandbox manager for development/testing
	fmt.Println("Warning: Using mock sandbox manager. Set PLOY_CONTROLLER env var for deployment integration.")
	return NewMockSandboxManager()
}

// RemoteFreeBSDJailManager manages jails on a remote FreeBSD host via SSH
type RemoteFreeBSDJailManager struct {
	*FreeBSDJailManager
	host string
	user string
	port int
}

// NewRemoteFreeBSDJailManager creates a new remote FreeBSD jail manager
func NewRemoteFreeBSDJailManager(host, jailBaseDir, templateDir string, maxSandboxes int, defaultTTL time.Duration, jailInterface string) *RemoteFreeBSDJailManager {
	baseMgr := NewFreeBSDJailManager(jailBaseDir, templateDir, maxSandboxes, defaultTTL, jailInterface)
	
	user := os.Getenv("ARF_FREEBSD_USER")
	if user == "" {
		user = "root"
	}
	
	port := 22
	if portStr := os.Getenv("ARF_FREEBSD_PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}
	
	return &RemoteFreeBSDJailManager{
		FreeBSDJailManager: baseMgr,
		host:               host,
		user:               user,
		port:               port,
	}
}

// executeRemoteCommand executes a command on the remote FreeBSD host
func (m *RemoteFreeBSDJailManager) executeRemoteCommand(ctx context.Context, command string) error {
	sshCmd := fmt.Sprintf("ssh -o StrictHostKeyChecking=no -p %d %s@%s '%s'", m.port, m.user, m.host, command)
	cmd := exec.CommandContext(ctx, "sh", "-c", sshCmd)
	return cmd.Run()
}

// executeRemoteCommandWithOutput executes a command on the remote FreeBSD host and returns output
func (m *RemoteFreeBSDJailManager) executeRemoteCommandWithOutput(ctx context.Context, command string) ([]byte, error) {
	sshCmd := fmt.Sprintf("ssh -o StrictHostKeyChecking=no -p %d %s@%s '%s'", m.port, m.user, m.host, command)
	cmd := exec.CommandContext(ctx, "sh", "-c", sshCmd)
	return cmd.Output()
}

// ListSandboxes returns information about all active sandboxes on remote FreeBSD host
func (m *RemoteFreeBSDJailManager) ListSandboxes(ctx context.Context) ([]SandboxInfo, error) {
	output, err := m.executeRemoteCommandWithOutput(ctx, "jls -h jid name path")
	if err != nil {
		// If remote command fails, return empty list to allow API to work
		return []SandboxInfo{}, nil
	}

	var sandboxes []SandboxInfo
	lines := strings.Split(string(output), "\n")
	
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // Skip header and empty lines
		}

		fields := strings.Fields(line)
		if len(fields) >= 3 && strings.HasPrefix(fields[1], "arf-sandbox-") {
			// Extract sandbox ID from jail name
			sandboxID := "arf-" + fields[1][12:] // Remove "arf-sandbox-" prefix

			// Get creation and expiration times (would be from database in production)
			now := time.Now()
			sandboxes = append(sandboxes, SandboxInfo{
				ID:        sandboxID,
				JailName:  fields[1],
				Status:    SandboxStatusReady, // Would query actual status
				CreatedAt: now.Add(-time.Hour), // Placeholder
				ExpiresAt: now.Add(time.Hour),  // Placeholder
			})
		}
	}

	return sandboxes, nil
}

// startJail starts a jail on the remote FreeBSD host
func (m *RemoteFreeBSDJailManager) startJail(ctx context.Context, jailName string) error {
	command := fmt.Sprintf("jail -c %s", jailName)
	return m.executeRemoteCommand(ctx, command)
}

// stopJail stops a jail on the remote FreeBSD host
func (m *RemoteFreeBSDJailManager) stopJail(ctx context.Context, jailName string) error {
	command := fmt.Sprintf("jail -r %s", jailName)
	return m.executeRemoteCommand(ctx, command)
}