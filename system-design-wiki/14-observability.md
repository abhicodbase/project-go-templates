# 14 — Observability

## TL;DR
> Observability = Logs + Metrics + Traces (the three pillars). You can't fix what you can't see.  
> SLO/SLI/SLA define reliability targets. Alerting on SLO burn rate is better than alerting on metrics.  
> Distributed tracing with correlation IDs is essential for debugging microservices.

---

## 1. The Three Pillars

```
              ┌─────────────────────────────────────┐
              │           Observability              │
              │                                      │
              │  Logs    +   Metrics   +   Traces    │
              │  (What    (How many/   (Why is it    │
              │  happened?) how fast?) slow/failing?)│
              └─────────────────────────────────────┘
```

| Pillar | Answers | Tooling |
|--------|---------|---------|
| **Logs** | What happened? When? In what context? | ELK Stack, Loki, CloudWatch |
| **Metrics** | How is the system performing? What are the rates? | Prometheus, Grafana, Datadog |
| **Traces** | Why is this request slow? Which service is the bottleneck? | Jaeger, Zipkin, AWS X-Ray |

---

## 2. Logging

### Log Levels
```
TRACE   → Most verbose; method entry/exit (dev only)
DEBUG   → Debugging information (dev/staging)
INFO    → Normal operational events (production)
WARN    → Something unexpected but recoverable
ERROR   → Something failed; action required
FATAL   → System cannot continue; immediate shutdown
```

### Structured Logging (JSON)
```json
// ❌ Unstructured (hard to query)
"2026-09-05 10:30:00 ERROR Payment failed for user 123 amount 99.99"

// ✅ Structured (queryable, parseable)
{
  "timestamp": "2026-09-05T10:30:00Z",
  "level": "ERROR",
  "service": "payment-service",
  "trace_id": "abc123def456",
  "user_id": "user_123",
  "amount": 99.99,
  "error": "card_declined",
  "message": "Payment failed"
}
```

### What to Log
| Always Log | Never Log |
|-----------|----------|
| Request in/out with trace_id | Passwords or secrets |
| Errors with stack traces | Full credit card numbers |
| Auth events (login, logout) | PII without masking (GDPR) |
| Config changes | Excessive debug logs in production |
| External API calls | Request body with sensitive fields |

### Centralized Log Aggregation
```
App Servers ──▶ Log Shipper (Filebeat/Fluentd)
                  ──▶ Logstash (transform/filter)
                       ──▶ Elasticsearch (storage)
                            ──▶ Kibana (dashboards)
```
Or: **Grafana Loki** (cheaper, labels-only indexing, integrates with Grafana)

### Correlation IDs (Trace ID)
```
// Generate at API Gateway entry point
X-Trace-ID: 550e8400-e29b-41d4-a716-446655440000

// Every service passes it through:
log.Info("Processing order",
  "trace_id", ctx.TraceID,
  "order_id", order.ID,
)

// Now you can search logs by trace_id and see full request path
```

---

## 3. Metrics

### The RED Method (for Services)
| Metric | Meaning |
|--------|---------|
| **R**ate | Requests per second |
| **E**rrors | Error rate (% of requests failing) |
| **D**uration | Latency distribution (p50, p95, p99) |

### The USE Method (for Resources)
| Metric | Meaning |
|--------|---------|
| **U**tilization | % time resource is busy |
| **S**aturation | How much extra work is queued |
| **E**rrors | Error count |

### Golden Signals (Google SRE)
1. **Latency** — how long requests take
2. **Traffic** — how much demand (RPS)
3. **Errors** — rate of failed requests
4. **Saturation** — how "full" the service is (CPU, memory, queue depth)

### Metric Types (Prometheus)
| Type | Description | Example |
|------|-------------|---------|
| **Counter** | Monotonically increasing | `http_requests_total` |
| **Gauge** | Up or down | `memory_usage_bytes` |
| **Histogram** | Distribution of values | `request_duration_seconds` |
| **Summary** | Pre-calculated quantiles | p50, p95, p99 latency |

### Percentiles vs Averages
```
Request latencies: 10ms, 12ms, 11ms, 10ms, 15ms, 500ms

Average: (10+12+11+10+15+500) / 6 = 93ms  ← MISLEADING!
p50: 11ms  (50% of requests ≤ 11ms)
p95: 500ms (5% of requests ≥ 500ms — this is what users experience as "slow")
p99: 500ms

→ Use p99 for latency SLOs — it captures the worst user experience
```

### Prometheus + Grafana Setup
```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'payment-service'
    scrape_interval: 15s
    static_configs:
      - targets: ['payment-service:9090']
```

```go
// Go: expose metrics endpoint
import "github.com/prometheus/client_golang/prometheus"

var requestDuration = prometheus.NewHistogramVec(
    prometheus.HistogramOpts{
        Name:    "http_request_duration_seconds",
        Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5},
    },
    []string{"method", "path", "status"},
)

// In handler:
timer := prometheus.NewTimer(requestDuration.WithLabelValues("POST", "/payments", "200"))
defer timer.ObserveDuration()
```

---

## 4. Distributed Tracing

Track a single request as it flows through multiple services.

### Trace Anatomy
```
Trace ID: abc123

┌─────────────────────────────────────────────────────────────┐
│ Span: API Gateway                                 200ms total│
│  ├── Span: Auth Service              10ms                    │
│  ├── Span: Order Service             150ms                   │
│  │    ├── Span: DB Query (orders)    80ms ← BOTTLENECK       │
│  │    └── Span: Cache Read           2ms                     │
│  └── Span: Notification Service     30ms                     │
└─────────────────────────────────────────────────────────────┘
```

Each **Span** contains:
- Trace ID (links all spans in request)
- Span ID (unique per span)
- Parent Span ID
- Service name, operation name
- Start time, duration
- Tags (key-value: http.method, db.query, error)
- Logs (timestamped events within span)

### Context Propagation
```
API Gateway creates trace:
  X-Trace-ID: abc123
  X-Span-ID: span001

Forwards headers to Order Service:
  X-Trace-ID: abc123
  X-Span-ID: span002  (new span, child of span001)
  X-Parent-Span-ID: span001

Order Service forwards to DB client library:
  → DB span created with parent span002
```

### OpenTelemetry (Standard)
```go
import "go.opentelemetry.io/otel"

tracer := otel.Tracer("payment-service")

ctx, span := tracer.Start(ctx, "processPayment")
defer span.End()

span.SetAttributes(
    attribute.String("user.id", userID),
    attribute.Float64("payment.amount", amount),
)

if err != nil {
    span.RecordError(err)
    span.SetStatus(codes.Error, err.Error())
}
```

### Sampling Strategies
```
100% sampling → too much data, expensive
1% sampling   → may miss rare errors

Better approaches:
  - Tail-based sampling: sample 100% of errored traces, 1% of successful
  - Rate-based: always sample first N requests per second
  - Head-based: decision made at trace start (random %)
```

---

## 5. SLO / SLI / SLA (Revisited in Depth)

### Definitions
```
SLA (Agreement): contractual promise to customers
  "We guarantee 99.9% uptime. Violation = credits."

SLO (Objective): internal target, stricter than SLA
  "Internal target: 99.95% uptime"

SLI (Indicator): actual measured metric
  SLI = (good requests / total requests) × 100%
  SLI = p99 latency of HTTP responses
```

### Common SLIs

**Availability SLI:**
```
SLI = (HTTP 2xx responses) / (total HTTP responses)
Target SLO: 99.9% (allows ~43 min/month downtime)
```

**Latency SLI:**
```
SLI = (requests served in < 200ms) / (total requests)
Target SLO: 95% of requests served in < 200ms
```

**Error Rate SLI:**
```
SLI = (successful requests) / (total requests)
Target SLO: < 0.1% error rate
```

### Error Budget
```
SLO = 99.9% availability
Error budget = 100% - 99.9% = 0.1%
In a 30-day month: 0.1% × 30 × 24 × 60 = 43.2 minutes of allowed downtime

Burn rate:
  Current downtime rate = 2× error budget → budget exhausted in 15 days
  → Alert! Freeze deployments, focus on reliability
```

### Alert on Burn Rate, Not Thresholds
```
Bad alert: "Error rate > 1%" → too much noise, not actionable
Good alert: "Error budget burn rate > 14.4× for 1 hour" 
            → means budget exhausted in 2 days → page on-call now
```

---

## 6. Alerting Best Practices

### Alert Fatigue
Too many alerts → on-call ignores them → real incidents missed.

**Rules:**
1. Alert on **symptoms** (user impact), not causes
2. Every alert must be **actionable** — if you can't do anything, don't alert
3. Alert on **SLO burn rate** not raw metrics
4. Use **severity levels**: P1 (page now), P2 (page in hours), P3 (ticket)

### Alert Examples
```
✅ Good alerts:
  - "p99 latency > 500ms sustained for 5 min" (user impact)
  - "Error budget burn rate > 5× for 30 min" (SLO risk)
  - "Zero successful health check responses from payment service" (service down)

❌ Bad alerts:
  - "CPU > 80%" (not necessarily a problem)
  - "Memory > 70%" (may be normal)
  - "Response time spike for 30 seconds" (may be transient)
```

---

## 7. Health Checks

```
GET /health/live   → 200 if process is alive (Kubernetes liveness probe)
GET /health/ready  → 200 if ready to serve traffic (Kubernetes readiness probe)

// Readiness checks:
func readinessHandler(w http.ResponseWriter, r *http.Request) {
    if err := db.Ping(); err != nil {
        w.WriteHeader(503)
        return
    }
    if err := cache.Ping(); err != nil {
        w.WriteHeader(503)
        return
    }
    w.WriteHeader(200)
}
```

---

## 8. Chaos Engineering

Deliberately inject failures to test resilience.

```
Principles:
1. Build a hypothesis (system will survive X failure)
2. Inject failure (kill a service, saturate network)
3. Observe (does the system degrade gracefully?)
4. Automate (run regularly in production)

Tools: Chaos Monkey (Netflix), Gremlin, LitmusChaos (K8s)

Examples:
  - Kill random pods in Kubernetes
  - Add 100ms latency to DB connections
  - Saturate disk I/O
  - Drain a load balancer node
```

---

## Key Takeaways

1. **Logs + Metrics + Traces** = full observability. Each serves a different purpose
2. **Structured JSON logs** with trace_id — searchable and correlatable
3. Use **p99 latency** not averages — tail latency is what users feel
4. **OpenTelemetry** as the standard instrumentation library — vendor-neutral
5. **Tail-based sampling**: 100% sample errors, 1% sample success
6. **Alert on SLO burn rate** — burns through budget quickly means real trouble
7. **Error budget** governance: freeze deploys when budget is low
8. **Chaos engineering** is how you validate resilience before real failures hit
