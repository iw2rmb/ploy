job "{{APP_NAME}}-c-build-{{VERSION}}" {
  datacenters = ["dc1"]
  type = "batch"

  group "build" {
    count = 1

    restart {
      attempts = 0
      interval = "1m"
      delay    = "10s"
      mode     = "fail"
    }

    task "osv-pack" {
      driver = "docker"

      env {
        CONTEXT_URL  = "{{CONTEXT_URL}}"
        OUTPUT_PATH  = "{{OUTPUT_PATH}}"
        MAIN_CLASS   = "{{MAIN_CLASS}}"
      }

      config {
        image = "maven:3.9.6-eclipse-temurin-17"
        entrypoint = ["/bin/sh", "-lc"]
        args = ["set -eux; WORKDIR=/workspace/app; mkdir -p $WORKDIR; curl -fsSL \"$CONTEXT_URL\" -o /workspace/src.tar; tar -xf /workspace/src.tar -C $WORKDIR; cd $WORKDIR; if [ -d src/healing/java ]; then mkdir -p src/main/java && cp -R src/healing/java/. src/main/java/; fi; mvn -B -q -DskipTests compile; mkdir -p $(dirname \"$OUTPUT_PATH\"); echo 'lane-c artifact placeholder' > \"$OUTPUT_PATH\";"]
        volumes = ["/opt/ploy:/host/opt/ploy"]
      }

      resources { cpu = 200 memory = 256 }

      logs { max_files = 3 max_file_size = 10 }
    }
  }
}
