package transfers_test

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/controlplane/transfers"
)

type guardStoreFake struct {
	slot   transfers.Slot
	err    error
	calls  int
	record transfers.SlotRecord
}

func (f *guardStoreFake) GetSlot(ctx context.Context, slotID string) (transfers.SlotRecord, error) {
	f.calls++
	if f.err != nil {
		return transfers.SlotRecord{}, f.err
	}
	if slotID != f.slot.ID {
		return transfers.SlotRecord{}, errors.New("unknown slot")
	}
	return transfers.SlotRecord{Slot: f.slot}, nil
}

type guardRunnerFake struct {
	bin   string
	args  []string
	env   []string
	calls int
}

func (r *guardRunnerFake) Exec(bin string, args []string, env []string) error {
	r.calls++
	r.bin = bin
	r.args = append([]string(nil), args...)
	r.env = append([]string(nil), env...)
	return nil
}

func TestGuardLaunchesSFTPForPendingSlot(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Date(2025, 10, 27, 15, 0, 0, 0, time.UTC)

	localPath := filepath.Join(baseDir, "slots", "slot-guard", "payload")
	slot := transfers.Slot{
		ID:         "slot-guard",
		JobID:      "job-guard",
		NodeID:     "node-a",
		RemotePath: "/slots/slot-guard/payload",
		LocalPath:  localPath,
		ExpiresAt:  now.Add(10 * time.Minute),
		State:      transfers.SlotPending,
	}
	store := &guardStoreFake{slot: slot}
	runner := &guardRunnerFake{}

	guard, err := transfers.NewGuard(transfers.GuardOptions{
		Logger:     log.New(os.Stderr, "", 0),
		SlotID:     slot.ID,
		Store:      store,
		BaseDir:    baseDir,
		ServerPath: "/usr/lib/openssh/sftp-server",
		Runner:     runner,
		Clock:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewGuard: %v", err)
	}
	if err := guard.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if runner.calls == 0 {
		t.Fatalf("expected guard to exec sftp server")
	}
	if runner.bin != "/usr/lib/openssh/sftp-server" {
		t.Fatalf("unexpected binary: %s", runner.bin)
	}
	expectedDir := filepath.Dir(localPath)
	if _, err := os.Stat(expectedDir); err != nil {
		t.Fatalf("expected directory %s to exist: %v", expectedDir, err)
	}
	found := false
	for idx := range runner.args {
		if runner.args[idx] == "-d" && idx+1 < len(runner.args) && runner.args[idx+1] == expectedDir {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected -d %s in args, got %v", expectedDir, runner.args)
	}
}

func TestGuardRejectsExpiredSlot(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Date(2025, 10, 27, 16, 0, 0, 0, time.UTC)
	slot := transfers.Slot{
		ID:         "slot-expired",
		JobID:      "job-expired",
		NodeID:     "node-a",
		RemotePath: "/slots/slot-expired/payload",
		LocalPath:  filepath.Join(baseDir, "slots", "slot-expired", "payload"),
		ExpiresAt:  now.Add(-1 * time.Minute),
		State:      transfers.SlotPending,
	}
	store := &guardStoreFake{slot: slot}
	runner := &guardRunnerFake{}

	guard, err := transfers.NewGuard(transfers.GuardOptions{
		Logger:     log.New(os.Stderr, "", 0),
		SlotID:     slot.ID,
		Store:      store,
		BaseDir:    baseDir,
		ServerPath: "/usr/lib/openssh/sftp-server",
		Runner:     runner,
		Clock:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewGuard: %v", err)
	}
	if err := guard.Run(context.Background()); err == nil {
		t.Fatalf("expected guard to reject expired slot")
	}
	if runner.calls > 0 {
		t.Fatalf("expected guard not to exec when slot expired")
	}
}
