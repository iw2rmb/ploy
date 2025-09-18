job "test-artifact-download" {
  datacenters = ["dc1"]
  type = "batch"
  
  group "test" {
    task "check-artifact" {
      driver = "docker"
      
      config {
        image = "alpine:latest"
        command = "sh"
        args = ["-c", "ls -la; echo 'Checking for artifacts:'; if [ -d local ]; then echo 'local directory exists:'; ls -la local/; else echo 'local directory not found'; fi"]
      }
      
      # Test artifact download with corrected path (no artifacts prefix)
      artifact {
        source = "http://seaweedfs-filer.storage.ploy.local:8888/artifacts/openrewrite/test-job/input.tar"
        destination = "local/"
      }
      
      resources {
        cpu    = 100
        memory = 128
      }
      
      # Set a reasonable timeout
      kill_timeout = "30s"
    }
    
    # Restart policy for batch job
    restart {
      attempts = 1
      delay = "5s"
      mode = "fail"
    }
  }
}