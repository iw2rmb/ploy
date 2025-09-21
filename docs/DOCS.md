# Documentation Conventions

Authoritative guidance for maintaining agent instructions and scoped READMEs. Apply these rules whenever editing `AGENTS.md` or README files inside subdirectories.

## AGENTS.md Structure
- **Purpose**: Operational handbook for every agent execution. Keep it concise, non-negotiable, and updated whenever workflow rules change.
- **Top Checklist**: Start with `Before You Start` checkbox list capturing TDD cadence, environment validation, and context awareness.
- **Lifecycle Sections**: Organise content into `Local Development`, `VPS Workflows`, and `Deploy & Release`. Each subsection should group tasks chronologically (tests → tooling → deployment).
- **Reference Panel**: End with a `Reference` section listing documents or tools without re-explaining their contents. Link to scoped READMEs instead of duplicating them.
- **Operational Detail**: Place lengthy procedures (e.g., Nomad log slicing) under clearly titled subsections so agents can skim. Prefer bullet lists over prose blocks.
- **Consistency Rule**: When AGENTS.md changes, confirm the same behaviour is documented (or intentionally omitted) in related READMEs and update this file if structural guidance shifts.

## Subfolder README Expectations
- **Audience**: Developers working inside the folder. Keep the focus tight—avoid repo-wide instructions already covered elsewhere.
- **Template**:
  1. `# <Component> Guide` title.
  2. `Purpose` summarising the module’s role and scope.
  3. `Narrative Summary` describing behaviour at a high level.
  4. `Key Files` with relative links using hash anchors (`./file.go#L10`). Group by category if needed.
  5. Additional sections tailored to the component (`Integration Points`, `Configuration`, `Key Patterns`, etc.).
  6. `Related Documentation` pointing to other sources rather than repeating content.
- **No Redundancy**: Avoid restating material word-for-word from AGENTS.md or other READMEs. Link instead.
- **Environment Values**: When referencing secrets or hosts, prefer placeholder tokens (e.g., ``TARGET_HOST``) rather than literal addresses.
- **Updates**: If a README introduces or removes commands, update AGENTS.md and CHANGELOG.md when relevant.

## Change Checklist
- Review this file before editing AGENTS.md or any scoped README.
- After changes, skim all affected Markdown for duplicated explanations; replace with links where possible.
- Ensure cross-references resolve (e.g., renamed files, new anchors). Use `rg` or `go test ./...` documentation checks as needed.
- Mention documentation updates in the CHANGELOG when behaviour changes or when a new guide is introduced.

## Runbooks
- Location: `docs/runbooks/`
- Audience: Operators executing repeatable infrastructure workflows (deployments, credential rotation).
- Structure: Start with an overview and prerequisites, then provide ordered sections for deployment, verification, operations, and troubleshooting. Link back to authoritative code paths (e.g., Nomad specs) instead of duplicating full context.
- Keep secret handling instructions generic (reference Nomad variables, masked outputs). Avoid embedding literal credentials or tokens.
- Reference new runbooks from `docs/REPO.md` (and vice versa) so navigation surfaces operational content alongside architectural guides.

Keeping these documents in sync prevents token-heavy prompts and conflicting instructions. When in doubt, document the rule here and reference it from the affected Markdown.
