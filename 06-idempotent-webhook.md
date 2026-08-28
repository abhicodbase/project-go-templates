# Idempotent Webhook Processing

## Use case
A payment partner sends a webhook on successful payment. Partners retry webhooks by
design whenever they don't get a fast, clear 200 OK — meaning your handler WILL
receive the same event more than once eventually. Without dedup, a retried webhook
double-credits a wallet. This also needs signature verification so you're not
processing forged requests.

## Diagram

```
   Partner ──POST /webhook (event evt_abc123)──> Handler
                                                     │
                                            verify signature (HMAC)
                                                     │
                                    ┌───────────────┴───────────────┐
                                    │ event ID seen before?            │
                                    └───────────────┬───────────────┘
                                     NO              │            YES
                                      v               │             v
                            credit wallet,       skip credit,
                            mark ID processed    still return 200
                                      \_______________|_______________/
                                                      v
                                              200 OK to partner
                                        (partner stops retrying either way)
```

## Code

```go
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
)

var ErrInvalidSignature = errors.New("invalid webhook signature")

type WebhookEvent struct {
	ID          string // partner-assigned, unique per logical event
	PaymentID   string
	UserID      string
	AmountCents int64
	Signature   string
	RawBody     string
}

func verifySignature(body, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

type ProcessedStore struct {
	mu        sync.Mutex
	processed map[string]struct{}
}

func NewProcessedStore() *ProcessedStore { return &ProcessedStore{processed: make(map[string]struct{})} }

// MarkIfNew atomically checks-and-marks in one locked section — this is
// what prevents a race between two concurrent deliveries of the same
// retried webhook.
func (p *ProcessedStore) MarkIfNew(eventID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.processed[eventID]; exists {
		return false
	}
	p.processed[eventID] = struct{}{}
	return true
}

type WalletService struct {
	mu       sync.Mutex
	balances map[string]int64
}

func NewWalletService() *WalletService { return &WalletService{balances: make(map[string]int64)} }

func (w *WalletService) Credit(userID string, amountCents int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.balances[userID] += amountCents
}

type WebhookHandler struct {
	secret  string
	store   *ProcessedStore
	wallets *WalletService
}

func NewWebhookHandler(secret string, store *ProcessedStore, wallets *WalletService) *WebhookHandler {
	return &WebhookHandler{secret: secret, store: store, wallets: wallets}
}

func (h *WebhookHandler) Handle(e WebhookEvent) error {
	if !verifySignature(e.RawBody, e.Signature, h.secret) {
		return ErrInvalidSignature
	}
	if !h.store.MarkIfNew(e.ID) {
		fmt.Printf("  event %s already processed — skipping credit, returning 200\n", e.ID)
		return nil
	}
	h.wallets.Credit(e.UserID, e.AmountCents)
	fmt.Printf("  event %s processed — credited %d cents to %s\n", e.ID, e.AmountCents, e.UserID)
	return nil
}

func main() {
	secret := "whsec_test123"
	store := NewProcessedStore()
	wallets := NewWalletService()
	handler := NewWebhookHandler(secret, store, wallets)

	body := `{"payment_id":"pay_1","user_id":"u1","amount_cents":5000}`
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	sig := hex.EncodeToString(mac.Sum(nil))

	event := WebhookEvent{ID: "evt_abc123", PaymentID: "pay_1", UserID: "u1", AmountCents: 5000, Signature: sig, RawBody: body}

	fmt.Println("first delivery:")
	handler.Handle(event)

	fmt.Println("partner retries the same webhook (network blip on their side):")
	handler.Handle(event)

	fmt.Printf("\nfinal wallet balance for u1: %d cents (should be 5000, not 10000)\n", wallets.balances["u1"])
}
```

**Verified output:**
```
first delivery:
  event evt_abc123 processed — credited 5000 cents to u1
partner retries the same webhook (network blip on their side):
  event evt_abc123 already processed — skipping credit, returning 200

final wallet balance for u1: 5000 cents (should be 5000, not 10000)
```

## Why it matters for the interview
- This is the single most common "spot the bug" scenario across payment/booking
  systems — missing idempotency is on almost every "badly designed system" list.
- Note the **order of operations**: signature check first (reject forgeries before
  doing anything else), then the atomic check-and-mark (not a separate check THEN
  mark — that reintroduces the race), then the side effect.
- Follow-up to expect: "where would `processed` actually live in production?" — a DB
  table with a unique constraint on event ID (so the DB itself enforces atomicity), or
  a distributed cache with an atomic SETNX, not an in-memory map that dies with the
  process.
