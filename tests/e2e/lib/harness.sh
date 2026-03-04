#!/usr/bin/env bash
set -euo pipefail

if [[ -n "${E2E_HARNESS_LOADED:-}" ]]; then
  return 0
fi
E2E_HARNESS_LOADED=1

E2E_HARNESS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=tests/e2e/lib/ensure_local_descriptor.sh
source "${E2E_HARNESS_DIR}/ensure_local_descriptor.sh"
# shellcheck source=tests/e2e/lib/harness_bootstrap.sh
source "${E2E_HARNESS_DIR}/harness_bootstrap.sh"
# shellcheck source=tests/e2e/lib/harness_artifacts.sh
source "${E2E_HARNESS_DIR}/harness_artifacts.sh"
# shellcheck source=tests/e2e/lib/harness_mig.sh
source "${E2E_HARNESS_DIR}/harness_mig.sh"
# shellcheck source=tests/e2e/lib/harness_codex_artifacts.sh
source "${E2E_HARNESS_DIR}/harness_codex_artifacts.sh"
# shellcheck source=tests/e2e/lib/harness_api.sh
source "${E2E_HARNESS_DIR}/harness_api.sh"
