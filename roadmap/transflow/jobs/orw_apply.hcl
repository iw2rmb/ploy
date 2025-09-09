job "transflow-orw-apply" {
  datacenters = ["dc1"]
  type        = "batch"

  group "orw" {
    task "openrewrite-apply" {
      driver = "docker"

      config {
        image = "${ORW_APPLY_IMAGE}" # pin digest
        force_pull = true
        volumes = [
          "${CONTEXT_HOST_DIR}:/workspace/context:ro",
          "${OUT_HOST_DIR}:/workspace/out"
        ]
      }

      env = {
        OUTPUT_DIR      = "/workspace/out"
        RECIPE          = "${RECIPE_CLASS}"
        DISCOVER_RECIPE = "true"
      }

      resources {
        cpu    = 500
        memory = 1024
      }

      kill_timeout = "5m"
    }

    # using docker bind mounts via config.volumes
    restart {
      attempts = 0
      mode     = "fail"
    }
  }
}
