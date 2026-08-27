# REST API Best Practices

## Core REST Principles (HATEOAS-lite)

```
Resource-based URLs (nouns, not verbs)
✅ GET    /hotels               — list hotels
✅ GET    /hotels/{id}          — get hotel
✅ POST   /hotels               — create hotel
✅ PUT    /hotels/{id}          — full update
✅ PATCH  /hotels/{id}          — partial update
✅ DELETE /hotels/{id}          — delete

❌ GET /getHotel?id=123         — verb in URL
❌ POST /createBooking          — verb in URL
❌ GET /deleteHotel/123         — delete via GET (unsafe!)
```

---

## API Versioning

```
Option 1: URL path (most common, most visible)
  /v1/hotels, /v2/hotels

Option 2: Header
  Accept: application/vnd.agoda.v2+json

Option 3: Query parameter
  /hotels?version=2

Recommendation: URL path for public APIs (easier to test, cache, document)
```

---

## Status Codes — Know These Cold

```
2xx Success:
  200 OK          — GET, PUT, PATCH success
  201 Created     — POST created resource (include Location header)
  202 Accepted    — async operation started
  204 No Content  — DELETE success, no body

3xx Redirection:
  301 Moved Permanently — resource URL changed
  304 Not Modified      — client cache is still valid (ETag match)

4xx Client Errors:
  400 Bad Request        — malformed request, validation failure
  401 Unauthorized       — not authenticated (missing/invalid token)
  403 Forbidden          — authenticated but not authorized
  404 Not Found          — resource doesn't exist
  409 Conflict           — duplicate resource, version conflict
  422 Unprocessable      — syntactically valid but semantically invalid
  429 Too Many Requests  — rate limited (include Retry-After header)

5xx Server Errors:
  500 Internal Server Error — unexpected error
  502 Bad Gateway           — upstream service error
  503 Service Unavailable   — temporarily down (maintenance, overload)
  504 Gateway Timeout       — upstream timeout
```

---

## Pagination

```go
// Cursor-based (preferred for large/changing datasets)
GET /hotels?cursor=eyJpZCI6MTIzfQ==&limit=20
Response:
{
  "data": [...],
  "next_cursor": "eyJpZCI6MTQzfQ==",
  "has_more": true
}

// Offset-based (simpler, but inconsistent with live data changes)
GET /hotels?offset=40&limit=20
Response:
{
  "data": [...],
  "total": 1500,
  "offset": 40,
  "limit": 20
}
```

**Cursor-based pros**: Stable (insertions don't shift pages), scalable (DB doesn't scan from start)
**Offset-based pros**: Random access to any page, easier to implement

---

## Request/Response Design

```go
// ✅ Consistent error response body
type APIError struct {
    Code    string `json:"code"`    // machine-readable "HOTEL_NOT_FOUND"
    Message string `json:"message"` // human-readable
    Details []FieldError `json:"details,omitempty"` // validation errors
    TraceID string `json:"trace_id"` // for debugging
}

type FieldError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
}

// Handler example
func (h *HotelHandler) GetHotel(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    
    hotel, err := h.service.GetHotel(r.Context(), id)
    if err != nil {
        if errors.Is(err, ErrNotFound) {
            writeError(w, http.StatusNotFound, "HOTEL_NOT_FOUND", "Hotel not found")
            return
        }
        writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
        return
    }
    
    writeJSON(w, http.StatusOK, hotel)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
    writeJSON(w, status, APIError{Code: code, Message: message})
}
```

---

## Idempotency

```go
// POST /bookings with Idempotency-Key header
// Client retries → same result, no duplicate booking
func (h *BookingHandler) Create(w http.ResponseWriter, r *http.Request) {
    idempotencyKey := r.Header.Get("Idempotency-Key")
    
    // Check if we already processed this request
    if idempotencyKey != "" {
        if cached, found := h.idempotencyCache.Get(idempotencyKey); found {
            writeJSON(w, http.StatusOK, cached) // return previous response
            return
        }
    }
    
    booking, err := h.service.CreateBooking(r.Context(), req)
    if err != nil { ... }
    
    // Cache the response for idempotency
    if idempotencyKey != "" {
        h.idempotencyCache.Set(idempotencyKey, booking, 24*time.Hour)
    }
    
    w.Header().Set("Location", "/bookings/"+booking.ID)
    writeJSON(w, http.StatusCreated, booking)
}
```

---

## Caching Headers

```go
// Set cache headers for GET responses
func cacheMiddleware(maxAge time.Duration) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if r.Method == "GET" {
                w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(maxAge.Seconds())))
                w.Header().Set("ETag", computeETag(r.URL.String()))
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

---

## Interview Q&A

**Q: What is the difference between PUT and PATCH?**
> A: PUT is for full replacement — the client sends the complete resource representation. If a field is missing in the PUT body, it gets cleared. PATCH is for partial update — only send the fields you want to change. In practice, PATCH is safer for clients (no accidental field clearing) but harder to implement idempotently.

**Q: When should you use 401 vs 403?**
> A: 401 means "I don't know who you are" — the request lacks valid authentication (missing or expired token). 403 means "I know who you are, but you don't have permission" — authenticated but not authorized for this resource. The 401 response should include a `WWW-Authenticate` header telling the client how to authenticate.

**Q: How do you handle API versioning at Agoda scale?**
> A: URL path versioning (`/v1/`, `/v2/`) is most practical because it's explicit, easy to route in API gateways, cache-friendly (CDN can cache per version), and easy to test. The key is maintaining backward compatibility within a version — only introduce breaking changes in new versions. We'd run multiple versions simultaneously during migration, with a sunset date communicated to API consumers. When retiring a version, we deprecate it with a `Deprecation` header first.
