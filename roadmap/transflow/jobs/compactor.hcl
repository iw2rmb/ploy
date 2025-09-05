job "transflow-kb-compactor" {
  datacenters = ["dc1"]
  type        = "batch"

  group "compactor" {
    task "kb-compactor" {
      driver = "docker"

      config {
        image = "ghcr.io/your-org/transflow-compactor:py-0.1.0" # pin digest
        command = "python"
        args    = ["-m", "compactor", "--kb", "/workspace/kb", "--out", "/workspace/out"]
      }

      env = {
        SNAPSHOT_RETENTION = "30d"
      }

      resources { cpu = 200, memory = 256 }

      volume_mount { volume = "kb"  destination = "/workspace/kb"  read_only = false }
      volume_mount { volume = "out" destination = "/workspace/out" read_only = false }

      restart { attempts = 0, mode = "fail" }
      timeout = "20m"
    }

    volume "kb"  { type = "host" source = "transflow-kb" }
    volume "out" { type = "host" source = "transflow-out" }
  }
}

