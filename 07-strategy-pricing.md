# Strategy Pattern (Dynamic Pricing)

## Use case
Different discount rules apply depending on context: loyalty tier, bulk quantity,
seasonal promotions. A naive implementation grows a long if/else or switch chain
that's edited every time a new rule is added — violating Open/Closed and making the
function increasingly risky to touch. The strategy pattern makes each rule its own
type behind a shared interface; adding a new rule means adding a new type, not editing
existing code.

## Diagram

```
                 ┌────────────────────┐
                 │   PricingStrategy    │  <<interface>>
                 │  Apply(Order) float64 │
                 └────────┬───────────┘
                            │ implements
        ┌──────────────────┼──────────────────┐
        v                   v                    v
┌───────────────┐ ┌───────────────────┐ ┌──────────────────┐
│ StandardPricing  │ │ LoyaltyTierDiscount │ │ BulkQuantityDiscount │
└───────────────┘ └───────────────────┘ └──────────────────┘
        ^
        │ selected at runtime by
   PricingEngine.PriceWith(name, order)
```

## Code

```go
package main

import "fmt"

type Order struct {
	BaseAmount float64
	UserTier   string
	ItemCount  int
}

type PricingStrategy interface {
	Name() string
	Apply(o Order) float64
}

type StandardPricing struct{}
func (StandardPricing) Name() string       { return "standard" }
func (StandardPricing) Apply(o Order) float64 { return o.BaseAmount }

type LoyaltyTierDiscount struct{}
func (LoyaltyTierDiscount) Name() string { return "loyalty-tier" }
func (LoyaltyTierDiscount) Apply(o Order) float64 {
	discount := 0.0
	switch o.UserTier {
	case "gold":
		discount = 0.15
	case "silver":
		discount = 0.08
	}
	return o.BaseAmount * (1 - discount)
}

type BulkQuantityDiscount struct{}
func (BulkQuantityDiscount) Name() string { return "bulk-quantity" }
func (BulkQuantityDiscount) Apply(o Order) float64 {
	if o.ItemCount >= 10 {
		return o.BaseAmount * 0.90
	}
	if o.ItemCount >= 5 {
		return o.BaseAmount * 0.95
	}
	return o.BaseAmount
}

type PricingEngine struct {
	strategies map[string]PricingStrategy
}

func NewPricingEngine() *PricingEngine { return &PricingEngine{strategies: make(map[string]PricingStrategy)} }

func (p *PricingEngine) Register(s PricingStrategy) { p.strategies[s.Name()] = s }

func (p *PricingEngine) PriceWith(strategyName string, o Order) (float64, error) {
	s, ok := p.strategies[strategyName]
	if !ok {
		return 0, fmt.Errorf("unknown pricing strategy: %s", strategyName)
	}
	return s.Apply(o), nil
}

// PriceBest applies every registered strategy and returns the cheapest —
// same "pick the minimum across candidates" idea as the flight-pricing
// supplier merge.
func (p *PricingEngine) PriceBest(o Order) (float64, string) {
	best := o.BaseAmount
	bestName := "standard"
	for name, s := range p.strategies {
		if price := s.Apply(o); price < best {
			best, bestName = price, name
		}
	}
	return best, bestName
}

func main() {
	engine := NewPricingEngine()
	engine.Register(StandardPricing{})
	engine.Register(LoyaltyTierDiscount{})
	engine.Register(BulkQuantityDiscount{})

	order := Order{BaseAmount: 1000, UserTier: "gold", ItemCount: 12}

	for _, name := range []string{"standard", "loyalty-tier", "bulk-quantity"} {
		price, _ := engine.PriceWith(name, order)
		fmt.Printf("%-14s -> %.2f\n", name, price)
	}

	best, bestName := engine.PriceBest(order)
	fmt.Printf("\nbest applicable strategy: %s -> %.2f\n", bestName, best)
}
```

**Verified output:**
```
standard       -> 1000.00
loyalty-tier   -> 850.00
bulk-quantity  -> 900.00

best applicable strategy: loyalty-tier -> 850.00
```

## Why it matters for the interview
- The go-to answer when a reviewed snippet has a long if/else or switch chain handling
  multiple types/suppliers/tiers — naming "I'd extract this into a strategy pattern"
  signals design maturity without needing to over-engineer the fix live.
- Ties directly to **Open/Closed** from SOLID: adding `SeasonalPromoDiscount` means
  adding a new type and calling `Register`, not touching `PricingEngine` or any
  existing strategy.
- Follow-up to expect: "how is this different from just a map of functions?" — a fair
  challenge; the interface version is preferred when strategies need shared state,
  more than one method, or dependency injection (e.g., a strategy that itself calls
  another service) — a plain function value is fine for the simplest cases.
