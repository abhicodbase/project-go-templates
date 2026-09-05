# 03 — APIs & Communication

## TL;DR
> REST is stateless and resource-based. GraphQL lets clients query exactly what they need.  
> gRPC is fast binary RPC for internal services. API Gateway is the single entry point for clients.  
> Rate limiting protects your system from abuse.

---

## 1. REST (Representational State Transfer)

REST is an architectural style for building APIs over HTTP.

### REST Constraints
1. **Stateless** — each request contains all necessary information (no server-side sessions)
2. **Client-Server** — separation of concerns
3. **Cacheable** — responses must define themselves as cacheable or not
4. **Uniform Interface** — consistent resource-based URLs
5. **Layered System** — client doesn't know if it's talking to origin or proxy

### HTTP Methods (CRUD Mapping)
| HTTP Method | Operation | Idempotent | Safe |
|-------------|-----------|-----------|------|
| GET | Read | ✅ Yes | ✅ Yes |
| POST | Create | ❌ No | ❌ No |
| PUT | Replace (full update) | ✅ Yes | ❌ No |
| PATCH | Partial update | ❌ No | ❌ No |
| DELETE | Delete | ✅ Yes | ❌ No |

> **Idempotent**: calling it multiple times = same result as calling once  
> **Safe**: calling it does not modify state

### REST URL Design Best Practices
```
✅ Good:
GET  /users                   → list all users
GET  /users/123               → get user 123
POST /users                   → create user
PUT  /users/123               → replace user 123
DELETE /users/123             → delete user 123
GET  /users/123/orders        → get orders for user 123

❌ Bad:
GET  /getUser?id=123          (verb in URL)
POST /users/create            (redundant "create")
GET  /users/123/deleteUser    (wrong HTTP method)
```

### HTTP Status Codes
| Code | Category | Examples |
|------|----------|---------|
| 2xx | Success | 200 OK, 201 Created, 204 No Content |
| 3xx | Redirection | 301 Moved Permanently, 304 Not Modified |
| 4xx | Client Error | 400 Bad Request, 401 Unauthorized, 403 Forbidden, 404 Not Found, 429 Too Many Requests |
| 5xx | Server Error | 500 Internal Server Error, 502 Bad Gateway, 503 Service Unavailable |

### REST API Versioning Strategies
```
1. URL versioning:        /v1/users  (most common, easy to test)
2. Header versioning:     Accept: application/vnd.api.v1+json
3. Query param:           /users?version=1
4. Subdomain:             v1.api.example.com/users
```

---

## 2. GraphQL

A query language and runtime for APIs where clients specify exactly what data they need.

### Problem GraphQL Solves
```
REST (over-fetching):
GET /users/123 → returns ALL user fields even if you only need name + email

REST (under-fetching):
GET /users/123              → get user
GET /users/123/posts        → get their posts
GET /posts/456/comments     → get post comments
(3 round trips!)

GraphQL (exact data, one request):
query {
  user(id: "123") {
    name
    email
    posts {
      title
      comments {
        text
      }
    }
  }
}
```

### GraphQL Core Concepts

**Query** (read):
```graphql
query GetUser($id: ID!) {
  user(id: $id) {
    name
    email
    avatar
  }
}
```

**Mutation** (write):
```graphql
mutation CreateUser($input: CreateUserInput!) {
  createUser(input: $input) {
    id
    name
    createdAt
  }
}
```

**Subscription** (real-time):
```graphql
subscription OnMessageAdded($chatId: ID!) {
  messageAdded(chatId: $chatId) {
    text
    sender { name }
    timestamp
  }
}
```

### GraphQL vs REST

| Feature | REST | GraphQL |
|---------|------|---------|
| Data fetching | Fixed endpoints | Client-defined queries |
| Over-fetching | Common | Eliminated |
| Under-fetching | Common (N+1) | Eliminated |
| Type system | Optional (OpenAPI) | Built-in |
| Caching | Easy (HTTP caching) | Hard (POST requests) |
| Learning curve | Low | Medium |
| Versioning | Required (v1, v2) | Evolve schema without versioning |
| Performance | Simple cases | N+1 problem if not using DataLoader |

### N+1 Problem in GraphQL
```
query {
  posts {          # 1 query
    author {       # N queries (one per post)
      name
    }
  }
}
# Solution: DataLoader batches & caches these N queries into 1
```

---

## 3. API Gateway

A single entry point that routes client requests to the appropriate backend service.

```
                         ┌──────────────────┐
                         │   API Gateway    │
                         │                  │
Mobile App ──────────────│  Auth            │──▶ User Service
Web App   ──────────────▶│  Rate Limiting   │──▶ Product Service
3rd Party ──────────────▶│  Routing         │──▶ Order Service
                         │  Logging         │──▶ Payment Service
                         │  SSL Termination │
                         └──────────────────┘
```

### API Gateway Responsibilities
| Function | Description |
|----------|-------------|
| **Routing** | Route `/users` to User Service, `/orders` to Order Service |
| **Authentication** | Validate JWT/API keys before forwarding |
| **Rate Limiting** | Limit requests per client per time window |
| **SSL Termination** | Handle TLS, forward plain HTTP internally |
| **Request Transformation** | Transform request format between client and service |
| **Response Aggregation** | Combine responses from multiple services |
| **Caching** | Cache common responses at the gateway |
| **Load Balancing** | Distribute requests across service instances |
| **Logging & Monitoring** | Centralized logging of all API traffic |
| **Circuit Breaking** | Stop calling a failing downstream service |

### Popular API Gateways
- **Kong** — open source, plugin ecosystem, Lua-based
- **AWS API Gateway** — managed, serverless integration
- **Nginx/Envoy** — high-performance, often used as gateway
- **Traefik** — cloud-native, auto-discovers Docker/K8s services
- **Apigee** — enterprise, Google Cloud

### BFF (Backend for Frontend)
A variation where each client type gets its own API Gateway tailored to its needs:

```
Mobile App ──▶ [Mobile BFF]  ──▶ Backend Services
Web App    ──▶ [Web BFF]     ──▶ Backend Services
TV App     ──▶ [TV BFF]      ──▶ Backend Services
```

**Why BFF?**
- Mobile needs smaller payloads
- Web needs richer data
- TV needs different auth flow
- Avoids one-size-fits-all API that compromises all clients

---

## 4. Rate Limiting

Controlling how many requests a client can make in a given time window.

### Why Rate Limit?
- Prevent abuse and DDoS attacks
- Ensure fair usage among clients
- Protect downstream services from overload
- Enforce business rules (free tier vs paid)

### Rate Limiting Algorithms

#### 1. Token Bucket
```
Bucket holds N tokens. Each request consumes 1 token. Tokens refill at rate R/sec.

  ┌──────────────┐
  │ Token Bucket │◀── refill R tokens/sec
  │ [●●●●●○○○○○] │    (capacity N)
  └──────────────┘
         │
    request arrives ──▶ take 1 token ──▶ allow
                        no token?    ──▶ reject/queue
```
- **Allows bursts** up to bucket capacity
- Smooth average rate

#### 2. Leaky Bucket
```
Requests enter bucket top. Bucket "leaks" (processes) at fixed rate.
Overflow = rejected.

  Requests ─▶ ┌──────┐
              │      │ ◀── overflows = rejected
              │      │
              └──┬───┘
                 │ leaks at fixed rate R req/sec
                 ▼
              Process
```
- **No bursts** — always steady output rate
- Good for smoothing traffic to downstream

#### 3. Fixed Window Counter
```
Window: [0s ──── 60s]  [60s ──── 120s]
Limit: 100 req/window

Problem: 100 requests at 59s + 100 requests at 61s = 200 req in 2 seconds!
```
- Simple but has **boundary burst problem**

#### 4. Sliding Window Log
```
Keep timestamps of all requests. Count requests in [now - window, now].
Accurate but memory intensive (stores all timestamps).
```

#### 5. Sliding Window Counter
```
Mix of fixed window + sliding adjustment.
estimate = prev_window_count × (1 - elapsed/window) + curr_window_count
```
- **Best balance** of accuracy and memory efficiency

### Rate Limiting Dimensions
| Dimension | Example |
|-----------|---------|
| Per IP | 100 req/min per IP |
| Per User | 1000 req/hour per user |
| Per API Key | 10,000 req/day per key |
| Per Endpoint | POST /orders: 10 req/min |
| Global | Total system: 1M req/min |

### Where to Implement Rate Limiting
```
Client ──▶ [API Gateway / Nginx] ──▶ [Rate Limiter Middleware] ──▶ Service
                                        ↕
                                     [Redis]
                                (shared counter across instances)
```

> Use **Redis** with atomic `INCR` + `EXPIRE` for distributed rate limiting

---

## 5. Authentication & Authorization

### Authentication Methods

#### API Keys
```
GET /api/data
X-API-Key: sk-abc123xyz

Pros: simple, easy to revoke
Cons: no expiry by default, hard to tie to user identity
```

#### JWT (JSON Web Token)
```
Header.Payload.Signature

eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1c2VyMTIzIiwiZXhwIjoxNjk5MDAwMDAwfQ.abc123

Payload (decoded):
{
  "sub": "user123",
  "role": "admin",
  "exp": 1699000000
}
```

**JWT Flow:**
```
Client ──POST /login──▶ Auth Server
Client ◀── JWT Token── Auth Server
Client ──GET /data + Bearer JWT──▶ API Gateway
                                   validates JWT (no DB call!)
                                  ──▶ Backend Service
```

- **Stateless**: server doesn't need to store session
- **Self-contained**: payload carries user info and claims
- **Trade-off**: can't revoke before expiry (use short TTL + refresh tokens)

#### OAuth 2.0
```
User ──▶ App ──▶ Authorization Server
                 (Google, GitHub, etc.)
      ◀──────────── Authorization Code
App ──▶ Auth Server (exchange code for tokens)
    ◀── Access Token + Refresh Token
App ──▶ Resource Server (API) with Access Token
    ◀── Protected Resource
```

**OAuth 2.0 Grant Types:**
| Grant Type | Use Case |
|-----------|---------|
| Authorization Code | Web/mobile apps (most common) |
| Client Credentials | Server-to-server (no user involved) |
| Device Code | Smart TVs, CLI tools |
| Implicit | (deprecated — use Authorization Code + PKCE) |

### Authentication vs Authorization
| Concept | Question Answered | Example |
|---------|-----------------|---------|
| **Authentication** | Who are you? | Login with username + password |
| **Authorization** | What can you do? | Admin can delete; user can only read |

---

## 6. Idempotency in APIs

An API is **idempotent** if making the same request multiple times produces the same result.

### Why It Matters
```
Client ──POST /payments/charge──▶ Server (processes payment)
         network timeout!        Server (payment succeeded, but client didn't get response)
Client ──POST /payments/charge──▶ Server (charges AGAIN! 💸)
```

### Solution: Idempotency Keys
```
Client ──POST /payments/charge──▶ Server
   Header: Idempotency-Key: a3f29d1c-4b8e-4f2a-9c6d-7e8f01234567
   Body: { amount: 100, currency: "USD" }

Server stores result keyed by Idempotency-Key in Redis.
Retry with same key → return stored result, don't reprocess.
```

---

## 7. API Pagination

For endpoints that return large collections.

### Offset Pagination
```
GET /posts?offset=0&limit=20   → posts 1-20
GET /posts?offset=20&limit=20  → posts 21-40

Pros: simple, can jump to any page
Cons: slow on large offsets (DB must skip N rows), inconsistent if items added/removed
```

### Cursor Pagination
```
GET /posts?limit=20                             → returns: data + next_cursor=abc123
GET /posts?limit=20&cursor=abc123               → next page from cursor position

Pros: consistent, fast (no offset), works with real-time data
Cons: can't jump to arbitrary page
```

### Page-based Pagination
```
GET /posts?page=1&page_size=20
GET /posts?page=2&page_size=20

Pros: intuitive for users
Cons: same problems as offset pagination under the hood
```

> **Recommendation**: Use **cursor pagination** for feeds and timelines. Use **offset** for admin UIs where page-jumping matters.

---

## Key Takeaways

1. **REST**: resource-based, stateless, use correct HTTP verbs + status codes
2. **GraphQL**: when clients need flexible queries; add DataLoader to prevent N+1
3. **API Gateway**: single entry point for auth, rate limiting, routing
4. **BFF**: tailor APIs to each client type for optimal experience
5. **Rate limiting**: use sliding window counter + Redis for distributed limiting
6. **JWT**: stateless auth — keep TTL short (15 min) and use refresh tokens
7. **Idempotency keys**: critical for payment and mutation APIs
8. **Cursor pagination**: for feeds; offset for admin/search
