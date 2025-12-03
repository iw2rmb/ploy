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
# Modes:
#   1) CA_CERTS_ZIP — Path to a ZIP file (inside the container) containing one
#      or more *.crt files. Each certificate is imported into the default Java
#      cacerts keystore (-cacerts) as a trusted root/intermediate.
#   2) MOD_ORW_CA_CERT_CRT — PEM-encoded root CA certificate(s). Imported into
#      the default Java cacerts keystore (-cacerts), matching common corporate
#      guidance for adding a trusted root.
#   3) MOD_ORW_CA_CERT_PEM / MOD_ORW_CA_CERT_PATH — PEM file contents or path.
#      The script creates a temporary keystore and injects it via
#      JAVA_TOOL_OPTIONS as javax.net.ssl.trustStore. This preserves the base
#      truststore untouched.
#
# CA_CERTS_ZIP and MOD_ORW_CA_CERT_CRT are additive and both update the default
# cacerts keystore when provided. The temporary trustStore path is only used
# when MOD_ORW_CA_CERT_PEM / MOD_ORW_CA_CERT_PATH are set.

# 1) Import multiple CA certs from CA_CERTS_ZIP (if provided).
if [[ -n "${CA_CERTS_ZIP:-}" ]]; then
  if [[ ! -f "${CA_CERTS_ZIP}" ]]; then
    echo "error: CA_CERTS_ZIP path does not exist: ${CA_CERTS_ZIP}" >&2
    exit 6
  fi
  zip_tmp_dir="$(mktemp -d)"
  if ! unzip -qq "${CA_CERTS_ZIP}" -d "${zip_tmp_dir}" >/dev/null 2>&1; then
    echo "error: failed to extract CA_CERTS_ZIP from ${CA_CERTS_ZIP}" >&2
    exit 6
  fi
  shopt -s nullglob
  for cert_path in "${zip_tmp_dir}"/*.crt; do
    base="$(basename "${cert_path}" .crt)"
    alias="mod_orw_zip_${base}"
    if ! keytool -importcert -noprompt -trustcacerts -cacerts -storepass changeit -alias "${alias}" -file "${cert_path}" >/dev/null 2>&1; then
      echo "error: failed to import certificate ${cert_path} into cacerts" >&2
      exit 6
    fi
  done
  shopt -u nullglob
fi

# 2) Import single-root CA from MOD_ORW_CA_CERT_CRT (if provided).
if [[ -n "${MOD_ORW_CA_CERT_CRT:-}" ]]; then
  crt_file="$(mktemp)"
  printf '%s\n' "${MOD_ORW_CA_CERT_CRT}" > "${crt_file}"
  if ! keytool -importcert -noprompt -trustcacerts -cacerts -storepass changeit -alias mod_orw_extra_root -file "${crt_file}" >/dev/null 2>&1; then
    echo "error: failed to import MOD_ORW_CA_CERT_CRT into default cacerts" >&2
    exit 6
  fi
fi

# 3) Optional temporary trustStore from PEM/path.
extra_ca_file=""
if [[ -n "${MOD_ORW_CA_CERT_PEM:-}" ]]; then
  extra_ca_file="$(mktemp)"
  printf '%s\n' "${MOD_ORW_CA_CERT_PEM}" > "${extra_ca_file}"
fi
if [[ -n "${MOD_ORW_CA_CERT_PATH:-}" ]]; then
  extra_ca_file="${MOD_ORW_CA_CERT_PATH}"
fi

if [[ -n "${extra_ca_file}" ]]; then
  ts="$(mktemp)"
  rm -f "${ts}"
  ts_pass="${MOD_ORW_CA_TRUSTSTORE_PASSWORD:-changeit}"
  if ! keytool -importcert -noprompt -keystore "${ts}" -storepass "${ts_pass}" -file "${extra_ca_file}" -alias mod_orw_extra_ca >/dev/null 2>&1; then
    echo "error: failed to import extra CA certificate from ${extra_ca_file}" >&2
    exit 6
  fi
  export JAVA_TOOL_OPTIONS="${JAVA_TOOL_OPTIONS:-} -Djavax.net.ssl.trustStore=${ts} -Djavax.net.ssl.trustStorePassword=${ts_pass}"
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

plugin_ver=${MAVEN_PLUGIN_VERSION:-6.18.0}

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
  echo "[mod-orw] Coordinates: $group:$artifact:$version (plugin $plugin_ver)"

  mvn -B "org.openrewrite.maven:rewrite-maven-plugin:${plugin_ver}:run" \
    -Drewrite.configLocation="$cfg" \
    -Drewrite.activeRecipes="$classname" \
    -Drewrite.recipeArtifactCoordinates="$group:$artifact:$version" \
    -DskipTests \
    -X | tee "$outdir/transform.log"

  status=${PIPESTATUS[0]}
else
  # Gradle project: prefer ./gradlew when present, otherwise fall back to
  # system gradle. The repository must apply the OpenRewrite Gradle plugin;
  # recipe coordinates are passed via standard rewrite.* system properties.
  gradle_cmd=""
  if [[ -x "./gradlew" ]]; then
    gradle_cmd="./gradlew"
  elif command -v gradle >/dev/null 2>&1; then
    gradle_cmd="gradle"
  else
    echo "error: gradle not found in PATH and ./gradlew is missing (Gradle project required)" >&2
    exit 127
  fi

  echo "[mod-orw] Running OpenRewrite recipe (Gradle): $classname"
  echo "[mod-orw] Coordinates: $group:$artifact:$version"

  # Use a Gradle init script so we don't require the project to preconfigure
  # the OpenRewrite plugin or tasks. This keeps the mod image universal and
  # avoids committing rewrite configuration into the target repo.
  init_script="$(mktemp)"
  cat >"$init_script" <<GRADLE
initscript {
  repositories {
    mavenCentral()
  }
  dependencies {
    classpath("org.openrewrite:rewrite-gradle-plugin:${plugin_ver}")
  }
}

allprojects {
  apply plugin: "org.openrewrite.rewrite"

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
