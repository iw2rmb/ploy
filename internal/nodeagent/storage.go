package nodeagent

import (
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shirou/gopsutil/v4/disk"
)

type storageDiagnostics struct {
	Paths          []storagePathDiagnostics `json:"paths"`
	SelectedPath   string                   `json:"selected_path,omitempty"`
	SelectedSource string                   `json:"selected_source,omitempty"`
	FreeBytes      int64                    `json:"free_bytes,omitempty"`
	TotalBytes     int64                    `json:"total_bytes,omitempty"`
}

type storagePathDiagnostics struct {
	Source            string  `json:"source"`
	Path              string  `json:"path"`
	TotalBytes        int64   `json:"total_bytes,omitempty"`
	FreeBytes         int64   `json:"free_bytes,omitempty"`
	UsedBytes         int64   `json:"used_bytes,omitempty"`
	UsedPercent       float64 `json:"used_percent,omitempty"`
	InodesTotal       int64   `json:"inodes_total,omitempty"`
	InodesFree        int64   `json:"inodes_free,omitempty"`
	InodesUsed        int64   `json:"inodes_used,omitempty"`
	InodesUsedPercent float64 `json:"inodes_used_percent,omitempty"`
	Error             string  `json:"error,omitempty"`
}

type storageProbeTarget struct {
	source string
	path   string
}

type storageUsageFunc func(string) (*disk.UsageStat, error)

func collectStorageDiagnostics() storageDiagnostics {
	return collectStorageDiagnosticsWith(os.Getenv, disk.Usage)
}

func collectStorageDiagnosticsWith(getenv func(string) string, usage storageUsageFunc) storageDiagnostics {
	targets := storageProbeTargets(getenv)
	paths := make([]storagePathDiagnostics, 0, len(targets))
	seen := map[string]bool{}
	for _, target := range targets {
		cleaned := filepath.Clean(strings.TrimSpace(target.path))
		if cleaned == "." || cleaned == "" || seen[cleaned] {
			continue
		}
		seen[cleaned] = true
		item := storagePathDiagnostics{Source: target.source, Path: cleaned}
		stat, err := usage(cleaned)
		if err != nil {
			item.Error = err.Error()
			paths = append(paths, item)
			continue
		}
		item.TotalBytes = uint64ToStorageInt64(stat.Total)
		item.FreeBytes = uint64ToStorageInt64(stat.Free)
		item.UsedBytes = uint64ToStorageInt64(stat.Used)
		item.UsedPercent = stat.UsedPercent
		item.InodesTotal = uint64ToStorageInt64(stat.InodesTotal)
		item.InodesFree = uint64ToStorageInt64(stat.InodesFree)
		item.InodesUsed = uint64ToStorageInt64(stat.InodesUsed)
		item.InodesUsedPercent = stat.InodesUsedPercent
		paths = append(paths, item)
	}
	sort.SliceStable(paths, func(i, j int) bool {
		if paths[i].Source == paths[j].Source {
			return paths[i].Path < paths[j].Path
		}
		return paths[i].Source < paths[j].Source
	})

	diag := storageDiagnostics{Paths: paths}
	for _, item := range paths {
		if item.Error != "" || item.TotalBytes <= 0 {
			continue
		}
		if diag.SelectedPath == "" || item.FreeBytes < diag.FreeBytes {
			diag.SelectedPath = item.Path
			diag.SelectedSource = item.Source
			diag.FreeBytes = item.FreeBytes
			diag.TotalBytes = item.TotalBytes
		}
	}
	return diag
}

func storageProbeTargets(getenv func(string) string) []storageProbeTarget {
	tmpdir := strings.TrimSpace(getenv("TMPDIR"))
	if tmpdir == "" {
		tmpdir = os.TempDir()
	}
	return []storageProbeTarget{
		{source: "root", path: "/"},
		{source: "DOCKER_ROOT_DIR", path: envOrDefault(getenv, "DOCKER_ROOT_DIR", "/var/lib/docker")},
		{source: "PLOYD_CACHE_HOME", path: getenv("PLOYD_CACHE_HOME")},
		{source: "PLOY_BUILDGATE_CACHE_ROOT", path: getenv("PLOY_BUILDGATE_CACHE_ROOT")},
		{source: "TMPDIR", path: tmpdir},
	}
}

func envOrDefault(getenv func(string) string, key, fallback string) string {
	value := strings.TrimSpace(getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func uint64ToStorageInt64(value uint64) int64 {
	if value > uint64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(value)
}

func (d storageDiagnostics) heartbeatDiskBytes(fallbackFree, fallbackTotal int64) (int64, int64) {
	if d.SelectedPath == "" {
		return fallbackFree, fallbackTotal
	}
	if d.FreeBytes < 0 || d.TotalBytes < 0 || d.FreeBytes > d.TotalBytes {
		return fallbackFree, fallbackTotal
	}
	return d.FreeBytes, d.TotalBytes
}
