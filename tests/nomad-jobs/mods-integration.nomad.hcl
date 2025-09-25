job "${MODS_INTEGRATION_JOB_NAME}" {
  datacenters = ["${MODS_INTEGRATION_DC}"]
  type        = "batch"
  priority    = 60

  group "runner" {
    count = 1

    restart {
      attempts = 0
      mode     = "fail"
    }

    ephemeral_disk {
      size = 4096
    }

    task "mods-integration" {
      driver = "docker"

      config {
        image   = "${MODS_INTEGRATION_IMAGE}"
        command = "/bin/bash"
        args = [
          "-lc",
          <<-EOT
            set -euo pipefail

            WORKDIR="${MODS_INTEGRATION_WORKDIR}"
            REPO="${MODS_INTEGRATION_REPO}"
            REF="${MODS_INTEGRATION_REF}"
            SHA="${MODS_INTEGRATION_SHA}"
            TIMEOUT="${MODS_INTEGRATION_TIMEOUT}"

            mkdir -p "$WORKDIR"
            cd "$WORKDIR"

            AUTH_REPO="$REPO"
            if [ -n "$GITHUB_PLOY_DEV_USERNAME" ] && [ -n "$GITHUB_PLOY_DEV_PAT" ]; then
              AUTH_REPO=$(printf '%s' "$REPO" | sed -e "s#https://#https://${GITHUB_PLOY_DEV_USERNAME}:${GITHUB_PLOY_DEV_PAT}@#")
            fi

            echo "Cloning $REF from $REPO"
            rm -rf source
            git clone --filter=blob:none --depth=1 --branch "$REF" "$AUTH_REPO" source
            cd source

            if [ -n "$SHA" ]; then
              echo "Checking out commit $SHA"
              git fetch --depth=1 origin "$SHA"
              git checkout "$SHA"
            fi

            export GIT_TERMINAL_PROMPT=0
            echo "Running go test ./internal/mods -tags=integration"
            go test ./internal/mods -tags=integration -v -timeout="$TIMEOUT"
          EOT,
        ]
      }

      env {
        GOFLAGS                 = "${GOFLAGS}"
        GITHUB_PLOY_DEV_USERNAME = "${GITHUB_PLOY_DEV_USERNAME}"
        GITHUB_PLOY_DEV_PAT       = "${GITHUB_PLOY_DEV_PAT}"
        MODS_INTEGRATION_REPO     = "${MODS_INTEGRATION_REPO}"
        MODS_INTEGRATION_REF      = "${MODS_INTEGRATION_REF}"
        MODS_INTEGRATION_SHA      = "${MODS_INTEGRATION_SHA}"
        MODS_INTEGRATION_TIMEOUT  = "${MODS_INTEGRATION_TIMEOUT}"
        MODS_INTEGRATION_WORKDIR  = "${MODS_INTEGRATION_WORKDIR}"
        PLOY_GITLAB_PAT           = "${PLOY_GITLAB_PAT}"
        PLOY_CONTROLLER           = "${PLOY_CONTROLLER}"
        PLOY_SEAWEEDFS_URL        = "${PLOY_SEAWEEDFS_URL}"
        MODS_SEAWEED_MASTER       = "${MODS_SEAWEED_MASTER}"
        MODS_SEAWEED_FALLBACKS    = "${MODS_SEAWEED_FALLBACKS}"
        MODS_ALLOW_PARTIAL_ORW    = "${MODS_ALLOW_PARTIAL_ORW}"
        MODS_REGISTRY             = "${MODS_REGISTRY}"
        MODS_PLANNER_IMAGE        = "${MODS_PLANNER_IMAGE}"
        MODS_REDUCER_IMAGE        = "${MODS_REDUCER_IMAGE}"
        MODS_LLM_EXEC_IMAGE       = "${MODS_LLM_EXEC_IMAGE}"
        MODS_ORW_APPLY_IMAGE      = "${MODS_ORW_APPLY_IMAGE}"
        NOMAD_ADDR                = "${NOMAD_ADDR}"
        CONSUL_HTTP_ADDR          = "${CONSUL_HTTP_ADDR}"
        SEAWEEDFS_FILER           = "${SEAWEEDFS_FILER}"
        SEAWEEDFS_MASTER          = "${SEAWEEDFS_MASTER}"
        SEAWEEDFS_COLLECTION      = "${SEAWEEDFS_COLLECTION}"
        TARGET_HOST               = "${TARGET_HOST}"
      }

      resources {
        cpu    = ${MODS_INTEGRATION_CPU}
        memory = ${MODS_INTEGRATION_MEMORY}
      }

      logs {
        max_files     = 5
        max_file_size = 20
      }
    }
  }
}
