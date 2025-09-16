package mods

import (
    "context"
    "testing"
)

func TestSummaryComputer_RunCompaction_Noops(t *testing.T) {
    mockStorage := new(MockKBStorage)
    mockLockMgr := new(SummaryMockLockManager)
    computer := NewSummaryComputer(mockStorage, mockLockMgr, DefaultSummaryConfig())

    stats, err := computer.RunCompaction(context.Background())
    if err != nil {
        t.Fatalf("RunCompaction returned error: %v", err)
    }
    if stats == nil {
        t.Fatal("expected non-nil stats")
    }
    if stats.ProcessedSignatures != 0 || stats.UpdatedSummaries != 0 || stats.ErrorCount != 0 {
        t.Fatalf("expected zeroed stats, got %+v", *stats)
    }
}

