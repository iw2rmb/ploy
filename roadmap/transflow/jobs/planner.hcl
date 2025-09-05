job "transflow-planner" {
  datacenters = ["dc1"]
  type        = "batch"

  group "planner" {
    task "langgraph-planner" {
      driver = "docker"

      config {
        image = "ghcr.io/your-org/langchain-runner:py-0.1.0" # pin exact digest in prod
        command = "python"
        args    = ["-m", "runner", "--mode", "planner"]
      }

      env = {
        MODEL       = "${MODEL}"           # e.g., gpt-4o-mini@2024-08-06
        TOOLS       = "${TOOLS_JSON}"      # JSON string allowlisting tools
        LIMITS      = "${LIMITS_JSON}"     # JSON limits (steps/tool_calls/timeout)
        CONTEXT_DIR = "/workspace/context"
        KB_DIR      = "/workspace/kb"
        OUTPUT_DIR  = "/workspace/out"
        RUN_ID      = "${RUN_ID}"
      }

      resources {
        cpu    = 500
        memory = 1024
      }

      volume_mount {
        volume      = "context"
        destination = "/workspace/context"
        read_only   = true
      }

      volume_mount {
        volume      = "kb"
        destination = "/workspace/kb"
        read_only   = true
      }

      volume_mount {
        volume      = "out"
        destination = "/workspace/out"
        read_only   = false
      }

      template {
        destination = "/workspace/out/.keep"
        data        = ""
      }

      kill_timeout = "5m"
      timeout      = "30m"
    }

    volume "context" { type = "host" source = "transflow-context" }
    volume "kb"      { type = "host" source = "transflow-kb" }
    volume "out"     { type = "host" source = "transflow-out" }

    restart {
      attempts = 0
      mode     = "fail"
    }
  }
}
