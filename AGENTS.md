# CLAUDE.md

**MANDATORY**: Follow this file for every prompt execution.

## TDD Framework (CRITICAL)

- **LOCAL**: Unit tests, build compilation (RED/GREEN phases)  
- **VPS**: Integration/E2E tests (REFACTOR phase)
- **Coverage**: 60% minimum, 90% for critical components
- **Cycle**: RED (write failing tests) → GREEN (minimal code) → REFACTOR (on VPS)

## Ploy Overview

Deployment lanes A-G auto-selected by project structure. Update `FEATURES.md`, `CHANGELOG.md` for changes.  
**WASM features**: Reference `docs/WASM.md` for Lane G implementation.

## Code Analysis

**USE**: MCP aster tools for semantic search (`mcp__aster__aster_search`, `mcp__aster__aster_slice`)  
**USE**: Grep for regex/strings, Glob for file patterns, Read for complete files

## VPS Testing

**Setup**: `ssh root@$TARGET_HOST` → `su - ploy`  
**Nomad**: ONLY use `/opt/hashicorp/bin/nomad-job-manager.sh` (never direct `nomad` commands)

## Commands

**LOCAL**: 
- `make test-unit`, `make test-coverage-threshold`, build verification
- Deploy API: `./bin/ployman api deploy --monitor` (run on workstation)

Notes:
- Run `./bin/ployman api deploy --monitor` on your workstation. Do not run it on the VPS.
- Never use direct Nomad commands; if needed remotely, only via `/opt/hashicorp/bin/nomad-job-manager.sh` as invoked by platform tooling.

**VPS**:
- Use for runtime inspection and logs only (e.g., `ssh root@$TARGET_HOST`, then `su - ploy`).
- Do not run `ployman` deploys directly on the VPS.

**NEVER**: Integration tests locally, direct Nomad commands

## Mandatory Update Protocol (CRITICAL)

For EVERY code change:

1. **Write failing tests** (RED phase)
2. **Write minimal code** to pass tests (GREEN phase)  
3. **Deploy to VPS** for integration testing (REFACTOR phase)
4. **Update documentation** (`CHANGELOG.md`, `FEATURES.md` as needed)
5. **Merge to main** and return to worktree branch

**NO EXCEPTIONS**.

## Specialized Agents

Use Task tool for complex domain-specific tasks. Available agents in `.claude/agents.json`.

## Sessions System Behaviors

@CLAUDE.sessions.md
