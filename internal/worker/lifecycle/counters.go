package lifecycle

import "github.com/shirou/gopsutil/v4/disk"

func cloneDiskCounters(src map[string]disk.IOCountersStat) map[string]disk.IOCountersStat {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]disk.IOCountersStat, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func deltaUint(prev, cur uint64) uint64 {
	if cur >= prev {
		return cur - prev
	}
	return 0
}
