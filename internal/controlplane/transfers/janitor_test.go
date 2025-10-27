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

type janitorStoreFake struct {
	slots     map[string]transfers.SlotRecord
	updateLog []updateCall
	err       error
}

type updateCall struct {
	slotID string
	state  transfers.SlotState
}

func (f *janitorStoreFake) GetSlot(ctx context.Context, slotID string) (transfers.SlotRecord, error) {
	if f.err != nil {
		return transfers.SlotRecord{}, f.err
	}
	rec, ok := f.slots[slotID]
	if !ok {
		return transfers.SlotRecord{}, errors.New("slot not found")
	}
	return rec, nil
}

func (f *janitorStoreFake) UpdateSlotState(ctx context.Context, slotID string, rev int64, state transfers.SlotState, digest string) (transfers.SlotRecord, error) {
	rec, ok := f.slots[slotID]
	if !ok {
		return transfers.SlotRecord{}, errors.New("slot not found")
	}
	rec.Slot.State = state
	f.slots[slotID] = rec
	f.updateLog = append(f.updateLog, updateCall{slotID: slotID, state: state})
	return rec, nil
}

func TestJanitorAbortsAndDeletesExpiredSlot(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Date(2025, 10, 27, 17, 30, 0, 0, time.UTC)
	slotID := "slot-expired"
	slotDir := filepath.Join(baseDir, "slots", slotID)
	if err := os.MkdirAll(slotDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	store := &janitorStoreFake{
		slots: map[string]transfers.SlotRecord{
			slotID: {
				Slot: transfers.Slot{
					ID:        slotID,
					JobID:     "job-janitor",
					NodeID:    "node-1",
					LocalPath: filepath.Join(slotDir, "payload"),
					ExpiresAt: now.Add(-5 * time.Minute),
					State:     transfers.SlotPending,
				},
			},
		},
	}
	janitor, err := transfers.NewJanitor(transfers.JanitorOptions{
		Logger:  log.New(os.Stderr, "", 0),
		BaseDir: baseDir,
		Store:   store,
		Clock:   func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewJanitor: %v", err)
	}
	if err := janitor.Sweep(context.Background()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if _, err := os.Stat(slotDir); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be removed, err=%v", slotDir, err)
	}
	if len(store.updateLog) == 0 {
		t.Fatalf("expected janitor to update slot state")
	}
	if store.slots[slotID].Slot.State != transfers.SlotAborted {
		t.Fatalf("expected slot to be aborted, got %s", store.slots[slotID].Slot.State)
	}
}

func TestJanitorKeepsActiveSlotDirectory(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Date(2025, 10, 27, 18, 0, 0, 0, time.UTC)
	slotID := "slot-active"
	slotDir := filepath.Join(baseDir, "slots", slotID)
	if err := os.MkdirAll(slotDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	store := &janitorStoreFake{
		slots: map[string]transfers.SlotRecord{
			slotID: {
				Slot: transfers.Slot{
					ID:        slotID,
					JobID:     "job-janitor",
					NodeID:    "node-1",
					LocalPath: filepath.Join(slotDir, "payload"),
					ExpiresAt: now.Add(10 * time.Minute),
					State:     transfers.SlotPending,
				},
			},
		},
	}
	janitor, err := transfers.NewJanitor(transfers.JanitorOptions{
		Logger:  log.New(os.Stderr, "", 0),
		BaseDir: baseDir,
		Store:   store,
		Clock:   func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewJanitor: %v", err)
	}
	if err := janitor.Sweep(context.Background()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if _, err := os.Stat(slotDir); err != nil {
		t.Fatalf("expected %s to remain: %v", slotDir, err)
	}
	if len(store.updateLog) != 0 {
		t.Fatalf("did not expect slot updates for active slot")
	}
}

func TestJanitorDeletesUnknownSlotDirectory(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Date(2025, 10, 27, 18, 30, 0, 0, time.UTC)
	slotID := "slot-orphan"
	slotDir := filepath.Join(baseDir, "slots", slotID)
	if err := os.MkdirAll(slotDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	store := &janitorStoreFake{
		slots: map[string]transfers.SlotRecord{},
	}
	janitor, err := transfers.NewJanitor(transfers.JanitorOptions{
		Logger:  log.New(os.Stderr, "", 0),
		BaseDir: baseDir,
		Store:   store,
		Clock:   func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewJanitor: %v", err)
	}
	if err := janitor.Sweep(context.Background()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if _, err := os.Stat(slotDir); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be removed, err=%v", slotDir, err)
	}
}
