package step

import (
	"context"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestRunner_Run(t *testing.T) {
	runner := Runner{}
	manifest := contracts.StepManifest{
		ID:    types.StepID("test-step"),
		Name:  "Test Step",
		Image: "test:latest",
	}

	result, err := runner.Run(context.Background(), Request{Manifest: manifest})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestNormalizeContainerResourceUsage(t *testing.T) {
	disk := int64(4096)
	zeroDisk := int64(0)
	negativeDisk := int64(-1)
	maxInt64 := int64(^uint64(0) >> 1)

	tests := []struct {
		name  string
		usage *contracts.BuildGateResourceUsage
		want  *types.RunStatsJobResources
	}{
		{name: "nil usage", usage: nil, want: nil},
		{
			name: "uses peak memory and writable layer size",
			usage: &contracts.BuildGateResourceUsage{
				CPUTotalNs:    100,
				MemUsageBytes: 200,
				MemMaxBytes:   300,
				SizeRwBytes:   &disk,
			},
			want: &types.RunStatsJobResources{CPUConsumedNs: 100, DiskConsumedBytes: disk, MemConsumedBytes: 300},
		},
		{
			name: "falls back to current memory and saturates unsigned counters",
			usage: &contracts.BuildGateResourceUsage{
				CPUTotalNs:    ^uint64(0),
				MemUsageBytes: ^uint64(0),
			},
			want: &types.RunStatsJobResources{CPUConsumedNs: maxInt64, MemConsumedBytes: maxInt64},
		},
		{
			name:  "ignores zero writable layer size",
			usage: &contracts.BuildGateResourceUsage{SizeRwBytes: &zeroDisk},
			want:  &types.RunStatsJobResources{},
		},
		{
			name:  "ignores negative writable layer size",
			usage: &contracts.BuildGateResourceUsage{SizeRwBytes: &negativeDisk},
			want:  &types.RunStatsJobResources{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeContainerResourceUsage(tt.usage)
			if got == nil || tt.want == nil {
				if got != tt.want {
					t.Fatalf("NormalizeContainerResourceUsage() = %+v, want %+v", got, tt.want)
				}
				return
			}
			if *got != *tt.want {
				t.Fatalf("NormalizeContainerResourceUsage() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
