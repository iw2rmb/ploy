#!/usr/bin/env bash
# orw-maven.sh: Apply OpenRewrite recipes to Maven projects.
#
# This script is the entrypoint for the orw-maven image. It invokes the
# rewrite-maven-plugin with recipe coordinates from environment variables.
#
# Environment (required):
#   RECIPE_GROUP       - Maven group ID (e.g., org.openrewrite.recipe)
#   RECIPE_ARTIFACT    - Artifact ID (e.g., rewrite-java-17)
#   RECIPE_VERSION     - Version (e.g., 2.6.0)
#   RECIPE_CLASSNAME   - Fully qualified recipe class name
#   MAVEN_PLUGIN_VERSION - Rewrite Maven plugin version (default: 6.18.0)
#
# Optional TLS:
#   CA_CERTS_PEM_BUNDLE - PEM-encoded CA bundle for custom trust
set -euo pipefail

usage() {
  cat <<USAGE
mods-orw-maven --apply [--dir <workspace>] [--out <dir>]

Environment (required):
  RECIPE_GROUP       e.g., org.openrewrite.recipe
  RECIPE_ARTIFACT    e.g., rewrite-java-17
  RECIPE_VERSION     e.g., 2.6.0
  RECIPE_CLASSNAME   e.g., org.openrewrite.java.migrate.UpgradeToJava17
  MAVEN_PLUGIN_VERSION  Rewrite Maven plugin version (default: 6.18.0)
USAGE
}

workspace=""
outdir="/out"
action=""

# Parse command-line arguments.
while [[ $# -gt 0 ]]; do
  case "$1" in
    --apply)
      action="apply"; shift ;;
    --dir)
      workspace="$2"; shift 2 ;;
    --out)
      outdir="$2"; shift 2 ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "unknown arg: $1" >&2; usage >&2; exit 1 ;;
  esac
done

# Self-test mode for smoke testing the container.
if [[ "${MODS_SELF_TEST:-}" == "1" ]]; then
  echo "[orw-maven] Self-test mode: writing success report to $outdir"
  mkdir -p "$outdir"
  echo '{"success":true,"self_test":true}' > "$outdir/report.json"
  exit 0
fi

# Validate required arguments.
if [[ -z "$action" ]]; then
  echo "error: action flag required (e.g., --apply)" >&2
  usage >&2
  exit 2
fi

if [[ -z "$workspace" ]]; then
  echo "error: --dir <workspace> is required" >&2
  usage >&2
  exit 2
fi

mkdir -p "$outdir"

# -----------------------------------------------------------------------------
# TLS CA injection: import custom CA certificates into the Java truststore.
#
# CA_CERTS_PEM_BUNDLE contains one or more PEM-encoded certificates. Each
# certificate is imported into the default Java cacerts keystore (-cacerts).
# This supports corporate proxies and private registries.
# -----------------------------------------------------------------------------
if [[ -n "${CA_CERTS_PEM_BUNDLE:-}" ]]; then
  pem_file="$(mktemp)"
  printf '%s\n' "${CA_CERTS_PEM_BUNDLE}" > "${pem_file}"
  pem_dir="$(mktemp -d)"

  # Split bundle into individual cert files: cert1.crt, cert2.crt, ...
  awk '/-----BEGIN CERTIFICATE-----/{n++} {print > (d"/cert" n ".crt")}' d="${pem_dir}" "${pem_file}"

  # Optionally update system CA store if update-ca-certificates is available.
  sys_ca_dir=""
  if command -v update-ca-certificates >/dev/null 2>&1; then
    sys_ca_dir="/usr/local/share/ca-certificates/orw-maven"
    mkdir -p "$sys_ca_dir"
  fi

  shopt -s nullglob
  for cert_path in "${pem_dir}"/*.crt; do
    base="$(basename "${cert_path}" .crt)"
    alias="orw_maven_pem_${base}"
    # Import into Java cacerts keystore (default password: changeit).
    if ! keytool -importcert -noprompt -trustcacerts -cacerts -storepass changeit -alias "${alias}" -file "${cert_path}" >/dev/null 2>&1; then
      echo "error: failed to import certificate ${cert_path} into cacerts" >&2
      exit 6
    fi
    if [[ -n "$sys_ca_dir" ]]; then
      cp "${cert_path}" "${sys_ca_dir}/" || true
    fi
  done
  shopt -u nullglob

  if [[ -n "$sys_ca_dir" ]]; then
    if ! update-ca-certificates >/dev/null 2>&1; then
      echo "warning: update-ca-certificates failed; system CA bundle may not include injected CAs" >&2
    fi
  fi
fi

# -----------------------------------------------------------------------------
# Recipe parameters: resolve from environment.
# -----------------------------------------------------------------------------
group=${RECIPE_GROUP:-}
artifact=${RECIPE_ARTIFACT:-}
version=${RECIPE_VERSION:-}
classname=${RECIPE_CLASSNAME:-}

if [[ -z "$group" || -z "$artifact" || -z "$version" || -z "$classname" ]]; then
  echo "error: recipe parameters must be provided via env RECIPE_GROUP/RECIPE_ARTIFACT/RECIPE_VERSION/RECIPE_CLASSNAME" >&2
  exit 4
fi

maven_plugin_ver=${MAVEN_PLUGIN_VERSION:-6.18.0}

# -----------------------------------------------------------------------------
# Validate workspace: must contain pom.xml (Maven project).
# -----------------------------------------------------------------------------
cd "$workspace"

if [[ ! -f pom.xml ]]; then
  echo "error: no pom.xml found in $workspace (Maven project required)" >&2
  exit 5
fi

# Determine rewrite configuration: prefer an existing rewrite.yml in the
# workspace; otherwise, prepare a temporary config to ensure activeRecipes
# are picked up by the Maven plugin.
cfg=""
active_recipe="$classname"

if [[ -f "rewrite.yml" ]]; then
  cfg="$PWD/rewrite.yml"
  if [[ -n "${REWRITE_ACTIVE_RECIPES:-}" ]]; then
    active_recipe="$REWRITE_ACTIVE_RECIPES"
  else
    yaml_name="$(awk '/^name:[[:space:]]*/{print $2; exit}' "$cfg" || true)"
    if [[ -n "$yaml_name" ]]; then
      active_recipe="$yaml_name"
    else
      echo "[orw-maven] rewrite.yml present but no top-level name:; falling back to RECIPE_CLASSNAME as active recipe" >&2
    fi
  fi
else
  cfg=$(mktemp)
  cat > "$cfg" <<YAML
type: specs.openrewrite.org/v1beta/recipe
name: PloyApply
recipeList:
  - $classname
YAML
fi

# -----------------------------------------------------------------------------
# Run OpenRewrite via Maven.
# -----------------------------------------------------------------------------
if ! command -v mvn >/dev/null 2>&1; then
  echo "error: mvn not found in PATH" >&2
  exit 127
fi

echo "[orw-maven] Running OpenRewrite recipe (Maven): $classname"
echo "[orw-maven] Coordinates: $group:$artifact:$version (maven plugin $maven_plugin_ver)"

status=0
mvn -B "org.openrewrite.maven:rewrite-maven-plugin:${maven_plugin_ver}:run" \
  -Drewrite.configLocation="$cfg" \
  -Drewrite.activeRecipes="$active_recipe" \
  -Drewrite.recipeArtifactCoordinates="$group:$artifact:$version" \
  -DskipTests \
  -X | tee "$outdir/transform.log" || status=$?

if [[ $status -ne 0 ]]; then
  echo "[orw-maven] OpenRewrite failed (exit $status)" >&2
  echo '{"success":false}' > "$outdir/report.json"
  exit "$status"
fi

# Write success report for downstream inspection.
cat > "$outdir/report.json" <<JSON
{
  "success": true,
  "recipe": "$classname",
  "coords": "$group:$artifact:$version"
}
JSON

# Cleanup: remove Maven build output to keep workspace clean.
find "$workspace" -type d -name target -prune -exec rm -rf {} + || true

echo "[orw-maven] Completed successfully"
