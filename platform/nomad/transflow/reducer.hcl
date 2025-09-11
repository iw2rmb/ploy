job "transflow-reducer" {
  type = "batch"
  datacenters = ["dc1"]

  group "reducer" {
    task "reduce" {
      driver = "docker"
      config {
        image = "${TRANSFLOW_REDUCER_IMAGE}"
      }
      env = {
        RUN_ID = "${RUN_ID}"
      }
      template {
        data        = file("${NOMAD_TASK_DIR}/context/history.json")
        destination = "local/history.json"
      }
      artifact {
        source      = "${TRANSFLOW_CONTEXT_URL}"
        destination = "local/context"
      }
    }
  }
}

