job "{{APP_NAME}}-lane-f" {
  datacenters = ["dc1"]
  type = "service"
  group "db" {
    count = 1
    network { port "db" { to = 5432 } }
    task "vm" {
      driver = "qemu"
      config {
        image_path = "{{IMAGE_PATH}}"
        args = ["-nographic"]
      }
{{ENV_VARS}}
      resources { cpu = 2000 memory = 4096 }
    }
  }
}
