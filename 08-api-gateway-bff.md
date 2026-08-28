# API Gateway / Backend-for-Frontend (BFF)

## Use case
A mobile home screen needs data from 3 different backend services (recent orders,
loyalty tier, recommendations). Without a gateway, the client makes 3 separate round
trips, each re-implementing auth, and each adding its own latency. A BFF sits in front,
authenticates once, fans out to backends **in parallel**, and returns one
purpose-shaped payload.

## Diagram

```
   Mobile Client
        │  1 request (token)
        v
   ┌──────────────────────────┐
   │        API Gateway / BFF     │  auth once, rate-limit once
   └───────────┬──────────────┘
                │ fans out in parallel
     ┌─────────┼─────────────┐
     v          v               v
┌─────────┐ ┌─────────┐ ┌───────────────┐
│  Order    │ │ Loyalty  │ │ Recommendation │
│  Service  │ │ Service  │ │ Service          │
└─────────┘ └─────────┘ └───────────────┘
     │          │               │
     └─────────┼───────────────┘
                v
     one aggregated HomeScreenPayload
                │
                v
          back to client
```

## Code

```go
package main

import (
	"fmt"
	"sync"
)

type AuthResult struct {
	UserID  string
	Allowed bool
}

type OrderServiceClient struct{}
func (OrderServiceClient) GetRecentOrders(userID string) []string { return []string{"order-101", "order-102"} }

type LoyaltyServiceClient struct{}
func (LoyaltyServiceClient) GetTier(userID string) string { return "gold" }

type RecommendationServiceClient struct{}
func (RecommendationServiceClient) GetRecommendations(userID string) []string { return []string{"hotel-A", "hotel-B"} }

type Gateway struct {
	orders  OrderServiceClient
	loyalty LoyaltyServiceClient
	recos   RecommendationServiceClient
	limiter *simpleLimiter
}

type simpleLimiter struct {
	mu    sync.Mutex
	count map[string]int
	max   int
}

func newSimpleLimiter(max int) *simpleLimiter { return &simpleLimiter{count: make(map[string]int), max: max} }

func (l *simpleLimiter) Allow(userID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.count[userID]++
	return l.count[userID] <= l.max
}

func authenticate(token string) AuthResult {
	// Production: verify JWT signature/expiry, or introspect an opaque
	// token against an auth service. Simplified here.
	if token == "valid-token" {
		return AuthResult{UserID: "u1", Allowed: true}
	}
	return AuthResult{Allowed: false}
}

type HomeScreenPayload struct {
	UserID          string
	LoyaltyTier     string
	RecentOrders    []string
	Recommendations []string
}

func (g *Gateway) GetHomeScreen(token string) (*HomeScreenPayload, error) {
	auth := authenticate(token)
	if !auth.Allowed {
		return nil, fmt.Errorf("unauthorized")
	}
	if !g.limiter.Allow(auth.UserID) {
		return nil, fmt.Errorf("rate limit exceeded for user %s", auth.UserID)
	}

	// Fan out in parallel — independent calls, no reason to serialize.
	var wg sync.WaitGroup
	var orders []string
	var tier string
	var recos []string

	wg.Add(3)
	go func() { defer wg.Done(); orders = g.orders.GetRecentOrders(auth.UserID) }()
	go func() { defer wg.Done(); tier = g.loyalty.GetTier(auth.UserID) }()
	go func() { defer wg.Done(); recos = g.recos.GetRecommendations(auth.UserID) }()
	wg.Wait()

	return &HomeScreenPayload{UserID: auth.UserID, LoyaltyTier: tier, RecentOrders: orders, Recommendations: recos}, nil
}

func main() {
	gateway := &Gateway{limiter: newSimpleLimiter(5)}

	payload, err := gateway.GetHomeScreen("valid-token")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("aggregated BFF response: %+v\n", *payload)

	_, err = gateway.GetHomeScreen("bad-token")
	fmt.Println("unauthenticated call result:", err)
}
```

**Verified output:**
```
aggregated BFF response: {UserID:u1 LoyaltyTier:gold RecentOrders:[order-101 order-102] Recommendations:[hotel-A hotel-B]}
unauthenticated call result: unauthorized
```

## Why it matters for the interview
- Directly answers "how would you avoid the client making 3 round trips" and "where
  does auth/rate-limiting belong" — centralizing cross-cutting concerns at the edge
  instead of duplicating them in every backend service.
- Note the **parallel fan-out** — this is the fix for the Loan Eligibility Engine's
  "5 sequential synchronous calls" anti-pattern, applied at the gateway layer.
- Follow-up to expect: "what if one of the 3 backend calls is slow?" — this simple
  version has no timeout/circuit breaker per call; in production you'd wrap each
  `go func()` call with its own timeout and either fail the whole request or return a
  partial response with the slow field omitted, depending on product requirements.
