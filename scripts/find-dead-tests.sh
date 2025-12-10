#!/usr/bin/env bash
set -euo pipefail

# Find tests that do not exercise non-test code in this module.
# For each Test* in the selected packages, this script:
#   - runs it in isolation with coverage
#   - inspects the coverprofile
#   - reports tests that only touch *_test.go files
#
# Usage:
#   scripts/find-dead-tests.sh                # scan ./...
#   scripts/find-dead-tests.sh ./cmd/ploy/... # scan a subtree
#
# Output format:
#   <package-import-path> <TestName>
#
# These are candidates for removal or refactor, since they never cover any
# non-test .go files in the module.

ROOT="${PWD}"
PATTERN="${1:-./...}"

log() {
  echo "[$(date -u +%H:%M:%S)] $*"
}

if ! command -v go >/dev/null 2>&1; then
  echo "error: go not found in PATH" >&2
  exit 1
fi

log "Scanning packages matching pattern: $PATTERN"

PKGS=$(go list "$PATTERN")

for pkg in $PKGS; do
  log "Listing tests in package: $pkg"
  # List Test* functions; filter out non-test lines.
  TESTS=$(go test -list '^Test' "$pkg" 2>/dev/null | sed -n 's/^\(Test[^(]*\).*/\1/p' || true)
  if [[ -z "${TESTS}" ]]; then
    continue
  fi

  for t in $TESTS; do
    profile="$(mktemp "${TMPDIR:-/tmp}/deadtest.${t}.XXXXXX")"
    # Run the single test with coverage over the module.
    if ! go test "$pkg" -run "^${t}\$" -coverpkg=./... -coverprofile="$profile" >/dev/null 2>&1; then
      log "WARN: $pkg $t failed (skipping coverage analysis)"
      rm -f "$profile"
      continue
    fi

    # Determine whether any non-test .go file in this module has non-zero coverage.
    # coverprofile format: "path:line.column,line.column numStatements count"
    if ! awk -v root="$ROOT" '
      BEGIN { hits = 0 }
      /^mode: / { next }
      {
        split($1, f, ":")
        file = f[1]
        # Normalize to absolute path when possible.
        if (substr(file, 1, 1) != "/") {
          file = root "/" file
        }
        # Only consider files under the current repo root.
        if (index(file, root) != 1) {
          next
        }
        # Ignore test files.
        if (file ~ /_test\.go$/) {
          next
        }
        # $3 is coverage percentage (e.g., "12.5%").
        if ($3 != "0.0%") {
          hits = 1
          exit
        }
      }
      END { exit(hits ? 0 : 1) }
    ' "$profile"; then
      # No non-test code hit by this test: report as candidate.
      echo "$pkg $t"
    fi

    rm -f "$profile"
  done
done

log "Done."
