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

  if [[ -f "${artifact_dir}/heal.json" ]]; then
    echo "  + heal.json present"
  else
    echo "  - heal.json missing"
    [[ "$mode" == "strict" ]] && failed=1
  fi

  if [[ "$mode" == "strict" ]] && ((failed > 0)); then
    return 1
  fi
}
