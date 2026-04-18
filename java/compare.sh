#!/bin/bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "Usage: $0 <old-deps.json> <new-deps.json>" >&2
  exit 1
fi

OLD="$1"
NEW="$2"

jq -n \
  --slurpfile old "$OLD" \
  --slurpfile new "$NEW" '
  def deps_array:
    if type == "array" then .
    elif (type == "object" and (.dependencies | type) == "array") then .dependencies
    else error("unsupported dependency file format")
    end;

  def to_map:
    reduce .[] as $d (
      {};
      if ($d.groupId and $d.artifactId and $d.version) then
        .[$d.groupId + ":" + $d.artifactId] = $d.version
      else
        .
      end
    );

  ($old[0] | deps_array | to_map) as $o
  | ($new[0] | deps_array | to_map) as $n
  | ((($o | keys_unsorted) + ($n | keys_unsorted)) | unique | sort) as $all
  |
  {
    changed: [
      $all[] as $k
      | select($o[$k] and $n[$k] and $o[$k] != $n[$k])
      | {ga: $k, from: $o[$k], to: $n[$k]}
    ],
    added: [
      $all[] as $k
      | select(($o[$k] | not) and $n[$k])
      | {ga: $k, version: $n[$k]}
    ],
    removed: [
      $all[] as $k
      | select($o[$k] and ($n[$k] | not))
      | {ga: $k, version: $o[$k]}
    ],
    summary: {
      old_count: ($o | length),
      new_count: ($n | length),
      changed_count: (
        [$all[] | select($o[.] and $n[.] and $o[.] != $n[.])] | length
      ),
      added_count: ([$all[] | select(($o[.] | not) and $n[.])] | length),
      removed_count: ([$all[] | select($o[.] and ($n[.] | not))] | length)
    }
  }
  | .changed = (.changed | sort_by(.ga))
  | .added = (.added | sort_by(.ga))
  | .removed = (.removed | sort_by(.ga))
'
