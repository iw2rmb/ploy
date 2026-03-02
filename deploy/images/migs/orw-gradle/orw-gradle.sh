#!/usr/bin/env bash
# orw-gradle.sh: Apply OpenRewrite recipes to Gradle projects.
#
# This script is the entrypoint for the orw-gradle image. It invokes the
# OpenRewrite Gradle plugin via rewriteRun with recipe coordinates from
# environment variables.
#
# Environment (required):
#   RECIPE_GROUP       - Maven group ID (e.g., org.openrewrite.recipe)
#   RECIPE_ARTIFACT    - Artifact ID (e.g., rewrite-java-17)
#   RECIPE_VERSION     - Version (e.g., 2.6.0)
#   RECIPE_CLASSNAME   - Fully qualified recipe class name
#   GRADLE_PLUGIN_VERSION - OpenRewrite Gradle plugin version (default: 7.21.0)
#
# Optional TLS:
#   CA_CERTS_PEM_BUNDLE - PEM-encoded CA bundle for custom trust
set -euo pipefail

usage() {
  cat <<USAGE
migs-orw-gradle --apply [--dir <workspace>] [--out <dir>]

Environment (required):
  RECIPE_GROUP       e.g., org.openrewrite.recipe
  RECIPE_ARTIFACT    e.g., rewrite-java-17
  RECIPE_VERSION     e.g., 2.6.0
  RECIPE_CLASSNAME   e.g., org.openrewrite.java.migrate.UpgradeToJava17
  GRADLE_PLUGIN_VERSION  OpenRewrite Gradle plugin version (default: 7.21.0)
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
  echo "[orw-gradle] Self-test mode: writing success report to $outdir"
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
    sys_ca_dir="/usr/local/share/ca-certificates/orw-gradle"
    mkdir -p "$sys_ca_dir"
  fi

  shopt -s nullglob
  for cert_path in "${pem_dir}"/*.crt; do
    base="$(basename "${cert_path}" .crt)"
    alias="orw_gradle_pem_${base}"
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

gradle_plugin_ver=${GRADLE_PLUGIN_VERSION:-7.21.0}

# -----------------------------------------------------------------------------
# Validate workspace: must contain build.gradle or build.gradle.kts (Gradle).
# -----------------------------------------------------------------------------
cd "$workspace"

if [[ ! -f build.gradle && ! -f build.gradle.kts ]]; then
  echo "error: no build.gradle or build.gradle.kts found in $workspace (Gradle project required)" >&2
  exit 5
fi

# Determine Gradle command: prefer project wrapper, fall back to system gradle.
gradle_cmd=""
if [[ -x "./gradlew" ]]; then
  gradle_cmd="./gradlew"
elif command -v gradle >/dev/null 2>&1; then
  gradle_cmd="gradle"
else
  echo "error: gradle not found in PATH and ./gradlew is missing (Gradle project required)" >&2
  exit 127
fi

# Determine build file style (Kotlin DSL or Groovy).
build_file=""
build_style=""
if [[ -f "build.gradle.kts" ]]; then
  build_file="build.gradle.kts"
  build_style="kts"
elif [[ -f "build.gradle" ]]; then
  build_file="build.gradle"
  build_style="groovy"
fi

# Determine rewrite configuration: prefer an existing rewrite.yml in the
# workspace; otherwise, prepare a temporary config to ensure activeRecipes
# are picked up by the Gradle plugin.
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
      echo "[orw-gradle] rewrite.yml present but no top-level name:; falling back to RECIPE_CLASSNAME as active recipe" >&2
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
# Inject OpenRewrite Gradle plugin and recipe dependency.
#
# We add the plugin and recipe dependency if absent, then remove them after
# rewriteRun completes. This leaves the build file in its original shape
# aside from changes made by the recipes themselves.
# -----------------------------------------------------------------------------
plugin_line='id("org.openrewrite.rewrite") version "'"${gradle_plugin_ver}"'"'
recipe_coord="${group}:${artifact}:${version}"
rewrite_dep_line='rewrite("'"${recipe_coord}"'")'

plugin_was_present=false
rewrite_was_present=false

if grep -q 'org.openrewrite.rewrite' "$build_file"; then
  plugin_was_present=true
fi
if grep -q "$recipe_coord" "$build_file"; then
  rewrite_was_present=true
fi

# Inject plugin into plugins block if absent.
if [[ "$plugin_was_present" == false ]]; then
  tmpf="$(mktemp)"
  awk -v line="$plugin_line" '
    $0 ~ /^plugins[[:space:]]*\{/ && !inserted {
      print $0
      print "    " line
      inserted=1
      next
    }
    { print $0 }
  ' "$build_file" > "$tmpf"
  mv "$tmpf" "$build_file"
fi

# Inject rewrite dependency into dependencies block if absent.
if [[ "$rewrite_was_present" == false ]]; then
  tmpf="$(mktemp)"
  awk -v line="$rewrite_dep_line" '
    $0 ~ /^dependencies[[:space:]]*\{/ && !inserted {
      print $0
      print "    " line
      inserted=1
      next
    }
    { print $0 }
    END {
      if (!inserted) {
        print ""
        print "dependencies {"
        print "    " line
        print "}"
      }
    }
  ' "$build_file" > "$tmpf"
  mv "$tmpf" "$build_file"
fi

echo "[orw-gradle] Running OpenRewrite recipe (Gradle): $classname"
echo "[orw-gradle] Coordinates: $group:$artifact:$version (gradle plugin $gradle_plugin_ver)"

# Use isolated Gradle caches to avoid conflicts with host Gradle processes.
gradle_home="$(mktemp -d)"
project_cache_dir="$(mktemp -d)"
export GRADLE_USER_HOME="$gradle_home"

# Inject remote build cache init script when cache URL is configured.
if [[ -n "${PLOY_GRADLE_BUILD_CACHE_URL:-}" ]]; then
  mkdir -p "$gradle_home/init.d"
  cat > "$gradle_home/init.d/ploy-remote-build-cache.init.gradle" <<'INITGRADLE'
settingsEvaluated { settings ->
  def cacheUrl = System.getenv("PLOY_GRADLE_BUILD_CACHE_URL")
  if (cacheUrl == null || cacheUrl.trim().isEmpty()) return

  settings.buildCache {
    local { enabled = true }
    remote(HttpBuildCache) {
      // NOTE: In Groovy closures, `this` refers to the script object, not the delegate.
      // Assign to the delegate property directly.
      url = new URI(cacheUrl)
      push = (System.getenv("PLOY_GRADLE_BUILD_CACHE_PUSH") ?: "true").toBoolean()
      allowInsecureProtocol = true
    }
  }
}
INITGRADLE
fi

status=0
"$gradle_cmd" --no-daemon --stacktrace --build-cache \
  --project-cache-dir "$project_cache_dir" \
  rewriteRun \
  -Drewrite.configLocation="$cfg" \
  -Drewrite.activeRecipes="$active_recipe" \
  -Drewrite.recipeArtifactCoordinates="$group:$artifact:$version" \
  -q | tee "$outdir/transform.log" || status=$?

# -----------------------------------------------------------------------------
# Cleanup: remove injected plugin/dependency lines if we added them.
# This preserves any other build file changes (including those from recipes).
# -----------------------------------------------------------------------------
if [[ "$plugin_was_present" == false ]]; then
  # Best-effort removal; ignore errors to avoid masking rewrite failures.
  sed -i '/org.openrewrite.rewrite/d' "$build_file" || true
fi
if [[ "$rewrite_was_present" == false ]]; then
  sed -i "/rewrite(\"$recipe_coord\")/d" "$build_file" || true
fi

if [[ $status -ne 0 ]]; then
  echo "[orw-gradle] OpenRewrite failed (exit $status)" >&2
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

# Cleanup: remove Gradle build output to keep workspace clean.
find "$workspace" -type d -name build -prune -exec rm -rf {} + || true

echo "[orw-gradle] Completed successfully"
