# Build Gate Logs

Build Gate preserves captured gate logs as execution metadata. It does not run
Maven, Gradle, or other tool-specific log trimmers.

On failed gate execution, `BuildGateStageMetadata.LogFindings[0]` contains:

- `severity: error`
- `message`: the raw canonical container logs, capped at 10 MiB
- no structured `evidence`

The same capped log text is stored in `BuildGateStageMetadata.LogsText`, and
`LogDigest` is computed from that capped text.

Successful Gradle gates may still add an informational `GRADLE_BUILD_CACHE_HIT`
finding when the gate image reports cache-hit tasks. That finding is not a log
trimmer result.

Reusable Java Gradle log trimming is exposed separately through the stateless
`POST /v1/trimmer/java/gradle` control-plane utility endpoint.
