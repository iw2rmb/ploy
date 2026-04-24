#!/usr/bin/env bash

json_escape() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' -e ':a;N;$!ba;s/\n/\\n/g'
}

write_success_report() {
  local message="$1"
  local msg_esc
  msg_esc=$(json_escape "$message")
  cat >"$outdir/report.json" <<JSON
{"success":true,"message":"${msg_esc}"}
JSON
}

write_failure_report() {
  local error_kind="$1"
  local reason="$2"
  local message="$3"
  local message_esc
  local reason_esc
  message_esc=$(json_escape "$message")
  reason_esc=$(json_escape "$reason")
  if [[ -n "$reason" ]]; then
    cat >"$outdir/report.json" <<JSON
{"success":false,"error_kind":"${error_kind}","reason":"${reason_esc}","message":"${message_esc}"}
JSON
  else
    cat >"$outdir/report.json" <<JSON
{"success":false,"error_kind":"${error_kind}","message":"${message_esc}"}
JSON
  fi
}

parse_bool_default_true() {
  local raw="${1:-}"
  local norm
  norm="$(echo "$raw" | tr '[:upper:]' '[:lower:]' | xargs)"
  case "$norm" in
    ""|1|true|yes|on)
      return 0
      ;;
    0|false|no|off)
      return 1
      ;;
    *)
      echo "invalid boolean value: ${raw}" >&2
      return 2
      ;;
  esac
}

parse_bool_default_false() {
  local raw="${1:-}"
  local norm
  norm="$(echo "$raw" | tr '[:upper:]' '[:lower:]' | xargs)"
  case "$norm" in
    ""|0|false|no|off)
      return 1
      ;;
    1|true|yes|on)
      return 0
      ;;
    *)
      echo "invalid boolean value: ${raw}" >&2
      return 2
      ;;
  esac
}

array_contains() {
  local needle="$1"
  shift
  local item
  for item in "$@"; do
    if [[ "$item" == "$needle" ]]; then
      return 0
    fi
  done
  return 1
}

extract_groovy_parse_failure_paths() {
  local log_path="$1"
  sed -nE \
    -e 's/.*Failed to parse ([^,]+), cursor position likely inaccurate.*/\1/p' \
    -e 's/.*Failed to parse ([^ ]+) at cursor position.*/\1/p' \
    "$log_path" | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//' | awk 'NF > 0'
}

strip_proto_comments() {
  local file_path="$1"
  awk '
    BEGIN { in_block = 0 }
    {
      line = $0
      out = ""
      i = 1
      while (i <= length(line)) {
        two = substr(line, i, 2)
        if (in_block) {
          if (two == "*/") {
            in_block = 0
            i += 2
          } else {
            i++
          }
        } else if (two == "/*") {
          in_block = 1
          i += 2
        } else if (two == "//") {
          break
        } else {
          out = out substr(line, i, 1)
          i++
        }
      }
      print out
    }
  ' "$file_path"
}

is_proto3_or_edition_proto() {
  local file_path="$1"
  local sanitized
  sanitized="$(strip_proto_comments "$file_path")"
  if grep -Eiq '^[[:space:]]*edition[[:space:]]*=[[:space:]]*"[^"]+"[[:space:]]*;' <<<"$sanitized"; then
    return 0
  fi
  if grep -Eiq '^[[:space:]]*syntax[[:space:]]*=[[:space:]]*"proto3"[[:space:]]*;' <<<"$sanitized"; then
    return 0
  fi
  return 1
}

to_exclude_pattern() {
  local raw_path="$1"
  local normalized
  normalized="${raw_path//$'\r'/}"
  normalized="${normalized//\\//}"
  normalized="${normalized#\`}"
  normalized="${normalized%\`}"
  normalized="${normalized#./}"
  normalized="${normalized#workspace/}"
  normalized="${normalized#/workspace/}"
  if [[ -z "$normalized" ]]; then
    return 1
  fi
  if [[ "$normalized" == "$workspace/"* ]]; then
    normalized="${normalized#"$workspace/"}"
  elif [[ "$normalized" == /* || "$normalized" =~ ^[A-Za-z]:/ ]]; then
    normalized="$(basename "$normalized")"
  fi
  if [[ -z "$normalized" ]]; then
    return 1
  fi
  if [[ "$normalized" == */* ]]; then
    printf '%s\n' "$normalized"
  else
    printf '**/%s\n' "$normalized"
  fi
}

collect_preflight_proto_exclude_patterns() {
  local workspace_dir="$1"
  local -a patterns=()
  local proto_path
  local pattern
  while IFS= read -r -d '' proto_path; do
    if ! is_proto3_or_edition_proto "$proto_path"; then
      continue
    fi
    pattern="$(to_exclude_pattern "$proto_path" || true)"
    if [[ -z "$pattern" ]]; then
      continue
    fi
    if (( ${#patterns[@]} == 0 )) || ! array_contains "$pattern" "${patterns[@]}"; then
      patterns+=("$pattern")
    fi
  done < <(find "$workspace_dir" -type f -name '*.proto' -print0 2>/dev/null)
  if (( ${#patterns[@]} > 0 )); then
    printf '%s\n' "${patterns[@]}"
  fi
}

build_groovy_parse_exclude_patterns() {
  local log_path="$1"
  local -a patterns=()
  local failed_path
  local pattern
  while IFS= read -r failed_path; do
    pattern="$(to_exclude_pattern "$failed_path" || true)"
    if [[ -z "$pattern" ]]; then
      continue
    fi
    if (( ${#patterns[@]} == 0 )) || ! array_contains "$pattern" "${patterns[@]}"; then
      patterns+=("$pattern")
    fi
  done < <(extract_groovy_parse_failure_paths "$log_path")
  if (( ${#patterns[@]} > 0 )); then
    printf '%s\n' "${patterns[@]}"
  fi
}

new_patterns_from_candidates() {
  local existing_csv="$1"
  local candidate_patterns="$2"
  local -a existing=()
  local -a new_patterns=()
  local existing_item
  local candidate_item

  IFS=',' read -r -a existing <<<"$existing_csv"
  for existing_item in "${existing[@]-}"; do
    existing_item="$(echo "$existing_item" | xargs)"
    if [[ -z "$existing_item" ]]; then
      continue
    fi
    if (( ${#new_patterns[@]} == 0 )) || ! array_contains "$existing_item" "${new_patterns[@]-}"; then
      new_patterns+=("$existing_item")
    fi
  done

  local -a added=()
  while IFS= read -r candidate_item; do
    candidate_item="$(echo "$candidate_item" | xargs)"
    if [[ -z "$candidate_item" ]]; then
      continue
    fi
    if (( ${#new_patterns[@]} > 0 )) && array_contains "$candidate_item" "${new_patterns[@]-}"; then
      continue
    fi
    new_patterns+=("$candidate_item")
    added+=("$candidate_item")
  done <<<"$candidate_patterns"

  if (( ${#added[@]} > 0 )); then
    printf '%s\n' "${added[@]}"
  fi
}

merge_exclude_patterns() {
  local existing_csv="$1"
  local added_patterns="$2"
  local -a merged=()
  local existing_item
  local added_item

  IFS=',' read -r -a existing <<<"$existing_csv"
  for existing_item in "${existing[@]-}"; do
    existing_item="$(echo "$existing_item" | xargs)"
    if [[ -z "$existing_item" ]]; then
      continue
    fi
    if (( ${#merged[@]} == 0 )) || ! array_contains "$existing_item" "${merged[@]-}"; then
      merged+=("$existing_item")
    fi
  done

  while IFS= read -r added_item; do
    added_item="$(echo "$added_item" | xargs)"
    if [[ -z "$added_item" ]]; then
      continue
    fi
    if (( ${#merged[@]} == 0 )) || ! array_contains "$added_item" "${merged[@]-}"; then
      merged+=("$added_item")
    fi
  done <<<"$added_patterns"

  local csv=""
  local item
  for item in "${merged[@]-}"; do
    if [[ -z "$csv" ]]; then
      csv="$item"
    else
      csv="${csv},${item}"
    fi
  done
  printf '%s' "$csv"
}

lines_to_csv() {
  local lines="$1"
  local csv=""
  local line
  while IFS= read -r line; do
    line="$(echo "$line" | xargs)"
    if [[ -z "$line" ]]; then
      continue
    fi
    if [[ -z "$csv" ]]; then
      csv="$line"
    else
      csv="${csv},${line}"
    fi
  done <<<"$lines"
  printf '%s' "$csv"
}

has_groovy_parse_failure() {
  local log_path="$1"
  grep -Eq 'GroovyParsingException: Failed to parse ' "$log_path"
}

run_rewrite_cli() {
  local run_status=0
  "$cli_bin" "${args[@]}" 2>&1 | tee -a "$transform_log" || run_status=$?
  return "$run_status"
}
