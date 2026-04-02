package step

// EnvMaterializer produces a shell preamble that materializes a special env
// key's value into runtime-specific behavior beyond plain env passthrough.
// Keys without a materializer use plain env passthrough (no preamble).
type EnvMaterializer func() string

// envMaterializerEntry pairs an env key with its materializer. The slice
// ordering is the concatenation order in envMaterializerPreamble.
type envMaterializerEntry struct {
	key          string
	materializer EnvMaterializer
}

// materializers is the single registration point for all special env key
// materializers. Add new entries here to extend the mechanism.
var materializers = []envMaterializerEntry{
	{"PLOY_CA_CERTS", ployCAcertsPreamble},
}

// MaterializerForKey returns the materializer for a special env key, or nil
// for keys that use plain env passthrough.
func MaterializerForKey(key string) EnvMaterializer {
	for _, e := range materializers {
		if e.key == key {
			return e.materializer
		}
	}
	return nil
}

// envMaterializerPreamble returns the combined shell preamble for all
// registered env materializers. This is prepended to gate container commands.
func envMaterializerPreamble() string {
	var preamble string
	for _, e := range materializers {
		preamble += e.materializer()
	}
	return preamble
}

// ployCAcertsPreamble returns a shell preamble that installs CA certificates
// from the PLOY_CA_CERTS environment variable into the system trust store and
// Java cacerts keystore.
//
// PLOY_CA_CERTS accepts either:
//   - inline PEM content (one or more concatenated PEM certificates), or
//   - a readable file path containing PEM certificates.
//
// The preamble detects which form is provided, extracts individual certs, and:
//  1. Installs them into /usr/local/share/ca-certificates and runs update-ca-certificates
//  2. Imports each cert into the Java cacerts keystore via keytool (if available)
func ployCAcertsPreamble() string {
	return `# --- PLOY_CA_CERTS materializer preamble ---
if [ -n "${PLOY_CA_CERTS:-}" ]; then
  ploy_ca_pem=""
  if [ -r "${PLOY_CA_CERTS}" ]; then
    ploy_ca_pem="$(cat "${PLOY_CA_CERTS}")"
  else
    ploy_ca_pem="${PLOY_CA_CERTS}"
  fi
  if [ -n "${ploy_ca_pem}" ]; then
    pem_file="$(mktemp)"
    printf '%s\n' "${ploy_ca_pem}" > "${pem_file}"
    pem_dir="$(mktemp -d)"
    awk '/-----BEGIN CERTIFICATE-----/{n++} {print > (d"/cert" n ".crt")}' d="${pem_dir}" "${pem_file}"
    if command -v update-ca-certificates >/dev/null 2>&1; then
      sys_ca_dir="/usr/local/share/ca-certificates/ploy-gate"
      mkdir -p "$sys_ca_dir"
      cp "${pem_dir}"/*.crt "$sys_ca_dir"/ 2>/dev/null || true
      update-ca-certificates >/dev/null 2>&1 || true
    fi
    if command -v keytool >/dev/null 2>&1; then
      for cert_path in "${pem_dir}"/*.crt; do
        [ -f "$cert_path" ] || continue
        base="$(basename "${cert_path}" .crt)"
        alias="ploy_gate_pem_${base}"
        keytool -importcert -noprompt -trustcacerts -cacerts -storepass changeit -alias "${alias}" -file "${cert_path}" >/dev/null 2>&1 || true
      done
    fi
  fi
fi
# --- End PLOY_CA_CERTS materializer preamble ---
`
}
