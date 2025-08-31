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

**LOCAL**: `make test-unit`, `make test-coverage-threshold`, build verification  
**VPS**: `./bin/ployman api deploy --monitor`  
**NEVER**: Run API locally, integration tests locally, direct Nomad commands

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

