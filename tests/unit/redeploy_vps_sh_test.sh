#!/usr/bin/env bash
# Unit tests for deploy/vps/run.sh helper behavior.
#
# Usage: bash tests/unit/redeploy_vps_sh_test.sh

set -uo pipefail

ROOT_DIR=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
SCRIPT="$ROOT_DIR/deploy/vps/run.sh"

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

test_reuses_existing_buildx_image() {
  run_test
  reset_docker_mock
  CLEAN=0
  MOCK_EXISTING_IMAGES="server:local"

  maybe_run_buildx_load "deploy/images/server/Dockerfile" "." "server:local"

  assert_file_contains "$DOCKER_CALLS_FILE" "image inspect server:local" "inspects existing buildx image"
  assert_file_not_contains "$DOCKER_CALLS_FILE" "buildx build" "skips buildx build when image exists"
}

test_builds_missing_image() {
  run_test
  reset_docker_mock
  CLEAN=0

  maybe_run_buildx_load "deploy/images/server/Dockerfile" "." "server:local" --secret id=test,src=/tmp/test

  assert_file_contains "$DOCKER_CALLS_FILE" "image inspect server:local" "checks missing image before build"
  assert_file_contains "$DOCKER_CALLS_FILE" "buildx build --platform linux/amd64 --secret id=test,src=/tmp/test --provenance=false --sbom=false --pull --progress=plain -f deploy/images/server/Dockerfile -t server:local --load ." "builds missing image with expected args"
}

test_buildx_load_without_extra_args() {
  run_test
  reset_docker_mock
  CLEAN=0

  maybe_run_buildx_load "deploy/images/server/Dockerfile" "." "server:local"

  assert_file_contains "$DOCKER_CALLS_FILE" "buildx build --platform linux/amd64 --provenance=false --sbom=false --pull --progress=plain -f deploy/images/server/Dockerfile -t server:local --load ." "builds without optional extra args"
}

test_clean_rebuilds_existing_image() {
  run_test
  reset_docker_mock
  CLEAN=1
  MOCK_EXISTING_IMAGES="server:local"

  maybe_run_buildx_load "deploy/images/server/Dockerfile" "." "server:local"

  assert_file_contains "$DOCKER_CALLS_FILE" "buildx build --platform linux/amd64 --provenance=false --sbom=false --pull --progress=plain -f deploy/images/server/Dockerfile -t server:local --load ." "clean mode rebuilds image even when present"
}

test_runtime_images_use_local_dist_binaries() {
  run_test
  reset_docker_mock
  CLEAN=1

  build_runtime_images

  assert_file_contains "$DOCKER_CALLS_FILE" "-t server:local --load -f - ." "server image is built from inline dockerfile"
  assert_file_contains "$DOCKER_CALLS_FILE" "-t node:local --load -f - ." "node image is built from inline dockerfile"
  assert_file_not_contains "$DOCKER_CALLS_FILE" "deploy/images/server/Dockerfile" "runtime server image does not use source-building dockerfile"
  assert_file_not_contains "$DOCKER_CALLS_FILE" "deploy/images/node/Dockerfile" "runtime node image does not use source-building dockerfile"
}

test_save_images_combines_base_and_workflow_refs() {
  run_test
  reset_docker_mock

  local refs_file output
  refs_file=$(mktemp)
  output=$(mktemp)
  printf '%s\n' "example.registry/ploy/shell:latest" > "$refs_file"

  save_images "$refs_file" "$output"

  assert_file_contains "$DOCKER_CALLS_FILE" "save -o $output server:local node:local ploy-garage-init:local dxflrs/amd64_garage:v2.2.0 amd64/registry:3 gradle/build-cache-node:21.2 example.registry/ploy/shell:latest" "saves base and workflow images in one docker save call"

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

  test_reuses_existing_buildx_image
  test_builds_missing_image
  test_buildx_load_without_extra_args
  test_clean_rebuilds_existing_image
  test_runtime_images_use_local_dist_binaries
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
