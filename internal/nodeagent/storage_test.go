package nodeagent

import (
	"errors"
	"testing"

	"github.com/shirou/gopsutil/v4/disk"
)

func TestCollectStorageDiagnostics(t *testing.T) {
	tests := []struct {
		name               string
		env                map[string]string
		stats              map[string]*disk.UsageStat
		wantSelectedPath   string
		wantSelectedSource string
		wantPathCount      int
	}{
		{
			name: "selects configured path with least free space",
			env: map[string]string{
				"DOCKER_ROOT_DIR":           "/mnt/ploy/docker",
				"PLOYD_CACHE_HOME":          "/mnt/ploy/caches/node",
				"PLOY_BUILDGATE_CACHE_ROOT": "/mnt/ploy/caches/build",
				"TMPDIR":                    "/mnt/ploy/caches/tmp",
			},
			stats: map[string]*disk.UsageStat{
				"/":                      {Path: "/", Total: 1000, Free: 900, Used: 100, UsedPercent: 10},
				"/mnt/ploy/docker":       {Path: "/mnt/ploy/docker", Total: 1000, Free: 400, Used: 600, UsedPercent: 60},
				"/mnt/ploy/caches/node":  {Path: "/mnt/ploy/caches/node", Total: 1000, Free: 10, Used: 990, UsedPercent: 99},
				"/mnt/ploy/caches/build": {Path: "/mnt/ploy/caches/build", Total: 1000, Free: 100, Used: 900, UsedPercent: 90},
				"/mnt/ploy/caches/tmp":   {Path: "/mnt/ploy/caches/tmp", Total: 1000, Free: 200, Used: 800, UsedPercent: 80},
			},
			wantSelectedPath:   "/mnt/ploy/caches/node",
			wantSelectedSource: "PLOYD_CACHE_HOME",
			wantPathCount:      5,
		},
		{
			name: "deduplicates identical paths and skips unset optional roots",
			env: map[string]string{
				"DOCKER_ROOT_DIR":  "/mnt/ploy",
				"PLOYD_CACHE_HOME": "/mnt/ploy",
				"TMPDIR":           "/tmp",
			},
			stats: map[string]*disk.UsageStat{
				"/":         {Path: "/", Total: 1000, Free: 800, Used: 200},
				"/mnt/ploy": {Path: "/mnt/ploy", Total: 1000, Free: 300, Used: 700},
				"/tmp":      {Path: "/tmp", Total: 1000, Free: 500, Used: 500},
			},
			wantSelectedPath:   "/mnt/ploy",
			wantSelectedSource: "DOCKER_ROOT_DIR",
			wantPathCount:      3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diag := collectStorageDiagnosticsWith(func(key string) string {
				return tc.env[key]
			}, func(path string) (*disk.UsageStat, error) {
				stat := tc.stats[path]
				if stat == nil {
					return nil, errors.New("missing stat")
				}
				return stat, nil
			})

			if diag.SelectedPath != tc.wantSelectedPath || diag.SelectedSource != tc.wantSelectedSource {
				t.Fatalf("selected = (%s,%s), want (%s,%s)", diag.SelectedPath, diag.SelectedSource, tc.wantSelectedPath, tc.wantSelectedSource)
			}
			if len(diag.Paths) != tc.wantPathCount {
				t.Fatalf("paths len = %d, want %d: %#v", len(diag.Paths), tc.wantPathCount, diag.Paths)
			}
			free, total := diag.heartbeatDiskBytes(1, 2)
			if free != diag.FreeBytes || total != diag.TotalBytes {
				t.Fatalf("heartbeat disk = (%d,%d), want selected (%d,%d)", free, total, diag.FreeBytes, diag.TotalBytes)
			}
		})
	}
}
