job "transflow-reducer" {
  datacenters = ["dc1"]
  type        = "batch"

  group "reducer" {
    task "langgraph-reducer" {
      driver = "docker"

      config {
        image = "ghcr.io/your-org/langchain-runner:py-0.1.0" # pin exact digest in prod
        command = "python"
        args    = ["-m", "runner", "--mode", "reducer", "--input", "/workspace/context/history.json"]
      }

      env = {
        MODEL       = "${MODEL}"
        TOOLS       = "${TOOLS_JSON}"
        LIMITS      = "${LIMITS_JSON}"
        OUTPUT_DIR  = "/workspace/out"
        RUN_ID      = "${RUN_ID}"
      }

      resources {
        cpu    = 300
        memory = 512
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

      template {
        destination = "/workspace/out/.keep"
        data        = ""
      }

      kill_timeout = "5m"
      timeout      = "15m"
    }

    volume "context" { type = "host" source = "transflow-history" }
    volume "out"     { type = "host" source = "transflow-out" }

    restart {
      attempts = 0
      mode     = "fail"
    }
  }
}
