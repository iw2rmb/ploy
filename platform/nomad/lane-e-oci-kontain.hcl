job "lane-e-oci-kontain" {
  datacenters = ["dc1"]
  type = "service"
  group "app" {
    count = 2
    network { port "http" { to = 8080 } }
    task "oci" {
      driver = "docker"
      config {
        image = "harbor.local/ploy/java-ordersvc:latest"
        runtime = "io.kontain"
        ports = ["http"]
      }
      service { name = "lane-e-oci-kontain" port = "http" }
      resources { cpu = 500 memory = 256 }
    }
  }
}
