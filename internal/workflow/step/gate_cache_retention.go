package step

import (
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

const buildGateCacheMinFreeBytes int64 = 2 << 30

var gateCacheFreeBytes = dirFreeBytes

type gateCacheEntry struct {
	path    string
	modTime int64
}

// pruneGateCacheDirOldestFirst removes top-level entries from cacheDir in
// oldest-first order until free space reaches buildGateCacheMinFreeBytes or the
// directory is exhausted.
func pruneGateCacheDirOldestFirst(cacheDir string) error {
	free, err := gateCacheFreeBytes(cacheDir)
	if err != nil || free >= buildGateCacheMinFreeBytes {
		return err
	}

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return err
	}
	candidates := make([]gateCacheEntry, 0, len(entries))
	for _, entry := range entries {
		info, ierr := entry.Info()
		if ierr != nil {
			continue
		}
		candidates = append(candidates, gateCacheEntry{
			path:    filepath.Join(cacheDir, entry.Name()),
			modTime: info.ModTime().UnixNano(),
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].modTime == candidates[j].modTime {
			return candidates[i].path < candidates[j].path
		}
		return candidates[i].modTime < candidates[j].modTime
	})

	for _, candidate := range candidates {
		if free >= buildGateCacheMinFreeBytes {
			return nil
		}
		if err := os.RemoveAll(candidate.path); err != nil {
			return err
		}
		free, err = gateCacheFreeBytes(cacheDir)
		if err != nil {
			return err
		}
	}
	return nil
}

func dirFreeBytes(path string) (int64, error) {
	var stats syscall.Statfs_t
	if err := syscall.Statfs(path, &stats); err != nil {
		return 0, err
	}
	return int64(stats.Bavail) * int64(stats.Bsize), nil
}

