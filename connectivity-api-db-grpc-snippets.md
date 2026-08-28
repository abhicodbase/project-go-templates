# 6 Targeted Snippets: Connectivity, API, DB, gRPC

Read each cold, narrate out loud what's wrong before checking the answer key below it.
Each maps to a specific "Connectivity Management" or "Code Review" theme from the toolkit.

---

## Snippet 1 — REST client: retries, timeouts, sync/async

```go
func GetSupplierRates(supplierID string) (*Rates, error) {
	resp, err := http.Get("https://supplier-" + supplierID + ".partner.com/rates")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rates Rates
	json.NewDecoder(resp.Body).Decode(&rates)
	return &rates, nil
}
```

**Issues to name:**
- `http.Get` uses the default client with **no timeout** — a hung partner hangs this call forever.
- **No retry/backoff** at all — a single transient failure (5xx, connection reset) fails the whole call.
- **No context.Context** passed in — caller can't cancel or set a deadline from upstream.
- Decode error ignored — malformed JSON silently produces a zero-value `Rates{}` instead of an error.
- No check on `resp.StatusCode` — a 404 or 500 response body still gets decoded as if it were valid rates.

**Fix direction:** inject `context.Context` with deadline, use a client with explicit `Timeout`, wrap in exponential backoff + jitter (bounded attempts), check status code before decoding, propagate decode errors.

---

## Snippet 2 — gRPC service: deadlines, error codes

```protobuf
service PricingService {
  rpc GetPrice(PriceRequest) returns (PriceResponse);
}
```

```go
func (s *server) GetPrice(ctx context.Context, req *pb.PriceRequest) (*pb.PriceResponse, error) {
	price, err := s.db.FetchPrice(req.FlightId)
	if err != nil {
		return nil, err // raw error returned
	}
	if price == nil {
		return nil, err // bug: err is nil here, returns nil, nil
	}
	return &pb.PriceResponse{Price: price.Amount}, nil
}
```

**Issues to name:**
- Returning the **raw Go error** instead of a proper `status.Error` with a gRPC code (`codes.NotFound`, `codes.Internal`, etc.) — clients can't distinguish "not found" from "internal failure" programmatically.
- **Bug**: `if price == nil { return nil, err }` — `err` is `nil` at that point (the earlier check already passed), so this silently returns `nil, nil`, an invalid gRPC response.
- No use of the incoming `ctx`'s deadline — if `s.db.FetchPrice` doesn't respect context cancellation, a client that gave up waiting still ties up server resources.
- No **interceptor-level** concern raised: logging, auth, and metrics are typically handled via gRPC interceptors, not inline in every handler — worth mentioning if asked "how would you apply this pattern at scale."

**Fix direction:** `return nil, status.Errorf(codes.NotFound, "price not found for flight %s", req.FlightId)`; pass `ctx` through to `FetchPrice` so it can respect cancellation; add auth/logging interceptors once, not per-handler.

---

## Snippet 3 — DB: connection init per call, N+1

```go
func GetUserLoans(userID string) ([]Loan, error) {
	db, err := sql.Open("mysql", dsn) // opened fresh every call
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, _ := db.Query("SELECT id FROM loans WHERE user_id = ?", userID)
	var loanIDs []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		loanIDs = append(loanIDs, id)
	}

	var loans []Loan
	for _, id := range loanIDs {
		row := db.QueryRow("SELECT amount, status FROM loans WHERE id = ?", id)
		var l Loan
		row.Scan(&l.Amount, &l.Status)
		loans = append(loans, l)
	}
	return loans, nil
}
```

**Issues to name:**
- `sql.Open` called **inside the function, per request** — this doesn't actually open a connection immediately, but recreating the pool object per call defeats connection reuse/pooling entirely and is a classic "expensive init per call" smell called out in the toolkit.
- **N+1 query** — fetches loan IDs, then queries once per ID individually instead of a single `WHERE id IN (...)` or a join.
- Errors from `Query`/`Scan` silently discarded.
- No pagination — `GetUserLoans` returns unbounded results; a user with thousands of loans returns everything at once.

**Fix direction:** inject a shared `*sql.DB` (pooled, created once at startup), collapse the two queries into one with `IN (...)` or a join, propagate errors, add cursor-based pagination if the result set can grow large.

---

## Snippet 4 — API authentication / hardcoded secrets

```go
func CallPartnerAPI(payload []byte) (*http.Response, error) {
	req, _ := http.NewRequest("POST", "https://partner.com/api/submit", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer xxxx")
	return http.DefaultClient.Do(req)
}
```

**Issues to name:**
- **Hardcoded API key/secret** directly in source — should come from a secrets manager or env-injected config, never committed to source control.
- Uses `http.DefaultClient` — shared across the whole process with no configured timeout, same issue as Snippet 1.
- `http.NewRequest` error ignored (`_`).
- No context passed for cancellation/deadline propagation.

**Fix direction:** pull credentials from a secrets manager/config at startup, inject as a dependency; use a configured client with timeout; handle the request-construction error.

---

## Snippet 5 — Sync vs async: webhook without idempotency (connectivity pattern)

```go
func HandlePartnerWebhook(w http.ResponseWriter, r *http.Request) {
	var event PartnerEvent
	json.NewDecoder(r.Body).Decode(&event)

	go processEvent(event) // fire and forget, no tracking

	w.WriteHeader(http.StatusOK)
}
```

**Issues to name:**
- **`go processEvent(event)`** launches an untracked goroutine per webhook with no supervision — if it panics, there's no recovery, no visibility, and the caller already got a `200 OK` believing it succeeded.
- No idempotency check on `event.ID` — a partner retry (which webhooks are designed to do) reprocesses the same event.
- No queue/durability — if the process crashes right after accepting the request, the event is lost entirely since it only lived in an in-memory goroutine.
- Decode error ignored.

**Fix direction:** acknowledge fast but durably — write the event to a queue (Kafka/SQS) synchronously before returning 200, process asynchronously from the queue with an idempotency check against `event.ID`, and use a DLQ for events that fail repeatedly.

---

## Snippet 6 — API contract/versioning break

```go
// v1 handler, still in production
type UserResponse struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// someone "improves" it in place:
type UserResponse struct {
	FullName string `json:"full_name"` // renamed field, same endpoint, same version
	Email    string `json:"email"`
}
```

**Issues to name:**
- **Breaking change shipped on an existing API version** — renaming `name` → `full_name` in place silently breaks every consumer still expecting `name`, with no version bump, no deprecation window, no communication.
- No **additive-only** discipline — the correct move is to add `full_name` alongside `name` (or bump to a new API version) and deprecate the old field on a timeline.
- No contract test would have caught this if consumer-driven contract testing wasn't in place.

**Fix direction:** treat this as a versioning/API-contract discussion — additive changes only within a version, explicit deprecation windows, contract tests against consumer expectations before merging.

---

## How to use these in the round

For each: **read → explain what it does → name the issues out loud in priority order (correctness/security first, then reliability, then style) → propose the fix, ideally rewriting the critical line live.** Then, if given the chance, zoom out: "at scale, this pattern shows up in X" — tie it back to a system-level concern (pooling, idempotency, versioning) rather than stopping at the line-level fix. That zoom-out is exactly what "System Analysis & Design" and "Connectivity Management" are grading for, per the toolkit.
