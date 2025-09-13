job "{{APP_NAME}}-lane-c" {
  datacenters = ["dc1"]
  type = "service"

  group "app" {
    count = {{INSTANCE_COUNT}}

    network {
      port "http"   { to = {{HTTP_PORT}} }
      port "metrics"{ to = 9090 }
      port "jmx"    { to = 9999 }
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
        METRICS_PORT = "9090"
        JMX_PORT = "9999"
        SERVICE_NAME = "{{APP_NAME}}-lane-c"
        NOMAD_ALLOC_ID = "${NOMAD_ALLOC_ID}"
        NOMAD_TASK_NAME = "${NOMAD_TASK_NAME}"
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

      {{#if CONSUL_CONFIG_ENABLED}}
      # Primary service with HTTP health check
      service {
        name = "{{APP_NAME}}-lane-c"
        port = "http"
        tags = [
          "lane-c",
          "app={{APP_NAME}}",
          "version={{VERSION}}",
          "runtime=jvm"
        ]
        check {
          type     = "http"
          path     = "/health"
          interval = "10s"
          timeout  = "2s"
        }
      }

      # Metrics service (HTTP)
      service {
        name = "{{APP_NAME}}-lane-c-metrics"
        port = "metrics"
        tags = [
          "lane-c",
          "app={{APP_NAME}}",
          "metrics",
          "runtime=jvm"
        ]
        check {
          type     = "http"
          path     = "/metrics"
          interval = "15s"
          timeout  = "2s"
        }
      }

      # JMX service (TCP)
      service {
        name = "{{APP_NAME}}-lane-c-jmx"
        port = "jmx"
        tags = [
          "lane-c",
          "app={{APP_NAME}}",
          "jmx",
          "runtime=jvm"
        ]
        check {
          type     = "tcp"
          interval = "20s"
          timeout  = "2s"
        }
      }
      {{/if}}
    }
  }
}
