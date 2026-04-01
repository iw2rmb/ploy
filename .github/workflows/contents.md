[build.yml](build.yml) Runs the main-branch build pipeline to compile binaries, generate an SBOM, scan vulnerabilities, and sign artifacts.
[ci.yml](ci.yml) Runs push/PR CI (main/develop pushes, main PRs) with pre-commit, lint, vet, staticcheck, unit tests, build, and supply-chain checks.
[coverage.yml](coverage.yml) Enforces component and overall coverage thresholds, comments coverage on PRs, and updates coverage badges on main.
[https-smoke.yml](https-smoke.yml) Provides a manual workflow to run HTTPS artifact upload/download smoke tests against a specified API host.
[release.yml](release.yml) Publishes semver-tagged releases via GoReleaser, signs runtime binaries, and pushes version-matched core GHCR images after pre-release checks.
[test.yml](test.yml) Runs an extended test suite with unit tests, build verification, quality checks, security scanning, and PR benchmark comparisons.
