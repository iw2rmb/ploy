# Homebrew Tap Setup

## Key Takeaways

- Installation target:
  - `brew install --cask iw2rmb/ploy/ploy`
- Tap repository:
  - `iw2rmb/homebrew-ploy` (cask path: `Casks/ploy.rb`)
- Automation:
  - GoReleaser updates the Homebrew cask during releases.
  - Required secret in `iw2rmb/ploy`: `HOMEBREW_TAP_GITHUB_TOKEN` (token must permit tap repo updates).
- Source of truth for config:
  - `.goreleaser.yml` (`homebrew_casks` section).
- Verification checks after a release:
  - Confirm `Casks/ploy.rb` was updated in `iw2rmb/homebrew-ploy`.
  - Run `brew install --cask iw2rmb/ploy/ploy` and `ploy version`.
  - Optionally run `brew audit --cask --strict --online iw2rmb/ploy/ploy`.
- Common failure modes:
  - Missing/expired `HOMEBREW_TAP_GITHUB_TOKEN`
  - Incorrect tap repo settings in `.goreleaser.yml`
  - Check release workflow logs in GitHub Actions.

## References

- [Homebrew Cask Cookbook](https://docs.brew.sh/Cask-Cookbook)
- [Homebrew Taps](https://docs.brew.sh/Taps)
- [GoReleaser Homebrew Integration](https://goreleaser.com/customization/homebrew/)
