job "{{APP_NAME}}-lane-c" {
  datacenters = ["dc1"]
  type = "service"
  group "app" {
    count = 2
    network { port "http" { to = 8080 } }
    task "osv" {
      driver = "qemu"
      config {
        image_path = "{{IMAGE_PATH}}"
        args = ["-nographic"]
      }
{{ENV_VARS}}
      service {
        name = "{{APP_NAME}}-lane-c-osv"
        port = "http"
        check { type="http" path="/healthz" interval="5s" timeout="1s" }
      }
      resources { cpu = 800 memory = 512 }
    }
  }
}
