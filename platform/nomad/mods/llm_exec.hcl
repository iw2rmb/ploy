job "mods-llm-exec-${RUN_ID}" {
  type = "batch"
  datacenters = ["dc1"]

  group "llm-exec" {
    task "llm" {
      driver = "docker"
      config {
        image = "${MODS_LLM_EXEC_IMAGE}"
      }
      env = {
        MODEL       = "${MODEL}"
        TOOLS_JSON  = "${TOOLS_JSON}"
        LIMITS_JSON = "${LIMITS_JSON}"
        RUN_ID      = "${RUN_ID}"
        SBOM_LATEST_URL = "${SBOM_LATEST_URL}"
        CONTROLLER_URL    = "${CONTROLLER_URL}"
        MODS_EXECUTION_ID = "${EXECUTION_ID}"
        PLOY_SEAWEEDFS_URL = "${PLOY_SEAWEEDFS_URL}"
        CONTEXT_DIR  = "${NOMAD_TASK_DIR}/context"
        OUTPUT_DIR   = "${NOMAD_TASK_DIR}/out"
      }
      template {
        data        = "${SBOM_LATEST_URL}"
        destination = "local/sbom_latest_url"
      }
      artifact {
        source      = "${MODS_CONTEXT_URL}"
        destination = "local/context"
      }
      template {
        data        = "# keep llm out dir"
        destination = "local/out/.keep"
      }
    }
  }
}
