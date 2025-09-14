#!/usr/bin/env bash
set -euo pipefail
usage(){ echo "usage: $0 --app <name> --src <dir> --tag <registry/app:sha>"; exit 1; }
APP=""; SRC=""; TAG=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --app) APP="$2"; shift 2;;
    --src) SRC="$2"; shift 2;;
    --tag) TAG="$2"; shift 2;;
    *) usage;;
  esac
done
[[ -z "$APP" || -z "$SRC" || -z "$TAG" ]] && usage
# Prefer Jib (Gradle/Maven) when present, as it sets a correct entrypoint
if [ -f "$SRC/gradlew" ]; then
  (cd "$SRC" && ./gradlew jib -Djib.to.image="$TAG")
elif [ -f "$SRC/pom.xml" ]; then
  (cd "$SRC" && ./mvnw -B com.google.cloud.tools:jib-maven-plugin:3.4.0:build -Dimage="$TAG")
elif [ -f "$SRC/Dockerfile" ]; then
  docker build -t "$TAG" "$SRC"
  docker push "$TAG"
else
  echo "No Dockerfile or Jib; cannot build OCI for $APP" >&2; exit 2
fi

# ---- Push verification (non-fatal) -----------------------------------------
verify_push() {
  local ref="$1"
  # Try to verify using docker manifest inspect; do not fail the build on errors
  if command -v docker >/dev/null 2>&1; then
    local output
    if output=$(docker manifest inspect "$ref" 2>&1); then
      # Try to extract a digest (works for both OCI and Docker schema2)
      local digest
      if command -v jq >/dev/null 2>&1; then
        digest=$(echo "$output" | jq -r '..|.digest? | select(.!=null) | strings | select(startswith("sha256:"))' | head -n1)
      else
        digest=$(echo "$output" | sed -n 's/.*\(sha256:[0-9a-f]\{64\}\).*/\1/p' | head -n1)
      fi
      if [ -n "$digest" ]; then
        echo "Push verification: OK (digest $digest)"
      else
        echo "Push verification: OK (manifest available; digest unknown)"
      fi
      return 0
    else
      echo "Push verification: FAILED (docker manifest inspect error)"
      echo "$output" | sed -e 's/^/  > /'
      return 0
    fi
  else
    echo "Push verification: SKIPPED (docker CLI not available)"
  fi
}

verify_push "$TAG"

# Generate comprehensive SBOM and signature for container image
if command -v syft >/dev/null 2>&1; then 
  echo "Generating comprehensive container SBOM for $TAG..."
  SBOM_FILE="/tmp/$APP-$(echo $TAG | tr '/:' '-').sbom.json"
  syft scan "$TAG" \
    -o spdx-json \
    --file "$SBOM_FILE" || true
  echo "Container SBOM saved to $SBOM_FILE"
  
  # Also generate source code SBOM for the build context
  if [ -d "$SRC" ]; then
    echo "Generating source dependencies SBOM..."
    syft scan "$SRC" \
      -o spdx-json \
      --file "$SRC/.sbom.json" || true
  fi
else
  echo "Warning: syft not found, skipping comprehensive SBOM generation"
fi

# Enhanced keyless OIDC container signing
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "$SCRIPT_DIR/../common/signing.sh" ]]; then
  source "$SCRIPT_DIR/../common/signing.sh"
  
  echo "🔐 Enhanced Container Signing Configuration:"
  print_signing_config
  
  echo "🖊️  Signing container with keyless OIDC..."
  if sign_ploy_artifact "$TAG" "container"; then
    echo "✅ Container signed successfully: $TAG"
  else
    echo "⚠️  Container signing failed, but continuing build"
  fi
else
  # Fallback to basic signing if common functions not available
  echo "⚠️  Common signing functions not found, using basic signing"
  if command -v cosign >/dev/null 2>&1; then 
    cosign sign --yes "$TAG" || true
  fi
fi

echo "$TAG"
