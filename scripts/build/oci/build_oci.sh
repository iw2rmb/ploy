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

# Generate SBOM and signature for image
if command -v syft >/dev/null 2>&1; then syft packages "$TAG" -o json > "/tmp/$APP-$(echo $TAG | tr '/:' '-').sbom.json" || true; fi
if command -v cosign >/dev/null 2>&1; then cosign sign --yes "$TAG" || true; fi

echo "$TAG"
