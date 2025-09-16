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
        # Ensure Docker task can resolve SeaweedFS Consul DNS name in Dev
        extra_hosts = ["seaweedfs-filer.service.consul:45.12.75.241"]
        entrypoint = ["/bin/sh", "-lc"]
        args = [
          "set -ex; apt-get update -y && apt-get install -y wget ca-certificates; mkdir -p /app; base=\${WASM_URL%/builds/*}; echo '[lane-g] resolved filer base:' $base; echo '[lane-g] downloading wazero-runner'; wget -O /app/wazero-runner $base/artifacts/wazero-runner/linux/amd64/wazero-runner; chmod +x /app/wazero-runner; echo '[lane-g] downloading module from' $WASM_URL; wget -O /app/module.wasm $WASM_URL; echo '[lane-g] module downloaded:'; ls -l /app/module.wasm; exec /app/wazero-runner -ignore-errors -module /app/module.wasm -port $PORT"
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
          type     = "http"
          path     = "/healthz"
          interval = "10s"
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
