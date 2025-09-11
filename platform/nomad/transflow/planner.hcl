job "transflow-planner" {
  type = "batch"
  datacenters = ["dc1"]

  group "planner" {
    task "planner" {
      driver = "docker"
      config {
        image = "${TRANSFLOW_PLANNER_IMAGE}"
      }
      env = {
        MODEL       = "${MODEL}"
        TOOLS_JSON  = "${TOOLS_JSON}"
        LIMITS_JSON = "${LIMITS_JSON}"
        RUN_ID      = "${RUN_ID}"
      }
      template {
        data        = file("${NOMAD_TASK_DIR}/context/inputs.json")
        destination = "local/inputs.json"
      }
      artifact {
        source      = "${TRANSFLOW_CONTEXT_URL}"
        destination = "local/context"
      }
    }
  }
}

