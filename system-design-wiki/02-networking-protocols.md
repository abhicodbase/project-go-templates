# 02 — Networking & Protocols

## TL;DR
> HTTP/2 multiplexes many requests on one connection. TCP ensures delivery; UDP is fast but lossy.  
> DNS resolves names to IPs. CDN caches content at the edge. WebSockets enable full-duplex real-time communication.

---

## 1. OSI Model (Quick Reference)

```
Layer 7 — Application   (HTTP, gRPC, WebSocket, DNS)
Layer 6 — Presentation  (TLS/SSL, encoding)
Layer 5 — Session       (session management)
Layer 4 — Transport     (TCP, UDP)
Layer 3 — Network       (IP, routing)
Layer 2 — Data Link     (Ethernet, MAC)
Layer 1 — Physical      (cables, signals)
```

In system design, you mostly care about **Layer 4 (TCP/UDP)** and **Layer 7 (HTTP, gRPC, WebSocket)**.

---

## 2. TCP vs UDP

| Feature | TCP | UDP |
|---------|-----|-----|
| Connection | Connection-oriented (3-way handshake) | Connectionless |
| Reliability | Guaranteed delivery, ordering, retransmission | Best-effort, no guarantees |
| Speed | Slower (overhead) | Faster |
| Use Cases | HTTP, databases, file transfer | Video streaming, gaming, DNS, VoIP |
| Header size | 20–60 bytes | 8 bytes |

### TCP 3-Way Handshake
```
Client                    Server
  │──── SYN ────────────▶ │
  │◀─── SYN-ACK ──────── │
  │──── ACK ────────────▶ │
  │   (connection open)   │
```
- **Cost**: ~1 RTT before any data flows
- **TLS adds**: 1–2 more RTTs (TLS 1.2) or 0–1 RTT (TLS 1.3)

---

## 3. HTTP Versions

### HTTP/1.1
- **One request per TCP connection** (unless pipelining, which is buggy)
- **Head-of-line blocking**: request B waits for request A to finish
- Workaround: browsers open 6 parallel TCP connections per domain

### HTTP/2
- **Multiplexing**: many requests/responses over **one** TCP connection (as streams)
- **Header compression** (HPACK): reduces overhead
- **Server Push**: server can proactively send resources
- **Binary framing**: more efficient than text

```
HTTP/1.1:                    HTTP/2:
Conn1: Req1 ──▶ Res1         One Conn:
Conn2: Req2 ──▶ Res2         Stream1: Req1 ──▶ Res1
Conn3: Req3 ──▶ Res3         Stream2: Req2 ──▶ Res2
                             Stream3: Req3 ──▶ Res3
```

### HTTP/3 (QUIC)
- Runs over **UDP** instead of TCP
- Eliminates TCP head-of-line blocking at transport layer
- Faster connection setup (0-RTT on resumed connections)
- Built-in TLS 1.3

---

## 4. HTTPS & TLS

**TLS (Transport Layer Security)** provides:
- **Encryption**: data in transit is unreadable to middlemen
- **Authentication**: proves server identity via certificates
- **Integrity**: data cannot be tampered without detection

### TLS 1.3 Handshake (1-RTT)
```
Client                           Server
  │─── ClientHello (key share) ─▶│
  │◀── ServerHello + Certificate─│
  │◀── Finished ─────────────────│
  │─── Finished ────────────────▶│
  │         (data flows)         │
```

### Certificate Chain of Trust
```
Root CA (trusted by OS)
  └── Intermediate CA
        └── Your Certificate (algomaster.io)
```

---

## 5. DNS (Domain Name System)

DNS translates human-readable domain names to IP addresses.

### Resolution Process
```
Browser cache → OS cache → Router cache → ISP DNS → Root DNS → TLD DNS → Authoritative DNS
```

```
User: "What is the IP for api.example.com?"
  │
  ▼
Recursive Resolver (ISP)
  │── asks Root Nameserver ──▶ "Go ask .com TLD"
  │── asks .com TLD ─────────▶ "Go ask example.com nameserver"
  │── asks example.com NS ───▶ "api.example.com = 93.184.216.34"
  │
  ▼
Returns IP to browser (TTL cached)
```

### DNS Record Types
| Record | Purpose | Example |
|--------|---------|---------|
| **A** | Domain → IPv4 | example.com → 93.184.216.34 |
| **AAAA** | Domain → IPv6 | example.com → 2606:2800::1 |
| **CNAME** | Domain → another domain | www → example.com |
| **MX** | Mail server | @ → mail.example.com |
| **NS** | Nameserver | example.com → ns1.provider.com |
| **TXT** | Arbitrary text (SPF, DKIM) | "v=spf1 include:..." |

### DNS Load Balancing
- **Round-robin DNS**: return different IPs on each query
- **Geolocation-based**: return IP of nearest data center
- **Weighted**: distribute traffic by weight to different IPs

### TTL (Time to Live)
- How long resolvers cache the DNS record
- Low TTL (60s) = faster failover, but more DNS queries
- High TTL (86400s) = less DNS load, but slow propagation

---

## 6. CDN (Content Delivery Network)

A geographically distributed network of servers that caches and serves content close to users.

```
Without CDN:
  User (India) ─── 200ms ──▶ Origin Server (US)

With CDN:
  User (India) ─── 10ms ───▶ CDN Edge (Mumbai) ──▶ Origin (on cache miss)
```

### How CDN Works
1. User requests `https://example.com/image.jpg`
2. DNS resolves to nearest CDN edge server
3. Edge checks if content is cached
   - **Cache HIT**: return immediately (fast!)
   - **Cache MISS**: fetch from origin, cache it, then return

### What CDNs Cache
- Static assets: HTML, CSS, JS, images, videos
- Dynamic content (with short TTL)
- API responses (carefully)

### CDN Benefits
| Benefit | How |
|---------|-----|
| Reduced latency | Serve from edge near user |
| Reduced origin load | Most requests never hit origin |
| DDoS protection | Absorbs traffic at edge |
| High availability | Multiple edge locations |

### Push vs Pull CDN
- **Pull CDN**: Lazily fetches from origin on first request (most common — CloudFront, Fastly)
- **Push CDN**: You proactively upload content to CDN nodes (for large files, known-in-advance content)

---

## 7. WebSockets

Full-duplex, persistent connection between client and server over a single TCP connection.

### HTTP vs WebSocket
```
HTTP (Request-Response):
Client ──GET /data──▶ Server
Client ◀──Response── Server
(connection closed or reused)

WebSocket (Full-Duplex):
Client ──WS Upgrade──▶ Server
Client ◀────────────── Server (server can push anytime)
Client ────────────────▶ Server (client can push anytime)
(persistent connection)
```

### WebSocket Handshake
```
GET /chat HTTP/1.1
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: dGhlIHNhbXBsZQ==

HTTP/1.1 101 Switching Protocols
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=
```

### Use Cases
- Chat applications
- Live dashboards / real-time analytics
- Online gaming
- Collaborative editing (Google Docs)
- Financial tickers / live prices

### Limitations
- Load balancers must support sticky sessions (or use pub/sub backend)
- Harder to scale horizontally (stateful connection)
- Not supported by HTTP/2 natively (different protocol)

---

## 8. Long Polling vs SSE vs WebSockets

| Feature | Long Polling | SSE | WebSocket |
|---------|-------------|-----|-----------|
| Direction | Client pulls | Server → Client | Full duplex |
| Protocol | HTTP | HTTP | WS |
| Overhead | High (reconnect each time) | Low | Low |
| Browser Support | All | All | All |
| Firewall friendly | Yes | Yes | Sometimes not |
| Use case | Notifications, simple updates | Live feeds, notifications | Chat, gaming |

### Long Polling Flow
```
Client ──GET /updates──▶ Server (holds request until update available)
                         Server ◀── new event occurs
Client ◀── response ──── Server
Client ──GET /updates──▶ Server (immediately re-opens)
```

### Server-Sent Events (SSE)
```
Client ──GET /stream──▶ Server
Client ◀── data: {...} (server streams events one-way)
Client ◀── data: {...}
Client ◀── data: {...}
```

---

## 9. gRPC

Google's high-performance RPC framework built on HTTP/2 and Protocol Buffers.

### Key Features
- **Protocol Buffers**: binary serialization (3–10x smaller than JSON)
- **HTTP/2**: multiplexing, bidirectional streaming
- **Strongly typed**: schema-first with `.proto` files
- **Code generation**: generates client/server stubs in any language

### gRPC vs REST

| Feature | gRPC | REST |
|---------|------|------|
| Protocol | HTTP/2 | HTTP/1.1 or 2 |
| Serialization | Protobuf (binary) | JSON (text) |
| Performance | ~10x faster | Baseline |
| Browser support | Limited (needs proxy) | Full |
| Streaming | Yes (bidirectional) | Limited (SSE) |
| Schema | Strict (.proto) | Optional (OpenAPI) |
| Use case | Internal microservices | Public APIs |

### gRPC Communication Patterns
```
1. Unary (request-response):
   Client ──req──▶ Server ──res──▶ Client

2. Server Streaming:
   Client ──req──▶ Server ──res1──▶ Client
                          ──res2──▶ Client

3. Client Streaming:
   Client ──req1──▶ Server
   Client ──req2──▶ Server
                   ──res──▶ Client

4. Bidirectional Streaming:
   Client ──req──▶ Server ──res──▶ Client (interleaved)
```

---

## 10. Network Topologies & Patterns

### Forward Proxy vs Reverse Proxy
```
Forward Proxy (client-side):          Reverse Proxy (server-side):
Client ──▶ [Proxy] ──▶ Internet       Client ──▶ [Reverse Proxy] ──▶ Server
  (client hides identity)               (server hides identity)
  Use: VPN, content filtering           Use: Load balancing, SSL termination
```

### Service Mesh (Sidecar Pattern)
Each microservice gets a sidecar proxy (e.g., Envoy) that handles:
- Service discovery
- Load balancing
- Circuit breaking
- mTLS between services
- Observability (metrics, tracing)

```
Service A ──▶ [Envoy Sidecar A] ──▶ [Envoy Sidecar B] ──▶ Service B
              (handles all network concerns)
```

---

## Key Takeaways

1. Use **TCP** when reliability matters, **UDP** when speed matters more
2. Prefer **HTTP/2 or HTTP/3** for new services — multiplexing is a big win
3. **DNS TTL** affects failover speed — keep it low for critical services
4. Use **CDN** for all static assets; it dramatically reduces latency and origin load
5. **WebSockets** for real-time bidirectional; **SSE** for server-to-client streams
6. **gRPC** for internal microservice communication; **REST** for public APIs
