#!/usr/bin/env bash
# Create GitHub repos for lanes A–G using dev PAT/username.
# Repos are named ploy-lane-<lane-letter>-<language>.

set -euo pipefail

USERNAME=${GITHUB_PLOY_DEV_USERNAME:-}
TOKEN=${GITHUB_PLOY_DEV_PAT:-}

if [[ -z "$USERNAME" || -z "$TOKEN" ]]; then
  echo "GITHUB_PLOY_DEV_USERNAME and GITHUB_PLOY_DEV_PAT are required" >&2
  exit 1
fi

LANES=(A B C D E F G)
NAMES=(
  ploy-lane-a-go
  ploy-lane-b-node
  ploy-lane-c-java
  ploy-lane-d-python
  ploy-lane-e-go
  ploy-lane-f-dotnet
  ploy-lane-g-rust
)
DESCS=(
  'Lane A (Unikraft Minimal) hello app in Go'
  'Lane B (Unikraft POSIX) hello app in Node.js'
  'Lane C (OSv Java/Scala) hello app in Java'
  'Lane D (FreeBSD Jails) hello app in Python'
  'Lane E (OCI+Kontain) hello app in Go'
  'Lane F (Full VM) hello app in .NET'
  'Lane G (WASM Runtime) hello app in Rust (wasm32-wasi)'
)

api() {
  local method=$1; shift
  local path=$1; shift
  curl -fsS -H "Authorization: token ${TOKEN}" -H 'Accept: application/vnd.github+json' \
    -X "$method" "https://api.github.com${path}" "$@"
}

for i in "${!LANES[@]}"; do
  lane=${LANES[$i]}
  name=${NAMES[$i]}
  desc=${DESCS[$i]}
  repo_path="/repos/${USERNAME}/${name}"

  echo "==> Lane $lane: ${USERNAME}/${name}"
  if api GET "$repo_path" >/dev/null 2>&1; then
    echo "    Repo exists; skipping create"
    continue
  fi

  payload=$(jq -n --arg name "$name" --arg desc "$desc" '{name: $name, description: $desc, private: false, auto_init: true}')
  api POST "/user/repos" -d "$payload" >/dev/null
  echo "    Created"
done

echo "All done."
