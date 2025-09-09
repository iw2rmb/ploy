job "transflow-planner" {
  datacenters = ["dc1"]
  type        = "batch"

  group "planner" {
    task "langgraph-planner" {
      driver = "docker"

      config {
        image = "alpine:3.19"
        command = "/bin/sh"
        args    = ["-lc", "mkdir -p /workspace/out && echo '{\"plan_id\":\"stub\",\"options\":[{\"id\":\"llm-1\",\"type\":\"llm-exec\"}]}' > /workspace/out/plan.json"]
        volumes = ["/tmp/plan-test-out:/workspace/out"]
      }

      resources {
        cpu    = 100
        memory = 128
      }

      kill_timeout = "2m"
    }

    restart {
      attempts = 0
      mode     = "fail"
    }
  }
}

