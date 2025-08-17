job "lane-b-unikraft-posix" {
  datacenters = ["dc1"]
  type = "service"
  group "app" {
    count = 2
    network { port "http" { to = 8080 } ssh = 2222 }
    task "unikernel" {
      driver = "qemu"
      config {
        image_path = "local/${NOMAD_TASK_DIR}/app-b.img"
        args = ["-nographic"]
      }
      template {
        data = "{{ env `AUTHORIZED_KEYS` }}"
        destination = "secrets/authorized_keys"
        change_mode = "restart"
      }
      service {
        name = "lane-b-unikraft-posix"
        port = "http"
        check { type="http" path="/healthz" interval="5s" timeout="1s" }
      }
      resources { cpu = 600 memory = 192 }
      logs { max_files = 5 max_file_size = 10 }
      env {
        DROPBEAR_SSH = "false" # set true in debug
        AUTHORIZED_KEYS = ""
      }
    }
  }
}
