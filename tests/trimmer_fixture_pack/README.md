# Gradle Trimmer Fixture Pack

Source: live `ploy run ls` results from the current `round-leaf-6114` control plane, fetched with `ploy mig fetch`.

Scope:
- Included only Amata healing-cycle provider outputs under `out/amata/runs/**/artifacts/**/stdout.txt`.
- Included failed Gradle command executions for build, assemble, check, test, compileJava, compileKotlin, and kaptKotlin tasks.
- Excluded pre-gate `in/build-gate.log` files.
- Kept one failed Gradle log per repository.

Result:
- Requested target: 15 distinct repositories.
- Found in live run corpus: 5 distinct repositories.
- `manifest.json` records the exact run, repo, source artifact path, command, raw fixture, and current trimmer output for each fixture.

Public supplement:
- `public/` contains extra public GitHub Actions fixtures gathered with `gh run view --log`.
- Public fixture logs are normalized by stripping GitHub job/step/timestamp prefixes before trimmer application.
- `public/original/*.github.log` preserves the original public GitHub log output.
- `public/manifest.json` records the GitHub repository and run URL for attribution.

Files:
- `logs/*.log` raw Gradle failure output extracted from Amata command execution.
- `trimmed/*.trimmed.log` current `internal/trimmer/java/gradle.Trim` message output when present. Complete compiler evidence omits the root message, so those logs are empty.
- `trimmed/*.trimmed.json` current full trimmer result, including evidence when present.

Regression test:
- Run `go test ./tests/trimmer_fixture_pack`.
- Refresh expected outputs with `PLOY_UPDATE_TRIMMER_FIXTURES=1 go test ./tests/trimmer_fixture_pack`.
