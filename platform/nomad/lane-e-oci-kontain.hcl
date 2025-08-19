job "{{APP_NAME}}-lane-e" {
  datacenters = ["dc1"]
  type = "service"
  group "app" {
    count = 2
    network { port "http" { to = 8080 } }
    task "oci" {
      driver = "docker"
      config {
        image = "{{DOCKER_IMAGE}}"
        runtime = "io.kontain"
        ports = ["http"]
      }
{{ENV_VARS}}
      service { name = "{{APP_NAME}}-lane-e-oci-kontain" port = "http" }
      resources { cpu = 500 memory = 256 }
    }
  }
}
