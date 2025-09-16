job "{{APP_NAME}}-g-build-{{VERSION}}" {
  datacenters = ["dc1"]
  type = "batch"

  group "build" {
    count = 1

    restart {
      attempts = 0
      interval = "1m"
      delay    = "10s"
      mode     = "fail"
    }

    network { port "http" {} }

    task "build-wasm" {
      driver = "docker"

      env {
        CONTEXT_URL     = "{{CONTEXT_URL}}"
        WASM_UPLOAD_URL = "{{WASM_UPLOAD_URL}}"
      }

      config {
        image = "rust:1.79-slim"
        network_mode = "host"
        entrypoint = ["/bin/sh", "-lc"]
        args = [
          "set -eu; apt-get update -y >/dev/null 2>&1 && apt-get install -y curl ca-certificates pkg-config build-essential >/dev/null 2>&1; mkdir -p /workspace; for i in 1 2 3; do curl -fsSL $CONTEXT_URL -o /workspace/src.tar && break; echo retry context; sleep 2; done; tar -xf /workspace/src.tar -C /workspace; cd /workspace; rustup target add wasm32-wasi >/dev/null 2>&1 || true; if [ -f Cargo.toml ]; then cargo build --release --target wasm32-wasi; wasm=$(ls target/wasm32-wasi/release/*.wasm 2>/dev/null | head -n1); test -n \"$wasm\"; curl -fsSL -X POST -F file=@$wasm $WASM_UPLOAD_URL; else echo 'Cargo.toml not found'; exit 2; fi;"
        ]
        ports = ["http"]
      }

      resources { cpu = 600; memory = 1024 }

      service {
        name = "{{APP_NAME}}-g-build"
        port = "http"
        tags = ["builder","wasm","lane-g"]
        check { type = "script"; command = "/bin/true"; interval = "30s"; timeout = "5s" }
        meta { app = "{{APP_NAME}}" version = "{{VERSION}}" }
      }
    }
  }
}

