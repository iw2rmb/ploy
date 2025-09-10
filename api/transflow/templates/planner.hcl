job "transflow-planner" {
  datacenters = ["${NOMAD_DC}"]
  type        = "batch"

  group "planner" {
    task "langgraph-planner" {
      driver = "docker"

      config {
        image = "${PLANNER_IMAGE}"
        force_pull = true
        volumes = [
          "${CONTEXT_HOST_DIR}:/workspace/context:ro",
          "${OUT_HOST_DIR}:/workspace/out"
        ]
      }

      env = {
        MODEL       = "${MODEL}"
        TOOLS       = "${TOOLS_JSON}"
        LIMITS      = "${LIMITS_JSON}"
        CONTEXT_DIR = "/workspace/context"
        KB_DIR      = "/workspace/kb"
        OUTPUT_DIR  = "/workspace/out"
        RUN_ID      = "${RUN_ID}"
        CONTROLLER_URL   = "${CONTROLLER_URL}"
        TRANSFLOW_EXECUTION_ID = "${EXECUTION_ID}"
      }

      resources {
        cpu    = 500
        memory = 1024
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
