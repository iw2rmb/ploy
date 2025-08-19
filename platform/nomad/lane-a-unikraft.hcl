job "{{APP_NAME}}-lane-a" {
  datacenters = ["dc1"]
  type = "service"
  group "app" {
    count = 2
    restart { 
      attempts = 3 
      interval = "30s" 
      delay = "5s" 
      mode = "fail" 
    }
    network { port "http" { to = 8080 } }
    task "unikernel" {
      driver = "qemu"
      config {
        image_path = "{{IMAGE_PATH}}"
        args = ["-nographic"]
      }
{{ENV_VARS}}
      service {
        name = "{{APP_NAME}}-lane-a-unikraft"
        port = "http"
        check { type="http" path="/healthz" interval="5s" timeout="1s" }
      }
      resources { cpu = 500 memory = 128 }
      logs { max_files = 5 max_file_size = 10 }
    }
  }
}
