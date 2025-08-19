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
if [ -f "$SRC/Dockerfile" ]; then
  docker build -t "$TAG" "$SRC"
  docker push "$TAG"
elif [ -f "$SRC/gradlew" ]; then
  (cd "$SRC" && ./gradlew jib -Djib.to.image="$TAG")
elif [ -f "$SRC/pom.xml" ]; then
  (cd "$SRC" && ./mvnw -B com.google.cloud.tools:jib-maven-plugin:3.4.0:build -Dimage="$TAG")
else
  echo "No Dockerfile or Jib; cannot build OCI for $APP" >&2; exit 2
fi

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
