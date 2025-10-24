package bootstrap

import (
	_ "embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	// Version identifies the bootstrap script revision applied to target hosts.
	Version = "2025-10-24"
	// DefaultMinDiskGB represents the minimum free disk required by the bootstrap script.
	DefaultMinDiskGB = 4
	// RequiredPackages lists user-facing package names emitted by the bootstrap preflight logs.
	RequiredPackages = "ipfs-cluster-service docker etcd go"
)

var defaultRequiredPorts = []int{2379, 2380, 9094, 9095}

//go:embed assets/bootstrap.sh
var scriptTemplate string

// Script returns the raw bootstrap shell script.
func Script() string {
	return scriptTemplate
}

// DefaultRequiredPorts returns a copy of the port list the bootstrap script checks before proceeding.
func DefaultRequiredPorts() []int {
	out := make([]int, len(defaultRequiredPorts))
	copy(out, defaultRequiredPorts)
	return out
}

// DefaultExports returns the baseline environment exports required before running the script.
func DefaultExports() map[string]string {
	ports := DefaultRequiredPorts()
	portStrings := make([]string, len(ports))
	for i, port := range ports {
		portStrings[i] = strconv.Itoa(port)
	}
	return map[string]string{
		"PLOY_BOOTSTRAP_VERSION": Version,
		"PLOY_MIN_DISK_GB":       strconv.Itoa(DefaultMinDiskGB),
		"PLOY_REQUIRED_PORTS":    strings.Join(portStrings, " "),
		"PLOY_REQUIRED_PACKAGES": RequiredPackages,
	}
}

// PrefixedScript renders the bootstrap script preceded by export statements derived from the provided map.
func PrefixedScript(exports map[string]string) string {
	builder := strings.Builder{}
	if len(exports) > 0 {
		keys := make([]string, 0, len(exports))
		for key := range exports {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			builder.WriteString(fmt.Sprintf("export %s=%q\n", key, exports[key]))
		}
		builder.WriteString("\n")
	}
	builder.WriteString(scriptTemplate)
	return builder.String()
}
