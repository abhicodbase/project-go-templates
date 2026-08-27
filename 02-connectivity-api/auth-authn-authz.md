# Authentication & Authorization

## Concepts

```
Authentication (AuthN): WHO are you? (identity verification)
Authorization  (AuthZ): WHAT can you do? (access control)

Sequence:
  Request → Auth Middleware → AuthN (is token valid?) → AuthZ (has permission?) → Handler
```

---

## JWT (JSON Web Token)

### Structure
```
header.payload.signature

Header:  {"alg": "RS256", "typ": "JWT"}
Payload: {"sub": "user123", "role": "admin", "exp": 1735689600, "iat": 1735686000}
Signature: RSA_Sign(base64(header) + "." + base64(payload), private_key)
```

### JWT Middleware in Go
```go
package auth

import (
    "context"
    "net/http"
    "strings"
    "time"

    "github.com/golang-jwt/jwt/v5"
)

type Claims struct {
    UserID string   `json:"sub"`
    Email  string   `json:"email"`
    Roles  []string `json:"roles"`
    jwt.RegisteredClaims
}

type JWTMiddleware struct {
    publicKey interface{} // RSA public key for RS256
    secretKey []byte      // HMAC secret for HS256
}

func (m *JWTMiddleware) Authenticate(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract token from Authorization header
        authHeader := r.Header.Get("Authorization")
        if !strings.HasPrefix(authHeader, "Bearer ") {
            http.Error(w, `{"code":"UNAUTHENTICATED"}`, http.StatusUnauthorized)
            return
        }
        tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

        // Parse and validate
        claims := &Claims{}
        token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
            // Verify signing algorithm
            if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
                return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
            }
            return m.secretKey, nil
        })

        if err != nil || !token.Valid {
            http.Error(w, `{"code":"INVALID_TOKEN"}`, http.StatusUnauthorized)
            return
        }

        // Check expiry
        if claims.ExpiresAt.Before(time.Now()) {
            http.Error(w, `{"code":"TOKEN_EXPIRED"}`, http.StatusUnauthorized)
            return
        }

        // Store claims in context
        ctx := context.WithValue(r.Context(), claimsKey, claims)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// Helper to extract claims from context
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
    claims, ok := ctx.Value(claimsKey).(*Claims)
    return claims, ok
}
```

### Role-Based Authorization
```go
func RequireRole(roles ...string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            claims, ok := ClaimsFromContext(r.Context())
            if !ok {
                http.Error(w, "unauthorized", http.StatusUnauthorized)
                return
            }

            for _, required := range roles {
                for _, userRole := range claims.Roles {
                    if userRole == required {
                        next.ServeHTTP(w, r)
                        return
                    }
                }
            }

            http.Error(w, "forbidden", http.StatusForbidden)
        })
    }
}

// Usage
mux.Handle("/admin/hotels", 
    authMiddleware.Authenticate(
        RequireRole("admin", "superuser")(
            http.HandlerFunc(h.AdminHotels),
        ),
    ),
)
```

---

## OAuth2 / OpenID Connect Flow

```
1. User clicks "Login with Google"
2. App redirects to Google's authorization endpoint
   GET https://accounts.google.com/oauth2/v2/auth
       ?client_id=xxx
       &redirect_uri=https://agoda.com/callback
       &scope=openid email profile
       &state=random_csrf_token
       &response_type=code

3. User logs in to Google, grants permission
4. Google redirects back:
   GET https://agoda.com/callback?code=AUTH_CODE&state=random_csrf_token

5. App exchanges code for tokens:
   POST https://oauth2.googleapis.com/token
   {code, client_id, client_secret, redirect_uri}
   → Returns: {access_token, id_token, refresh_token}

6. App validates id_token (JWT) to get user identity
```

---

## API Key Authentication

```go
// Simple API key validation middleware
func APIKeyMiddleware(validKeys map[string]string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            apiKey := r.Header.Get("X-API-Key")
            if apiKey == "" {
                apiKey = r.URL.Query().Get("api_key")
            }

            clientID, valid := validKeys[apiKey]
            if !valid {
                http.Error(w, "invalid API key", http.StatusUnauthorized)
                return
            }

            ctx := context.WithValue(r.Context(), clientIDKey, clientID)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

---

## mTLS (Mutual TLS) — for service-to-service

```
Regular TLS: Client verifies server certificate
mTLS:        Client AND server verify each other's certificates

Use case: Internal microservices — ensures only legitimate services can call each other
Implementation: Usually handled by service mesh (Istio/Linkerd), not application code
```

---

## Interview Q&A

**Q: What is the difference between JWT and session-based authentication?**
> A: Session-based stores session state on the server (in-memory or Redis), and the client holds a session ID cookie. The server must look up the session on every request. JWT is stateless — the server puts all claims in the token and signs it; no server-side storage needed. JWT pros: scales horizontally (any server can validate without a shared store), good for microservices. JWT cons: tokens can't be revoked before expiry (need blocklist for logout), token size is larger than session ID. For Agoda's scale with many microservices, JWT is better — each service can independently validate without calling a central auth service.

**Q: How do you handle JWT token refresh securely?**
> A: Use short-lived access tokens (15-60 min) and long-lived refresh tokens (days/weeks). Refresh tokens are stored server-side (in DB) so they can be revoked. When the access token expires, the client sends the refresh token to get a new access token. On logout, invalidate the refresh token in the DB. Optionally implement refresh token rotation: each use issues a new refresh token and invalidates the old one — if an old refresh token is used, it indicates theft and all tokens for that user are revoked.
