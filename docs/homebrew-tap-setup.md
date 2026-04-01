# Homebrew Tap Setup

This guide explains how to set up the Homebrew tap for Ploy. The tap enables users to install Ploy via:

```bash
brew install --cask iw2rmb/ploy/ploy
```

## Overview

Homebrew taps are GitHub repositories that contain install metadata for Homebrew. The Ploy Homebrew tap is:

- **Repository**: `iw2rmb/homebrew-ploy`
- **Cask**: `Casks/ploy.rb`
- **Auto-updated**: GoReleaser automatically updates the cask on each release

## One-Time Setup

### 1. Create the Tap Repository

Create a new repository on GitHub:
- **Name**: `homebrew-ploy`
- **Owner**: `iw2rmb`
- **Visibility**: Public (required for Homebrew taps)
- **Description**: "Homebrew tap for Ploy"
- **Initialize**: With README

Full URL: https://github.com/iw2rmb/homebrew-ploy

### 2. Initialize the Repository Structure

```bash
# Clone the new repository
git clone https://github.com/iw2rmb/homebrew-ploy.git
cd homebrew-ploy

# Create Casks directory
mkdir -p Casks

# Create .gitignore
cat > .gitignore <<'EOF'
.DS_Store
*.swp
*.swo
*~
EOF

# Create README.md (see template below)
cat > README.md <<'EOF'
# Ploy Homebrew Tap

Official Homebrew tap for [Ploy](https://github.com/iw2rmb/ploy).

## Installation

```bash
brew install --cask iw2rmb/ploy/ploy
```

## Usage

After installation, verify:

```bash
ploy version
```

For documentation and usage, see the [main repository](https://github.com/iw2rmb/ploy).

## Updating

```bash
brew update
brew upgrade ploy
```

## Uninstalling

```bash
brew uninstall ploy
brew untap iw2rmb/ploy
```

## Cask

The cask is automatically updated by GoReleaser when new releases are published.

## Support

For issues with Ploy itself, please use the [main repository](https://github.com/iw2rmb/ploy/issues).
EOF

# Commit and push
git add .
git commit -m "Initial tap setup"
git push origin main
```

### 3. Create GitHub Personal Access Token

GoReleaser needs a token to update the tap automatically.

1. Go to: https://github.com/settings/tokens/new
2. **Note**: "GoReleaser Homebrew Tap for Ploy"
3. **Expiration**: No expiration (or set a reminder to rotate)
4. **Scopes**:
   - ✅ `repo` (Full control of private repositories)
     - This includes public repositories
5. Click "Generate token"
6. **Copy the token** — you won't see it again!

### 4. Add Token to Repository Secrets

1. Go to: https://github.com/iw2rmb/ploy/settings/secrets/actions
2. Click "New repository secret"
3. **Name**: `HOMEBREW_TAP_GITHUB_TOKEN`
4. **Value**: [paste the token from step 3]
5. Click "Add secret"

### 5. Verify Configuration

Check that `.goreleaser.yml` has the correct tap configuration:

```yaml
homebrew_casks:
  - name: ploy
    repository:
      owner: iw2rmb
      name: homebrew-ploy
      token: "{{ .Env.HOMEBREW_TAP_GITHUB_TOKEN }}"
    homepage: https://github.com/iw2rmb/ploy
    description: "Ploy - Infrastructure orchestration and deployment platform"
    license: "Apache-2.0"
    directory: Casks
    ids:
      - ploy
    binary: ploy
```

## Testing the Tap

### After First Release

Once you create the first release with a version tag (e.g., `v0.1.0`):

1. **Verify the cask was created**:
   - Check: https://github.com/iw2rmb/homebrew-ploy/blob/main/Casks/ploy.rb
   - Should be auto-created by GoReleaser

2. **Test installation locally**:
   ```bash
   # Install ploy
   brew install --cask iw2rmb/ploy/ploy

   # Verify
   ploy version
   ```

3. **Test upgrade**:
   ```bash
   # After creating a new release
   brew update
   brew upgrade ploy
   ploy version  # Should show new version
   ```

### Manual Cask Update (If Needed)

If GoReleaser doesn't create the cask automatically, update it manually:

```bash
cd homebrew-ploy

# Edit Casks/ploy.rb to point to the desired release artifact/checksum

# Commit and push
git add Casks/ploy.rb
git commit -m "Add ploy v0.1.0"
git push
```

## Troubleshooting

### Cask Not Auto-Updated

**Possible causes**:
1. `HOMEBREW_TAP_GITHUB_TOKEN` not set or expired
2. Token doesn't have `repo` scope
3. Repository name mismatch in `.goreleaser.yml`
4. Network/API issues during release

**Solution**:
- Check GitHub Actions logs for the release workflow
- Verify token exists: https://github.com/iw2rmb/ploy/settings/secrets/actions
- Manually update the cask as a workaround (see above)

### Installation Fails

**Check cask install details**:
```bash
brew install --cask --debug --verbose iw2rmb/ploy/ploy
```

**Audit the cask**:
```bash
brew audit --cask --strict --online iw2rmb/ploy/ploy
```

### SHA256 Mismatch

If Homebrew reports a SHA256 mismatch:
1. The binary was modified after the cask was created
2. The checksum in the cask is incorrect

**Fix**:
- Re-download the binary and recalculate SHA256:
  ```bash
  curl -LO https://github.com/iw2rmb/ploy/releases/download/vX.Y.Z/ploy_X.Y.Z_darwin_amd64.tar.gz
  shasum -a 256 ploy_X.Y.Z_darwin_amd64.tar.gz
  ```
- Update the cask with the correct checksum

## Maintenance

### Token Rotation

If you set an expiration on the GitHub token:

1. Create a new token (same steps as above)
2. Update the secret:
   - Go to: https://github.com/iw2rmb/ploy/settings/secrets/actions
   - Click `HOMEBREW_TAP_GITHUB_TOKEN`
   - Click "Update"
   - Paste the new token
3. The next release will use the new token

### Cask Updates

GoReleaser automatically updates:
- Version number
- Download URLs
- SHA256 checksums

You may manually update:
- Description
- Homepage
- License
- Test commands

## Resources

- [Homebrew Cask Cookbook](https://docs.brew.sh/Cask-Cookbook)
- [Homebrew Taps](https://docs.brew.sh/Taps)
- [GoReleaser Homebrew Integration](https://goreleaser.com/customization/homebrew/)

---

**Related Documentation**:
- [Releasing Guide](./releasing.md)
- [Testing workflow](./testing-workflow.md)
