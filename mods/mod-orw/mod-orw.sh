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

if ! command -v mvn >/dev/null 2>&1; then
  echo "error: mvn not found in PATH" >&2
  exit 127
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
if [[ ! -f pom.xml ]]; then
  echo "error: pom.xml not found in $workspace (Maven project required)" >&2
  exit 5
fi

echo "[mod-orw] Running OpenRewrite recipe: $classname"
echo "[mod-orw] Coordinates: $group:$artifact:$version (plugin $plugin_ver)"

# Prepare a temporary rewrite.yml to ensure activeRecipes are picked up reliably.
cfg=$(mktemp)
cat > "$cfg" <<YAML
type: specs.openrewrite.org/v1beta/recipe
name: PloyApply
recipeList:
  - $classname
YAML

# Run OpenRewrite; skip tests, be verbose for diagnostics
mvn -B "org.openrewrite.maven:rewrite-maven-plugin:${plugin_ver}:run" \
  -Drewrite.configLocation="$cfg" \
  -Drewrite.activeRecipes="$classname" \
  -Drewrite.recipeArtifactCoordinates="$group:$artifact:$version" \
  -DskipTests \
  -X | tee "$outdir/transform.log"

status=${PIPESTATUS[0]}
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

echo "[mod-orw] Completed successfully"
