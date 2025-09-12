job "${RUN_ID}" {
  datacenters = ["${NOMAD_DC}"]
  type        = "batch"

  group "orw-apply" {
    task "openrewrite-apply" {
      driver = "docker"

      config {
        image = "${ORW_IMAGE}"
        force_pull = true
        network_mode = "host"
      }

      env = {
        OUTPUT_DIR            = "/workspace/out"
        JOB_ID                = ""
        TRANSFORMATION_ID     = ""
        RECIPE                = "${RECIPE_CLASS}"
        RECIPE_GROUP          = "${RECIPE_GROUP}"
        RECIPE_ARTIFACT       = "${RECIPE_ARTIFACT}"
        RECIPE_VERSION        = "${RECIPE_VERSION}"
        MAVEN_PLUGIN_VERSION  = "${MAVEN_PLUGIN_VERSION}"
        CONTROLLER_URL        = "${CONTROLLER_URL}"
        TRANSFLOW_EXECUTION_ID= "${EXECUTION_ID}"
        DIFF_KEY              = "${DIFF_KEY}"
        SEAWEEDFS_URL         = "${SEAWEEDFS_URL}"
        INPUT_URL             = "${INPUT_URL}"
      }

      resources {
        cpu    = 300
        memory = 512
      }

      kill_timeout = "5m"
    }

    restart {
      attempts = 0
      mode     = "fail"
    }
  }
}
