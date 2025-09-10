job "transflow-reducer" {
  datacenters = ["dc1"]
  type        = "batch"

  group "reducer" {
    task "langgraph-reducer" {
      driver = "docker"

      config {
        image = "${REDUCER_IMAGE}"
        volumes = [
          "${CONTEXT_HOST_DIR}:/workspace/context:ro",
          "${OUT_HOST_DIR}:/workspace/out"
        ]
      }

      env = {
        MODEL       = "${MODEL}"
        TOOLS       = "${TOOLS_JSON}"
        LIMITS      = "${LIMITS_JSON}"
        OUTPUT_DIR  = "/workspace/out"
        RUN_ID      = "${RUN_ID}"
        CONTROLLER_URL   = "${CONTROLLER_URL}"
        TRANSFLOW_EXECUTION_ID = "${EXECUTION_ID}"
      }

      resources {
        cpu    = 300
        memory = 512
      }

      # Using docker bind mounts via config.volumes

      kill_timeout = "5m"
    }

    # no external volumes; using docker bind mounts via config.volumes

    restart {
      attempts = 0
      mode     = "fail"
    }
  }
}
