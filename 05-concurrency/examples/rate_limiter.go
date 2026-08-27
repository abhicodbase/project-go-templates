// rate_limiter.go — Token bucket rate limiter (in-memory)
package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// TokenBucket implements the token bucket rate limiting algorithm
type TokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	capacity   float64       // max tokens
	refillRate float64       // tokens per second
	lastRefill time.Time
}

func NewTokenBucket(capacity float64, refillRate float64) *TokenBucket {
	return &TokenBucket{
		tokens:     capacity,
		capacity:   capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed, consuming 1 token
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.lastRefill = now

	// Refill tokens based on elapsed time
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}

	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}
	return false
}

// WaitForToken blocks until a token is available or context is done
func (tb *TokenBucket) WaitForToken(ctx context.Context) error {
	for {
		if tb.Allow() {
			return nil
		}
		select {
		case <-time.After(10 * time.Millisecond):
			// retry after short sleep
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// =========================================================
// Per-key rate limiter (for API key limiting)
// =========================================================

type MultiKeyRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*TokenBucket
	capacity   float64
	refillRate float64
}

func NewMultiKeyRateLimiter(capacity float64, refillRate float64) *MultiKeyRateLimiter {
	return &MultiKeyRateLimiter{
		buckets:    make(map[string]*TokenBucket),
		capacity:   capacity,
		refillRate: refillRate,
	}
}

func (m *MultiKeyRateLimiter) Allow(apiKey string) bool {
	m.mu.Lock()
	bucket, exists := m.buckets[apiKey]
	if !exists {
		bucket = NewTokenBucket(m.capacity, m.refillRate)
		m.buckets[apiKey] = bucket
	}
	m.mu.Unlock()

	return bucket.Allow()
}

// =========================================================
// Demo
// =========================================================

func main() {
	fmt.Println("=== Token Bucket Rate Limiter Demo ===")
	fmt.Println("Limit: 5 requests/second, capacity 5 tokens\n")

	limiter := NewTokenBucket(5, 5) // 5 tokens capacity, 5/sec refill

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	requestCount := 0
	allowedCount := 0
	rejectedCount := 0

	ticker := time.NewTicker(100 * time.Millisecond) // 10 requests/sec attempted
	defer ticker.Stop()

	for {
		select {
		case t := <-ticker.C:
			requestCount++
			if limiter.Allow() {
				allowedCount++
				fmt.Printf("[%v] Request %d: ALLOWED ✓\n", t.Format("15:04:05.000"), requestCount)
			} else {
				rejectedCount++
				fmt.Printf("[%v] Request %d: REJECTED ✗ (rate limited)\n", t.Format("15:04:05.000"), requestCount)
			}
		case <-ctx.Done():
			fmt.Printf("\nSummary: Total=%d, Allowed=%d, Rejected=%d\n",
				requestCount, allowedCount, rejectedCount)
			fmt.Printf("Allow rate: %.1f%%\n", float64(allowedCount)/float64(requestCount)*100)
			return
		}
	}
}
