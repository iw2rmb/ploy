job "transflow-orw-apply" {
  datacenters = ["dc1"]
  type        = "batch"

  group "orw" {
    task "openrewrite-apply" {
      driver = "docker"

      config {
        image = "ghcr.io/your-org/openrewrite-runner:jvm-6.17.0" # pin digest
        command = "/usr/local/bin/openrewrite"
        args = [
          "--engine", "openrewrite",
          "--recipe-class", "${RECIPE_CLASS}",
          "--coords", "${RECIPE_COORDS}",
          "--timeout", "${RECIPE_TIMEOUT}"
        ]
      }

      env = {
        OUTPUT_DIR  = "/workspace/out"
      }

      resources { cpu = 500, memory = 1024 }

      volume_mount { volume = "context" destination = "/workspace/context" read_only = true }
      volume_mount { volume = "out"     destination = "/workspace/out"     read_only = false }

      kill_timeout = "5m"
      timeout      = "30m"
    }

    volume "context" { type = "host" source = "transflow-context" }
    volume "out"     { type = "host" source = "transflow-out" }
  }
}

