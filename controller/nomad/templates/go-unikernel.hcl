job "go-unikernel" {
  datacenters = ["dc1"]
  group "g" {
    network { port "http" { to = 8080 } }
    task "app" {
      driver = "qemu"
      config { image_path = "local/go-unikernel.img" args = ["-nographic"] }
      service { name = "go-unikernel" port = "http" check { type="http" path="/healthz" interval="5s" timeout="1s" } }
      resources { cpu = 500 memory = 128 }
    }
  }
}
