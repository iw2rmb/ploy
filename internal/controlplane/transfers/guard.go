package transfers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// GuardStore defines the minimal slot store interface required by the guard.
type GuardStore interface {
	GetSlot(ctx context.Context, slotID string) (SlotRecord, error)
}

// GuardRunner executes the configured SFTP server binary.
type GuardRunner interface {
	Exec(bin string, args []string, env []string) error
}

// GuardOptions configure the SlotGuard helper.
type GuardOptions struct {
	Logger     *log.Logger
	SlotID     string
	Store      GuardStore
	BaseDir    string
	ServerPath string
	Runner     GuardRunner
	Clock      func() time.Time
}

// Guard validates slot metadata and launches the SFTP subsystem.
type Guard struct {
	opts     GuardOptions
	runner   GuardRunner
	logger   *log.Logger
	now      func() time.Time
	baseDir  string
	server   string
	slotID   string
	slot     *Slot
	store    GuardStore
	onceExec bool
}

// NewGuard initialises a SlotGuard instance.
func NewGuard(opts GuardOptions) (*Guard, error) {
	slotID := strings.TrimSpace(opts.SlotID)
	if slotID == "" {
		return nil, errors.New("slot guard: slot id required")
	}
	if opts.Store == nil {
		return nil, errors.New("slot guard: store required")
	}
	server := strings.TrimSpace(opts.ServerPath)
	if server == "" {
		server = "/usr/lib/openssh/sftp-server"
	}
	baseDir := strings.TrimSpace(opts.BaseDir)
	if baseDir == "" {
		baseDir = "/var/lib/ploy/ssh-artifacts"
	}
	runner := opts.Runner
	if runner == nil {
		runner = execRunner{}
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	logger := opts.Logger
	if logger == nil {
		logger = log.New(os.Stderr, "slot-guard: ", log.LstdFlags)
	}
	return &Guard{
		opts:    opts,
		runner:  runner,
		logger:  logger,
		now:     clock,
		baseDir: baseDir,
		server:  server,
		slotID:  slotID,
		store:   opts.Store,
	}, nil
}

// Run executes the guard flow for the configured slot.
func (g *Guard) Run(ctx context.Context) error {
	record, err := g.store.GetSlot(ctx, g.slotID)
	if err != nil {
		return fmt.Errorf("slot guard: load slot %s: %w", g.slotID, err)
	}
	slot := record.Slot
	g.slot = &slot

	if slot.State != SlotPending {
		return fmt.Errorf("slot guard: slot %s not pending", slot.ID)
	}
	if !slot.ExpiresAt.IsZero() && !slot.ExpiresAt.After(g.now()) {
		return fmt.Errorf("slot guard: slot %s expired", slot.ID)
	}

	localPath, err := g.resolveLocalPath(slot)
	if err != nil {
		return err
	}
	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("slot guard: prepare dir %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0o750); err != nil {
		// chmod can fail if fs lacks support; log and continue.
		g.logger.Printf("chmod %s: %v", dir, err)
	}
	args := []string{"-d", dir}
	if err := g.runner.Exec(g.server, args, os.Environ()); err != nil {
		return fmt.Errorf("slot guard: exec %s: %w", g.server, err)
	}
	return nil
}

func (g *Guard) resolveLocalPath(slot Slot) (string, error) {
	if trimmed := strings.TrimSpace(slot.LocalPath); trimmed != "" {
		if !filepath.IsAbs(trimmed) {
			return "", fmt.Errorf("slot guard: local path %s not absolute", trimmed)
		}
		if !strings.HasPrefix(trimmed, g.baseDir) {
			return "", fmt.Errorf("slot guard: local path %s outside base dir", trimmed)
		}
		return trimmed, nil
	}
	path := strings.TrimSpace(slot.RemotePath)
	if path == "" {
		return "", errors.New("slot guard: remote path missing")
	}
	clean := filepath.Clean(path)
	if strings.HasPrefix(clean, "/") {
		clean = strings.TrimPrefix(clean, "/")
	}
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("slot guard: remote path %s invalid", slot.RemotePath)
	}
	local := filepath.Join(g.baseDir, clean)
	return local, nil
}

type execRunner struct{}

func (execRunner) Exec(bin string, args []string, env []string) error {
	return syscall.Exec(bin, append([]string{bin}, args...), env)
}
