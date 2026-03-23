# 9 JVM flags in scripts/configs

Parent: `roadmap.md` item `1.9`.
Source: `../post-orw-java11-to-17-migration.md` section `8`.

## Goal
Remove obsolete JVM flags from repository scripts/config while keeping unresolved cases explicit.

## Detailed actions
1. Search VCS-tracked launch scripts/config for `--illegal-access`, `--add-opens`, `--add-exports`, `--add-modules java.se.ee`, `-Xverify:none`.
2. Remove flags made unnecessary by code cleanup.
3. For still-needed flags, keep them with TODO notes naming required module/package access.
4. Do not add new tuning/diagnostic JVM flags in this migration slice.

## Verification checklist
- Each risky flag is either removed or justified with a concrete TODO.
- No new compatibility flags were introduced.

## Sizing
- CFP_delta: 5
- Base reasoning: medium
- Shifted for assumption-bound: high
