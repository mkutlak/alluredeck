# Observability — Operator Reference

AllureDeck exposes OpenTelemetry traces, Prometheus-scrapeable metrics, and trace-correlated structured logs when `observability.enabled: true`.

---

## Overview

| Signal | Transport | Consumer |
|---|---|---|
| **Traces** | OTLP HTTP or gRPC push | Tempo, Jaeger, OTLP-compatible collector |
| **Metrics** | Prometheus scrape (`/metrics`) | Prometheus, Thanos, VictoriaMetrics |
| **Logs** | Zap JSON to stdout, with `trace_id`/`span_id` fields | Loki, any log shipper |

### What gets instrumented

- **HTTP requests** — every inbound request gets a span with route pattern (e.g. `GET /api/v1/projects/{id}`), method, status code, and duration. Metrics: `http.server.request.duration` (histogram), `http.server.active_requests` (gauge).
- **PostgreSQL queries** — every pgx query gets a span with `db.statement` (truncated to 1 KB), `db.operation`, pool utilisation. Metrics: `db.client.operation.duration` (histogram), `db.client.connections.usage` (gauge).
- **River background jobs** — every job execution gets a span linked to the originating request span. Metrics: `river.job.duration.seconds` (histogram by kind), `river.jobs.completed.total` (by kind + outcome), `river.queue.depth` (gauge).
- **Outbound HTTP** — S3 operations, webhook delivery, and OIDC token exchange are each wrapped with `otelhttp.NewTransport`, producing child spans.
- **Go runtime** — `go.goroutines`, `go.memory.used`, `go.gc.duration` and related runtime metrics emitted automatically.

### Trace propagation

The W3C **tracecontext** and **baggage** propagators are configured. Incoming `traceparent` / `tracestate` / `baggage` headers are respected, so AllureDeck participates in distributed traces initiated by an upstream service or API gateway.

---

## Quick Start

Minimum Helm values to enable observability:

```yaml
observability:
  enabled: true
  traces:
    endpoint: "http://otel-collector.observability.svc:4318"
  serviceMonitor:
    enabled: true          # requires Prometheus Operator
    labels:
      prometheus: kube-prometheus   # match your Prometheus serviceMonitorSelector
```

With these values:

1. The API container exposes port **9464** for Prometheus scrapes.
2. A dedicated `ClusterIP` Service (`<release>-alluredeck-api-metrics`) is created targeting port 9464.
3. A `ServiceMonitor` resource is created so Prometheus Operator discovers the endpoint automatically.
4. Traces are pushed to the OTLP collector at the endpoint you specified.

Metrics only (no traces):

```yaml
observability:
  enabled: true
  traces:
    endpoint: ""           # leave empty — no traces exported
  serviceMonitor:
    enabled: true
```

---

## Configuration Reference

All values live under the `observability:` key in `values.yaml`.

### Top-level

| Value | Type | Default | Description |
|---|---|---|---|
| `observability.enabled` | bool | `false` | Master switch. When `false`, no metrics port, no ServiceMonitor, no OTEL env vars are applied. |
| `observability.serviceName` | string | `"alluredeck-api"` | `service.name` resource attribute on all spans and metrics. Maps to `OTEL_SERVICE_NAME`. |
| `observability.environment` | string | `""` | `deployment.environment` resource attribute (e.g. `"production"`). Maps to `OTEL_RESOURCE_ATTRIBUTES=deployment.environment=<value>`. Omitted when empty. |

### `observability.traces`

| Value | Type | Default | Env var |
|---|---|---|---|
| `traces.endpoint` | string | `""` | `OTEL_EXPORTER_OTLP_ENDPOINT` |
| `traces.protocol` | string | `"http/protobuf"` | `OTEL_EXPORTER_OTLP_PROTOCOL` |
| `traces.sampleRatio` | float | `1.0` | `OTEL_TRACES_SAMPLER_ARG` (used with `parentbased_traceidratio`) |
| `traces.insecure` | bool | `false` | `OTEL_EXPORTER_OTLP_INSECURE` |

`traces.protocol` accepts `"http/protobuf"` (default, port 4318) or `"grpc"` (port 4317).

Set `traces.endpoint: ""` to disable trace export while keeping metrics.

### `observability.metrics`

| Value | Type | Default | Description |
|---|---|---|---|
| `metrics.enabled` | bool | `true` | Expose the `/metrics` endpoint and create the metrics Service. Requires `observability.enabled: true`. |
| `metrics.port` | int | `9464` | Container port and Service port for the Prometheus scrape target. |
| `metrics.path` | string | `"/metrics"` | HTTP path scraped by Prometheus. |

### `observability.serviceMonitor`

| Value | Type | Default | Description |
|---|---|---|---|
| `serviceMonitor.enabled` | bool | `false` | Create a Prometheus Operator `ServiceMonitor` resource. Requires `metrics.enabled: true`. |
| `serviceMonitor.interval` | string | `"30s"` | Prometheus scrape interval. |
| `serviceMonitor.scrapeTimeout` | string | `"10s"` | Maximum time to wait for a scrape response. |
| `serviceMonitor.labels` | map | `{}` | Extra labels on the `ServiceMonitor` metadata — use to match your `Prometheus.serviceMonitorSelector`. |
| `serviceMonitor.relabelings` | list | `[]` | `relabelings` block passed verbatim to the ServiceMonitor endpoint. |
| `serviceMonitor.metricRelabelings` | list | `[]` | `metricRelabelings` block passed verbatim to the ServiceMonitor endpoint. |

---

## Sample PromQL

The metric names below follow the OpenTelemetry semantic conventions as exported by the Prometheus exporter (dots replaced with underscores, `_total` suffix for counters).

### p99 request latency by route

```promql
histogram_quantile(
  0.99,
  sum by (le, http_route, http_request_method) (
    rate(http_server_request_duration_seconds_bucket[5m])
  )
)
```

### HTTP error rate (5xx) by route

```promql
sum by (http_route) (
  rate(http_server_request_duration_seconds_count{http_response_status_code=~"5.."}[5m])
)
/
sum by (http_route) (
  rate(http_server_request_duration_seconds_count[5m])
)
```

### Database connection pool saturation

```promql
db_client_connections_usage{state="used"}
  /
(db_client_connections_usage{state="used"} + db_client_connections_usage{state="idle"})
```

### River queue depth (pending jobs)

```promql
river_queue_depth{queue="default"}
```

### p95 background job duration by kind

```promql
histogram_quantile(
  0.95,
  sum by (le, job_kind) (
    rate(river_job_duration_seconds_bucket[10m])
  )
)
```

### Slowest job kinds (mean duration, last 1 h)

```promql
sum by (job_kind) (rate(river_job_duration_seconds_sum[1h]))
  /
sum by (job_kind) (rate(river_job_duration_seconds_count[1h]))
```

---

## Trace Propagation

AllureDeck configures two W3C propagators:

- **tracecontext** (`traceparent` / `tracestate` headers) — carries the trace and span IDs across service boundaries.
- **baggage** — carries key/value metadata (e.g. `user_id`, `project_id`) that flows through the entire distributed trace.

If your ingress controller or API gateway injects `traceparent`, AllureDeck will continue the existing trace rather than starting a new root span. All child spans (pgx queries, S3 operations, River jobs) chain under the same root.

Log lines emitted inside a traced request carry `"trace_id"` and `"span_id"` JSON fields, enabling log-to-trace correlation in Grafana (click a log line → jump to Tempo).

---

## Troubleshooting

### No traces appearing in the collector

1. Confirm `observability.enabled: true` and `observability.traces.endpoint` is set to a reachable OTLP endpoint.
2. Check `observability.traces.sampleRatio` — a value of `0.0` disables all sampling.
3. Verify network connectivity from the API pod: `kubectl exec -n <ns> <pod> -- wget -qO- <endpoint>/health` (or equivalent).
4. Check collector logs for rejected or dropped spans; the most common cause is a mismatched protocol (`http/protobuf` vs `grpc`).
5. If using `traces.insecure: false` with a self-signed certificate, either add the CA to the pod's trust store or set `insecure: true` for internal collectors.

### ServiceMonitor not picked up by Prometheus

1. Confirm `observability.serviceMonitor.enabled: true` and `observability.metrics.enabled: true`.
2. Verify the `serviceMonitor.labels` map matches your `Prometheus` CR's `serviceMonitorSelector`. For kube-prometheus-stack the default selector is `release: <helm-release-name>`.
3. Run `kubectl get servicemonitor -n <ns> <release>-alluredeck-api` and confirm it exists.
4. Check `kubectl describe prometheusrule` or the Prometheus Operator logs for selector mismatches.

### Metrics endpoint returns connection refused

1. Confirm `observability.enabled: true` and `observability.metrics.enabled: true` — these gates must both be true for the port to be opened and the Service to exist.
2. Verify the pod is healthy: `kubectl exec -n <ns> <pod> -- wget -qO- http://localhost:9464/metrics`.
3. Check that the `service-metrics` Service was created: `kubectl get svc -n <ns> <release>-alluredeck-api-metrics`.

### Sampler set to 0 — no spans recorded

`observability.traces.sampleRatio: 0.0` is equivalent to turning off tracing while keeping the metrics endpoint alive. Set it to `1.0` for 100 % sampling (recommended in dev/staging) or a value between `0.0` and `1.0` for production rate limiting.
