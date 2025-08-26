# OpenRewrite Service API Specification

## Endpoints

### POST /transform
Start a new transformation job.

#### Request
```json
{
  "job_id": "arf-bench-1234567890",  // Client-provided unique ID
  "recipe_id": "java11to17_migration",
  "recipe_config": {
    "recipe": "org.openrewrite.java.migrate.UpgradeToJava17",
    "artifacts": "org.openrewrite.recipe:rewrite-migrate-java:3.15.0",
    "options": {
      "addJava17Features": true,
      "updateBuildFiles": true
    }
  },
  "tar_archive": "<base64-encoded-tar>",
  "metadata": {
    "client": "benchmark-manager",
    "priority": "normal",
    "dry_run": false
  }
}
```

#### Response (202 Accepted)
```json
{
  "job_id": "arf-bench-1234567890",
  "status_url": "consul://ploy/openrewrite/jobs/arf-bench-1234567890",
  "message": "Transformation job queued",
  "estimated_wait_time": 15,  // seconds
  "queue_position": 3
}
```

#### Error Response (400 Bad Request)
```json
{
  "error": "invalid_request",
  "message": "Recipe ID not found in storage",
  "details": {
    "recipe_id": "java11to17_migration",
    "available_recipes": ["java8to11", "java11to17", "spring_boot_3"]
  }
}
```

#### Error Response (429 Too Many Requests)
```json
{
  "error": "rate_limit_exceeded",
  "message": "Client has exceeded rate limit",
  "retry_after": 3600,  // seconds
  "limits": {
    "concurrent": 10,
    "hourly": 100
  }
}
```

### GET /health
Check service health and readiness.

#### Response (200 OK)
```json
{
  "status": "healthy",
  "instance_id": "openrewrite-abc123",
  "version": "1.0.0",
  "uptime_seconds": 3600,
  "last_activity": "2024-01-26T10:00:00Z",
  "checks": {
    "consul": "healthy",
    "seaweedfs": "healthy",
    "maven": "healthy",
    "disk_space": "healthy"
  },
  "metrics": {
    "jobs_processing": 3,
    "jobs_queued": 12,
    "jobs_completed_1h": 45,
    "jobs_failed_1h": 2,
    "avg_duration_seconds": 120
  }
}
```

## Consul KV Status Structure

### Path Convention
`/ploy/openrewrite/jobs/{job_id}`

### Status: Queued
```json
{
  "job_id": "arf-bench-1234567890",
  "status": "queued",
  "recipe_id": "java11to17_migration",
  "created_at": "2024-01-26T10:00:00Z",
  "instance_id": null,
  "priority": "normal",
  "metadata": {
    "client": "benchmark-manager",
    "tar_size_bytes": 156789,
    "recipe_complexity": "high"
  }
}
```

### Status: Running
```json
{
  "job_id": "arf-bench-1234567890",
  "status": "running",
  "recipe_id": "java11to17_migration",
  "created_at": "2024-01-26T10:00:00Z",
  "started_at": "2024-01-26T10:00:05Z",
  "instance_id": "openrewrite-abc123",
  "progress": 45,
  "current_step": "Applying recipe",
  "steps_completed": [
    {
      "name": "git_init",
      "duration_ms": 230
    },
    {
      "name": "maven_setup",
      "duration_ms": 1500
    }
  ],
  "estimated_completion": "2024-01-26T10:02:35Z"
}
```

### Status: Completed
```json
{
  "job_id": "arf-bench-1234567890",
  "status": "completed",
  "recipe_id": "java11to17_migration",
  "created_at": "2024-01-26T10:00:00Z",
  "started_at": "2024-01-26T10:00:05Z",
  "completed_at": "2024-01-26T10:02:35Z",
  "duration_seconds": 150,
  "instance_id": "openrewrite-abc123",
  "diff_url": "seaweedfs://ploy-diffs/arf-bench-1234567890.diff.gz",
  "diff_size_bytes": 4521,
  "diff_sha256": "abc123def456...",
  "statistics": {
    "files_changed": 23,
    "insertions": 145,
    "deletions": 89,
    "files_added": 2,
    "files_removed": 0,
    "build_system": "maven",
    "java_version_before": "11",
    "java_version_after": "17"
  },
  "recipe_results": {
    "recipes_applied": 15,
    "recipes_skipped": 3,
    "warnings": [
      "Deprecated API usage in UserService.java:45"
    ]
  },
  "performance_metrics": {
    "git_init_ms": 230,
    "recipe_download_ms": 890,
    "transformation_ms": 148000,
    "diff_generation_ms": 1200,
    "upload_ms": 350
  }
}
```

### Status: Failed
```json
{
  "job_id": "arf-bench-1234567890",
  "status": "failed",
  "recipe_id": "java11to17_migration",
  "created_at": "2024-01-26T10:00:00Z",
  "started_at": "2024-01-26T10:00:05Z",
  "failed_at": "2024-01-26T10:00:45Z",
  "instance_id": "openrewrite-abc123",
  "error": "Recipe execution failed",
  "error_code": "RECIPE_EXECUTION_ERROR",
  "error_details": {
    "phase": "maven_execution",
    "command": "mvn org.openrewrite.maven:rewrite-maven-plugin:5.34.0:run",
    "exit_code": 1,
    "stdout_tail": "...",
    "stderr_tail": "Could not resolve dependencies..."
  },
  "retry_attempt": 2,
  "retryable": true,
  "suggested_action": "Check Maven repository configuration"
}
```

## Client Integration Examples

### Go Client
```go
// Initialize client
client := openrewrite.NewClient(
    openrewrite.WithConsul("consul:8500"),
    openrewrite.WithSeaweedFS("seaweedfs:9333"),
    openrewrite.WithTimeout(5 * time.Minute),
)

// Submit transformation
job, err := client.Transform(ctx, openrewrite.TransformRequest{
    JobID:      uuid.New().String(),
    RecipeID:   "java11to17",
    TarArchive: tarData,
    Options: map[string]interface{}{
        "dry_run": false,
        "priority": "high",
    },
})

// Poll for completion
result, err := client.WaitForCompletion(ctx, job.ID,
    openrewrite.WithPollingInterval(2 * time.Second),
    openrewrite.WithProgressCallback(func(progress int) {
        log.Printf("Progress: %d%%", progress)
    }),
)

// Download diff
diff, err := client.DownloadDiff(ctx, result.DiffURL)
```

### Direct Consul Polling
```go
// Poll Consul KV directly
consulClient, _ := consul.NewClient(consul.DefaultConfig())
kv := consulClient.KV()

for {
    pair, _, err := kv.Get(fmt.Sprintf("ploy/openrewrite/jobs/%s", jobID), nil)
    if err != nil {
        return err
    }
    
    var status JobStatus
    json.Unmarshal(pair.Value, &status)
    
    switch status.Status {
    case "completed":
        // Download from SeaweedFS
        diff := seaweedfs.Download(status.DiffURL)
        return diff
    case "failed":
        return fmt.Errorf("Job failed: %s", status.Error)
    case "running":
        log.Printf("Progress: %d%%", status.Progress)
    }
    
    time.Sleep(2 * time.Second)
}
```

## WebSocket Progress Streaming (Future)
```javascript
const ws = new WebSocket('wss://openrewrite.service/ws/jobs/abc123');

ws.on('message', (data) => {
  const event = JSON.parse(data);
  switch(event.type) {
    case 'progress':
      console.log(`Progress: ${event.progress}%`);
      break;
    case 'step_completed':
      console.log(`Completed: ${event.step_name}`);
      break;
    case 'completed':
      console.log(`Diff URL: ${event.diff_url}`);
      break;
    case 'failed':
      console.error(`Failed: ${event.error}`);
      break;
  }
});
```