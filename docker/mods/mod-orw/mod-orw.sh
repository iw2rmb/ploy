#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
mods-orw --apply --recipe-json <file> --dir <workspace> --out <dir>

Environment:
  MAVEN_PLUGIN_VERSION  Rewrite Maven plugin version (default: 6.18.0)
USAGE
}

recipe_json=""
workspace=""
outdir="/out"
action=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --apply)
      action="apply"; shift ;;
    --recipe-json)
      recipe_json="$2"; shift 2 ;;
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

if [[ -z "$action" || -z "$recipe_json" || -z "$workspace" ]]; then
  echo "error: required flags missing" >&2
  usage >&2
  exit 2
fi

mkdir -p "$outdir"

if ! command -v mvn >/dev/null 2>&1; then
  echo "error: mvn not found in PATH" >&2
  exit 127
fi

if [[ ! -f "$recipe_json" ]]; then
  echo "error: recipe json not found at $recipe_json" >&2
  exit 3
fi

group=$(jq -r '.group' "$recipe_json")
artifact=$(jq -r '.artifact' "$recipe_json")
version=$(jq -r '.version' "$recipe_json")
classname=$(jq -r '.name' "$recipe_json")

if [[ -z "$group" || -z "$artifact" || -z "$version" || -z "$classname" ]]; then
  echo "error: recipe json must include keys: group, artifact, version, name" >&2
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

# Run OpenRewrite; skip tests, be verbose for diagnostics
mvn -B "org.openrewrite.maven:rewrite-maven-plugin:${plugin_ver}:run" \
  -Drewrite.recipe="$classname" \
  -Drewrite.recipeArtifactCoordinates="$group:$artifact:$version" \
  -DskipTests \
  -X | tee "$outdir/transform.log"

status=${PIPESTATUS[0]}
if [[ $status -ne 0 ]]; then
  echo "[mod-orw] OpenRewrite failed (exit $status)" >&2
  echo '{"success":false}' > "$outdir/report.json"
  exit $status
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
