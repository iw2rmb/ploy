package step

import (
	"errors"
	"syscall"
)

const buildGateCacheMinFreeBytes int64 = 2 << 30

var gateCacheFreeBytes = dirFreeBytes

var ErrNotEnoughSpace = errors.New("Not enough space")

func IsNotEnoughSpaceError(err error) bool {
	return errors.Is(err, ErrNotEnoughSpace)
}

func ensureGateCacheCapacity(cacheDir string) error {
	free, err := gateCacheFreeBytes(cacheDir)
	if err != nil {
		return err
	}
	if free < buildGateCacheMinFreeBytes {
		return ErrNotEnoughSpace
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
