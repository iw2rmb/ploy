job "mods-planner" {
  type = "batch"
  datacenters = ["dc1"]

  group "planner" {
    task "planner" {
      driver = "docker"
      config {
        image = "${MODS_PLANNER_IMAGE}"
      }
      env = {
        MODEL       = "${MODEL}"
        TOOLS_JSON  = "${TOOLS_JSON}"
        LIMITS_JSON = "${LIMITS_JSON}"
        RUN_ID      = "${RUN_ID}"
        SBOM_LATEST_URL = "${SBOM_LATEST_URL}"
        CONTROLLER_URL    = "${CONTROLLER_URL}"
        MODS_EXECUTION_ID = "${EXECUTION_ID}"
        CONTEXT_DIR  = "${NOMAD_TASK_DIR}/context"
        OUTPUT_DIR   = "${NOMAD_TASK_DIR}/out"
      }
      template {
        data        = "${SBOM_LATEST_URL}"
        destination = "local/sbom_latest_url"
      }
      template {
        data        = file("${NOMAD_TASK_DIR}/context/inputs.json")
        destination = "local/inputs.json"
      }
      template {
        data        = ""
        destination = "local/out/.keep"
      }
      artifact {
        source      = "${MODS_CONTEXT_URL}"
        destination = "local/context"
      }
    }
  }
}
