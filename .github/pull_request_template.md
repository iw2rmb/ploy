## Description

<!-- Provide a brief description of the changes in this PR -->

## Type of Change

<!-- Mark the relevant option with an "x" -->

- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Refactoring (no functional changes, no API changes)
- [ ] Documentation update
- [ ] Test coverage improvement
- [ ] Build/CI improvement

## RED → GREEN → REFACTOR Checklist

<!-- This enforces our TDD workflow from ROADMAP.md line 8 and docs/testing-workflow.md -->

### RED Phase

- [ ] I wrote failing tests **BEFORE** implementing the feature/fix
- [ ] The tests correctly captured the requirements

### GREEN Phase

- [ ] I implemented the minimal code to make tests pass
- [ ] All new/modified tests now pass: `make test`

### REFACTOR Phase

- [ ] I refactored code for clarity and maintainability
- [ ] Tests remained green after refactoring
- [ ] I ran tests with race detector: `make test-race` (if applicable)

## Testing

- [ ] I added unit tests for new/changed code
- [ ] All tests pass locally: `make test`
- [ ] Coverage meets threshold: `make test-coverage` (≥60% overall)
- [ ] I verified coverage on critical paths (scheduler/PKI/ingest should be ≥90%)

## Code Quality

- [ ] Code is formatted: `make fmt`
- [ ] No vet issues: `make vet`
- [ ] No lint issues: `make lint` (or `golangci-lint run`)
- [ ] No staticcheck issues: `make staticcheck`
- [ ] All CI checks pass locally: `make ci-check`

## Documentation

- [ ] I updated relevant documentation (README.md, docs/api, docs/envs, docs/how-to)
- [ ] I added/updated code comments for exported functions
- [ ] OpenAPI spec updated (if API changes): `docs/api/OpenAPI.yaml`

## Related Issues

<!-- Link to related issues, e.g., "Closes #123" or "Relates to #456" -->

## Additional Context

<!-- Add any additional context, screenshots, or notes for reviewers -->

## Reviewer Checklist

<!-- For reviewers to verify -->

- [ ] Tests are table-driven with clear failure messages
- [ ] Error handling follows Go best practices (lowercase, no trailing punctuation, `%w` for wrapping)
- [ ] No security issues (SQL injection, XSS, command injection, etc.)
- [ ] Database queries use context and handle errors correctly
- [ ] HTTP handlers set appropriate timeouts and close response bodies
- [ ] Follows GOLANG.md engineering standards
- [ ] Coverage is adequate for the changes
- [ ] Documentation is clear and complete

## Pre-Merge Verification

- [ ] CI tests pass
- [ ] Coverage thresholds met (per-component and overall)
- [ ] No merge conflicts
- [ ] Branch is up to date with main/master
