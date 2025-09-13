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
        PLOY_OPENAI_API_KEY = "${PLOY_OPENAI_API_KEY}"
        SBOM_LATEST_URL = "${SBOM_LATEST_URL}"
      }
      template {
        data        = "${SBOM_LATEST_URL}"
        destination = "local/sbom_latest_url"
      }
      template {
        data        = file("${NOMAD_TASK_DIR}/context/inputs.json")
        destination = "local/inputs.json"
      }
      artifact {
        source      = "${MODS_CONTEXT_URL}"
        destination = "local/context"
      }
    }
  }
}
