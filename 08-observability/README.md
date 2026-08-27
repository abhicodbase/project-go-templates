# Observability — The Three Pillars

## Logging + Metrics + Tracing

```
Logging:  WHAT happened — individual events, errors (text records)
Metrics:  HOW MUCH — aggregated numbers over time (counters, gauges, histograms)
Tracing:  WHERE it happened — end-to-end request flow across services (spans)

Analogy:
  Logging  → "User got a 500 error at 10:32:15"
  Metrics  → "500 error rate is 2% this minute"
  Tracing  → "Request went: Gateway (5ms) → Hotel Service (150ms) → DB (300ms) → TIMEOUT"
```

---

## Metrics with Prometheus

```go
package metrics

import (
    "net/http"
    "time"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    // Counter — only goes up (total requests, total errors)
    httpRequestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "http_requests_total",
            Help: "Total HTTP requests",
        },
        []string{"method", "path", "status"},
    )

    // Histogram — distribution of values (latency)
    httpRequestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "http_request_duration_seconds",
            Help:    "HTTP request duration",
            Buckets: prometheus.DefBuckets, // .005,.01,.025,.05,.1,.25,.5,1,2.5,5,10 seconds
        },
        []string{"method", "path"},
    )

    // Gauge — can go up and down (active connections, queue depth)
    activeConnections = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "active_connections",
            Help: "Number of active HTTP connections",
        },
    )

    // Summary — similar to histogram but with quantiles (percentiles)
    cacheHitRatio = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "cache_hit_ratio",
            Help: "Cache hit ratio (0-1)",
        },
    )
)

// Middleware that records HTTP metrics
func MetricsMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        activeConnections.Inc()
        defer activeConnections.Dec()

        wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}
        next.ServeHTTP(wrapped, r)

        duration := time.Since(start).Seconds()
        status := fmt.Sprintf("%d", wrapped.statusCode)

        httpRequestsTotal.WithLabelValues(r.Method, r.URL.Path, status).Inc()
        httpRequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration)
    })
}

// Expose metrics endpoint
func StartMetricsServer(addr string) {
    mux := http.NewServeMux()
    mux.Handle("/metrics", promhttp.Handler())
    http.ListenAndServe(addr, mux)
}
```

---

## Key Metrics to Track (SLOs)

```
For an HTTP API service, track:

Availability:  request_success_rate = 1 - (5xx_count / total_count)
Latency:       p50, p95, p99 of http_request_duration
Throughput:    requests per second = rate(http_requests_total[5m])
Error Rate:    rate(http_requests_total{status=~"5.."}[5m])

SLO Example (Agoda-style):
  Availability: 99.9% (max 8.7 hours downtime/year)
  Latency p99:  < 500ms
  Error rate:   < 0.1%
```

---

## Distributed Tracing with OpenTelemetry

```go
package tracing

import (
    "context"
    "net/http"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
    "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
    "go.opentelemetry.io/otel/trace"
)

// Initialize OpenTelemetry tracer
func InitTracer(serviceName, otlpEndpoint string) (func(), error) {
    exporter, err := otlptracehttp.New(context.Background(),
        otlptracehttp.WithEndpoint(otlpEndpoint),
    )
    if err != nil {
        return nil, err
    }

    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exporter),
        sdktrace.WithResource(resource.NewWithAttributes(
            semconv.SchemaURL,
            semconv.ServiceName(serviceName),
            semconv.ServiceVersion("1.0.0"),
            attribute.String("environment", "production"),
        )),
        sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.1)), // sample 10%
    )
    otel.SetTracerProvider(tp)

    return func() { tp.Shutdown(context.Background()) }, nil
}

var tracer = otel.Tracer("hotel-service")

// Add tracing to your service methods
func (s *HotelService) GetHotel(ctx context.Context, id string) (*Hotel, error) {
    ctx, span := tracer.Start(ctx, "HotelService.GetHotel",
        trace.WithAttributes(attribute.String("hotel.id", id)),
    )
    defer span.End()

    // Cache lookup — create child span
    ctx, cacheSpan := tracer.Start(ctx, "cache.get")
    hotel, found := s.cache.Get(ctx, id)
    cacheSpan.SetAttributes(attribute.Bool("cache.hit", found))
    cacheSpan.End()

    if found {
        return hotel, nil
    }

    // DB query — create child span
    ctx, dbSpan := tracer.Start(ctx, "db.query",
        trace.WithAttributes(
            attribute.String("db.statement", "SELECT * FROM hotels WHERE id=?"),
            attribute.String("db.type", "postgresql"),
        ),
    )
    result, err := s.repo.Get(ctx, id)
    if err != nil {
        dbSpan.RecordError(err) // Record the error in the span
        dbSpan.SetStatus(codes.Error, err.Error())
    }
    dbSpan.End()

    return result, err
}
```

---

## Health Check Endpoint

```go
type HealthStatus struct {
    Status     string            `json:"status"`      // "healthy", "degraded", "unhealthy"
    Version    string            `json:"version"`
    Checks     map[string]Check  `json:"checks"`
    Timestamp  time.Time         `json:"timestamp"`
}

type Check struct {
    Status  string `json:"status"`
    Message string `json:"message,omitempty"`
    Latency string `json:"latency,omitempty"`
}

func (h *HealthHandler) Handle(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()

    status := HealthStatus{
        Status:    "healthy",
        Version:   "1.2.3",
        Checks:    make(map[string]Check),
        Timestamp: time.Now(),
    }

    // Check DB
    start := time.Now()
    if err := h.db.PingContext(ctx); err != nil {
        status.Status = "unhealthy"
        status.Checks["database"] = Check{Status: "down", Message: err.Error()}
    } else {
        status.Checks["database"] = Check{Status: "up", Latency: time.Since(start).String()}
    }

    // Check Redis
    start = time.Now()
    if err := h.redis.Ping(ctx).Err(); err != nil {
        status.Status = "degraded" // Redis down but service still works
        status.Checks["redis"] = Check{Status: "down", Message: err.Error()}
    } else {
        status.Checks["redis"] = Check{Status: "up", Latency: time.Since(start).String()}
    }

    httpStatus := http.StatusOK
    if status.Status == "unhealthy" {
        httpStatus = http.StatusServiceUnavailable
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(httpStatus)
    json.NewEncoder(w).Encode(status)
}
```

---

## Interview Q&A

**Q: What is the difference between metrics and tracing?**
> A: Metrics are aggregated numbers over time — they tell you the overall health of the system (how many requests per second, what's the p99 latency, error rate). They're cheap to store and query. Tracing gives you the detailed path of a single request through your system — it shows exactly which service, which function, and what DB call caused a slow response. Tracing is essential for debugging latency issues in microservices. Metrics tell you "something is wrong," tracing tells you "where exactly is the problem."

**Q: How do you implement distributed tracing across microservices?**
> A: Using OpenTelemetry (OTel) as the instrumentation standard. Each service creates spans and propagates the trace context via HTTP headers (`traceparent`, `tracestate`). Service A starts a trace, creates a root span, and passes the trace ID + parent span ID in the HTTP header to Service B. Service B creates a child span with the received parent ID. All spans for the same trace ID form a tree. The trace data is exported to a backend (Jaeger, Zipkin, or cloud-native like AWS X-Ray or Google Cloud Trace). Sampling: in production, trace 1-10% of requests to control volume.
