#!/usr/bin/env bash
set -euo pipefail

# Hygiene validation for roadmap item 1.6.4.
# Runs the full test/vet/staticcheck suite from the repository root.
# Must pass before 1.6 is considered complete.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

cd "$REPO_ROOT"

echo "=== 1.6.4 hygiene validation ==="
echo "Repository: $REPO_ROOT"
echo ""

echo "--- make test ---"
make test
echo ""

echo "--- make vet ---"
make vet
echo ""

echo "--- make staticcheck ---"
make staticcheck
echo ""

echo "OK: all hygiene checks passed."
