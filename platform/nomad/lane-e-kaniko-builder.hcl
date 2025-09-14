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
        entrypoint = ["/busybox/sh", "-lc"]
        args = [
          "set -euo pipefail; echo Fetching context: $CONTEXT_URL; \
           wget -qO /tmp/src.tar \"$CONTEXT_URL\"; \
           mkdir -p /workspace; tar -xf /tmp/src.tar -C /workspace; \
           echo Running Kaniko build to $DOCKER_IMAGE with dockerfile=$DOCKERFILE_PATH; \
           /kaniko/executor --context=/workspace --dockerfile=/$${DOCKERFILE_PATH:-Dockerfile} --destination=\"$DOCKER_IMAGE\" --reproducible \
           --snapshotMode=redo --single-snapshot --use-new-run;"
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
