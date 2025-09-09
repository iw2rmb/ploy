job "{{APP_NAME}}-lane-c" {
  datacenters = ["dc1"]
  type = "service"

  group "app" {
    count = {{INSTANCE_COUNT}}

    network {
      port "http" {
        to = {{HTTP_PORT}}
      }
    }

    task "osv-jvm" {
      driver = "qemu"

      config {
        image_path = "{{IMAGE_PATH}}"
        args = ["-nographic"]
      }

      env {
        APP_NAME = "{{APP_NAME}}"
        VERSION = "{{VERSION}}"
        LANE = "C"
        MAIN_CLASS = "{{MAIN_CLASS}}"
        SERVER_PORT = "{{HTTP_PORT}}"
      }

      resources {
        cpu    = {{CPU_LIMIT}}
        memory = {{MEMORY_LIMIT}}
      }

      logs {
        max_files     = 5
        max_file_size = 20
      }

      kill_timeout = "60s"
      kill_signal  = "SIGTERM"
    }
  }
}
