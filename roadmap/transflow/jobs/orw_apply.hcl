job "${RUN_ID}" {
  datacenters = ["dc1"]
  type        = "batch"

  group "orw" {
    task "openrewrite-apply" {
      driver = "docker"

      config {
        image = "${ORW_IMAGE}" # pin digest
        force_pull = true
        volumes = [
          "${CONTEXT_HOST_DIR}:/workspace/context:ro",
          "${INPUT_TAR_HOST_PATH}:/workspace/input.tar:ro",
          "${OUT_HOST_DIR}:/workspace/out"
        ]
      }

      env = {
        OUTPUT_DIR       = "/workspace/out"
        RECIPE           = "${RECIPE_CLASS}"
        RECIPE_GROUP     = "${RECIPE_GROUP}"
        RECIPE_ARTIFACT  = "${RECIPE_ARTIFACT}"
        RECIPE_VERSION   = "${RECIPE_VERSION}"
        MAVEN_PLUGIN_VERSION = "${MAVEN_PLUGIN_VERSION}"
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
