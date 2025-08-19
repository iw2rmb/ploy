job "{{APP_NAME}}-lane-b" {
  datacenters = ["dc1"]
  type = "service"
  group "app" {
    count = 2
    network { 
      port "http" { 
        to = 8080 
      }
      port "ssh" {
        static = 2222
      }
    }
    task "unikernel" {
      driver = "qemu"
      config {
        image_path = "{{IMAGE_PATH}}"
        args = ["-nographic"]
      }
      template {
        data = "{{ env `AUTHORIZED_KEYS` }}"
        destination = "secrets/authorized_keys"
        change_mode = "restart"
      }
      service {
        name = "{{APP_NAME}}-lane-b-unikraft-posix"
        port = "http"
        check { 
          type = "http" 
          path = "/healthz" 
          interval = "5s" 
          timeout = "1s" 
        }
      }
      resources { 
        cpu = 600 
        memory = 192 
      }
      logs { 
        max_files = 5 
        max_file_size = 10 
      }
{{ENV_VARS}}
    }
  }
}
