package transfers

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// JanitorStore abstracts slot lookups and state updates for janitor sweeps.
type JanitorStore interface {
	GetSlot(ctx context.Context, slotID string) (SlotRecord, error)
	UpdateSlotState(ctx context.Context, slotID string, revision int64, state SlotState, digest string) (SlotRecord, error)
}

// JanitorOptions configure the janitor.
type JanitorOptions struct {
	Logger   *log.Logger
	BaseDir  string
	Store    JanitorStore
	Clock    func() time.Time
	Interval time.Duration
}

// Janitor removes stale slot directories and aborts expired slots.
type Janitor struct {
	opts     JanitorOptions
	logger   *log.Logger
	baseDir  string
	store    JanitorStore
	now      func() time.Time
	interval time.Duration
	wg       sync.WaitGroup
}

// NewJanitor constructs a janitor instance.
func NewJanitor(opts JanitorOptions) (*Janitor, error) {
	if opts.Store == nil {
		return nil, errors.New("slot janitor: store required")
	}
	baseDir := strings.TrimSpace(opts.BaseDir)
	if baseDir == "" {
		baseDir = "/var/lib/ploy/ssh-artifacts"
	}
	logger := opts.Logger
	if logger == nil {
		logger = log.New(os.Stderr, "slot-janitor: ", log.LstdFlags)
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = time.Minute
	}
	return &Janitor{
		opts:     opts,
		logger:   logger,
		baseDir:  baseDir,
		store:    opts.Store,
		now:      clock,
		interval: interval,
	}, nil
}

// Start launches the periodic janitor loop. The returned cancel function stops it.
func (j *Janitor) Start(ctx context.Context) context.CancelFunc {
	runCtx, cancel := context.WithCancel(ctx)
	j.wg.Add(1)
	go func() {
		defer j.wg.Done()
		j.loop(runCtx)
	}()
	return cancel
}

// Wait blocks until the janitor loop exits.
func (j *Janitor) Wait() {
	j.wg.Wait()
}

// Sweep performs a single cleanup pass.
func (j *Janitor) Sweep(ctx context.Context) error {
	slotsDir := filepath.Join(j.baseDir, "slots")
	entries, err := os.ReadDir(slotsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("slot janitor: read %s: %w", slotsDir, err)
	}
	now := j.now()
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slotID := entry.Name()
		dir := filepath.Join(slotsDir, slotID)
		j.handleSlot(ctx, slotID, dir, now)
	}
	return nil
}

func (j *Janitor) loop(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		if err := j.Sweep(ctx); err != nil {
			j.logger.Printf("sweep error: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (j *Janitor) handleSlot(ctx context.Context, slotID, dir string, now time.Time) {
	record, err := j.store.GetSlot(ctx, slotID)
	if err != nil {
		if removeErr := os.RemoveAll(dir); removeErr != nil {
			j.logger.Printf("remove orphan %s: %v (lookup err: %v)", dir, removeErr, err)
		}
		return
	}
	slot := record.Slot
	// Determine the authoritative local path.
	localPath := strings.TrimSpace(slot.LocalPath)
	if localPath == "" {
		cleanRemote := strings.TrimPrefix(filepath.Clean(strings.TrimSpace(slot.RemotePath)), "/")
		localPath = filepath.Join(j.baseDir, cleanRemote)
	}
	slotDir := filepath.Dir(localPath)
	switch {
	case slot.State != SlotPending:
		j.removeDir(slotDir)
	case !slot.ExpiresAt.IsZero() && !slot.ExpiresAt.After(now):
		if _, err := j.store.UpdateSlotState(ctx, slotID, record.Revision, SlotAborted, slot.Digest); err != nil {
			j.logger.Printf("abort slot %s: %v", slotID, err)
			return
		}
		j.removeDir(slotDir)
	default:
		// Keep active slot.
	}
}

func (j *Janitor) removeDir(dir string) {
	if err := os.RemoveAll(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		j.logger.Printf("remove slot dir %s: %v", dir, err)
	}
}
