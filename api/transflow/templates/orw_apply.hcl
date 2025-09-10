job "${RUN_ID}" {
  datacenters = ["${NOMAD_DC}"]
  type        = "batch"

  group "orw" {
    task "openrewrite-apply" {
      driver = "docker"

      config {
        image = "${ORW_IMAGE}" # pin digest
        force_pull = true
        # Use host networking so container inherits host's DNS (Consul) and can resolve service.consul
        network_mode = "host"
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
        SEAWEEDFS_URL    = "${SEAWEEDFS_URL}"
        INPUT_URL        = "${INPUT_URL}"
      }

      # SEAWEEDFS_URL and INPUT_URL are computed server-side (controller) and injected above.
      # This avoids resolving to host IPs via consul-template and ensures Consul DNS names are used.

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
