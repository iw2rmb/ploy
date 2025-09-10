job "${RUN_ID}" {
  datacenters = ["${NOMAD_DC}"]
  type        = "batch"

  group "orw" {
    task "openrewrite-apply" {
      driver = "docker"

      config {
        image = "${ORW_IMAGE}" # pin digest
        force_pull = true
      }

      env = {
        OUTPUT_DIR       = "/workspace/out"
        RECIPE           = "${RECIPE_CLASS}"
        DISCOVER_RECIPE  = "${DISCOVER_RECIPE}"
        RECIPE_GROUP     = "${RECIPE_GROUP}"
        RECIPE_ARTIFACT  = "${RECIPE_ARTIFACT}"
        RECIPE_VERSION   = "${RECIPE_VERSION}"
        CONTROLLER_URL   = "${CONTROLLER_URL}"
        TRANSFLOW_EXECUTION_ID = "${EXECUTION_ID}"
        DIFF_KEY         = "${DIFF_KEY}"
      }

      # Inject SeaweedFS Filer via Consul DNS (avoid host IPs)
      template {
        destination = "local/seaweed.env"
        env         = true
        data        = <<EOH
SEAWEEDFS_URL=http://seaweedfs-filer.service.consul:8888
INPUT_URL=http://seaweedfs-filer.service.consul:8888/artifacts/${INPUT_KEY}
EOH
      }

      resources {
        cpu    = 500
        memory = 1024
      }

      kill_timeout = "5m"
    }

    # using docker bind mounts via config.volumes
    restart {
      attempts = 0
      mode     = "fail"
    }
  }
}
