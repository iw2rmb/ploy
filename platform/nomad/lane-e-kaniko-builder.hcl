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
        # Optional dev guard: delay before executor to allow log streamer to attach
        PLOY_KANIKO_ATTACH_DELAY = "{{ATTACH_DELAY}}"
      }

      config {
        image = "{{KANIKO_IMAGE}}"
        # Builder needs resolver access for .service.consul and filer; host networking is scoped to this short-lived task
        network_mode = "host"
        entrypoint = ["/busybox/sh", "-lc"]
        args = [
          "set -euo pipefail; mkdir -p /workspace; for i in 1 2 3; do wget -qO /workspace/src.tar $CONTEXT_URL && break; echo 'retrying context fetch...'; sleep 2; done; test -s /workspace/src.tar; tar -xf /workspace/src.tar -C /workspace; if [ -n \"$PLOY_KANIKO_ATTACH_DELAY\" ]; then echo 'attach delay' && sleep \"$PLOY_KANIKO_ATTACH_DELAY\"; fi; /kaniko/executor --context=/workspace --dockerfile=$DOCKERFILE_PATH --destination=$DOCKER_IMAGE --reproducible --snapshotMode=redo --single-snapshot --use-new-run;"
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
        memory = 2048
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
