job "mods-reducer" {
  type = "batch"
  datacenters = ["dc1"]

  group "reducer" {
    task "reduce" {
      driver = "docker"
      config {
        image = "${REDUCER_IMAGE}"
      }
      env = {
        RUN_ID = "${RUN_ID}"
        SBOM_LATEST_URL = "${SBOM_LATEST_URL}"
      }
      template {
        data        = "${SBOM_LATEST_URL}"
        destination = "local/sbom_latest_url"
      }
      artifact {
        source      = "${MODS_CONTEXT_URL}"
        destination = "local/context"
      }
    }
  }
}
