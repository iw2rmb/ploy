# shellcheck shell=bash

install_go() {
  local current archive url
  if command -v go >/dev/null 2>&1; then
    current="$(go version | awk '{print $3}' | tr -d 'go')"
    if [[ "$current" == "$GO_VERSION" ]] || [[ "$current" == 1.25.* ]]; then
      log "Go already at $current; skipping"
      return
    fi
    warn "Go version $current detected, upgrading to $GO_VERSION"
  else
    log "Go not present; installing $GO_VERSION"
  fi
  archive="go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
  url="https://go.dev/dl/${archive}"
  with_tmpdir install_go_from_archive "$archive" "$url"
}

install_go_from_archive() {
  local tmpdir="$1" archive="$2" url="$3"
  curl -fsSL "$url" -o "${tmpdir}/${archive}"
  rm -rf /usr/local/go
  tar -xzf "${tmpdir}/${archive}" -C /usr/local
  ln -sf /usr/local/go/bin/go "${BIN_DIR}/go"
  log "installed Go ${GO_VERSION}"
}
