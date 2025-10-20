#!/usr/bin/env bash

set -euo pipefail

usage() {
	cat <<'USAGE'
publish_mods_images.sh --registry <host/path> [options]

Builds (when Docker contexts are available) or mirrors the Mods container images
to a Docker Registry v2 endpoint. The script expects Docker to be installed and
configured with buildx support.

Required flags:
  --registry <host/path>   Registry prefix used for the published images
                           (example: registry.example.com/ploy)

Optional flags:
  --username <value>       Registry username (falls back to REGISTRY_USERNAME env)
  --password <value>       Registry password/token (falls back to REGISTRY_PASSWORD env)
  --source <prefix>        Source registry for mirroring when build contexts are
                           unavailable (default: registry.dev/ploy)
  --context-root <dir>     Root directory containing Mods Docker build contexts
                           (default: ../docker/mods relative to repo root)
  --tag <value>            Image tag to publish (default: latest)
  --platform <value>       Target platform for docker buildx (default: linux/amd64)
  --image <name>           Limit to a specific Mods image (repeatable)
  --dry-run                Print build/push commands without executing them
  -h, --help               Show this help message

Per-image context overrides can be provided via environment variables of the
form MODS_IMAGE_<NAME>_CONTEXT. Example:
  export MODS_IMAGE_MODS_PLAN_CONTEXT=/path/to/mods-plan

Environment overrides:
  REGISTRY_USERNAME, REGISTRY_PASSWORD, SOURCE_REGISTRY, CONTEXT_ROOT,
  TAG, PLATFORM

Examples:
  scripts/publish_mods_images.sh \
    --registry registry.example.com/ploy \
    --username svc-account --password "$(op read op://reg/secret)"

  CONTEXT_ROOT=../mods/docker \
    scripts/publish_mods_images.sh --registry registry.example.com/ploy
USAGE
}

require_cmd() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "error: required command '$1' not found in PATH" >&2
		exit 1
	}
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"

registry=""
username="${REGISTRY_USERNAME:-}"
password="${REGISTRY_PASSWORD:-}"
source_registry="${SOURCE_REGISTRY:-registry.dev/ploy}"
default_context_root="${repo_root}/docker/mods"
context_root="${CONTEXT_ROOT:-$default_context_root}"
tag="${TAG:-latest}"
platform="${PLATFORM:-linux/amd64}"
dry_run=false
declare -a images=()

while [[ $# -gt 0 ]]; do
	case "$1" in
		--registry)
			registry="${2:-}"
			shift 2
			;;
		--username)
			username="${2:-}"
			shift 2
			;;
		--password)
			password="${2:-}"
			shift 2
			;;
		--source)
			source_registry="${2:-}"
			shift 2
			;;
		--context-root)
			context_root="$(cd "$2" && pwd)"
			shift 2
			;;
		--tag)
			tag="${2:-}"
			shift 2
			;;
		--platform)
			platform="${2:-}"
			shift 2
			;;
		--image)
			images+=("${2:-}")
			shift 2
			;;
		--dry-run)
			dry_run=true
			shift
			;;
		-h|--help)
			usage
			exit 0
			;;
		*)
			echo "error: unknown argument: $1" >&2
			usage >&2
			exit 1
			;;
	esac
done

if [[ -z "$registry" ]]; then
	echo "error: --registry is required" >&2
	usage >&2
	exit 1
fi

if [[ ${#images[@]} -eq 0 ]]; then
	images=(mods-plan mods-openrewrite mods-llm mods-human)
fi

require_cmd docker

trimmed_registry="${registry#https://}"
trimmed_registry="${trimmed_registry#http://}"
trimmed_registry="${trimmed_registry%%/}"
registry_host="${trimmed_registry%%/*}"
target_prefix="${trimmed_registry}"

docker_login() {
	if $dry_run; then
		echo "[dry-run] docker login $registry_host"
		return
	fi
	if [[ -z "$username" ]]; then
		return
	}
	if [[ -z "$password" ]]; then
		read -rsp "Password for $username@$registry_host: " password
		echo ""
	fi
	printf '%s\n' "$password" | docker login "$registry_host" --username "$username" --password-stdin
}

docker_login

build_image() {
	local image="$1"
	local context="$2"
	local target="$3"

	if $dry_run; then
		echo "[dry-run] docker buildx build --platform ${platform} --tag ${target} --load ${context}"
		return
	}
	docker buildx build \
		--platform "${platform}" \
		--tag "${target}" \
		--load \
		"${context}"
}

mirror_image() {
	local image="$1"
	local source="$2"
	local target="$3"

	if $dry_run; then
		echo "[dry-run] docker pull ${source}"
		echo "[dry-run] docker tag ${source} ${target}"
		return
	}
	docker pull "${source}"
	docker tag "${source}" "${target}"
}

push_image() {
	local target="$1"
	if $dry_run; then
		echo "[dry-run] docker push ${target}"
		return
	}
	docker push "${target}"
}

for image in "${images[@]}"; do
	trimmed_image="${image##*/}"
	if [[ -z "$trimmed_image" ]]; then
		continue
	}

	suffix="${trimmed_image^^}"
	suffix="${suffix//-/_}"
	override_var="MODS_IMAGE_${suffix}_CONTEXT"
	override_value="${!override_var-}"

	if [[ -n "${override_value}" ]]; then
		context="$(cd "${override_value}" && pwd)"
	else
		context="${context_root}/${trimmed_image}"
	fi

	target_image="${target_prefix%/}/${trimmed_image}:${tag}"

	echo "==> Publishing ${trimmed_image}:${tag} to ${target_image}"

	if [[ -d "$context" ]]; then
		echo " - building from context ${context}"
		build_image "${trimmed_image}" "${context}" "${target_image}"
	else
		if [[ -z "$source_registry" ]]; then
			echo "error: build context ${context} missing and no --source registry provided" >&2
			exit 1
		fi
		source_image="${source_registry%/}/${trimmed_image}:${tag}"
		echo " - context not found; mirroring ${source_image}"
		mirror_image "${trimmed_image}" "${source_image}" "${target_image}"
	fi

	push_image "${target_image}"
done

if ! $dry_run; then
	echo "All Mods images published to ${target_prefix}"
else
	echo "[dry-run] Completed without pushing images"
fi
