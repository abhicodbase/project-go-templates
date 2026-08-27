# API Error Handling

## Principles of Good Error Handling

1. **Distinguish client errors from server errors** (4xx vs 5xx)
2. **Never expose internal details** in error responses (stack traces, SQL errors)
3. **Return machine-readable error codes** alongside human-readable messages
4. **Include trace ID** for debugging
5. **Consistent error format** across all endpoints

---

## Error Response Standard

```go
// Standard error response format
type ErrorResponse struct {
    Code    string       `json:"code"`              // machine-readable
    Message string       `json:"message"`           // human-readable
    Details []FieldError `json:"details,omitempty"` // validation errors
    TraceID string       `json:"trace_id"`          // for support/debugging
}

type FieldError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
}

// Example responses:
// 400 validation error:
// {"code":"VALIDATION_ERROR","message":"Invalid request","details":[{"field":"check_in","message":"must be a future date"}],"trace_id":"abc123"}

// 404 not found:
// {"code":"HOTEL_NOT_FOUND","message":"Hotel with ID 123 not found","trace_id":"abc123"}

// 500 internal error (no internal details):
// {"code":"INTERNAL_ERROR","message":"An unexpected error occurred","trace_id":"abc123"}
```

---

## Sentinel Errors and Error Wrapping

```go
// Define domain errors
var (
    ErrNotFound        = errors.New("not found")
    ErrDuplicate       = errors.New("duplicate")
    ErrValidation      = errors.New("validation error")
    ErrUnauthorized    = errors.New("unauthorized")
    ErrForbidden       = errors.New("forbidden")
)

// Wrap errors with context
func (r *HotelRepository) Get(ctx context.Context, id string) (*Hotel, error) {
    var hotel Hotel
    err := r.db.QueryRowContext(ctx, "SELECT * FROM hotels WHERE id=$1", id).Scan(&hotel.ID, &hotel.Name)
    if err == sql.ErrNoRows {
        return nil, fmt.Errorf("hotel %s: %w", id, ErrNotFound) // wrap with context
    }
    if err != nil {
        return nil, fmt.Errorf("hotel query: %w", err)
    }
    return &hotel, nil
}

// In the HTTP handler — use errors.Is() to check error type
func (h *HotelHandler) GetHotel(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    hotel, err := h.service.GetHotel(r.Context(), id)
    if err != nil {
        switch {
        case errors.Is(err, ErrNotFound):
            writeError(w, http.StatusNotFound, "HOTEL_NOT_FOUND", "Hotel not found")
        case errors.Is(err, ErrValidation):
            writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
        case errors.Is(err, ErrUnauthorized):
            writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
        case errors.Is(err, ErrForbidden):
            writeError(w, http.StatusForbidden, "FORBIDDEN", "Access denied")
        default:
            log.Error("unexpected error", "err", err, "trace_id", traceID(r.Context()))
            writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred")
        }
        return
    }
    writeJSON(w, http.StatusOK, hotel)
}
```

---

## Input Validation

```go
type CreateBookingRequest struct {
    HotelID  string    `json:"hotel_id" validate:"required,uuid"`
    CheckIn  time.Time `json:"check_in" validate:"required"`
    CheckOut time.Time `json:"check_out" validate:"required"`
    Guests   int       `json:"guests" validate:"required,min=1,max=20"`
}

func (h *BookingHandler) Create(w http.ResponseWriter, r *http.Request) {
    var req CreateBookingRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "INVALID_JSON", "Request body is not valid JSON")
        return
    }

    // Validate
    if errs := validate(req); len(errs) > 0 {
        resp := ErrorResponse{
            Code:    "VALIDATION_ERROR",
            Message: "Request validation failed",
            Details: errs,
        }
        writeJSON(w, http.StatusBadRequest, resp)
        return
    }

    // Business validation
    if !req.CheckOut.After(req.CheckIn) {
        writeError(w, http.StatusUnprocessableEntity, "INVALID_DATES", "check_out must be after check_in")
        return
    }
    ...
}
```

---

## Retry Strategies for Clients

```go
// What is retryable?
func isRetryable(err error) bool {
    if err == nil { return false }
    
    // Network/timeout errors — retryable
    var netErr *net.OpError
    if errors.As(err, &netErr) { return true }
    
    // HTTP status codes
    var httpErr *HTTPError
    if errors.As(err, &httpErr) {
        // 429 (rate limit) and 503 (temporarily unavailable) — retryable
        // 5xx server errors — retryable (usually transient)
        // 4xx client errors — NOT retryable (client bug)
        return httpErr.StatusCode == 429 || 
               httpErr.StatusCode == 503 || 
               (httpErr.StatusCode >= 500 && httpErr.StatusCode != 501)
    }
    return false
}
```

---

## Interview Q&A

**Q: How should you handle errors returned by external APIs vs your own service errors?**
> A: Categorize external API errors before exposing them: (1) Retryable vs non-retryable — retry 503/timeout, don't retry 400s; (2) Map to your domain — translate external error codes to your own error codes; (3) Log the original error for debugging but surface a friendly message to the client; (4) Circuit-break repeated failures from the same external service. Never leak raw external API errors to your clients — they change their error formats, contain internal details, and break your API contract.
