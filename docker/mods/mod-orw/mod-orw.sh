#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
mods-orw --apply [--dir <workspace>] [--out <dir>]

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

if [[ "${MODS_SELF_TEST:-}" == "1" ]]; then
  echo "[mod-orw] Self-test mode: writing success report to $outdir"
  mkdir -p "$outdir"
  echo '{"success":true,"self_test":true}' > "$outdir/report.json"
  exit 0
fi

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

# Optional TLS trust configuration for Maven/Gradle (e.g., corporate proxy).
#
# CA_CERTS_PEM_BUNDLE — PEM-encoded bundle containing one or more
# certificates (`-----BEGIN CERTIFICATE-----` blocks). The script splits
# the bundle and imports each cert into the default Java cacerts keystore
# (-cacerts).

# Import CA certs from CA_CERTS_PEM_BUNDLE (if provided).
if [[ -n "${CA_CERTS_PEM_BUNDLE:-}" ]]; then
  pem_file="$(mktemp)"
  printf '%s\n' "${CA_CERTS_PEM_BUNDLE}" > "${pem_file}"
  pem_dir="$(mktemp -d)"
  # Split bundle into individual cert files cert1.crt, cert2.crt, ...
  awk '/-----BEGIN CERTIFICATE-----/{n++} {print > (d"/cert" n ".crt")}' d="${pem_dir}" "${pem_file}"

  sys_ca_dir=""
  if command -v update-ca-certificates >/dev/null 2>&1; then
    sys_ca_dir="/usr/local/share/ca-certificates/mod-orw"
    mkdir -p "$sys_ca_dir"
  fi

  shopt -s nullglob
  for cert_path in "${pem_dir}"/*.crt; do
    base="$(basename "${cert_path}" .crt)"
    alias="mod_orw_pem_${base}"
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

# Resolve recipe parameters strictly from env
group=${RECIPE_GROUP:-}
artifact=${RECIPE_ARTIFACT:-}
version=${RECIPE_VERSION:-}
classname=${RECIPE_CLASSNAME:-}

if [[ -z "$group" || -z "$artifact" || -z "$version" || -z "$classname" ]]; then
  echo "error: recipe parameters must be provided via env RECIPE_GROUP/RECIPE_ARTIFACT/RECIPE_VERSION/RECIPE_CLASSNAME" >&2
  exit 4
fi

maven_plugin_ver=${MAVEN_PLUGIN_VERSION:-6.18.0}
gradle_plugin_ver=${GRADLE_PLUGIN_VERSION:-7.21.0}

cd "$workspace"
is_maven=false
is_gradle=false

if [[ -f pom.xml ]]; then
  is_maven=true
fi
if [[ -f build.gradle || -f build.gradle.kts ]]; then
  is_gradle=true
fi

if [[ "$is_maven" == "false" && "$is_gradle" == "false" ]]; then
  echo "error: no build file found in $workspace (expected pom.xml or build.gradle(.kts))" >&2
  exit 5
fi

# Prepare a temporary rewrite.yml to ensure activeRecipes are picked up reliably.
cfg=$(mktemp)
cat > "$cfg" <<YAML
type: specs.openrewrite.org/v1beta/recipe
name: PloyApply
recipeList:
  - $classname
YAML

status=0
if [[ "$is_maven" == "true" ]]; then
  # Maven project: invoke rewrite-maven-plugin directly so the recipe coordinates
  # can be supplied without requiring the plugin to be configured in pom.xml.
  if ! command -v mvn >/dev/null 2>&1; then
    echo "error: mvn not found in PATH" >&2
    exit 127
  fi

  echo "[mod-orw] Running OpenRewrite recipe (Maven): $classname"
  echo "[mod-orw] Coordinates: $group:$artifact:$version (maven plugin $maven_plugin_ver)"

  mvn -B "org.openrewrite.maven:rewrite-maven-plugin:${maven_plugin_ver}:run" \
    -Drewrite.configLocation="$cfg" \
    -Drewrite.activeRecipes="$classname" \
    -Drewrite.recipeArtifactCoordinates="$group:$artifact:$version" \
    -DskipTests \
    -X | tee "$outdir/transform.log"

  status=${PIPESTATUS[0]}
else
  # Gradle project: prefer system Gradle when present to avoid wrapper-managed
  # distribution downloads from services.gradle.org; fall back to ./gradlew
  # only when a system gradle binary is unavailable.
  gradle_cmd=""
  if command -v gradle >/dev/null 2>&1; then
    gradle_cmd="gradle"
  elif [[ -x "./gradlew" ]]; then
    gradle_cmd="./gradlew"
  else
    echo "error: gradle not found in PATH and ./gradlew is missing (Gradle project required)" >&2
    exit 127
  fi

  echo "[mod-orw] Running OpenRewrite recipe (Gradle): $classname"
  echo "[mod-orw] Coordinates: $group:$artifact:$version (gradle plugin $gradle_plugin_ver)"

  # Use a Gradle init script that resolves the OpenRewrite Gradle integration
  # directly from Maven Central and applies the plugin class for all projects.
  # This avoids reliance on plugin markers and keeps the target repo unchanged.
  init_script="$(mktemp)"
  cat >"$init_script" <<GRADLE
initscript {
  repositories {
    mavenCentral()
  }
  dependencies {
    classpath("org.openrewrite:rewrite-gradle:${gradle_plugin_ver}")
  }
}

allprojects {
  apply plugin: org.openrewrite.gradle.RewritePlugin

  rewrite {
    activeRecipe("${classname}")
    recipeArtifact("${group}:${artifact}:${version}")
  }
}
GRADLE

  "$gradle_cmd" --no-daemon --stacktrace -I "$init_script" rewriteRun \
    -Drewrite.configLocation="$cfg" \
    -Drewrite.activeRecipes="$classname" \
    -Drewrite.recipeArtifactCoordinates="$group:$artifact:$version" \
    -q | tee "$outdir/transform.log"

  status=${PIPESTATUS[0]}
fi

if [[ $status -ne 0 ]]; then
  echo "[mod-orw] OpenRewrite failed (exit $status)" >&2
  echo '{"success":false}' > "$outdir/report.json"
  exit "$status"
fi

# Minimal report for downstream inspection
cat > "$outdir/report.json" <<JSON
{
  "success": true,
  "recipe": "$classname",
  "coords": "$group:$artifact:$version"
}
JSON

# Cleanup: remove build output directories to keep workspace clean
find "$workspace" -type d \( -name target -o -name build \) -prune -exec rm -rf {} + || true

echo "[mod-orw] Completed successfully"
