package bootstrap

import (
	"embed"
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

const inlineLibPlaceholder = "# @@BOOTSTRAP_INLINE_LIBS@@"

var defaultRequiredPorts = []int{2379, 2380, 9094, 9095}

//go:embed assets/bootstrap.sh
var scriptTemplate string

//go:embed assets/lib/*.sh
var libFiles embed.FS

var inlineLibBlock = buildInlineLibSnippet()

// Script returns the raw bootstrap shell script.
func Script() string {
	if strings.Contains(scriptTemplate, inlineLibPlaceholder) {
		return strings.Replace(scriptTemplate, inlineLibPlaceholder, inlineLibBlock, 1)
	}
	return inlineLibBlock + "\n" + scriptTemplate
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
	script := Script()
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
	builder.WriteString(script)
	return builder.String()
}

func buildInlineLibSnippet() string {
	entries, err := libFiles.ReadDir("assets/lib")
	if err != nil {
		panic(fmt.Errorf("bootstrap: read inline libs: %w", err))
	}
	if len(entries) == 0 {
		return "bootstrap_inline_libdir() { printf '%s\\n' '${TMPDIR:-/tmp}'; }"
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	type lib struct {
		name string
		data string
	}
	libs := make([]lib, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, readErr := libFiles.ReadFile("assets/lib/" + entry.Name())
		if readErr != nil {
			panic(fmt.Errorf("bootstrap: read inline lib %s: %w", entry.Name(), readErr))
		}
		libs = append(libs, lib{name: entry.Name(), data: string(content)})
	}
	builder := strings.Builder{}
	builder.WriteString("bootstrap_inline_libdir() {\n")
	builder.WriteString("  local dir\n")
	builder.WriteString("  dir=\"$(mktemp -d \"${TMPDIR:-/tmp}/ploy-bootstrap-lib.XXXXXX\")\"\n")
	builder.WriteString("  if [[ -z \"$dir\" ]]; then\n")
	builder.WriteString("    printf '[bootstrap][error] failed to create inline library dir\\n' >&2\n")
	builder.WriteString("    exit 1\n")
	builder.WriteString("  fi\n")
	for _, lib := range libs {
		marker := strings.ToUpper(strings.NewReplacer(".", "_", "-", "_", "/", "_").Replace(lib.name))
		builder.WriteString(fmt.Sprintf("  cat <<'%s' >\"${dir}/%s\"\n", marker, lib.name))
		builder.WriteString(lib.data)
		if !strings.HasSuffix(lib.data, "\n") {
			builder.WriteString("\n")
		}
		builder.WriteString(marker + "\n")
	}
	builder.WriteString("  printf '%s\\n' \"$dir\"\n")
	builder.WriteString("}\n")
	return builder.String()
}
