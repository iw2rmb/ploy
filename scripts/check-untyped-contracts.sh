#!/usr/bin/env bash
#
# check-untyped-contracts.sh — Guardrail against untyped contracts at API boundaries
#
# This script detects new map[string]any / map[string]interface{} usage in boundary
# packages (internal/server/handlers, internal/nodeagent, cmd/ploy) that could regress
# the type hardening work.
#
# ## Rationale
#
# Type hardening replaced untyped map[string]any with typed structs
# at critical API boundaries. This script prevents regression by flagging new untyped
# usage outside approved parsing modules.
#
# ## Approved Modules (Excluded)
#
# These files contain centralized parsing that converts untyped JSON to typed structs:
#   - internal/nodeagent/claimer_spec.go       — Spec JSON → RunOptions parsing
#   - internal/nodeagent/run_options.go        — map[string]any → typed RunOptions
#   - cmd/ploy/mod_run_spec.go                 — CLI spec file parsing
#   - internal/server/handlers/spec_utils.go   — Server spec manipulation utilities
#
# ## Additional Exclusions
#
# These files are excluded for specific reasons:
#   - internal/nodeagent/manifest.go                    — Populates typed StepManifest.Options field
#   - internal/nodeagent/execution_orchestrator.go      — Uses typed StepManifest.Options field
#   - internal/nodeagent/execution_healing.go           — Uses typed StepManifest.Options field
#   - internal/nodeagent/execution_orchestrator_rehydrate.go — Uses typed StepManifest.Options field
#
# ## Test Files (Excluded)
#
# Test files are excluded because map[string]any is acceptable for building test fixtures
# and asserting on JSON payloads.
#
# ## Comment Lines (Excluded)
#
# Lines that are pure comments (starting with //) are excluded since they document
# the typed approach rather than introducing untyped contracts.
#
# ## Usage
#
#   ./scripts/check-untyped-contracts.sh          # Exit 0 if clean, 1 if violations
#   ./scripts/check-untyped-contracts.sh --count  # Print count of violations
#   ./scripts/check-untyped-contracts.sh --list   # List violations without summary
#
# ## Exit Codes
#
#   0 — No violations found
#   1 — Violations detected (prints file:line for each)
#
set -euo pipefail

# Boundary packages to check for untyped contract usage.
# These are the packages where type safety is critical.
BOUNDARY_PATHS=(
    "internal/server/handlers"
    "internal/nodeagent"
    "cmd/ploy"
)

# Approved parsing modules that are allowed to use map[string]any.
# These files centralize the untyped → typed conversion or use typed struct fields.
APPROVED_FILES=(
    # Centralized parsing: converts untyped JSON to typed RunOptions
    "internal/nodeagent/claimer_spec.go"
    "internal/nodeagent/run_options.go"
    "cmd/ploy/mod_run_spec.go"
    "internal/server/handlers/spec_utils.go"
    # Uses typed contracts.StepManifest.Options field (defined in contracts package)
    "internal/nodeagent/manifest.go"
    "internal/nodeagent/execution_orchestrator.go"
    "internal/nodeagent/execution_healing.go"
	    "internal/nodeagent/execution_orchestrator_rehydrate.go"
	    "internal/nodeagent/execution_healing_helpers.go"
	    "internal/nodeagent/execution_mr_job.go"
	    "internal/nodeagent/execution_orchestrator_gate.go"
	    "internal/nodeagent/healing_injection.go"
	    # HTTP uploader payloads (wire format, not contract parsing)
	    "internal/nodeagent/statusuploader.go"
	    "internal/nodeagent/diffuploader.go"
	    "internal/nodeagent/artifactuploader.go"
    # Simple HTTP response encoding (acceptable for simple responses)
    "internal/server/handlers/health.go"
    "internal/server/handlers/artifacts_download.go"
    "internal/server/handlers/jobs_artifact.go"
    "internal/server/handlers/jobs_diff.go"
    "internal/server/handlers/runs_diffs.go"
    "internal/server/handlers/runs_events.go"
    "internal/server/handlers/nodes_events.go"
    "internal/server/handlers/nodes_claim.go"
    # Comments documenting typed approach (already hardened)
    "internal/nodeagent/claimer_loop.go"
    "internal/nodeagent/heartbeat.go"
    "internal/nodeagent/handlers.go"
    "internal/server/handlers/jobs_complete.go"
    # CLI command files using simple request bodies
    "cmd/ploy/token_commands.go"
    "cmd/ploy/node_command.go"
)

# Pattern to match map[string]any or map[string]interface{}.
# Uses ripgrep regex syntax.
PATTERN='map\[string\](any|interface\{\})'

# Colors for output (disabled if not a terminal).
if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    NC='\033[0m' # No Color
else
    RED=''
    GREEN=''
    YELLOW=''
    NC=''
fi

# Build rg --glob exclusion patterns for approved files and test files.
build_exclusions() {
    local exclusions=""

    # Exclude test files (acceptable for test fixtures).
    exclusions+=" --glob '!*_test.go'"
    exclusions+=" --glob '!*_fuzz_test.go'"

    # Exclude approved parsing modules by full path (avoid accidental exclusions via basename collisions).
    for file in "${APPROVED_FILES[@]}"; do
        exclusions+=" --glob '!$file'"
    done

    echo "$exclusions"
}

# Check for untyped contracts in boundary packages.
# Returns: multi-line output where last line is the violation count.
check_boundaries() {
    local violations=0
    local exclusions
    exclusions=$(build_exclusions)

    for path in "${BOUNDARY_PATHS[@]}"; do
        if [ ! -d "$path" ]; then
            continue
        fi

        # Use eval to expand the glob patterns.
        # rg returns exit code 1 when no matches found, so we handle it.
        local matches
        if matches=$(eval "rg -n --type go $exclusions '$PATTERN' '$path'" 2>/dev/null); then
            # Filter out pure comment lines (lines where the match is inside //)
            # This is a simple heuristic: if the line starts with optional whitespace
            # followed by //, it's a comment.
            while IFS= read -r line; do
                # Extract the content after file:line:
                local content
                content=$(echo "$line" | cut -d: -f3-)

                # Skip if the content starts with // (pure comment)
                if echo "$content" | grep -qE '^\s*//'; then
                    continue
                fi

                echo "$line"
                violations=$((violations + 1))
            done <<< "$matches"
        fi
    done

    echo "$violations"
}

# Main entry point.
main() {
    local mode="normal"
    if [ "${1:-}" = "--count" ]; then
        mode="count"
    elif [ "${1:-}" = "--list" ]; then
        mode="list"
    fi

    # Capture output and count.
    local output
    output=$(check_boundaries)

    # Last line is the count.
    local violations
    violations=$(echo "$output" | tail -n 1)
    violations=${violations:-0}

    # Get violation lines (all lines except the last count line).
    # Use sed for portability (BSD head does not support `head -n -1`).
    local violation_lines
    violation_lines=$(printf '%s\n' "$output" | sed '$d')

    case "$mode" in
        count)
            echo "$violations"
            exit 0
            ;;
        list)
            if [ -n "$violation_lines" ]; then
                printf '%s\n' "$violation_lines"
            fi
            [ "$violations" -eq 0 ] && exit 0 || exit 1
            ;;
        normal)
            if [ "$violations" -gt 0 ]; then
                echo -e "${RED}ERROR: Found $violations untyped contract(s) at API boundaries${NC}"
                echo ""
                echo -e "${YELLOW}Violations:${NC}"
                printf '%s\n' "$violation_lines"
                echo ""
                echo -e "${YELLOW}Resolution:${NC}"
                echo "  1. Use typed structs instead of map[string]any at API boundaries."
                echo "  2. If this is approved usage, add the file to APPROVED_FILES in this script."
                echo "  3. See docs/api/ and docs/mods-lifecycle.md for contract guidance."
                exit 1
            else
                echo -e "${GREEN}OK: No untyped contracts found at API boundaries${NC}"
                exit 0
            fi
            ;;
    esac
}

main "$@"
