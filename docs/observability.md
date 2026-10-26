# Observability — Operator Reference

AllureDeck exposes OpenTelemetry traces, Prometheus-scrapeable metrics, and trace-correlated structured logs when `observability.enabled: true` (Helm) or `OBSERVABILITY_ENABLED=true` (environment). The environment variables are listed in [configuration.md](configuration.md#observability-opentelemetry-and-prometheus); this page covers the Helm values, the signals, and operations. Both the API and the [MCP server](operations/mcp-server.md) emit them.

---

## Overview

| Signal | Transport | Consumer |
|---|---|---|
| **Traces** | OTLP HTTP or gRPC push | Tempo, Jaeger, OTLP-compatible collector |
| **Metrics** | Prometheus scrape (`/metrics`) | Prometheus, Thanos, VictoriaMetrics |
| **Logs** | Zap JSON to stdout, with `trace_id`/`span_id` fields | Loki, any log shipper |

### What gets instrumented

- **HTTP requests** — every inbound request gets a span named after the method and request path (e.g. `GET /api/v1/projects/42/builds` — the concrete path, not the route template), with status code and duration. Metrics: `http_server_request_duration_seconds` plus request and response body-size histograms, labelled by method and status code. There is no route label.
- **PostgreSQL queries** — every pgx query gets a span carrying the SQL statement (bind parameters are not recorded). Metrics: `db_client_operation_duration_seconds` (histogram, labelled `pgx_operation_type`) and `db_client_operation_errors_total`. Connection-pool gauges are not exported.
- **River background jobs** — every job execution gets a `river.job.work/<kind>` span (attributes: kind, queue, attempt) chained to the request that enqueued it. The only job metric is `aldeck_jobs_failed_total` (see [Job failure metric](#job-failure-metric)).
- **Outbound HTTP** — webhook delivery and OIDC token exchange are each wrapped with `otelhttp.NewTransport`, producing child spans. S3 operations get child spans from the AWS SDK's native tracing (`smithyoteltracing`).
- **Go runtime and process** — `go_goroutines`, `go_memory_used_bytes`, `go_gc_duration_seconds`, `process_resident_memory_bytes`, `process_open_fds`, and related series.

### Trace propagation

The W3C **tracecontext** and **baggage** propagators are configured. Incoming `traceparent` / `tracestate` / `baggage` headers are respected, so AllureDeck participates in distributed traces initiated by an upstream service or API gateway.

---

## Quick Start

Minimum Helm values to enable observability:

```yaml
observability:
  enabled: true
  traces:
    protocol: grpc                                       # see "Known limitation" below before using http/protobuf
    endpoint: "otel-collector.observability.svc:4317"   # host:port, no scheme
    insecure: true                                       # collector speaks plain gRPC (no TLS)
  serviceMonitor:
    enabled: true          # requires Prometheus Operator
    labels:
      prometheus: kube-prometheus   # match your Prometheus serviceMonitorSelector
```

With these values:

1. The API container exposes port **9464** for Prometheus scrapes.
2. A dedicated `ClusterIP` Service (`<fullname>-api-metrics`) is created targeting port 9464. `<fullname>` is the release name when it already contains `alluredeck` (release `alluredeck` gives `alluredeck-api-metrics`), otherwise `<release>-alluredeck`.
3. A `ServiceMonitor` resource is created so Prometheus Operator discovers the endpoint automatically.
4. Traces are pushed to the OTLP collector at the endpoint you specified.

Metrics only (no traces): leave `traces.endpoint` empty and sample (almost) nothing. An empty endpoint does **not** turn export off — the exporter then falls back to its default, `localhost:4318` for `http/protobuf` or `localhost:4317` for `grpc`.

```yaml
observability:
  enabled: true
  traces:
    sampleRatio: 0.0001    # Helm renders 0 as 1 (see the note under observability.traces)
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
| `observability.serviceName` | string | `"alluredeck-api"` | `service.name` resource attribute on all spans and metrics. Maps to `OBSERVABILITY_SERVICE_NAME`. The MCP pod reads the same value; set it to `""` to get `alluredeck-api` on the API and `alluredeck-mcp` on the MCP server. |
| `observability.environment` | string | `""` | `deployment.environment.name` resource attribute (e.g. `"production"`). Maps to `OBSERVABILITY_ENVIRONMENT`, which the chart omits when the value is empty; the attribute is then still present with an empty value. |

### `observability.traces`

| Value | Type | Default | Env var |
|---|---|---|---|
| `traces.endpoint` | string | `""` | `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` (set only when non-empty) |
| `traces.protocol` | string | `"http/protobuf"` | `OTEL_EXPORTER_OTLP_TRACES_PROTOCOL` |
| `traces.sampleRatio` | float | `1.0` | `OTEL_TRACES_SAMPLER_ARG` (parent-based trace-ID ratio sampler) |
| `traces.insecure` | bool | `false` | `OBSERVABILITY_TRACES_INSECURE` (set only when `true`) |

`traces.endpoint` is `host:port` **without a scheme or path** (`otel-collector:4317`). A value such as `http://otel-collector:4318` is not parsed as a URL by the exporter; it builds an invalid address from it, logs `invalid port` errors, and delivers no spans. With the default `traces.insecure: false` the exporter uses TLS; set `insecure: true` for a plain-HTTP/plain-gRPC in-cluster collector.

`traces.protocol` accepts `"http/protobuf"` (default, port 4318) or `"grpc"` (port 4317).

#### Known limitation: `http/protobuf` with the environment variable (Helm)

Besides AllureDeck's own handling, the OpenTelemetry SDK reads `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` itself. For a **hostname** such as `otel-collector:4318`, the SDK parses the value as a URL with the scheme `otel-collector`, ends up with the request path `/`, and AllureDeck only replaces the host. Result: spans are POSTed to `/` instead of `/v1/traces`, and the collector rejects them. The Helm chart always sets that environment variable, so the problem affects every Helm install that uses `http/protobuf` with a hostname. This was reproduced against the pinned exporter (`otlptracehttp` v1.47.0) with a local sink, calling the exporter the same way AllureDeck does (`WithEndpoint` plus `WithInsecure`):

| Setup | Spans delivered to |
|---|---|
| `grpc`, `host:4317`, from the environment variable, YAML, or Helm | the collector (works) |
| `http/protobuf`, `observability.traces.endpoint` set in `config.yaml`, environment variable **not** set | `POST /v1/traces` (works) |
| `http/protobuf`, environment variable or Helm, IP literal (`10.0.0.5:4318`) | `POST /v1/traces` (works) |
| `http/protobuf`, environment variable or Helm, hostname (`otel-collector:4318`) | `POST /` (**not delivered**) |
| any protocol, value with a scheme (`http://…`) | nothing (invalid address) |

Until this is fixed in the code, use `protocol: grpc` (port 4317) under Helm. The chart does not write `observability.*` into the API ConfigMap, so the YAML route is only available when you mount your own `config.yaml`.

An empty `traces.endpoint` is not "off": the exporter falls back to its default (`localhost:4318` for `http/protobuf`, `localhost:4317` for `grpc`). To keep metrics but stop tracing, lower `traces.sampleRatio`. Helm's `default` function treats `0` as unset, so `sampleRatio: 0` renders as `1`; use a tiny ratio such as `0.0001`, or pass `OTEL_TRACES_SAMPLER_ARG=0` through `api.extraEnv`.

### `observability.metrics`

| Value | Type | Default | Description |
|---|---|---|---|
| `metrics.enabled` | bool | `true` | Declare the metrics container port and create the metrics Service. Requires `observability.enabled: true`. The chart always renders `OBSERVABILITY_METRICS_ENABLED=true`, so `false` only removes the port, Service, and ServiceMonitor — the listener still starts inside the pod. |
| `metrics.port` | int | `9464` | Container port and Service port for the Prometheus scrape target. Maps to `OBSERVABILITY_METRICS_ADDR` (`:<port>`). |
| `metrics.path` | string | `"/metrics"` | HTTP path scraped by Prometheus. Maps to `OBSERVABILITY_METRICS_PATH`. |

The chart has no value for pprof; see [pprof](#pprof).

### MCP pod

When `observability.enabled` and `mcp.enabled` are both true, the MCP deployment receives `OBSERVABILITY_ENABLED`, `OBSERVABILITY_SERVICE_NAME`, `OBSERVABILITY_ENVIRONMENT`, `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`, `OTEL_EXPORTER_OTLP_TRACES_PROTOCOL`, `OTEL_TRACES_SAMPLER_ARG`, and `OBSERVABILITY_TRACES_INSECURE` from the same `observability.*` values, so its traces go to the same collector. The chart creates no metrics port, Service, or ServiceMonitor for the MCP pod. Its liveness, readiness, and startup probes use `GET /health` on port 8081.

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

### p99 request latency by method

```promql
histogram_quantile(
  0.99,
  sum by (le, http_request_method) (
    rate(http_server_request_duration_seconds_bucket[5m])
  )
)
```

### HTTP error rate (5xx)

```promql
sum(
  rate(http_server_request_duration_seconds_count{http_response_status_code=~"5.."}[5m])
)
/
sum(
  rate(http_server_request_duration_seconds_count[5m])
)
```

### p99 database operation latency by operation type

```promql
histogram_quantile(
  0.99,
  sum by (le, pgx_operation_type) (
    rate(db_client_operation_duration_seconds_bucket[5m])
  )
)
```

`pgx_operation_type="acquire"` measures the wait for a pooled connection, so a rising value means pool exhaustion.

### Database operation errors

```promql
sum(rate(db_client_operation_errors_total[5m]))
```

### Jobs that exhausted their retries (last hour)

```promql
sum by (kind) (increase(aldeck_jobs_failed_total[1h])) > 0
```

### Job failure metric

`aldeck_jobs_failed_total` is a counter labelled `kind` (the River job kind, such as `generate_report`). It increments when a job fails its final attempt, the same moment AllureDeck queues a `report_failed` webhook for the project. The series does not exist until the first such failure, so alerts should tolerate an absent series. Retried failures that later succeed are not counted. Job state and manual retry live in the System Monitor page and `POST /api/v1/admin/jobs/{job_id}/retry`.

---

## Trace Propagation

AllureDeck configures two W3C propagators:

- **tracecontext** (`traceparent` / `tracestate` headers) — carries the trace and span IDs across service boundaries.
- **baggage** — carries key/value metadata that flows through the entire distributed trace.

If your ingress controller or API gateway injects `traceparent`, AllureDeck will continue the existing trace rather than starting a new root span. All child spans (pgx queries, S3 operations, River jobs) chain under the same root.

Log lines emitted inside a traced request carry `"trace_id"` and `"span_id"` JSON fields, enabling log-to-trace correlation in Grafana (click a log line → jump to Tempo).

---

## Health Endpoints

Served on the API port (not the metrics port):

| Endpoint | Behavior |
|---|---|
| `GET /health` (alias `/healthz`) | Liveness. Returns `200 {"status":"ok"}` as soon as the listener is bound — before slow startup work finishes — and checks no dependencies. |
| `GET /ready` (alias `/readyz`) | Readiness. Returns `503` with `"bootstrap":"pending"` until startup bootstrap completes, then checks the database, storage backend, and job queue. Responds `200` only when all are healthy; otherwise `503`. |

Example `/ready` body:

```json
{"bootstrap":"ok","db":"ok","queue":"ok","status":"ok","storage":"ok"}
```

Each dependency reports `ok`, `error`, or `skipped` (not wired), so the body shows which one is degraded. The Helm chart wires `/health` to the liveness and startup probes and `/ready` to the readiness probe.

---

## pprof

Go profiling is off by default. Set `OBSERVABILITY_METRICS_ENABLE_PROFILING=true` to serve `/debug/pprof/` on the metrics server, next to `/metrics`. Besides `cmdline`, `profile`, `symbol`, and `trace`, the index handler also exposes the named profiles (`heap`, `goroutine`, `allocs`, `block`, `mutex`, `threadcreate`). It needs `OBSERVABILITY_ENABLED=true` and the metrics server running, and it has **no authentication** — never expose the metrics port publicly.

The chart has no value for it. Pass it through `api.extraEnv`:

```yaml
api:
  extraEnv:
    - name: OBSERVABILITY_METRICS_ENABLE_PROFILING
      value: "true"
```

Then profile through the metrics Service:

```bash
kubectl port-forward svc/<fullname>-api-metrics 9464:9464
go tool pprof "http://localhost:9464/debug/pprof/profile?seconds=30"
```

---

## Troubleshooting

### No traces appearing in the collector

1. Confirm `observability.enabled: true` and `observability.traces.endpoint` is a reachable `host:port` with no scheme. A value like `http://collector:4318` produces an `invalid port` export error in the API log and no spans arrive. On Helm with `http/protobuf` and a hostname, spans are POSTed to `/` and rejected — switch to `grpc` (see [Known limitation](#known-limitation-httpprotobuf-with-the-environment-variable-helm)).
2. Check `observability.traces.sampleRatio` — a very small ratio samples almost nothing.
3. Verify network connectivity from the API pod to the collector host and port.
4. Check collector logs for rejected or dropped spans; the most common cause is a mismatched protocol (`http/protobuf` vs `grpc`).
5. `traces.insecure: false` (default) makes the exporter use TLS. For a plain-HTTP in-cluster collector set `insecure: true`; for a TLS collector with a self-signed certificate add the CA to the pod's trust store.

### ServiceMonitor not picked up by Prometheus

1. Confirm `observability.serviceMonitor.enabled: true` and `observability.metrics.enabled: true`.
2. Verify the `serviceMonitor.labels` map matches your `Prometheus` CR's `serviceMonitorSelector`. For kube-prometheus-stack the default selector is `release: <helm-release-name>`.
3. Run `kubectl get servicemonitor -n <ns> <fullname>-api` and confirm it exists.
4. Check `kubectl describe prometheusrule` or the Prometheus Operator logs for selector mismatches.

### Metrics endpoint returns connection refused

1. Confirm `observability.enabled: true` and `observability.metrics.enabled: true` — these gates must both be true for the port to be opened and the Service to exist.
2. Verify the pod is healthy: `kubectl exec -n <ns> <pod> -- wget -qO- http://localhost:9464/metrics`.
3. Check that the `service-metrics` Service was created: `kubectl get svc -n <ns> <fullname>-api-metrics`.

### Sampler set to 0 — no spans recorded

A sample ratio of `0` is equivalent to turning off tracing while keeping the metrics endpoint alive (parent-based: spans from callers that already sampled the trace are still recorded). Set it to `1.0` for 100 % sampling (recommended in dev/staging) or a value between `0.0` and `1.0` for production rate limiting. Via Helm, `sampleRatio: 0` renders as `1` — see the note under [`observability.traces`](#observabilitytraces).
