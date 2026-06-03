package step

import (
	"os"
	"strconv"
	"strings"

	units "github.com/docker/go-units"
)

const (
	gateLimitMemoryEnv = "PLOY_BUILDGATE_LIMIT_MEMORY_BYTES"
	gateLimitCPUEnv    = "PLOY_BUILDGATE_LIMIT_CPU_MILLIS"
	gateLimitDiskEnv   = "PLOY_BUILDGATE_LIMIT_DISK_SPACE"
	gateCacheRootEnv   = "PLOY_BUILDGATE_CACHE_ROOT"
	gateCacheRootDir   = "/var/cache/ploy/gates"
	gateTmpCacheRoot   = "ploy/gates"
)

func parseInt64(s string) (int64, error) { return strconv.ParseInt(strings.TrimSpace(s), 10, 64) }

func parseInt64LimitEnv(key string) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0
	}
	n, err := parseInt64(value)
	if err != nil {
		return 0
	}
	return n
}

// parseBytesLimitEnv reads a byte-quantity env var. Accepts RAM (e.g. "2g"),
// human size ("2GB"), or a raw int64. Returns the parsed bytes and the original
// string for passthrough to Docker --storage-opt size=.
func parseBytesLimitEnv(key string) (int64, string) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0, ""
	}

	if n, err := units.RAMInBytes(value); err == nil {
		return n, value
	}
	if n, err := units.FromHumanSize(value); err == nil {
		return n, value
	}
	if n, err := parseInt64(value); err == nil {
		return n, value
	}
	return 0, value
}
