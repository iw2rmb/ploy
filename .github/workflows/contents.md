[build.yml](build.yml) Runs a main-branch build pipeline that compiles binaries, generates an SBOM, scans vulnerabilities, and signs artifacts.
[ci.yml](ci.yml) Executes the primary CI pipeline for pushes and PRs with pre-commit, linting, vet, tests, build, and supply-chain checks.
[coverage.yml](coverage.yml) Enforces component and overall coverage thresholds, comments coverage on PRs, and updates coverage badges on main.
[https-smoke.yml](https-smoke.yml) Provides a manual workflow to run HTTPS artifact upload/download smoke tests against a specified API host.
[release.yml](release.yml) Publishes semver-tagged releases via GoReleaser, signs runtime binaries, and pushes version-matched core GHCR images after pre-release checks.
[test.yml](test.yml) Runs an extended test suite with unit tests, build verification, quality checks, security scanning, and PR benchmark comparisons.
