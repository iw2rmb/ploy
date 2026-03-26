#!/usr/bin/env bash
set -euo pipefail

# E2E: blocked archive entries (traversal path + symlink).
#
# Validates that spec bundles containing dangerous entries are rejected by the
# node agent with deterministic error messages, and runs end with status "Fail"
# rather than "Success".
#
# Part 1 — traversal:
#   1. Craft a tar.gz archive containing a traversal entry ("../evil.txt").
#   2. Upload the archive directly to POST /v1/spec-bundles with bearer auth.
#   3. Build a MigRunSpec that references the uploaded bundle via tmp_bundle:.
#   4. Submit the run and follow to completion.
#   5. Assert the final repo status is "Fail" and message contains "path traversal in entry".
#
# Part 2 — symlink:
#   6. Craft a tar.gz archive containing a TypeSymlink entry ("evil.sh" -> /etc/passwd).
#   7. Upload the symlink archive.
#   8. Build a MigRunSpec referencing it.
#   9. Submit the run and follow to completion.
#  10. Assert the final repo status is "Fail" and message contains "not permitted in spec bundle".

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=tests/e2e/lib/harness.sh
source "${SCRIPT_DIR}/../../lib/harness.sh"

e2e_init "${BASH_SOURCE[0]}"
e2e_artifacts_init "$REPO_ROOT/tmp/migs/scenario-tmpdir-blocked"

REPO="${PLOY_E2E_REPO_OVERRIDE:-https://github.com/octocat/Hello-World.git}"
BASE_REF="${PLOY_E2E_BASE_REF:-master}"
TARGET_REF="${PLOY_E2E_TARGET_REF:-e2e/tmpdir-blocked}"

SERVER_URL="$(e2e_descriptor_address)"
TOKEN="$(e2e_descriptor_token)"

echo "=========================================="
echo "TmpDir Blocked Entries E2E Scenario"
echo "=========================================="
echo "Repo:       $REPO"
echo "Base ref:   $BASE_REF"
echo "Target ref: $TARGET_REF"
echo "Server:     $SERVER_URL"
echo "Artifacts:  $E2E_ARTIFACT_DIR"
echo "=========================================="

# Step 1: craft a traversal archive via Python.
ARCHIVE="$(mktemp "${TMPDIR:-/tmp}/ploy-e2e-blocked.XXXXXX.tar.gz")"
trap 'rm -f "$ARCHIVE"' EXIT

python3 - "$ARCHIVE" <<'PYEOF'
import sys, tarfile, io, gzip

archive_path = sys.argv[1]
buf = io.BytesIO()
with tarfile.open(fileobj=buf, mode="w:gz") as tf:
    content = b"this should not escape\n"
    info = tarfile.TarInfo(name="../evil.txt")
    info.size = len(content)
    tf.addfile(info, io.BytesIO(content))

with open(archive_path, "wb") as f:
    f.write(buf.getvalue())

print(f"crafted traversal archive: {archive_path}", file=sys.stderr)
PYEOF

echo "Crafted traversal archive at: $ARCHIVE"

# Step 2: upload the archive to the spec-bundle API.
UPLOAD_RESPONSE="$(curl -fsS \
  -X POST \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/octet-stream" \
  --data-binary "@${ARCHIVE}" \
  "${SERVER_URL}/v1/spec-bundles")"

echo "Upload response: $UPLOAD_RESPONSE"

BUNDLE_ID="$(printf '%s' "$UPLOAD_RESPONSE" | jq -r '.bundle_id')"
CID="$(printf '%s' "$UPLOAD_RESPONSE" | jq -r '.cid')"
DIGEST="$(printf '%s' "$UPLOAD_RESPONSE" | jq -r '.digest')"

if [[ -z "$BUNDLE_ID" || "$BUNDLE_ID" == "null" ]]; then
  echo "error: failed to extract bundle_id from upload response" >&2
  exit 1
fi

echo "Bundle uploaded: id=${BUNDLE_ID} cid=${CID}"

# Step 3: build a spec referencing the uploaded bundle via tmp_bundle:.
SPEC_FILE="$(mktemp "${TMPDIR:-/tmp}/ploy-e2e-blocked-spec.XXXXXX.yaml")"
trap 'rm -f "$ARCHIVE" "$SPEC_FILE"' EXIT

cat >"$SPEC_FILE" <<YAML
apiVersion: ploy.mig/v1alpha1
kind: MigRunSpec
steps:
  - image: alpine:3.20
    command: echo "should not reach here"
    tmp_bundle:
      bundle_id: ${BUNDLE_ID}
      cid: ${CID}
      digest: ${DIGEST}
      entries:
        - evil.txt
YAML

# Step 4: submit the run; expect non-zero exit (run fails on the node).
RUN_JSON=""
set +e
RUN_JSON="$(e2e_mig_run_json \
  --repo-url "$REPO" \
  --repo-base-ref "$BASE_REF" \
  --repo-target-ref "$TARGET_REF" \
  --spec "$SPEC_FILE" \
  --follow 2>&1)"
RUN_EXIT=$?
set -e

printf '%s\n' "$RUN_JSON" >"${E2E_ARTIFACT_DIR}/run-blocked.json"

# Step 5: assert the run failed (status "Fail", not "Success").
FAILED=0

# Assert the output contains the expected traversal-rejection error message.
EXPECTED_MSG="path traversal in entry"
if printf '%s' "$RUN_JSON" | grep -qF "$EXPECTED_MSG"; then
  echo "  + failure message: found '${EXPECTED_MSG}' (expected)"
else
  echo "  ! failure message: expected substring '${EXPECTED_MSG}' not found in output" >&2
  FAILED=1
fi

REPO_STATUS="$(printf '%s' "$RUN_JSON" | jq -r '.repos[0].status // empty' 2>/dev/null || echo "")"
if [[ "$REPO_STATUS" == "Fail" ]]; then
  echo "  + repo status: Fail (expected)"
elif [[ -n "$REPO_STATUS" && "$REPO_STATUS" != "Success" ]]; then
  echo "  + repo status: ${REPO_STATUS} (non-Success, accepted)"
else
  echo "  ! repo status: expected Fail, got '${REPO_STATUS}'" >&2
  FAILED=1
fi

# Verify the exit code from ploy was non-zero (failure).
if [[ "$RUN_EXIT" -ne 0 ]]; then
  echo "  + ploy exit code: ${RUN_EXIT} (non-zero, expected)"
else
  echo "  ! ploy exit code: 0 (expected non-zero for failed run)" >&2
  FAILED=1
fi

if ((FAILED > 0)); then
  echo "FAIL: scenario-tmpdir-blocked (traversal) — expected run to fail but it did not."
  echo "Artifacts saved to: ${E2E_ARTIFACT_DIR}"
  exit 1
fi

echo ""
echo "OK: scenario-tmpdir-blocked (traversal archive rejected — run ended with Fail)."

# ---------------------------------------------------------------------------
# Part 2: symlink-entry blocking.
#
# Validates that a spec bundle containing a symlink entry is rejected by the
# node agent with a deterministic error message, and the run ends with status
# "Fail" rather than "Success".
# ---------------------------------------------------------------------------

echo ""
echo "=========================================="
echo "TmpDir Blocked Entries — Symlink Check"
echo "=========================================="

# Step 6: craft a symlink archive via Python.
ARCHIVE_SYM="$(mktemp "${TMPDIR:-/tmp}/ploy-e2e-blocked-sym.XXXXXX.tar.gz")"
trap 'rm -f "$ARCHIVE" "$SPEC_FILE" "$ARCHIVE_SYM"' EXIT

python3 - "$ARCHIVE_SYM" <<'PYEOF'
import sys, tarfile, io

archive_path = sys.argv[1]
buf = io.BytesIO()
with tarfile.open(fileobj=buf, mode="w:gz") as tf:
    info = tarfile.TarInfo(name="evil.sh")
    info.type = tarfile.SYMTYPE   # TypeSymlink — rejected by node agent
    info.linkname = "/etc/passwd"
    info.size = 0
    tf.addfile(info, io.BytesIO(b""))

with open(archive_path, "wb") as f:
    f.write(buf.getvalue())

print(f"crafted symlink archive: {archive_path}", file=sys.stderr)
PYEOF

echo "Crafted symlink archive at: $ARCHIVE_SYM"

# Step 7: upload the symlink archive.
UPLOAD_RESPONSE_SYM="$(curl -fsS \
  -X POST \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/octet-stream" \
  --data-binary "@${ARCHIVE_SYM}" \
  "${SERVER_URL}/v1/spec-bundles")"

echo "Upload response (symlink): $UPLOAD_RESPONSE_SYM"

BUNDLE_ID_SYM="$(printf '%s' "$UPLOAD_RESPONSE_SYM" | jq -r '.bundle_id')"
CID_SYM="$(printf '%s' "$UPLOAD_RESPONSE_SYM" | jq -r '.cid')"
DIGEST_SYM="$(printf '%s' "$UPLOAD_RESPONSE_SYM" | jq -r '.digest')"

if [[ -z "$BUNDLE_ID_SYM" || "$BUNDLE_ID_SYM" == "null" ]]; then
  echo "error: failed to extract bundle_id from symlink upload response" >&2
  exit 1
fi

echo "Symlink bundle uploaded: id=${BUNDLE_ID_SYM} cid=${CID_SYM}"

# Step 8: build a spec referencing the symlink bundle.
SPEC_FILE_SYM="$(mktemp "${TMPDIR:-/tmp}/ploy-e2e-blocked-sym-spec.XXXXXX.yaml")"
trap 'rm -f "$ARCHIVE" "$SPEC_FILE" "$ARCHIVE_SYM" "$SPEC_FILE_SYM"' EXIT

cat >"$SPEC_FILE_SYM" <<YAML
apiVersion: ploy.mig/v1alpha1
kind: MigRunSpec
steps:
  - image: alpine:3.20
    command: echo "should not reach here"
    tmp_bundle:
      bundle_id: ${BUNDLE_ID_SYM}
      cid: ${CID_SYM}
      digest: ${DIGEST_SYM}
      entries:
        - evil.sh
YAML

# Step 9: submit the run; expect non-zero exit (run fails on the node).
RUN_JSON_SYM=""
set +e
RUN_JSON_SYM="$(e2e_mig_run_json \
  --repo-url "$REPO" \
  --repo-base-ref "$BASE_REF" \
  --repo-target-ref "$TARGET_REF" \
  --spec "$SPEC_FILE_SYM" \
  --follow 2>&1)"
RUN_EXIT_SYM=$?
set -e

printf '%s\n' "$RUN_JSON_SYM" >"${E2E_ARTIFACT_DIR}/run-blocked-symlink.json"

# Step 10: assert the symlink run failed with a deterministic message.
FAILED_SYM=0

EXPECTED_MSG_SYM="not permitted in spec bundle"
if printf '%s' "$RUN_JSON_SYM" | grep -qF "$EXPECTED_MSG_SYM"; then
  echo "  + failure message: found '${EXPECTED_MSG_SYM}' (expected)"
else
  echo "  ! failure message: expected substring '${EXPECTED_MSG_SYM}' not found in output" >&2
  FAILED_SYM=1
fi

REPO_STATUS_SYM="$(printf '%s' "$RUN_JSON_SYM" | jq -r '.repos[0].status // empty' 2>/dev/null || echo "")"
if [[ "$REPO_STATUS_SYM" == "Fail" ]]; then
  echo "  + repo status: Fail (expected)"
elif [[ -n "$REPO_STATUS_SYM" && "$REPO_STATUS_SYM" != "Success" ]]; then
  echo "  + repo status: ${REPO_STATUS_SYM} (non-Success, accepted)"
else
  echo "  ! repo status: expected Fail, got '${REPO_STATUS_SYM}'" >&2
  FAILED_SYM=1
fi

if [[ "$RUN_EXIT_SYM" -ne 0 ]]; then
  echo "  + ploy exit code: ${RUN_EXIT_SYM} (non-zero, expected)"
else
  echo "  ! ploy exit code: 0 (expected non-zero for failed run)" >&2
  FAILED_SYM=1
fi

if ((FAILED_SYM > 0)); then
  echo "FAIL: scenario-tmpdir-blocked (symlink) — expected run to fail but it did not."
  echo "Artifacts saved to: ${E2E_ARTIFACT_DIR}"
  exit 1
fi

echo ""
echo "OK: scenario-tmpdir-blocked (symlink archive rejected — run ended with Fail)."
echo "Artifacts saved to: ${E2E_ARTIFACT_DIR}"
