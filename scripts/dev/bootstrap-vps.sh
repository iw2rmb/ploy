#!/usr/bin/env bash
set -euo pipefail

# Simple wrapper to provision a fresh Ploy dev environment on a clean VPS.
# Usage:
#   scripts/dev/bootstrap-vps.sh [options] <target-host>
#
# Options:
#   --apps-domain VALUE            Fully-qualified apps domain (e.g. dev.ployd.app)
#   --apps-provider VALUE          Apps domain DNS provider id (e.g. namecheap)
#   --platform-domain VALUE        Platform domain (e.g. dev.ployman.app)
#   --platform-provider VALUE      Platform domain DNS provider id
#   --registry-domain VALUE        Registry domain (e.g. registry.dev.ployman.app)
#   -h, --help                     Show this message
#
# Environment:
#   If the options above are not supplied, matching PLOY_* variables must be
#   exported before running this script. No defaults are applied. Optional
#   variables include NAMECHEAP_*, CLOUDFLARE_*, and GITHUB_PLOY_DEV_*.

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
PLAYBOOK_DIR="$REPO_ROOT/iac/dev"
VALIDATE_SCRIPT="$PLAYBOOK_DIR/scripts/validate-deployment.sh"

usage() {
  sed -n '1,80p' "$0"
}

require_env() {
  local name=$1
  if [[ -z "${!name:-}" ]]; then
    echo "ERROR: $name must be set via option or environment before running." >&2
    exit 1
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --apps-domain)
      PLOY_APPS_DOMAIN=$2
      shift 2
      ;;
    --apps-provider)
      PLOY_APPS_DOMAIN_PROVIDER=$2
      shift 2
      ;;
    --platform-domain)
      PLOY_PLATFORM_DOMAIN=$2
      shift 2
      ;;
    --platform-provider)
      PLOY_PLATFORM_DOMAIN_PROVIDER=$2
      shift 2
      ;;
    --registry-domain)
      PLOY_REGISTRY_DOMAIN=$2
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      break
      ;;
    *)
      TARGET_HOST="${TARGET_HOST:-$1}"
      shift
      ;;
  esac
done

if [[ -z "${TARGET_HOST:-}" ]]; then
  echo "ERROR: target host not provided. Pass it as an argument or set TARGET_HOST." >&2
  exit 1
fi

require_env PLOY_APPS_DOMAIN
require_env PLOY_APPS_DOMAIN_PROVIDER
require_env PLOY_PLATFORM_DOMAIN
require_env PLOY_PLATFORM_DOMAIN_PROVIDER
require_env PLOY_REGISTRY_DOMAIN

export PLOY_APPS_DOMAIN
export PLOY_APPS_DOMAIN_PROVIDER
export PLOY_PLATFORM_DOMAIN
export PLOY_PLATFORM_DOMAIN_PROVIDER
export PLOY_REGISTRY_DOMAIN
export TARGET_HOST

if ! command -v ansible-playbook >/dev/null 2>&1; then
  echo "ERROR: ansible-playbook not found. Install Ansible on your local machine first." >&2
  exit 1
fi

if [[ -x "$VALIDATE_SCRIPT" ]]; then
  echo "==> Running pre-flight validation"
  (cd "$PLAYBOOK_DIR" && ./scripts/validate-deployment.sh)
fi

echo "==> Provisioning Ploy dev stack on $TARGET_HOST"
(cd "$PLAYBOOK_DIR" && ansible-playbook site.yml -e target_host="$TARGET_HOST")

echo "✅ Dev environment provisioning finished."
echo "   SSH into $TARGET_HOST and check Nomad/Traefik/SeaweedFS services if you plan to run integration tests."
