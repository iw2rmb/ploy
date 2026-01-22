# Releasing Ploy

This guide covers the release process for Ploy maintainers. Releases are automated via GoReleaser and GitHub Actions.

## Versioning Strategy

Ploy follows [Semantic Versioning 2.0.0](https://semver.org/):

- **MAJOR** version (v**X**.0.0) — Incompatible API changes, breaking changes
- **MINOR** version (v0.**X**.0) — New features, backwards-compatible functionality
- **PATCH** version (v0.0.**X**) — Bug fixes, backwards-compatible fixes

Pre-release versions can use suffixes: `v1.0.0-alpha.1`, `v1.0.0-beta.2`, `v1.0.0-rc.1`

## Release Artifacts

Each release publishes the following binaries:

### Ploy CLI (`ploy`)
- Linux: amd64, arm64
- macOS: amd64 (Intel), arm64 (Apple Silicon)
- Windows: amd64

### Server Daemon (`ployd`)
- Linux: amd64, arm64
- macOS: amd64, arm64

### Worker Node Daemon (`ployd-node`)
- Linux: amd64, arm64
- macOS: amd64, arm64

All binaries include:
- SHA256 checksums (`checksums.txt`)
- SBOMs (Software Bill of Materials)
- Cosign keyless signatures
- Version information embedded via ldflags

## Prerequisites

Before creating a release, ensure you have:

1. **Permissions**:
   - Write access to the `iw2rmb/ploy` repository
   - Write access to the `iw2rmb/homebrew-ploy` tap (for Homebrew)
   - Personal access token with `repo` scope (for Homebrew tap updates)

2. **Environment Setup**:
   - Git configured with signing enabled (optional but recommended)
   - Clean working directory (`git status` shows no uncommitted changes)
   - Latest code from `main` branch

3. **Quality Gates**:
   - All CI checks passing on `main`
   - Test coverage ≥60% overall
   - Critical path coverage ≥90%
   - No known critical bugs

## Pre-Release Checklist

Before creating a release tag:

- [ ] Update `CHANGELOG.md` with release notes for this version
  - Document breaking changes (if any)
  - List new features
  - Document bug fixes
  - Note performance improvements
  - Acknowledge contributors
- [ ] Verify all tests pass: `make test`
- [ ] Verify coverage thresholds: `make test-coverage`
- [ ] Run full CI checks locally: `make ci-check`
- [ ] Verify build succeeds: `make build`
- [ ] Review open issues and PRs for critical fixes
- [ ] Update documentation if API changes exist
- [ ] Verify environment variable docs are current (`docs/envs/README.md`)

## Release Process

### Step 1: Determine the Version

Based on the changes since the last release, determine the appropriate version number:

```bash
# View the last release tag
git describe --tags --abbrev=0

# View changes since last release
git log $(git describe --tags --abbrev=0)..HEAD --oneline

# View commits by category
git log $(git describe --tags --abbrev=0)..HEAD --pretty=format:"%s" | grep -E "^(feat|fix|perf|BREAKING)"
```

### Step 2: Update CHANGELOG.md

Add a new section at the top of `CHANGELOG.md`:

```markdown
## [X.Y.Z] - YYYY-MM-DD

### Breaking Changes
- Description of breaking changes (if MAJOR version bump)

### Features
- New feature descriptions

### Bug Fixes
- Bug fix descriptions

### Performance
- Performance improvement descriptions

### Documentation
- Documentation updates
```

Commit the changelog:

```bash
git add CHANGELOG.md
git commit -m "docs: update CHANGELOG for vX.Y.Z"
git push origin main
```

### Step 3: Create and Push the Tag

```bash
# Create annotated tag (use the version number)
git tag -a vX.Y.Z -m "Release vX.Y.Z"

# Push the tag to GitHub
git push origin vX.Y.Z
```

**Important**: The tag MUST start with `v` and follow semantic versioning (`vX.Y.Z`).

### Step 4: Monitor the Release Workflow

1. Go to the [Actions tab](https://github.com/iw2rmb/ploy/actions/workflows/release.yml)
2. Watch the release workflow for your tag
3. The workflow will:
   - Run pre-release checks (tests, coverage, linting)
   - Build binaries for all platforms
   - Generate checksums and SBOMs
   - Sign artifacts with Cosign
   - Create a GitHub Release with all artifacts
   - Update the Homebrew tap (if `HOMEBREW_TAP_GITHUB_TOKEN` is configured)

The workflow typically takes 5-10 minutes.

### Step 5: Verify the Release

Once the workflow completes:

1. **Check the GitHub Release**:
   - Visit: https://github.com/iw2rmb/ploy/releases/tag/vX.Y.Z
   - Verify all binaries are present
   - Verify checksums.txt is present
   - Verify release notes look correct

2. **Test the Installation Methods**:

   ```bash
   # Test Homebrew (if tap is configured)
   brew update
   brew upgrade iw2rmb/ploy/ploy
   ploy version  # Should show vX.Y.Z

   # Test install script
   curl -fsSL https://raw.githubusercontent.com/iw2rmb/ploy/main/scripts/install.sh | bash
   ploy version

   # Test direct download
   # Download a binary from the release page and verify it runs
   ```

3. **Verify Checksums**:

   ```bash
   # Download a binary and checksums.txt
   wget https://github.com/iw2rmb/ploy/releases/download/vX.Y.Z/ploy_X.Y.Z_linux_amd64.tar.gz
   wget https://github.com/iw2rmb/ploy/releases/download/vX.Y.Z/checksums.txt

   # Verify checksum
   sha256sum -c checksums.txt --ignore-missing
   ```

## Homebrew Tap Setup

The Homebrew tap requires a separate repository and GitHub token.

### One-Time Setup

1. **Create the tap repository**:
   ```bash
   # On GitHub, create a new repository: iw2rmb/homebrew-ploy
   ```

2. **Initialize the repository**:
   ```bash
   mkdir homebrew-ploy
   cd homebrew-ploy

   # Create Formula directory
   mkdir -p Formula

   # Initialize git
   git init
   git add .
   git commit -m "Initial commit"
   git remote add origin https://github.com/iw2rmb/homebrew-ploy.git
   git push -u origin main
   ```

3. **Create GitHub token**:
   - Go to: https://github.com/settings/tokens/new
   - Name: "GoReleaser Homebrew Tap"
   - Scopes: `repo` (full control)
   - Generate and copy the token

4. **Add token to repository secrets**:
   - Go to: https://github.com/iw2rmb/ploy/settings/secrets/actions
   - Click "New repository secret"
   - Name: `HOMEBREW_TAP_GITHUB_TOKEN`
   - Value: [paste the token]
   - Click "Add secret"

5. **Verify on next release**:
   - After the next release, check that the Formula was updated:
     https://github.com/iw2rmb/homebrew-ploy/blob/main/Formula/ploy.rb

## Troubleshooting

### Release Workflow Failed

**Check the workflow logs**:
1. Go to: https://github.com/iw2rmb/ploy/actions
2. Click on the failed workflow
3. Expand failed steps to see error messages

**Common issues**:

- **Tests failing**: Fix tests and push changes, then delete and recreate the tag
  ```bash
  git tag -d vX.Y.Z
  git push origin :refs/tags/vX.Y.Z
  # Fix issues, commit, push
  git tag -a vX.Y.Z -m "Release vX.Y.Z"
  git push origin vX.Y.Z
  ```

- **GoReleaser build failed**: Check build errors in logs, fix issues, recreate tag
- **Homebrew tap update failed**: Verify `HOMEBREW_TAP_GITHUB_TOKEN` is set and valid
- **Permission denied**: Ensure you have write access to the repository

### Binary Missing from Release

If a specific platform binary is missing:

1. Check GoReleaser logs for that platform
2. Verify `.goreleaser.yml` includes that platform
3. Check if the platform was explicitly excluded
4. Re-run GoReleaser locally to test:
   ```bash
   goreleaser release --snapshot --clean
   ```

### Homebrew Formula Not Updated

If the Homebrew tap wasn't updated:

1. Verify `HOMEBREW_TAP_GITHUB_TOKEN` is set in repository secrets
2. Check release workflow logs for Homebrew-related errors
3. Manually update the formula if needed:
   ```bash
   cd /tmp
   git clone https://github.com/iw2rmb/homebrew-ploy.git
   cd homebrew-ploy
   # Edit Formula/ploy.rb
   # Update version and sha256
   git commit -am "ploy vX.Y.Z"
   git push
   ```

### Version Information Incorrect

If `ploy version` shows incorrect information:

1. Verify the tag follows format `vX.Y.Z` exactly
2. Check that ldflags in `.goreleaser.yml` are correct
3. Verify the version package exists: `internal/version/version.go`

## Manual Release (Emergency)

If automated releases fail completely, you can create a manual release:

```bash
# Build locally with GoReleaser
goreleaser release --clean

# This creates dist/ with all binaries and archives
# Manually upload to GitHub Releases
```

## Rollback Procedure

If a release has critical issues:

1. **Mark the release as pre-release**:
   - Edit the GitHub Release
   - Check "This is a pre-release"
   - Add a warning to the description

2. **Create a patch release**:
   - Fix the critical issue
   - Follow the release process for vX.Y.Z+1
   - Reference the previous release in changelog

3. **Do NOT delete releases** — this breaks downloads and can cause confusion

## Support

For questions or issues with the release process:
- Open an issue: https://github.com/iw2rmb/ploy/issues
- Contact maintainers directly

---

**Related Documentation**:
- GoReleaser Configuration: [`.goreleaser.yml`](../.goreleaser.yml)
- Release Workflow: [`.github/workflows/release.yml`](../.github/workflows/release.yml)
- Changelog: [`CHANGELOG.md`](../CHANGELOG.md)
- Contributing Guide: [`AGENTS.md`](../AGENTS.md)
