job "transflow-llm-exec" {
  datacenters = ["dc1"]
  type        = "batch"

  group "llm-exec" {
    task "langgraph-llm-exec" {
      driver = "docker"

      config {
        image   = "ghcr.io/your-org/langchain-runner:py-0.1.0" # pin exact digest in prod
        command = "python"
        args    = ["-m", "runner", "--mode", "exec"]
      }

      env = {
        MODEL       = "${MODEL}"
        TOOLS       = "${TOOLS_JSON}"
        LIMITS      = "${LIMITS_JSON}"
        CONTEXT_DIR = "/workspace/context"
        OUTPUT_DIR  = "/workspace/out"
        RUN_ID      = "${RUN_ID}"
      }

      resources {
        cpu    = 700
        memory = 1024
      }

      volume_mount {
        volume      = "context"
        destination = "/workspace/context"
        read_only   = true
      }

      volume_mount {
        volume      = "out"
        destination = "/workspace/out"
        read_only   = false
      }

      # The runner should write /workspace/out/diff.patch on success
      kill_timeout = "5m"
      timeout      = "30m"
    }

    volume "context" { type = "host" source = "transflow-context" }
    volume "out"     { type = "host" source = "transflow-out" }

    restart {
      attempts = 0
      mode     = "fail"
    }
  }
}

