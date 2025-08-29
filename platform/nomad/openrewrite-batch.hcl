job "openrewrite-{{JOB_ID}}" {
  datacenters = ["dc1"]
  type = "batch"  # Runs once then exits
  priority = 80   # High priority for quick processing
  
  # Batch job specific settings
  parameterized {
    payload       = "optional"
    meta_required = ["recipe", "input_url", "output_url"]
  }
  
  group "transform" {
    count = 1
    
    # Ephemeral disk for temporary processing
    ephemeral_disk {
      size    = 1024  # 1GB for code processing
      migrate = false
      sticky  = false
    }
    
    task "openrewrite-jvm" {
      driver = "docker"
      
      config {
        image = "registry.dev.ployman.app/openrewrite-jvm:latest"
        
        # Mount ephemeral disk
        volumes = [
          "local:/workspace"
        ]
        
        # Security: read-only root filesystem
        readonly_rootfs = true
        
        # Minimal container
        network_mode = "none"  # No network needed for transformation
      }
      
      # Download input artifact
      artifact {
        source      = "${NOMAD_META_input_url}"
        destination = "local/input.tar"
        mode        = "file"
      }
      
      # Process the transformation
      template {
        data = <<EOF
#!/bin/sh
set -e

# Run transformation
/openrewrite /local/input.tar /local/output.tar ${NOMAD_META_recipe}

# Upload results to SeaweedFS or S3
curl -X POST "${NOMAD_META_output_url}" \
  -F "file=@/local/output.tar" \
  -F "metadata=@/local/output.json"

# Signal completion to Consul
consul kv put "ploy/openrewrite/jobs/{{JOB_ID}}/status" "completed"
consul kv put "ploy/openrewrite/jobs/{{JOB_ID}}/output" "${NOMAD_META_output_url}"
consul kv put "ploy/openrewrite/jobs/{{JOB_ID}}/completed_at" "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
EOF
        destination = "local/transform.sh"
        perms       = "0755"
      }
      
      config {
        command = "/bin/sh"
        args    = ["local/transform.sh"]
      }
      
      env {
        JOB_ID = "{{JOB_ID}}"
        RECIPE = "${NOMAD_META_recipe}"
      }
      
      # Minimal resources for native binary
      resources {
        cpu    = 500   # 0.5 vCPU
        memory = 256   # 256MB (native uses ~50MB)
      }
      
      # Quick timeout for batch processing
      kill_timeout = "30s"
      
      logs {
        max_files     = 1
        max_file_size = 10
      }
    }
  }
  
  # Batch job cleanup
  reschedule {
    attempts  = 3
    interval  = "1m"
    delay     = "10s"
    unlimited = false
  }
  
  # Auto-cleanup after completion
  garbage_collection {
    # Clean up job after 1 hour
    max_age = "1h"
  }
}