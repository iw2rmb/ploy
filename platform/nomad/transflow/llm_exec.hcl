job "transflow-llm-exec-${RUN_ID}" {
  type = "batch"
  datacenters = ["dc1"]

  group "llm-exec" {
    task "llm" {
      driver = "docker"
      config {
        image = "${TRANSFLOW_LLM_EXEC_IMAGE}"
      }
      env = {
        MODEL       = "${MODEL}"
        TOOLS_JSON  = "${TOOLS_JSON}"
        LIMITS_JSON = "${LIMITS_JSON}"
        RUN_ID      = "${RUN_ID}"
      }
      artifact {
        source      = "${TRANSFLOW_CONTEXT_URL}"
        destination = "local/context"
      }
    }
  }
}

