package step

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// caPreambleScript returns a shell preamble that installs CA certificates from the
// CA_CERTS_PEM_BUNDLE environment variable into the system trust store and Java
// cacerts keystore. This enables build-gate containers to trust corporate proxies
// and private registries when the global config provides a CA bundle.
//
// The preamble:
// 1. Splits CA_CERTS_PEM_BUNDLE into individual PEM files
// 2. Installs them into /usr/local/share/ca-certificates and runs update-ca-certificates
// 3. Imports each cert into the Java cacerts keystore via keytool (if available)
//
// This preamble is prepended to Maven, Gradle, and plain Java build commands so that
// custom CA certificates injected via `ploy config env set --key CA_CERTS_PEM_BUNDLE ...`
// are honored inside gate containers.
func caPreambleScript() string {
	return `# --- CA bundle injection preamble (ploy global config) ---
if [ -n "${CA_CERTS_PEM_BUNDLE:-}" ]; then
  pem_file="$(mktemp)"
  printf '%s\n' "${CA_CERTS_PEM_BUNDLE}" > "${pem_file}"
  pem_dir="$(mktemp -d)"
  # Split bundle into individual cert files: cert1.crt, cert2.crt, ...
  awk '/-----BEGIN CERTIFICATE-----/{n++} {print > (d"/cert" n ".crt")}' d="${pem_dir}" "${pem_file}"
  # Update system CA store if update-ca-certificates is available
  if command -v update-ca-certificates >/dev/null 2>&1; then
    sys_ca_dir="/usr/local/share/ca-certificates/ploy-gate"
    mkdir -p "$sys_ca_dir"
    cp "${pem_dir}"/*.crt "$sys_ca_dir"/ 2>/dev/null || true
    update-ca-certificates >/dev/null 2>&1 || true
  fi
  # Import into Java cacerts keystore if keytool is available
  if command -v keytool >/dev/null 2>&1; then
    for cert_path in "${pem_dir}"/*.crt; do
      [ -f "$cert_path" ] || continue
      base="$(basename "${cert_path}" .crt)"
      alias="ploy_gate_pem_${base}"
      keytool -importcert -noprompt -trustcacerts -cacerts -storepass changeit -alias "${alias}" -file "${cert_path}" >/dev/null 2>&1 || true
    done
  fi
fi
# --- End CA bundle preamble ---
`
}

// buildCommandForTool returns the default all-tests command for the given tool.
func buildCommandForTool(workspace string, tool string) ([]string, error) {
	return buildCommandForToolTarget(workspace, tool, contracts.GateProfileTargetAllTests)
}

// buildCommandForToolTarget returns a deterministic command for a tool/target pair.
func buildCommandForToolTarget(workspace string, tool string, target string) ([]string, error) {
	preamble := caPreambleScript()
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "maven":
		switch strings.TrimSpace(target) {
		case contracts.GateProfileTargetBuild:
			script := preamble + "mvn --ff -B -q -e -DskipTests=true -Dstyle.color=never -f /workspace/pom.xml clean install"
			return []string{"/bin/sh", "-lc", script}, nil
		case contracts.GateProfileTargetAllTests:
			script := preamble + "mvn --ff -B -q -e -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml clean install"
			return []string{"/bin/sh", "-lc", script}, nil
		default:
			return nil, fmt.Errorf("unsupported maven target: %q", target)
		}
	case "gradle":
		gradleExec := "gradle"
		if hasGradleWrapperSpecified(workspace) {
			gradleExec = "./gradlew"
		}
		switch strings.TrimSpace(target) {
		case contracts.GateProfileTargetBuild:
			script := preamble + gradleExec + " -q --stacktrace --build-cache build -x test -p /workspace"
			return []string{"/bin/sh", "-lc", script}, nil
		case contracts.GateProfileTargetAllTests:
			script := preamble + gradleExec + " -q --stacktrace --build-cache test -p /workspace"
			return []string{"/bin/sh", "-lc", script}, nil
		default:
			return nil, fmt.Errorf("unsupported gradle target: %q", target)
		}
	case "go":
		script := preamble + "go test ./..."
		return []string{"/bin/sh", "-lc", script}, nil
	case "cargo":
		script := preamble + "cargo test"
		return []string{"/bin/sh", "-lc", script}, nil
	case "pip", "poetry":
		script := preamble + "python -m compileall -q /workspace"
		return []string{"/bin/sh", "-lc", script}, nil
	default:
		return nil, fmt.Errorf("unsupported build tool: %q", tool)
	}
}

func hasGradleWrapperSpecified(workspace string) bool {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return false
	}
	p := filepath.Join(workspace, "gradle", "wrapper", "gradle-wrapper.properties")
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
