#!/usr/bin/env bash
# redundancy-check.sh — LOC and duplication guardrails for hotspot packages.
#
# Checks performed (all scoped to top-level package files only):
#   1. LOC guardrail    — no single test file exceeds TEST_FILE_LOC_LIMIT lines.
#   2. Parallel family  — no versioned entrypoint pairs (FooV1+FooV2, or Foo+FooV2)
#                         in production code of hotspot packages.
#   3. Legacy pair      — no FooLegacy / FooDeprecated / FooFallback symbol alongside
#                         its base Foo in production code of hotspot packages.
#
# Exit codes:
#   0 — all checks passed
#   1 — one or more findings; details printed to stdout
#
# Usage: bash scripts/redundancy-check.sh
#        make redundancy-check

set -euo pipefail

# Hotspot packages — top-level only, subdirectories are not scanned.
HOTSPOTS=(
  internal/server/handlers
  internal/nodeagent
  internal/workflow/contracts
  internal/store
)

# Hard LOC ceiling for any single _test.go file in a hotspot package.
# Files approaching this size should be split by behavior domain first.
# See docs/testing-workflow.md for split guidance.
TEST_FILE_LOC_LIMIT=1000

FINDINGS=0
FINDINGS_FILE=$(mktemp)
trap 'rm -f "$FINDINGS_FILE"' EXIT

add_finding() {
  echo "  $1" >> "$FINDINGS_FILE"
  FINDINGS=$((FINDINGS + 1))
}

# ---------------------------------------------------------------------------
# check_test_file_loc <pkg>
#   Fail if any *_test.go file in <pkg> top-level exceeds the LOC limit.
# ---------------------------------------------------------------------------
check_test_file_loc() {
  local pkg="$1"
  for f in "$pkg"/*_test.go; do
    [ -f "$f" ] || continue
    loc=$(wc -l < "$f")
    if [ "$loc" -gt "$TEST_FILE_LOC_LIMIT" ]; then
      add_finding "LOC: $f: $loc lines (limit $TEST_FILE_LOC_LIMIT) — split by behavior domain before adding more cases"
    fi
  done
}

# ---------------------------------------------------------------------------
# check_parallel_entrypoints <pkg>
#   Fail if production (non-test) Go files in <pkg> top-level contain:
#     a) Two or more versioned forms of the same base: FooV1 + FooV2, ...
#     b) An unversioned base alongside a versioned form: Foo + FooV2
#     c) A legacy/deprecated shadow alongside its base: FooLegacy + Foo
# ---------------------------------------------------------------------------
check_parallel_entrypoints() {
  local pkg="$1"
  local tmpnames versioned_bases

  tmpnames=$(mktemp)
  versioned_bases=$(mktemp)
  # shellcheck disable=SC2064
  trap "rm -f '$tmpnames' '$versioned_bases'" RETURN

  # Collect exported function names from non-test production files.
  for f in "$pkg"/*.go; do
    [[ "$f" == *_test.go ]] && continue
    [ -f "$f" ] || continue
    grep -oE "^func [A-Z][a-zA-Z0-9_]*" "$f" 2>/dev/null | awk '{print $2}' >> "$tmpnames" || true
  done
  sort -u -o "$tmpnames" "$tmpnames"

  # (a) + (b): versioned pair detection.
  # Strip V[0-9]+ suffix from versioned names; deduplicate bases.
  grep -E "V[0-9]+$" "$tmpnames" | sed 's/V[0-9][0-9]*$//' | sort -u > "$versioned_bases" || true

  while IFS= read -r base; do
    [ -n "$base" ] || continue

    # Count how many Vn forms exist for this base.
    count=$(grep -cE "^${base}V[0-9]+$" "$tmpnames" 2>/dev/null || echo 0)

    if [ "$count" -gt 1 ]; then
      versions=$(grep -oE "^${base}V[0-9]+$" "$tmpnames" | tr '\n' ' ')
      add_finding "PARALLEL_FAMILY: $pkg: '$base' has $count versioned forms (${versions% }) — consolidate into one"
    fi

    # Check whether the unversioned base is also present (Foo + FooV2 pair).
    if grep -qxF "$base" "$tmpnames" 2>/dev/null; then
      versioned=$(grep -oE "^${base}V[0-9]+$" "$tmpnames" | head -1)
      add_finding "PARALLEL_FAMILY: $pkg: '$base' and '$versioned' coexist — remove the superseded form"
    fi
  done < "$versioned_bases"

  # (c): legacy/deprecated shadow detection.
  while IFS= read -r name; do
    [ -n "$name" ] || continue
    if [[ "$name" =~ ^([A-Z][a-zA-Z0-9_]*)(Legacy|Deprecated|Fallback)$ ]]; then
      base="${BASH_REMATCH[1]}"
      if grep -qxF "$base" "$tmpnames" 2>/dev/null; then
        add_finding "PARALLEL_FAMILY: $pkg: '$base' and '$name' coexist — delete the superseded symbol"
      fi
    fi
  done < "$tmpnames"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
for pkg in "${HOTSPOTS[@]}"; do
  if [ ! -d "$pkg" ]; then
    echo "warning: hotspot package directory not found: $pkg" >&2
    continue
  fi
  check_test_file_loc "$pkg"
  check_parallel_entrypoints "$pkg"
done

if [ "$FINDINGS" -gt 0 ]; then
  echo "=== redundancy-check: FAIL ==="
  cat "$FINDINGS_FILE"
  echo ""
  echo "  $FINDINGS finding(s) in hotspot packages."
  echo "  Remediation: see docs/testing-workflow.md#redundancy-guardrails"
  exit 1
fi

echo "redundancy-check: OK (0 findings across ${#HOTSPOTS[@]} hotspot packages)"
