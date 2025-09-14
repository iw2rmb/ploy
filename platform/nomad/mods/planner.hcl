job "mods-planner" {
  type = "batch"
  datacenters = ["dc1"]

  group "planner" {
    task "planner" {
      driver = "docker"
      config {
        image = "${PLANNER_IMAGE}"
        force_pull = true
      }
      env = {
        MODEL       = "${MODEL}"
        TOOLS_JSON  = "${TOOLS_JSON}"
        LIMITS_JSON = "${LIMITS_JSON}"
        RUN_ID      = "${RUN_ID}"
        SBOM_LATEST_URL = "${SBOM_LATEST_URL}"
        CONTROLLER_URL    = "${CONTROLLER_URL}"
        MOD_ID = "${MOD_ID}"
        PLOY_SEAWEEDFS_URL = "${PLOY_SEAWEEDFS_URL}"
        CONTEXT_DIR  = "${NOMAD_TASK_DIR}/context"
        OUTPUT_DIR   = "${NOMAD_TASK_DIR}/out"
      }
      template {
        data        = "${SBOM_LATEST_URL}"
        destination = "local/sbom_latest_url"
      }
      template {
        data        = "# keep planner out dir"
        destination = "local/out/.keep"
      }
      artifact {
        source      = "${MODS_CONTEXT_URL}"
        destination = "local/context"
      }
    }
  }
}
