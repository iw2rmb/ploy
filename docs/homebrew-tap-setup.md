# Homebrew Tap Setup

This guide explains how to set up the Homebrew tap for Ploy. The tap enables users to install Ploy via:

```bash
brew install iw2rmb/ploy/ploy
```

## Overview

Homebrew taps are GitHub repositories that contain formulae (installation instructions) for Homebrew. The Ploy Homebrew tap is:

- **Repository**: `iw2rmb/homebrew-ploy`
- **Formula**: `Formula/ploy.rb`
- **Auto-updated**: GoReleaser automatically updates the formula on each release

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

# Create Formula directory
mkdir -p Formula

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
brew install iw2rmb/ploy/ploy
```

Or tap first, then install:

```bash
brew tap iw2rmb/ploy
brew install ploy
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

## Formula

The formula is automatically updated by GoReleaser when new releases are published.

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
brews:
  - name: ploy
    repository:
      owner: iw2rmb
      name: homebrew-ploy
      token: "{{ .Env.HOMEBREW_TAP_GITHUB_TOKEN }}"
    homepage: https://github.com/iw2rmb/ploy
    description: "Ploy - Infrastructure orchestration and deployment platform"
    license: "Apache-2.0"
    folder: Formula
    ids:
      - ploy
    install: |
      bin.install "ploy"
    test: |
      system "#{bin}/ploy", "version"
```

## Testing the Tap

### After First Release

Once you create the first release with a version tag (e.g., `v0.1.0`):

1. **Verify the formula was created**:
   - Check: https://github.com/iw2rmb/homebrew-ploy/blob/main/Formula/ploy.rb
   - Should be auto-created by GoReleaser

2. **Test installation locally**:
   ```bash
   # Add the tap
   brew tap iw2rmb/ploy

   # Install ploy
   brew install ploy

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

### Manual Formula Creation (If Needed)

If GoReleaser doesn't create the formula automatically, you can create it manually:

```bash
cd homebrew-ploy

# Create Formula/ploy.rb
cat > Formula/ploy.rb <<'EOF'
class Ploy < Formula
  desc "Infrastructure orchestration and deployment platform"
  homepage "https://github.com/iw2rmb/ploy"
  version "0.1.0"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/iw2rmb/ploy/releases/download/v0.1.0/ploy_0.1.0_darwin_arm64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256"
    else
      url "https://github.com/iw2rmb/ploy/releases/download/v0.1.0/ploy_0.1.0_darwin_amd64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/iw2rmb/ploy/releases/download/v0.1.0/ploy_0.1.0_linux_arm64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256"
    else
      url "https://github.com/iw2rmb/ploy/releases/download/v0.1.0/ploy_0.1.0_linux_amd64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256"
    end
  end

  def install
    bin.install "ploy"
  end

  test do
    system "#{bin}/ploy", "version"
  end
end
EOF

# Calculate SHA256 checksums from the release
# Download checksums.txt from the release and extract values

# Commit and push
git add Formula/ploy.rb
git commit -m "Add ploy v0.1.0"
git push
```

## Troubleshooting

### Formula Not Auto-Updated

**Possible causes**:
1. `HOMEBREW_TAP_GITHUB_TOKEN` not set or expired
2. Token doesn't have `repo` scope
3. Repository name mismatch in `.goreleaser.yml`
4. Network/API issues during release

**Solution**:
- Check GitHub Actions logs for the release workflow
- Verify token exists: https://github.com/iw2rmb/ploy/settings/secrets/actions
- Manually update the formula as a workaround (see above)

### Installation Fails

**Check formula syntax**:
```bash
brew install --debug --verbose iw2rmb/ploy/ploy
```

**Audit the formula**:
```bash
brew audit --strict --online iw2rmb/ploy/ploy
```

### SHA256 Mismatch

If Homebrew reports a SHA256 mismatch:
1. The binary was modified after the formula was created
2. The checksum in the formula is incorrect

**Fix**:
- Re-download the binary and recalculate SHA256:
  ```bash
  curl -LO https://github.com/iw2rmb/ploy/releases/download/vX.Y.Z/ploy_X.Y.Z_darwin_amd64.tar.gz
  shasum -a 256 ploy_X.Y.Z_darwin_amd64.tar.gz
  ```
- Update the formula with the correct checksum

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

### Formula Updates

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

- [Homebrew Formula Cookbook](https://docs.brew.sh/Formula-Cookbook)
- [Homebrew Taps](https://docs.brew.sh/Taps)
- [GoReleaser Homebrew Integration](https://goreleaser.com/customization/homebrew/)

---

**Related Documentation**:
- [Releasing Guide](./releasing.md)
- [Testing workflow](./testing-workflow.md)
