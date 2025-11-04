package lifecycle

import (
	"path"
	"strings"

	"github.com/shirou/gopsutil/v4/net"
)

func (c *Collector) filterInterfaces(stats []net.IOCountersStat) []net.IOCountersStat {
	if len(stats) == 0 {
		return nil
	}
	out := make([]net.IOCountersStat, 0, len(stats))
	for _, stat := range stats {
		name := strings.TrimSpace(stat.Name)
		if name == "" {
			continue
		}
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "lo") {
			continue
		}
		if shouldIgnoreInterface(lower, c.ignoreInterfaces) {
			continue
		}
		out = append(out, stat)
	}
	return out
}

func netSliceToMap(stats []net.IOCountersStat) map[string]net.IOCountersStat {
	if len(stats) == 0 {
		return nil
	}
	dst := make(map[string]net.IOCountersStat, len(stats))
	for _, stat := range stats {
		dst[stat.Name] = stat
	}
	return dst
}

func normalizePatterns(patterns []string) []string {
	if len(patterns) == 0 {
		return nil
	}
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		if trimmed := strings.TrimSpace(pattern); trimmed != "" {
			out = append(out, strings.ToLower(trimmed))
		}
	}
	return out
}

func shouldIgnoreInterface(name string, patterns []string) bool {
	if len(patterns) == 0 || name == "" {
		return false
	}
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		p := strings.ToLower(pattern)
		if matched, err := path.Match(p, name); err == nil && matched {
			return true
		}
	}
	return false
}
