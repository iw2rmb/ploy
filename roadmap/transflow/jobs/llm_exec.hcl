job "transflow-llm-exec" {
  datacenters = ["dc1"]
  type        = "batch"

  group "llm-exec" {
    task "langgraph-llm-exec" {
      driver = "docker"

      config {
        image = "${LLM_EXEC_IMAGE}"
        # Use the image's default entrypoint to generate a diff patch.
        volumes = [
          "${CONTEXT_HOST_DIR}:/workspace/context:ro",
          "${OUT_HOST_DIR}:/workspace/out",
        ]
      }

      env = {
        # Core LLM configuration
        MODEL       = "${MODEL}"
        TOOLS       = "${TOOLS_JSON}"
        LIMITS      = "${LIMITS_JSON}"
        CONTEXT_DIR = "/workspace/context"
        OUTPUT_DIR  = "/workspace/out"
        RUN_ID      = "${RUN_ID}"
        
        # MCP integration configuration
        MCP_TOOLS_JSON      = "${MCP_TOOLS_JSON}"
        MCP_CONTEXT_JSON    = "${MCP_CONTEXT_JSON}"
        MCP_ENDPOINTS_JSON  = "${MCP_ENDPOINTS_JSON}"
        MCP_BUDGETS_JSON    = "${MCP_BUDGETS_JSON}"
        MCP_PROMPTS_JSON    = "${MCP_PROMPTS_JSON}"
        MCP_TIMEOUT         = "${MCP_TIMEOUT}"
        MCP_SECURITY_MODE   = "${MCP_SECURITY_MODE}"
        
        # Python runtime configuration
        PYTHONDONTWRITEBYTECODE = "1"
        PYTHONUNBUFFERED        = "1"
      }

      resources {
        cpu    = 700
        memory = 1024
      }

      network {
        # Allow outbound connections for MCP endpoints
        mode = "bridge"
        port "http" {
          to = 8080
        }
        dns {
          servers = ["8.8.8.8", "1.1.1.1"]
        }
      }

      # The runner should write /workspace/out/diff.patch on success
      kill_timeout = "5m"
    }

    # Using docker bind mounts via config.volumes

    restart {
      attempts = 0
      mode     = "fail"
    }
  }
}
