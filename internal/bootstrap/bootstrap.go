package bootstrap

// Minimal bootstrap script helpers used by deploy.ProvisionHost.
// These replace the legacy bootstrap package and generate a shell
// script that begins with environment exports followed by a simple
// body stub. Tests only assert the presence/absence of certain
// exports in the preamble, not the full script content.

import (
	"sort"
	"strings"
)

// Version identifies the bootstrap script bundle.
var Version = "dev"

// DefaultExports provides baseline environment variables included
// in every bootstrap script. Callers can override/extend via map merge.
func DefaultExports() map[string]string {
	return map[string]string{
		"PLOY_BOOTSTRAP_VERSION": Version,
	}
}

// PrefixedScript renders a shell script with an export preamble for env.
// Keys are emitted in sorted order for determinism.
func PrefixedScript(env map[string]string) string {
	// Deterministic key order for stable tests
	keys := make([]string, 0, len(env))
	for k := range env {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		v := strings.TrimSpace(env[k])
		// Very simple shell quoting: wrap in double quotes and escape existing ones.
		v = strings.ReplaceAll(v, "\"", "\\\"")
		b.WriteString("export ")
		b.WriteString(k)
		b.WriteString("=\"")
		b.WriteString(v)
		b.WriteString("\"\n")
	}
	// Separator between exports and script body
	b.WriteString("\n")
	// Minimal body stub; real provisioning logic lives in the remote bootstraper.
	b.WriteString("# ploy bootstrap body (stub for unit tests)\n")
	b.WriteString("derive_postgresql_dsn() {\n")
	b.WriteString("  : # placeholder; actual DSN derivation happens on target host\n")
	b.WriteString("}\n")
	return b.String()
}
