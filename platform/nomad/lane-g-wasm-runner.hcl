job "{{APP_NAME}}-lane-g" {
  datacenters = ["dc1"]
  type = "service"

  group "app" {
    count = 1

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
        image = "{{WASM_RUNTIME_IMAGE}}"
        entrypoint = ["/runner"]
        args = ["-ignore-errors", "-url", "{{WASM_URL}}", "-port", "{{HTTP_PORT}}"]
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
