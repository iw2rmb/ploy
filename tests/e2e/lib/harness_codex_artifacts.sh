#!/usr/bin/env bash
set -euo pipefail

e2e_extract_mig_out_bundles() {
  local artifact_dir="${1:?artifact_dir is required}"
  local extract_failed=0
  local bundle=""
  local -a bundles=()

  shopt -s nullglob
  bundles=("${artifact_dir}"/*_mig-out.bin)
  shopt -u nullglob

  if ((${#bundles[@]} == 0)); then
    echo "  - no mig-out bundles found in ${artifact_dir}"
    return 0
  fi

  for bundle in "${bundles[@]}"; do
    echo "  extracting $(basename "$bundle")"
    if ! tar -xzf "$bundle" -C "$artifact_dir"; then
      echo "  ! failed to extract ${bundle}"
      extract_failed=1
    fi
  done

  if ((extract_failed > 0)); then
    return 1
  fi
}

e2e_validate_codex_handshake() {
  local artifact_dir="${1:?artifact_dir is required}"
  local mode="${2:-advisory}"
  local failed=0
  local session_id=""
  local manifest_has_session=0
  local manifest_has_resumed=0

  if [[ -f "${artifact_dir}/codex.log" ]]; then
    echo "  + codex.log present"
  else
    echo "  - codex.log missing"
    [[ "$mode" == "strict" ]] && failed=1
  fi

  if [[ -f "${artifact_dir}/heal.json" ]]; then
    echo "  + heal.json present"
  fi

  if [[ -f "${artifact_dir}/codex-session.txt" ]]; then
    session_id="$(tr -d '\r\n' < "${artifact_dir}/codex-session.txt")"
    if [[ -n "$session_id" ]]; then
      echo "  + codex-session.txt present (${session_id:0:20}...)"
    else
      echo "  - codex-session.txt is empty"
      [[ "$mode" == "strict" ]] && failed=1
    fi
  else
    echo "  - codex-session.txt missing"
    [[ "$mode" == "strict" ]] && failed=1
  fi

  if [[ -f "${artifact_dir}/codex-run.json" ]]; then
    if grep -q '"session_id"' "${artifact_dir}/codex-run.json"; then
      manifest_has_session=1
      echo "  + codex-run.json contains session_id"
    else
      echo "  - codex-run.json missing session_id"
      [[ "$mode" == "strict" ]] && failed=1
    fi

    if grep -q '"resumed"' "${artifact_dir}/codex-run.json"; then
      manifest_has_resumed=1
      echo "  + codex-run.json contains resumed"
    else
      echo "  - codex-run.json missing resumed"
      [[ "$mode" == "strict" ]] && failed=1
    fi
  else
    echo "  - codex-run.json missing"
    [[ "$mode" == "strict" ]] && failed=1
  fi

  if [[ "$mode" == "strict" ]] && ((failed > 0)); then
    return 1
  fi

  if [[ "$mode" != "strict" ]] && ((manifest_has_session == 0 || manifest_has_resumed == 0)); then
    echo "  - advisory: handshake artifacts are incomplete"
  fi
}
