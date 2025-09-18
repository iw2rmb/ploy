package build

import (
	"os"
	"strings"
)

// buildLogsURL constructs a public URL for builder logs stored under the unified storage bucket.
func buildLogsURL(logsKey string) string {
	if logsKey == "" {
		return ""
	}
	base := os.Getenv("PLOY_SEAWEEDFS_URL")
	if strings.TrimSpace(base) == "" {
		base = "http://seaweedfs-filer.storage.ploy.local:8888"
	}
	if !strings.HasPrefix(base, "http") {
		base = "http://" + base
	}
	base = strings.TrimRight(base, "/")
	// Ensure the artifacts collection prefix is present exactly once.
	if !strings.HasSuffix(base, "/artifacts") {
		base += "/artifacts"
	}
	return base + "/" + strings.TrimLeft(logsKey, "/")
}
