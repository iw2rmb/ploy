job "{{APP_NAME}}-lane-d" {
  datacenters = ["dc1"]
  type        = "service"
  priority    = 50

  update {
    max_parallel      = 2
    min_healthy_time  = "20s"
    healthy_deadline  = "3m"
    progress_deadline = "10m"
    auto_revert       = true
    canary            = 1
    stagger           = "20s"
  }

  group "app" {
    count = {{INSTANCE_COUNT}}

    restart {
      attempts = 5
      interval = "2m"
      delay    = "15s"
      mode     = "fail"
    }

    reschedule {
      delay          = "20s"
      delay_function = "exponential"
      max_delay      = "1h"
      unlimited      = true
    }

    network {
      port "http" {
        to = {{HTTP_PORT}}
      }
      port "metrics" {
        to = 9090
      }
      {{#if GRPC_PORT}}
      port "grpc" {
        to = {{GRPC_PORT}}
      }
      {{/if}}
    }

    task "docker-runtime" {
      driver = "docker"

      config {
        image = "{{DOCKER_IMAGE}}"
        ports = ["http", "metrics"{{#if GRPC_PORT}}, "grpc"{{/if}}]
        hostname = "{{APP_NAME}}-${NOMAD_ALLOC_INDEX}"

        labels = {
          "ploy.app"     = "{{APP_NAME}}"
          "ploy.lane"     = "D"
          "ploy.version"  = "{{VERSION}}"
          "ploy.runtime"  = "docker"
        }

        logging {
          type = "json-file"
          config {
            max-size = "50m"
            max-file = "10"
            labels   = "ploy.app,ploy.lane,ploy.version"
          }
        }
      }

      env {
        APP_NAME  = "{{APP_NAME}}"
        VERSION   = "{{VERSION}}"
        LANE      = "D"
        RUNTIME   = "docker"
        PORT      = "${NOMAD_PORT_http}"
        HTTP_PORT = "${NOMAD_PORT_http}"
        METRICS_PORT = "${NOMAD_PORT_metrics}"
        {{#if GRPC_PORT}}
        GRPC_PORT = "${NOMAD_PORT_grpc}"
        {{/if}}
        HOSTNAME = "${NOMAD_ALLOC_ID}"
        POD_NAME = "{{APP_NAME}}-${NOMAD_ALLOC_INDEX}"
      }

      resources {
        cpu    = {{CPU_LIMIT}}
        memory = {{MEMORY_LIMIT}}
      }

      logs {
        max_files     = 5
        max_file_size = 20
      }
    }

    service {
      name = "{{APP_NAME}}"
      port = "http"
      tags = ["lane-d", "docker"]

      check {
        type     = "http"
        path     = "/health"
        interval = "30s"
        timeout  = "5s"
      }
    }
  }
}
