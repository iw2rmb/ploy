job "oci-kontain" {
  datacenters = ["dc1"]
  group "g" {
    network { port "http" { to = 8080 } }
    task "app" {
      driver = "docker"
      config {
        image = "harbor.local/ploy/java-ordersvc:latest"
        runtime = "io.kontain"
        ports = ["http"]
      }
      service { name = "oci-kontain" port = "http" }
      resources { cpu = 500 memory = 256 }
    }
  }
}
