#!/usr/bin/env bash
# Unit tests for deploy/vps/redeploy.sh helper behavior.
#
# Usage: bash tests/unit/redeploy_vps_sh_test.sh

set -uo pipefail

ROOT_DIR=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
SCRIPT="$ROOT_DIR/deploy/vps/redeploy.sh"

TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

DOCKER_CALLS_FILE=""
MOCK_EXISTING_IMAGES=""

pass() {
  TESTS_PASSED=$((TESTS_PASSED + 1))
  echo "  ok $1"
}

fail() {
  TESTS_FAILED=$((TESTS_FAILED + 1))
  echo "  not ok $1: $2"
}

run_test() {
  TESTS_RUN=$((TESTS_RUN + 1))
}

docker() {
  printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
  if [[ "$1" == "image" && "$2" == "inspect" ]]; then
    case " $MOCK_EXISTING_IMAGES " in
      *" $3 "*) return 0 ;;
      *) return 1 ;;
    esac
  fi
  return 0
}

reset_docker_mock() {
  : > "$DOCKER_CALLS_FILE"
  MOCK_EXISTING_IMAGES=""
}

assert_file_contains() {
  local file="$1"
  local pattern="$2"
  local message="$3"
  if grep -Fq -- "$pattern" "$file"; then
    pass "$message"
  else
    fail "$message" "missing pattern: $pattern"
  fi
}

assert_file_not_contains() {
  local file="$1"
  local pattern="$2"
  local message="$3"
  if grep -Fq -- "$pattern" "$file"; then
    fail "$message" "unexpected pattern: $pattern"
  else
    pass "$message"
  fi
}

test_low_disk_reuses_existing_buildx_image() {
  run_test
  reset_docker_mock
  LOW_DISK_MODE=1
  MOCK_EXISTING_IMAGES="ploy-server:vps"

  maybe_run_buildx_load "deploy/images/server/Dockerfile" "." "ploy-server:vps"

  assert_file_contains "$DOCKER_CALLS_FILE" "image inspect ploy-server:vps" "inspects existing buildx image"
  assert_file_not_contains "$DOCKER_CALLS_FILE" "buildx build" "skips buildx build when image exists"
}

test_low_disk_builds_missing_image() {
  run_test
  reset_docker_mock
  LOW_DISK_MODE=1

  maybe_run_buildx_load "deploy/images/server/Dockerfile" "." "ploy-server:vps" --secret id=test,src=/tmp/test

  assert_file_contains "$DOCKER_CALLS_FILE" "image inspect ploy-server:vps" "checks missing image before build"
  assert_file_contains "$DOCKER_CALLS_FILE" "buildx build --platform linux/amd64 --secret id=test,src=/tmp/test --provenance=false --sbom=false --pull --progress=plain -f deploy/images/server/Dockerfile -t ploy-server:vps --load ." "builds missing image with expected args"
}

test_buildx_load_without_extra_args() {
  run_test
  reset_docker_mock
  LOW_DISK_MODE=0

  maybe_run_buildx_load "deploy/images/server/Dockerfile" "." "ploy-server:vps"

  assert_file_contains "$DOCKER_CALLS_FILE" "buildx build --platform linux/amd64 --provenance=false --sbom=false --pull --progress=plain -f deploy/images/server/Dockerfile -t ploy-server:vps --load ." "builds without optional extra args"
}

test_save_images_combines_base_and_workflow_refs() {
  run_test
  reset_docker_mock

  local refs_file output
  refs_file=$(mktemp)
  output=$(mktemp)
  printf '%s\n' "example.registry/ploy/migs-shell:latest" > "$refs_file"

  save_images "$refs_file" "$output"

  assert_file_contains "$DOCKER_CALLS_FILE" "save -o $output ploy-server:vps ploy-node:vps ploy-garage-init:vps dxflrs/garage:v2.2.0 registry:2.8.3 gradle/build-cache-node:21.2 example.registry/ploy/migs-shell:latest" "saves base and workflow images in one docker save call"

  rm -f "$refs_file" "$output"
}

test_configure_docker_registry_ca_is_noop_without_bundle() {
  run_test
  reset_docker_mock
  unset PLOY_CA_CERTS

  configure_docker_registry_ca_if_needed

  if [[ -s "$DOCKER_CALLS_FILE" ]]; then
    fail "skips docker CA setup without bundle" "expected no docker calls"
  else
    pass "skips docker CA setup without bundle"
  fi
}

main() {
  DOCKER_CALLS_FILE=$(mktemp)

  # shellcheck disable=SC1090
  source "$SCRIPT"

  test_low_disk_reuses_existing_buildx_image
  test_low_disk_builds_missing_image
  test_buildx_load_without_extra_args
  test_save_images_combines_base_and_workflow_refs
  test_configure_docker_registry_ca_is_noop_without_bundle

  rm -f "$DOCKER_CALLS_FILE"

  echo
  echo "Tests run: $TESTS_RUN"
  echo "Passed: $TESTS_PASSED"
  echo "Failed: $TESTS_FAILED"

  if [[ $TESTS_FAILED -ne 0 ]]; then
    exit 1
  fi
}

main "$@"
