# Testing Strategies — The Test Pyramid

## Test Pyramid

```
           /\
          /  \
         / E2E \           ← Few, slow, expensive, high confidence
        /--------\
       /Integration\       ← Some, moderate speed/cost
      /--------------\
     /   Unit Tests   \    ← Many, fast, cheap, isolated
    /------------------\
```

**Rule of thumb**: 70% unit, 20% integration, 10% E2E

---

## Unit Tests

Test a single function/method in complete isolation.

```go
// hotel_service_test.go
package service_test

import (
    "context"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

// Mock repository
type MockHotelRepository struct {
    mock.Mock
}

func (m *MockHotelRepository) Get(ctx context.Context, id string) (*Hotel, error) {
    args := m.Called(ctx, id)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*Hotel), args.Error(1)
}

func TestGetHotel_Success(t *testing.T) {
    // Arrange
    mockRepo := &MockHotelRepository{}
    service := NewHotelService(mockRepo)
    
    expectedHotel := &Hotel{ID: "hotel-123", Name: "Grand Palace"}
    mockRepo.On("Get", mock.Anything, "hotel-123").Return(expectedHotel, nil)

    // Act
    hotel, err := service.GetHotel(context.Background(), "hotel-123")

    // Assert
    assert.NoError(t, err)
    assert.Equal(t, expectedHotel.Name, hotel.Name)
    mockRepo.AssertExpectations(t)
}

func TestGetHotel_NotFound(t *testing.T) {
    mockRepo := &MockHotelRepository{}
    service := NewHotelService(mockRepo)
    
    mockRepo.On("Get", mock.Anything, "missing").Return(nil, ErrNotFound)

    _, err := service.GetHotel(context.Background(), "missing")
    
    assert.ErrorIs(t, err, ErrNotFound)
    mockRepo.AssertExpectations(t)
}
```

---

## Table-Driven Tests (Go Idiomatic)

```go
func TestCalculateRefund(t *testing.T) {
    tests := []struct {
        name           string
        booking        Booking
        cancellationDate time.Time
        want           float64
        wantErr        bool
    }{
        {
            name:    "full refund when cancelled 7+ days before check-in",
            booking: Booking{CheckIn: date("2024-02-15"), TotalPrice: 500},
            cancellationDate: date("2024-02-07"),
            want:    500,
        },
        {
            name:    "50% refund when cancelled 3-6 days before",
            booking: Booking{CheckIn: date("2024-02-15"), TotalPrice: 500},
            cancellationDate: date("2024-02-11"),
            want:    250,
        },
        {
            name:    "no refund when cancelled < 3 days before",
            booking: Booking{CheckIn: date("2024-02-15"), TotalPrice: 500},
            cancellationDate: date("2024-02-13"),
            want:    0,
        },
        {
            name:    "error when cancellation date is after check-in",
            booking: Booking{CheckIn: date("2024-02-15"), TotalPrice: 500},
            cancellationDate: date("2024-02-16"),
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := CalculateRefund(tt.booking, tt.cancellationDate)
            if tt.wantErr {
                assert.Error(t, err)
                return
            }
            assert.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

---

## TDD Cycle (Red → Green → Refactor)

```
1. RED    → Write a failing test for the new behavior
2. GREEN  → Write the minimum code to make the test pass
3. REFACTOR → Clean up the code while keeping tests green
```

```go
// Step 1: RED — write failing test
func TestTokenBucket_Allow(t *testing.T) {
    tb := NewTokenBucket(3, 1) // 3 tokens, 1/sec refill
    
    // Should allow first 3 requests
    assert.True(t, tb.Allow())
    assert.True(t, tb.Allow())
    assert.True(t, tb.Allow())
    
    // 4th should be rejected (bucket empty)
    assert.False(t, tb.Allow())
}

// Step 2: GREEN — implement TokenBucket to pass the test
// Step 3: REFACTOR — improve the implementation
```

---

## Integration Tests

Test multiple components working together (with real DB, real cache, etc.)

```go
//go:build integration

package integration_test

import (
    "context"
    "database/sql"
    "testing"
    _ "github.com/lib/pq"
)

// Use testcontainers for real DB
func TestHotelRepository_Integration(t *testing.T) {
    // Start a real PostgreSQL container
    ctx := context.Background()
    
    db, cleanup := setupTestDB(t)
    defer cleanup()
    
    repo := NewHotelRepository(db)
    
    // Test real DB behavior
    hotel := &Hotel{ID: "test-hotel", Name: "Test Palace", City: "Bangkok"}
    
    err := repo.Create(ctx, hotel)
    assert.NoError(t, err)
    
    retrieved, err := repo.Get(ctx, hotel.ID)
    assert.NoError(t, err)
    assert.Equal(t, hotel.Name, retrieved.Name)
    
    err = repo.Delete(ctx, hotel.ID)
    assert.NoError(t, err)
    
    _, err = repo.Get(ctx, hotel.ID)
    assert.ErrorIs(t, err, ErrNotFound)
}

// Run integration tests separately:
// go test -tags=integration ./...
```

---

## Non-Functional Testing

```
Load Testing    → Can the system handle expected traffic?
Stress Testing  → At what point does the system break?
Spike Testing   → Can it handle sudden traffic bursts?
Soak Testing    → Does it degrade over long periods? (memory leaks)
Chaos Testing   → What happens when things fail? (Chaos Monkey)
```

Tools: k6, Gatling, Apache JMeter, Locust

```javascript
// k6 load test example
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '2m', target: 100 },  // ramp up to 100 users
    { duration: '5m', target: 100 },  // sustain
    { duration: '2m', target: 0 },    // ramp down
  ],
  thresholds: {
    http_req_duration: ['p95<500'],   // 95th percentile < 500ms
    http_req_failed: ['rate<0.01'],   // < 1% error rate
  },
};

export default function() {
  const res = http.get('https://api.agoda.com/v1/hotels/123');
  check(res, { 'status 200': (r) => r.status === 200 });
  sleep(1);
}
```

---

## Interview Q&A

**Q: How do you decide what to unit test vs integration test?**
> A: Unit tests focus on business logic — the rules and calculations that don't depend on infrastructure (database, HTTP calls, etc.). These are fast and can run on every commit. Integration tests cover the contract between your code and external systems — does your SQL query actually work against a real DB schema? Does your HTTP client handle the actual API response format? Integration tests are slower but give confidence that components work together. A good heuristic: mock at the boundary of your domain (repositories, external APIs), don't mock your own business logic.

**Q: What is your testing approach for a new feature?**
> A: I follow TDD for business logic — write the test first, implement to pass it. For a new booking feature: (1) unit tests for business rules (refund calculation, validation); (2) integration tests for the repository layer against a real test DB using testcontainers; (3) API-level tests to verify HTTP handler behavior; (4) contract tests if other services depend on the API. I also consider the test pyramid — not everything needs to be tested at every level. High-risk business logic gets the most unit test coverage.
