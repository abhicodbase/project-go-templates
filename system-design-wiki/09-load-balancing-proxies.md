# 09 — Load Balancing & Proxies

## TL;DR
> Load balancers distribute traffic across multiple servers. Layer 4 LB routes by IP/port; Layer 7 by content.  
> Consistent hashing or sticky sessions for stateful workloads.  
> Service mesh handles inter-service load balancing with mTLS, circuit breaking, and observability.

---

## 1. Why Load Balancing?

```
Without LB:
  All clients ──▶ Single Server → bottleneck, SPOF

With LB:
  Clients ──▶ Load Balancer ──▶ Server A (healthy)
                             ──▶ Server B (healthy)
                             ──▶ Server C (healthy)
                             ✗── Server D (unhealthy — removed from pool)
```

**Benefits:**
- **Horizontal scaling** — add more servers to handle more traffic
- **High availability** — remove unhealthy servers from rotation
- **SSL termination** — decrypt TLS at LB, forward plain HTTP
- **Reduced latency** — route to geographically closer server

---

## 2. Layer 4 vs Layer 7 Load Balancing

### Layer 4 (Transport Layer)
Routes based on **IP address and TCP/UDP port** — doesn't look at application data.

```
Client → L4 LB → Backend (TCP connection forwarded)
  Routes by: source IP, destination IP, TCP port
  Doesn't inspect: HTTP headers, URL, cookies
```

- **Faster** (less processing)
- **Stateful** (maintains TCP connection mapping)
- Cannot route based on URL path or HTTP headers
- Examples: AWS NLB, HAProxy (TCP mode)

### Layer 7 (Application Layer)
Routes based on **HTTP content** — URL, headers, cookies, HTTP method.

```
Client → L7 LB → Backend (new TCP connection per route)
  Routes by: /api/users → User Service
             /api/orders → Order Service
             Cookie: user_id → Sticky Server
```

- **Smarter routing** (content-based)
- **SSL termination** (inspect encrypted traffic)
- **Compression**, **caching**, **request rewriting**
- Slightly more overhead than L4
- Examples: Nginx, AWS ALB, HAProxy (HTTP mode)

### When to Use Each
| Use Case | LB Type |
|---------|---------|
| Simple TCP traffic distribution | L4 |
| Microservices with path-based routing | L7 |
| gRPC (HTTP/2) | L7 |
| WebSocket with sticky sessions | L7 |
| Raw UDP (gaming, VoIP) | L4 |
| Need maximum performance | L4 |

---

## 3. Load Balancing Algorithms

### Round Robin
```
Request 1 → Server A
Request 2 → Server B
Request 3 → Server C
Request 4 → Server A (wrap around)
```
- Simple, equal distribution
- **Problem**: ignores server capacity differences

### Weighted Round Robin
```
Server A (weight=3): gets 3 out of every 5 requests
Server B (weight=1): gets 1 out of every 5 requests
Server C (weight=1): gets 1 out of every 5 requests
```
- Good when servers have different capacity
- Manually configured weights

### Least Connections
```
Server A: 10 active connections
Server B: 3 active connections   ← send next request here
Server C: 8 active connections
```
- Best for variable request duration (some requests take longer)
- Prevents hot spots from slow requests

### Least Response Time
```
Server A: avg response 50ms
Server B: avg response 10ms   ← send next request here
Server C: avg response 80ms
```
- Combine least connections + fastest response time
- Most optimal for latency-sensitive workloads

### IP Hash (Source Affinity)
```
shard = hash(client_ip) % num_servers
Client 192.168.1.1 always → Server A
Client 192.168.1.2 always → Server B
```
- Same client always hits same server
- Useful when server-side session state cannot be shared
- **Problem**: adding/removing server disrupts all mappings

### Consistent Hashing (for LB)
Same ring concept — used to route specific keys to specific servers while minimizing disruption.

### Random
- Simple random selection
- Good enough at scale (law of large numbers balances it out)

---

## 4. Health Checks

Load balancer continuously checks server health:

### Passive Health Check
Monitor real traffic; detect failures from timeout/error responses.

### Active Health Check
```
LB ──GET /health──▶ Server A (every 5s)
  200 OK → Server A is healthy → keep in rotation
  timeout / 5xx → Server A is unhealthy → remove from rotation
  
After 3 consecutive successes → add back to rotation
```

### Health Check Endpoints
```go
// Best practice: /health/live and /health/ready
GET /health/live   → 200 if process is running (don't remove from LB)
GET /health/ready  → 200 if ready to serve traffic (remove if starting up)

// Kubernetes liveness vs readiness probes map to these
```

---

## 5. Sticky Sessions (Session Affinity)

Route the same client to the same server for the duration of a session.

### Cookie-Based Stickiness
```
Client's first request → LB assigns Server A → sets cookie: LB_SERVER=A
All subsequent requests with cookie → LB sends to Server A
```

### IP-Based Stickiness
```
hash(client_ip) % N → always Server A for this IP
```

**Problems with Sticky Sessions:**
- If Server A goes down → all that server's sessions lost
- Uneven load if some users are more active
- Makes horizontal scaling harder

**Solution**: Move session state out of servers into shared store:
```
App Server A ──▶ Redis Session Store ◀── App Server B
(any server can handle any request — stateless)
```

---

## 6. SSL/TLS Termination

Load balancer decrypts HTTPS, forwards plain HTTP to backend.

```
Client ──HTTPS──▶ Load Balancer (terminates TLS) ──HTTP──▶ Backend
         (encrypted)   ↑ handles certs here       (plain, on private network)
```

**Benefits:**
- Backend servers don't need TLS logic
- Centralized certificate management
- LB can inspect, modify, route HTTP traffic

**Trade-off**: Traffic on internal network is unencrypted
**Solution**: mTLS between services (Service Mesh)

---

## 7. Global Server Load Balancing (GSLB)

Route users to the nearest or best-performing data center geographically.

```
User in India ──DNS lookup──▶ GSLB (GeoDNS) → "use Mumbai DC"
User in USA   ──DNS lookup──▶ GSLB (GeoDNS) → "use US-East DC"

GSLB makes DNS routing decisions based on:
  - Client IP → geographic region
  - DC health (is Mumbai DC alive?)
  - DC latency (which is faster right now?)
  - Traffic weights (canary deployments)
```

**Examples**: AWS Route 53 with latency-based routing, Cloudflare, Akamai

---

## 8. Reverse Proxy

A server that sits in front of web servers, forwarding client requests.

```
Internet ──▶ [Reverse Proxy: Nginx] ──▶ App Server 1
                                      ──▶ App Server 2
```

**Functions:**
- Load balancing
- SSL termination
- Caching (serve static files directly)
- Compression (gzip)
- Request buffering (protect backend from slow clients)
- Security (hide backend IPs, WAF)

### Nginx as Reverse Proxy
```nginx
upstream backend {
    server app1:8080 weight=3;
    server app2:8080;
    server app3:8080;
    keepalive 32;  # connection pooling to backends
}

server {
    listen 443 ssl;
    ssl_certificate /etc/nginx/certs/cert.pem;

    location /api/ {
        proxy_pass http://backend;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }

    location /static/ {
        root /var/www;  # serve static files directly from Nginx
        expires 30d;
    }
}
```

---

## 9. Service Mesh

Infrastructure layer that handles service-to-service communication automatically.

```
Without Service Mesh:
  Service A ──HTTP──▶ Service B
  (each service implements retry, timeout, circuit breaker, mTLS manually)

With Service Mesh (Sidecar Pattern):
  Service A ──▶ [Envoy Proxy A] ──mTLS──▶ [Envoy Proxy B] ──▶ Service B
               (handles all network concerns automatically)
```

### What Service Mesh Provides
| Feature | How |
|---------|-----|
| **Load balancing** | Sidecar proxies balance across instances |
| **mTLS** | Automatic mutual TLS between all services |
| **Circuit breaking** | Stop calls to failing services |
| **Retries + timeouts** | Configurable per route |
| **Observability** | Metrics, traces, logs at network layer |
| **Traffic splitting** | Canary/blue-green deployments |
| **Service discovery** | Find other services by name |

### Popular Service Meshes
| Mesh | Sidecar | Control Plane | Language |
|------|---------|--------------|---------|
| **Istio** | Envoy | istiod | Go |
| **Linkerd** | Linkerd-proxy | Linkerd control plane | Rust/Go |
| **Consul Connect** | Envoy | Consul | Go |
| **AWS App Mesh** | Envoy | AWS managed | - |

---

## 10. Load Balancer Tiers

```
User
  │
  ▼
[DNS / GSLB]          ← Route to nearest region
  │
  ▼
[Edge LB / CDN]       ← Handle DDoS, serve static content
  │
  ▼
[L7 Load Balancer]    ← Path routing, SSL termination, WAF
  │
  ▼
[Service Mesh / L4 LB] ← Internal service-to-service routing
  │
  ▼
[App Servers]
```

---

## Key Takeaways

1. **L7 LB** (Nginx, ALB) for HTTP microservices; **L4 LB** (NLB) for TCP/UDP high throughput
2. **Least connections** algorithm best for variable-length requests
3. **Health checks** are critical — LB must detect and route around failures fast
4. **Sticky sessions** are a code smell — externalize session state to Redis instead
5. **SSL termination** at the LB, mTLS between services (service mesh)
6. **GSLB/GeoDNS** to route users to nearest data center
7. **Service mesh** (Istio/Linkerd) for automatic mTLS, observability, circuit breaking
