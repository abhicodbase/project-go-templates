# Saga Pattern

## Use case
Booking a flight involves three separate services (Hold Flight, Charge Payment,
Reserve Seat) each with their own database — there's no distributed transaction
spanning all three. If step 3 fails after steps 1 and 2 already committed, you need a
way to undo them. The saga pattern chains each step with a compensating action, run in
reverse order if a later step fails.

## Diagram

```
  HoldFlight ──success──> ChargePayment ──success──> ReserveSeat
      │                        │                          │
      │                        │                       FAILS
      │                        │                          │
      │                        │<──── compensate ─────────┘
      │                   refund payment
      │<──── compensate ──────┘
   release hold
```

## Code

```go
package main

import (
	"errors"
	"fmt"
)

// Step is one unit of work in the saga, paired with its compensating
// action to undo it if a LATER step fails. There's no cross-service
// distributed transaction — each step commits independently, and
// correctness comes from the compensation chain, not atomicity.
type Step struct {
	Name       string
	Action     func() error
	Compensate func() error
}

type Orchestrator struct {
	steps []Step
}

func (o *Orchestrator) AddStep(s Step) {
	o.steps = append(o.steps, s)
}

func (o *Orchestrator) Run() error {
	completed := []Step{}

	for _, step := range o.steps {
		fmt.Printf("-> running step: %s\n", step.Name)
		if err := step.Action(); err != nil {
			fmt.Printf("   FAILED: %s (%v) — starting compensation\n", step.Name, err)
			o.compensate(completed)
			return fmt.Errorf("saga aborted at %s: %w", step.Name, err)
		}
		completed = append(completed, step)
	}
	return nil
}

func (o *Orchestrator) compensate(completed []Step) {
	for i := len(completed) - 1; i >= 0; i-- {
		s := completed[i]
		fmt.Printf("<- compensating: %s\n", s.Name)
		if err := s.Compensate(); err != nil {
			// In production: alert loudly — a failed compensation leaves
			// the system inconsistent and needs manual/reconciliation follow-up.
			fmt.Printf("   compensation FAILED for %s: %v (needs manual intervention)\n", s.Name, err)
		}
	}
}

func main() {
	orchestrator := &Orchestrator{}
	flightHeld := false
	paymentCharged := false

	orchestrator.AddStep(Step{
		Name:   "HoldFlight",
		Action: func() error { flightHeld = true; fmt.Println("   flight seat held"); return nil },
		Compensate: func() error {
			flightHeld = false
			fmt.Println("   flight hold released")
			return nil
		},
	})

	orchestrator.AddStep(Step{
		Name:   "ChargePayment",
		Action: func() error { paymentCharged = true; fmt.Println("   payment charged"); return nil },
		Compensate: func() error {
			paymentCharged = false
			fmt.Println("   payment refunded")
			return nil
		},
	})

	orchestrator.AddStep(Step{
		Name:       "ReserveSeat",
		Action:     func() error { return errors.New("seat map service unavailable") },
		Compensate: func() error { return nil },
	})

	err := orchestrator.Run()
	fmt.Printf("\nfinal result: err=%v flightHeld=%v paymentCharged=%v\n", err, flightHeld, paymentCharged)
}
```

**Verified output:**
```
-> running step: HoldFlight
   flight seat held
-> running step: ChargePayment
   payment charged
-> running step: ReserveSeat
   FAILED: ReserveSeat (seat map service unavailable) — starting compensation
<- compensating: ChargePayment
   payment refunded
<- compensating: HoldFlight
   flight hold released

final result: err=saga aborted at ReserveSeat: seat map service unavailable flightHeld=false paymentCharged=false
```

## Why it matters for the interview
- The direct answer to "why not just use a distributed transaction / two-phase commit
  here" — 2PC requires all participants to support it and blocks on a coordinator,
  which doesn't scale across independently-owned services or external partners.
- Know the two flavors: **orchestration** (shown here — one coordinator sequences
  steps) vs **choreography** (each service reacts to the previous step's event
  independently, no central coordinator). Orchestration is easier to reason about;
  choreography is more decoupled but harder to trace end-to-end.
- Common follow-up: "what if a compensation itself fails?" — the honest answer is you
  need alerting and a manual/reconciliation path; sagas give you eventual consistency,
  not a guarantee that undo always succeeds.
