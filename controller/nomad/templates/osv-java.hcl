job "osv-java" {
  datacenters = ["dc1"]
  group "g" {
    network { port "http" { to = 8080 } }
    task "app" {
      driver = "qemu"
      config { image_path = "local/java-osv.img" args = ["-nographic"] }
      service { name = "osv-java" port = "http" check { type="http" path="/healthz" interval="5s" timeout="1s" } }
      resources { cpu = 800 memory = 512 }
    }
  }
}
