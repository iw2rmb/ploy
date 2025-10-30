# Observability

Ploy Next publishes Prometheus metrics directly from the control plane. Operators can
scrape the `/metrics` endpoint on every control-plane instance and wire alerts using
these collectors.

## Prometheus Endpoint

- **Path**: `/metrics`
- **Handler**: native `promhttp` served by the control-plane HTTP API.
- **Authentication**: none (scope the listener to the cluster network or front it with
  an ingress proxy if exposure beyond trusted networks is required).

### Sample Scrape Configuration

```yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: ploy-control-plane
    metrics_path: /metrics
    scheme: https
    tls_config:
      insecure_skip_verify: false
    static_configs:
      - targets:
          - control-plane-1.example.com:9443
          - control-plane-2.example.com:9443
    relabel_configs:
      - source_labels: [__address__]
        target_label: instance
```

Adjust the TLS configuration to match the deployment. If the control plane runs behind
an ingress controller, use the ingress host/port instead of the node endpoints.

## Control Plane Metrics

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `ploy_controlplane_queue_depth` | Gauge | `priority` | Number of queued jobs per priority band. |
| `ploy_controlplane_claim_latency_seconds` | Histogram | `priority` | Time between enqueue and successful claim. |
| `ploy_controlplane_job_retry_total` | Counter | `priority`, `reason` | Count of scheduler-triggered retries (e.g. `reason="lease_expired"`). |
| `ploy_controlplane_shift_duration_seconds` | Histogram | `step_id`, `result` | Measured SHIFT execution duration for each job step. `result` normalises to `passed` / `failed` / `unknown`. |

### SHIFT Duration Semantics

SHIFT duration is captured by the runtime as the elapsed wall time for the SHIFT build
gate invocation. The scheduler stores the latest duration per job attempt and feeds the
`ploy_controlplane_shift_duration_seconds` histogram. Failed runs continue to report the
observed duration with `result="failed"`, enabling alerting on slow failures or timeouts.

## Sample Alert Rules

```yaml
groups:
  - name: ploy-control-plane
    rules:
      - alert: PloyQueueDepthHigh
        expr: ploy_controlplane_queue_depth{priority="default"} > 50
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: High queue depth in default priority
          description: |
            Queue depth has been above 50 for more than 10 minutes.
            Consider scaling schedulers or diagnosing stalled nodes.

      - alert: PloyShiftDurationP99Slow
        expr: histogram_quantile(0.99, rate(ploy_controlplane_shift_duration_seconds_bucket[10m])) > 180
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: SHIFT p99 duration exceeded 3 minutes
          description: |
            The 99th percentile SHIFT duration has stayed above 3 minutes for at least 5 minutes.
            Investigate node saturation or failing build gate environments.

      - alert: PloyJobRetriesBurst
        expr: increase(ploy_controlplane_job_retry_total[15m]) > 25
        labels:
          severity: warning
        annotations:
          summary: Elevated scheduler retries detected
          description: |
            More than 25 scheduler retries occurred within 15 minutes.
            Check node heartbeat health and etcd latency.
```

Validate rule files with `promtool check rules` before deploying them to production.

## GC and Retention Metrics

The control plane forwards GC and retention state through the same `/metrics` surface.
When [`docs/design/gc-audit-metrics/README.md`](../design/gc-audit-metrics/README.md) ships,
register those collectors alongside the scheduler metrics so operators get a unified
Prometheus scrape. Document any additional GC gauges or counters in this file when that
slice lands.

## Operational Checklist

1. Expose `/metrics` on every control-plane instance.
2. Configure Prometheus (or your chosen scraper) using the sample configuration above.
3. Import the alert examples and adjust thresholds for your workload.
4. Run `promtool check rules` on every change to the alert ruleset.
5. After deploying GC metrics, update this document and ensure they appear in the same
   scrape target for holistic dashboards.
```
