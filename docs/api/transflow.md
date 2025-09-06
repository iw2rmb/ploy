# Transflow API Reference

This document describes the REST API endpoints for Ploy's Transflow automated code transformation system.

## Base URL

All API endpoints are available under:
```
https://api.ployd.app/v1/
```

For local development:
```
http://localhost:8080/v1/
```

## Authentication

All API requests require authentication via the `Authorization` header:
```bash
curl -H "Authorization: Bearer your-api-token" \
  https://api.ployd.app/v1/transflows
```

## Transflow Workflows

### Create Transflow Workflow

Create and execute a new transflow workflow.

```http
POST /v1/transflows
```

**Request Body:**
```json
{
  "config": {
    "version": "v1alpha1",
    "id": "java-11-to-17-migration",
    "target_repo": "https://gitlab.com/org/project.git",
    "target_branch": "refs/heads/main",
    "base_ref": "refs/heads/main",
    "lane": "C",
    "build_timeout": "10m",
    "steps": [
      {
        "type": "recipe",
        "id": "java-migration",
        "engine": "openrewrite",
        "recipes": [
          "org.openrewrite.java.migrate.Java11toJava17",
          "org.openrewrite.java.cleanup.CommonStaticAnalysis"
        ]
      }
    ],
    "self_heal": {
      "enabled": true,
      "kb_learning": true,
      "max_retries": 2,
      "cooldown": "30s"
    },
    "llm_model": "gpt-4o-mini@2024-08-06"
  },
  "options": {
    "test_mode": false,
    "dry_run": false,
    "verbose": true
  }
}
```

**Response:**
```json
{
  "id": "tf-abc123def456",
  "status": "running",
  "created_at": "2025-01-09T10:30:00Z",
  "config": { "...": "original config" },
  "progress": {
    "current_step": "java-migration",
    "steps_completed": 0,
    "steps_total": 1
  },
  "urls": {
    "status": "/v1/transflows/tf-abc123def456",
    "logs": "/v1/transflows/tf-abc123def456/logs",
    "cancel": "/v1/transflows/tf-abc123def456/cancel"
  }
}
```

### Get Transflow Status

Get the current status and progress of a transflow workflow.

```http
GET /v1/transflows/{id}
```

**Response:**
```json
{
  "id": "tf-abc123def456",
  "status": "completed",
  "created_at": "2025-01-09T10:30:00Z",
  "completed_at": "2025-01-09T10:37:30Z",
  "duration": "7m30s",
  "config": { "...": "original config" },
  "result": {
    "success": true,
    "mr_url": "https://gitlab.com/org/project/-/merge_requests/42",
    "build_passed": true,
    "healing_summary": {
      "attempts": 1,
      "successful_strategy": "llm-exec",
      "duration": "2m15s"
    }
  },
  "steps": [
    {
      "id": "java-migration",
      "status": "completed",
      "started_at": "2025-01-09T10:30:15Z",
      "completed_at": "2025-01-09T10:35:00Z",
      "duration": "4m45s",
      "changes_applied": 127,
      "files_modified": 23
    }
  ],
  "errors": []
}
```

### List Transflows

Get a list of transflow workflows with filtering and pagination.

```http
GET /v1/transflows?status=completed&limit=20&offset=0&sort=created_at
```

**Query Parameters:**
- `status` (string): Filter by status (`pending`, `running`, `completed`, `failed`, `cancelled`)
- `repo` (string): Filter by repository URL
- `limit` (int): Number of results per page (default: 50, max: 100)
- `offset` (int): Pagination offset (default: 0)
- `sort` (string): Sort field (`created_at`, `completed_at`, `duration`)

**Response:**
```json
{
  "transflows": [
    {
      "id": "tf-abc123def456",
      "status": "completed",
      "created_at": "2025-01-09T10:30:00Z",
      "completed_at": "2025-01-09T10:37:30Z",
      "duration": "7m30s",
      "config": {
        "id": "java-11-to-17-migration",
        "target_repo": "https://gitlab.com/org/project.git"
      },
      "result": {
        "success": true,
        "mr_url": "https://gitlab.com/org/project/-/merge_requests/42"
      }
    }
  ],
  "pagination": {
    "total": 156,
    "limit": 20,
    "offset": 0,
    "has_more": true
  }
}
```

### Cancel Transflow

Cancel a running transflow workflow.

```http
POST /v1/transflows/{id}/cancel
```

**Response:**
```json
{
  "id": "tf-abc123def456",
  "status": "cancelling",
  "message": "Cancellation requested, cleanup in progress"
}
```

### Get Transflow Logs

Stream or retrieve logs from a transflow workflow.

```http
GET /v1/transflows/{id}/logs?follow=true&lines=100
```

**Query Parameters:**
- `follow` (bool): Stream logs in real-time (default: false)
- `lines` (int): Number of recent lines to retrieve (default: all)
- `level` (string): Filter by log level (`debug`, `info`, `warn`, `error`)

**Response (Stream):**
```
2025-01-09T10:30:00Z [INFO] Starting transflow workflow: java-11-to-17-migration
2025-01-09T10:30:05Z [INFO] Cloning repository: https://gitlab.com/org/project.git
2025-01-09T10:30:15Z [INFO] Executing OpenRewrite recipe: Java11toJava17
2025-01-09T10:35:00Z [INFO] Recipe execution completed: 127 changes applied
2025-01-09T10:35:05Z [INFO] Starting build validation
2025-01-09T10:35:45Z [ERROR] Build failed: compilation error in UserService.java:42
2025-01-09T10:35:50Z [INFO] Self-healing enabled, analyzing error
2025-01-09T10:36:10Z [INFO] Generated healing strategy: llm-exec
2025-01-09T10:37:15Z [INFO] Healing successful, build passed
2025-01-09T10:37:30Z [INFO] Workflow completed successfully
```

## Knowledge Base API

### Get Error Information

Get information about a specific error signature.

```http
GET /v1/llms/kb/errors/{signature}
```

**Response:**
```json
{
  "signature": "java-compilation-missing-symbol-abc123",
  "canonical_form": "java.compilation.missing_symbol",
  "first_seen": "2025-01-01T12:00:00Z",
  "last_seen": "2025-01-09T10:35:45Z",
  "occurrences": 23,
  "success_rate": 0.82,
  "avg_healing_time": "2m15s",
  "best_strategies": [
    {
      "strategy": "llm-exec",
      "success_rate": 0.91,
      "avg_time": "1m45s"
    },
    {
      "strategy": "orw-gen", 
      "success_rate": 0.73,
      "avg_time": "3m10s"
    }
  ]
}
```

### Query Healing Cases

Get healing cases for a specific error signature.

```http
GET /v1/llms/kb/errors/{signature}/cases?limit=10&successful_only=true
```

**Response:**
```json
{
  "cases": [
    {
      "id": "case-def789ghi012",
      "error_signature": "java-compilation-missing-symbol-abc123",
      "timestamp": "2025-01-09T10:35:45Z",
      "context": {
        "file": "UserService.java",
        "line": 42,
        "method": "findUser",
        "missing_symbol": "Optional"
      },
      "healing_attempts": [
        {
          "strategy": "llm-exec",
          "success": true,
          "duration": "1m30s",
          "patch_fingerprint": "patch-xyz789",
          "confidence": 0.89
        }
      ],
      "final_result": {
        "success": true,
        "build_passed": true,
        "strategy_used": "llm-exec"
      }
    }
  ]
}
```

### Get KB Statistics

Get overall knowledge base learning statistics.

```http
GET /v1/llms/kb/stats
```

**Response:**
```json
{
  "total_cases": 15420,
  "unique_errors": 342,
  "success_rate": 0.78,
  "avg_healing_time": "2m45s",
  "storage_usage": "2.1GB",
  "cache_hit_ratio": 0.89,
  "learning_trends": {
    "last_30_days": {
      "new_cases": 1247,
      "success_rate": 0.83,
      "improvement": 0.05
    }
  },
  "top_error_types": [
    {
      "type": "java-compilation-missing-symbol",
      "count": 2456,
      "success_rate": 0.91
    },
    {
      "type": "java-compilation-type-mismatch",
      "count": 1834,
      "success_rate": 0.76
    }
  ]
}
```

## Model Registry API

### List Models

Get available LLM models for healing operations.

```http
GET /v1/llms/models?provider=openai&capability=code_generation
```

**Query Parameters:**
- `provider` (string): Filter by provider (`openai`, `anthropic`, `azure`, `local`)
- `capability` (string): Filter by capability (`code_generation`, `error_analysis`, `text_completion`)

**Response:**
```json
{
  "models": [
    {
      "id": "gpt-4o-mini@2024-08-06",
      "name": "GPT-4o Mini",
      "provider": "openai",
      "capabilities": ["code_generation", "error_analysis"],
      "config": {
        "max_tokens": 4096,
        "temperature": 0.1,
        "timeout": "30s"
      },
      "pricing": {
        "input_per_1k_tokens": 0.15,
        "output_per_1k_tokens": 0.60
      },
      "status": "active"
    }
  ]
}
```

### Get Model Details

Get detailed information about a specific model.

```http
GET /v1/llms/models/{model_id}
```

**Response:**
```json
{
  "id": "gpt-4o-mini@2024-08-06",
  "name": "GPT-4o Mini",
  "provider": "openai",
  "capabilities": ["code_generation", "error_analysis"],
  "config": {
    "max_tokens": 4096,
    "temperature": 0.1,
    "timeout": "30s",
    "api_endpoint": "https://api.openai.com/v1/chat/completions"
  },
  "pricing": {
    "input_per_1k_tokens": 0.15,
    "output_per_1k_tokens": 0.60
  },
  "usage_stats": {
    "total_requests": 15420,
    "successful_requests": 14891,
    "avg_response_time": "1.2s",
    "last_used": "2025-01-09T10:37:00Z"
  },
  "status": "active"
}
```

## Configuration API

### Get Transflow Configuration

Get default transflow configuration template.

```http
GET /v1/transflows/config/template?type=java-migration
```

**Response:**
```yaml
version: v1alpha1
id: java-migration-template
target_repo: "https://git.example.com/org/repo.git"
target_branch: refs/heads/main
base_ref: refs/heads/main
build_timeout: 15m

steps:
  - type: recipe
    id: java-migration
    engine: openrewrite
    recipes:
      - org.openrewrite.java.migrate.Java11toJava17
      - org.openrewrite.java.cleanup.CommonStaticAnalysis

self_heal:
  enabled: true
  kb_learning: true
  max_retries: 3
  cooldown: 30s

llm_model: gpt-4o-mini@2024-08-06
```

### Validate Configuration

Validate a transflow configuration without executing it.

```http
POST /v1/transflows/config/validate
```

**Request Body:**
```json
{
  "config": {
    "version": "v1alpha1",
    "id": "test-workflow",
    "...": "configuration to validate"
  }
}
```

**Response:**
```json
{
  "valid": true,
  "warnings": [
    "build_timeout is longer than recommended (20m > 15m)"
  ],
  "errors": [],
  "suggestions": [
    {
      "field": "self_heal.max_retries",
      "message": "Consider reducing max_retries to 2 for faster failure detection",
      "severity": "info"
    }
  ]
}
```

## Health and Monitoring

### System Health

Get overall system health status.

```http
GET /v1/health
```

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2025-01-09T10:40:00Z",
  "components": {
    "transflow_runner": {
      "status": "healthy",
      "active_workflows": 3,
      "queue_length": 0
    },
    "knowledge_base": {
      "status": "healthy",
      "storage_connection": "ok",
      "cache_hit_ratio": 0.89
    },
    "model_registry": {
      "status": "healthy",
      "available_models": 12,
      "active_providers": 3
    },
    "dependencies": {
      "consul": "healthy",
      "nomad": "healthy", 
      "seaweedfs": "healthy",
      "gitlab": "healthy"
    }
  }
}
```

### Metrics

Get system metrics for monitoring.

```http
GET /v1/metrics
```

**Response (Prometheus format):**
```
# HELP transflow_workflows_total Total number of transflow workflows
# TYPE transflow_workflows_total counter
transflow_workflows_total{status="completed"} 1247
transflow_workflows_total{status="failed"} 156
transflow_workflows_total{status="running"} 3

# HELP transflow_healing_attempts_total Total number of healing attempts
# TYPE transflow_healing_attempts_total counter
transflow_healing_attempts_total{strategy="llm-exec",result="success"} 891
transflow_healing_attempts_total{strategy="human-step",result="success"} 234

# HELP kb_learning_cases_total Total KB learning cases
# TYPE kb_learning_cases_total counter
kb_learning_cases_total{error_type="java-compilation"} 3456
kb_learning_cases_total{error_type="build-failure"} 1234
```

## Error Responses

All API endpoints use consistent error response format:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid configuration: missing required field 'target_repo'",
    "details": {
      "field": "target_repo",
      "required": true
    },
    "timestamp": "2025-01-09T10:30:00Z",
    "request_id": "req-abc123def456"
  }
}
```

### Common Error Codes

- `VALIDATION_ERROR` (400): Invalid request data
- `AUTHENTICATION_ERROR` (401): Invalid or missing authentication
- `AUTHORIZATION_ERROR` (403): Insufficient permissions
- `NOT_FOUND` (404): Resource not found
- `CONFLICT` (409): Resource conflict (e.g., duplicate workflow ID)
- `RATE_LIMIT_EXCEEDED` (429): Too many requests
- `INTERNAL_ERROR` (500): Server error
- `SERVICE_UNAVAILABLE` (503): Service temporarily unavailable

## Rate Limiting

API requests are subject to rate limiting:

- **Default**: 1000 requests per hour per API key
- **Burst**: 100 requests per minute
- **Headers**: Rate limit information in response headers

```http
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 847
X-RateLimit-Reset: 1641724800
```

## Webhooks

Configure webhooks to receive real-time notifications about transflow events.

### Webhook Events

- `transflow.started`: Workflow execution started
- `transflow.step.completed`: Workflow step completed
- `transflow.healing.started`: Self-healing process started
- `transflow.healing.completed`: Self-healing process completed
- `transflow.completed`: Workflow execution completed
- `transflow.failed`: Workflow execution failed

### Webhook Payload

```json
{
  "event": "transflow.completed",
  "timestamp": "2025-01-09T10:37:30Z",
  "data": {
    "workflow_id": "tf-abc123def456",
    "status": "completed",
    "result": {
      "success": true,
      "mr_url": "https://gitlab.com/org/project/-/merge_requests/42"
    }
  }
}
```