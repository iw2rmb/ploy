job "transflow-orw-apply-${RUN_ID}" {
  type = "batch"
  datacenters = ["dc1"]

  group "orw-apply" {
    task "openrewrite" {
      driver = "docker"
      config {
        image = "${TRANSFLOW_ORW_APPLY_IMAGE}"
      }
      env = {
        RECIPE_CLASS        = "${RECIPE_CLASS}"
        RECIPE_GROUP        = "${RECIPE_GROUP}"
        RECIPE_ARTIFACT     = "${RECIPE_ARTIFACT}"
        RECIPE_VERSION      = "${RECIPE_VERSION}"
        MAVEN_PLUGIN_VERSION= "${MAVEN_PLUGIN_VERSION}"
        DISCOVER_RECIPE     = "${DISCOVER_RECIPE}"
        INPUT_TAR_HOST_PATH = "${INPUT_TAR_HOST_PATH}"
        RUN_ID              = "${RUN_ID}"
      }
      artifact {
        source      = "${TRANSFLOW_CONTEXT_URL}"
        destination = "local/context"
      }
    }
  }
}

