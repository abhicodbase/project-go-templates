# Kubernetes Core Concepts

## Why Kubernetes for Microservices?

```
Problems K8s solves:
- How do I run 10 instances of my service?
- What happens if one crashes? → Auto-restart
- How do services find each other? → Service discovery
- How do I roll out updates without downtime? → Rolling updates
- How do I manage config and secrets? → ConfigMaps/Secrets
- How do I scale based on load? → HPA
```

---

## Core Objects

### Pod — Smallest Deployable Unit
```yaml
# A Pod contains one or more containers
apiVersion: v1
kind: Pod
metadata:
  name: hotel-service-pod
  labels:
    app: hotel-service
spec:
  containers:
  - name: hotel-service
    image: agoda/hotel-service:1.2.3
    ports:
    - containerPort: 8080
    resources:
      requests:          # guaranteed resources
        memory: "128Mi"
        cpu: "250m"      # 0.25 CPU core
      limits:            # max resources (OOMKill if exceeded)
        memory: "256Mi"
        cpu: "500m"
    env:
    - name: DB_URL
      valueFrom:
        secretKeyRef:    # from K8s Secret
          name: hotel-db-secret
          key: url
    readinessProbe:
      httpGet:
        path: /health
        port: 8080
      initialDelaySeconds: 5
      periodSeconds: 10
    livenessProbe:
      httpGet:
        path: /health
        port: 8080
      initialDelaySeconds: 15
      periodSeconds: 20
```

### Deployment — Manages Pods
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hotel-service
spec:
  replicas: 3                  # run 3 instances
  selector:
    matchLabels:
      app: hotel-service
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1              # max extra pods during update
      maxUnavailable: 0        # zero-downtime: never take pod down before new one is ready
  template:
    metadata:
      labels:
        app: hotel-service
    spec:
      containers:
      - name: hotel-service
        image: agoda/hotel-service:1.2.3
```

### Service — Stable Network Endpoint
```yaml
# ClusterIP — internal only (default)
apiVersion: v1
kind: Service
metadata:
  name: hotel-service
spec:
  selector:
    app: hotel-service   # routes to pods with this label
  ports:
  - port: 80
    targetPort: 8080
  type: ClusterIP

# Other types:
# NodePort — expose on each node's IP (dev/testing)
# LoadBalancer — provision cloud LB (production external traffic)
```

### ConfigMap & Secret
```yaml
# ConfigMap — non-sensitive config
apiVersion: v1
kind: ConfigMap
metadata:
  name: hotel-service-config
data:
  LOG_LEVEL: "info"
  CACHE_TTL: "300"
  MAX_RETRIES: "3"

---
# Secret — sensitive data (base64 encoded, but use Vault in prod!)
apiVersion: v1
kind: Secret
metadata:
  name: hotel-db-secret
type: Opaque
data:
  url: cG9zdGdyZXM6Ly91c2VyOnBhc3NAZGIvYWdvZGE=  # base64
```

---

## Graceful Shutdown in Go (K8s-compatible)

```go
func main() {
    srv := &http.Server{Addr: ":8080", Handler: router}

    // Start server
    go func() {
        log.Println("Server starting on :8080")
        if err := srv.ListenAndServe(); err != http.ErrServerClosed {
            log.Fatalf("Server failed: %v", err)
        }
    }()

    // Wait for SIGTERM (K8s sends this before killing the pod)
    stop := make(chan os.Signal, 1)
    signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
    <-stop

    log.Println("Shutdown signal received, draining connections...")

    // K8s waits terminationGracePeriodSeconds (default 30s) before SIGKILL
    ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
    defer cancel()

    // Gracefully shutdown: stop accepting new, finish in-flight
    if err := srv.Shutdown(ctx); err != nil {
        log.Printf("Shutdown error: %v", err)
    }

    log.Println("Shutdown complete")
}
```

---

## Health Checks (Readiness vs Liveness)

```
Readiness Probe: Is this pod READY to receive traffic?
  - If fails → K8s stops routing traffic to this pod, but doesn't restart it
  - Use case: pod is starting up, DB connection not ready yet

Liveness Probe:  Is this pod ALIVE/healthy?
  - If fails → K8s RESTARTS the pod
  - Use case: pod is deadlocked, memory leak, stuck in bad state

Startup Probe:   Has the app finished initializing?
  - Disables liveness probe until startup is done
  - Use for slow-starting apps
```

```go
// /health endpoint for K8s probes
func healthHandler(w http.ResponseWriter, r *http.Request) {
    // Readiness: check dependencies
    if !db.IsConnected() || !cache.IsConnected() {
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode(map[string]string{"status": "not ready"})
        return
    }
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}
```

---

## Horizontal Pod Autoscaler (HPA)

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: hotel-service-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: hotel-service
  minReplicas: 2
  maxReplicas: 20
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70  # scale up when avg CPU > 70%
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

---

## Interview Q&A

**Q: What is the difference between a liveness and readiness probe?**
> A: Readiness probe determines if the pod should receive traffic. If it fails, the pod is removed from the Service's endpoint list but continues running — useful during startup, graceful shutdown, or temporary dependency unavailability. Liveness probe determines if the pod is still alive and working. If it fails, the pod is restarted — useful for deadlocks or corrupted state. Key distinction: readiness failure = "don't send me traffic"; liveness failure = "I need to be restarted." Always implement both. Readiness failure should be your first line of defense.

**Q: How does K8s achieve zero-downtime deployments?**
> A: Rolling updates: K8s brings up new pods one by one (controlled by `maxSurge` and `maxUnavailable` settings) before terminating old ones. The key: new pods must pass readiness checks before K8s sends traffic to them and removes old pods. For the application: (1) implement readiness probe that only returns 200 when truly ready; (2) implement graceful shutdown to finish in-flight requests; (3) set `terminationGracePeriodSeconds` to give pods enough time to drain; (4) ensure `preStop` hook or readiness probe fails fast when K8s starts shutting down the pod (so the load balancer removes it from rotation before SIGTERM).
