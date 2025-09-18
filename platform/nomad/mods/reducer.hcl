job "mods-reducer" {
  type = "batch"
  datacenters = ["dc1"]

  group "reducer" {
    task "reduce" {
      driver = "docker"
      config {
        image = "${REDUCER_IMAGE}"
        force_pull = true
        network_mode = "host"
        tls {
          insecure_skip_verify = true
        }
      }
      env = {
        RUN_ID = "${RUN_ID}"
        CONTROLLER_URL = "${CONTROLLER_URL}"
        MOD_ID = "${MOD_ID}"
        PLOY_SEAWEEDFS_URL = "${PLOY_SEAWEEDFS_URL}"
        CONTEXT_DIR  = "${NOMAD_TASK_DIR}/context"
        OUTPUT_DIR   = "${NOMAD_TASK_DIR}/out"
      }
      template {
        data        = "# keep reducer out dir"
        destination = "local/out/.keep"
      }
      artifact {
        source      = "${MODS_CONTEXT_URL}"
        destination = "local/context"
      }
    }
  }
}
