job "{{APP_NAME}}-lane-g" {
  datacenters = ["dc1"]
  type = "service"

  group "app" {
    count = {{INSTANCE_COUNT}}

    network {
      port "http" { to = {{HTTP_PORT}} }
    }

    task "wasm" {
      driver = "docker"

      env {
        WASM_URL = "{{WASM_URL}}"
        PORT     = "{{HTTP_PORT}}"
      }

      config {
        image = "debian:bookworm-slim"
        entrypoint = ["/bin/sh", "-lc"]
        args = [
          "set -euo pipefail; apt-get update -y >/dev/null 2>&1 && apt-get install -y wget ca-certificates >/dev/null 2>&1; mkdir -p /app; wget -qO /app/wazero-runner http://seaweedfs-filer.service.consul:8888/artifacts/wazero-runner/linux/amd64/wazero-runner; chmod +x /app/wazero-runner; wget -qO /app/module.wasm $WASM_URL; echo 'Fetched WASM module'; /app/wazero-runner -module /app/module.wasm -port $PORT || true; echo 'Runner finished or detached; keeping task alive'; sleep 3600"
        ]
        ports = ["http"]
      }

      resources {
        cpu    = {{CPU_LIMIT}}
        memory = {{MEMORY_LIMIT}}
      }

      service {
        name = "{{APP_NAME}}-lane-g"
        port = "http"
        tags = [
          "lane-g",
          "wasm",
        ]
        check {
          type     = "script"
          command  = "/bin/true"
          interval = "15s"
          timeout  = "3s"
        }
      }

      logs {
        max_files     = 3
        max_file_size = 10
      }
    }
  }
}
