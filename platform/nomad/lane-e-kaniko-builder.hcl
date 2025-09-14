job "{{APP_NAME}}-e-build-{{VERSION}}" {
  datacenters = ["dc1"]
  type = "batch"

  group "build" {
    count = 1

    restart {
      attempts = 0
      interval = "1m"
      delay    = "10s"
      mode     = "fail"
    }

    network {
      port "http" {}
    }

    task "kaniko" {
      driver = "docker"

      env {
        CONTEXT_URL     = "{{CONTEXT_URL}}"
        DOCKER_IMAGE    = "{{DOCKER_IMAGE}}"
        DOCKERFILE_PATH = "{{DOCKERFILE_PATH}}"
      }

      config {
        image = "{{KANIKO_IMAGE}}"
        # Use host networking so the builder can reach local service endpoints (e.g., SeaweedFS filer)
        network_mode = "host"
        entrypoint = ["/busybox/sh", "-lc"]
        args = [
          "set -euo pipefail; wget -qO /tmp/src.tar $CONTEXT_URL; mkdir -p /workspace; tar -xf /tmp/src.tar -C /workspace; /kaniko/executor --context=/workspace --dockerfile=$DOCKERFILE_PATH --destination=$DOCKER_IMAGE --reproducible --snapshotMode=redo --single-snapshot --use-new-run;"
        ]

        ports = ["http"]

        logging {
          type = "json-file"
          config {
            max-size = "10m"
            max-file = "3"
          }
        }
      }

      resources {
        cpu    = 500
        memory = 512
      }

      service {
        name = "{{APP_NAME}}-e-build"
        port = "http"
        tags = [
          "builder",
          "kaniko",
          "lane-e",
        ]
        check {
          type     = "script"
          command  = "/bin/true"
          interval = "30s"
          timeout  = "5s"
        }
        meta {
          app     = "{{APP_NAME}}"
          version = "{{VERSION}}"
          image   = "{{DOCKER_IMAGE}}"
        }
      }

      logs {
        max_files     = 3
        max_file_size = 10
      }

      kill_timeout = "30s"
    }
  }
}
