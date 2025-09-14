job "{{APP_NAME}}-c-build-{{VERSION}}" {
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

    task "osv-pack" {
      driver = "docker"

      env {
        CONTEXT_URL  = "{{CONTEXT_URL}}"
        OUTPUT_PATH  = "{{OUTPUT_PATH}}"
        BASE_IMAGE   = "{{BASE_IMAGE}}"
        MAIN_CLASS   = "{{MAIN_CLASS}}"
      }

      config {
        image = "alpine:3.19"
        entrypoint = ["/bin/sh", "-lc"]
        # Simple pack: ensure base image exists and copy to output; placeholder for OSv composition
        args = [
          "set -eu; mkdir -p /workspace; wget -qO /workspace/src.tar \"$CONTEXT_URL\" || true; mkdir -p $(dirname \"$OUTPUT_PATH\"); if [ ! -f \"$BASE_IMAGE\" ]; then echo 'Missing base image' >&2; exit 1; fi; cp \"$BASE_IMAGE\" \"$OUTPUT_PATH\"; echo packaged > /workspace/status.txt;"
        ]
        # Bind host /opt/ploy to write artifacts
        volumes = ["/opt/ploy:/host/opt/ploy"]
      }

      resources { cpu = 200 memory = 256 }

      logs { max_files = 3 max_file_size = 10 }
    }
  }
}

