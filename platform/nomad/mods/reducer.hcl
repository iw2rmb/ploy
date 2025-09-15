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
      }
      env = {
        RUN_ID = "${RUN_ID}"
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
