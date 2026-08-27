# SOLID Principles in Go

> SOLID is heavily tested in **Code Review** rounds. Expect to spot SOLID violations in code snippets.

## S — Single Responsibility Principle

*Each type/function should have ONE reason to change.*

```go
// ❌ Violation — one struct doing too many things
type BookingService struct{}

func (s *BookingService) CreateBooking(b Booking) error {
    // Business logic
    if !b.CheckOut.After(b.CheckIn) { return errors.New("invalid dates") }
    
    // Database access (should be separate)
    db.Exec("INSERT INTO bookings ...")
    
    // Email sending (should be separate)
    smtp.Send("booking@agoda.com", "Your booking is confirmed")
    
    // PDF generation (should be separate)
    pdf.Generate(b)
    
    return nil
}

// ✅ Fixed — each type has one responsibility
type BookingService struct {
    repo         BookingRepository
    notifier     NotificationService
    pdfGenerator PDFGenerator
}

func (s *BookingService) CreateBooking(ctx context.Context, b Booking) error {
    if err := s.validate(b); err != nil { return err }       // business logic
    if err := s.repo.Create(ctx, b); err != nil { return err } // delegate storage
    s.notifier.SendConfirmation(ctx, b)                        // delegate notification
    return nil
}
```

---

## O — Open/Closed Principle

*Open for extension, closed for modification.*

```go
// ❌ Violation — adding a new payment method requires modifying existing code
func (s *PaymentService) Process(method string, amount float64) error {
    switch method {
    case "credit_card":
        return processCard(amount)
    case "paypal":
        return processPayPal(amount)
    // Adding "crypto" requires modifying this switch ❌
    }
    return errors.New("unknown method")
}

// ✅ Fixed — extend by adding new implementations, not modifying existing
type PaymentProcessor interface {
    Process(ctx context.Context, amount float64) error
}

type CreditCardProcessor struct{ apiKey string }
func (p *CreditCardProcessor) Process(ctx context.Context, amount float64) error { ... }

type PayPalProcessor struct{ clientID string }
func (p *PayPalProcessor) Process(ctx context.Context, amount float64) error { ... }

type CryptoProcessor struct{ walletAddr string }
func (p *CryptoProcessor) Process(ctx context.Context, amount float64) error { ... } // new — no existing code changed!

// Registry pattern — register processors without modifying core logic
type PaymentService struct {
    processors map[string]PaymentProcessor
}

func (s *PaymentService) Process(ctx context.Context, method string, amount float64) error {
    p, ok := s.processors[method]
    if !ok {
        return fmt.Errorf("unsupported payment method: %s", method)
    }
    return p.Process(ctx, amount)
}
```

---

## L — Liskov Substitution Principle

*A subtype must be substitutable for its base type without breaking correctness.*

```go
// In Go: any type satisfying an interface must behave correctly

type Storage interface {
    Save(ctx context.Context, key string, data []byte) error
    Load(ctx context.Context, key string) ([]byte, error)
}

// ❌ Violation — this implementation silently ignores the Save
type NullStorage struct{}
func (n *NullStorage) Save(ctx context.Context, key string, data []byte) error {
    return nil // silently drops data — caller expects it was saved!
}
func (n *NullStorage) Load(ctx context.Context, key string) ([]byte, error) {
    return nil, errors.New("not found") // Always fails — violates LSP
}

// ✅ For testing, use an explicit in-memory implementation that WORKS correctly
type InMemoryStorage struct {
    mu   sync.RWMutex
    data map[string][]byte
}
func (s *InMemoryStorage) Save(ctx context.Context, key string, data []byte) error {
    s.mu.Lock(); defer s.mu.Unlock()
    s.data[key] = data; return nil
}
func (s *InMemoryStorage) Load(ctx context.Context, key string) ([]byte, error) {
    s.mu.RLock(); defer s.mu.RUnlock()
    d, ok := s.data[key]
    if !ok { return nil, ErrNotFound }
    return d, nil
}
```

---

## I — Interface Segregation Principle

*No client should be forced to depend on methods it does not use.*

```go
// ❌ Violation — fat interface forces implementers to implement everything
type HotelRepository interface {
    GetHotel(ctx context.Context, id string) (*Hotel, error)
    CreateHotel(ctx context.Context, h *Hotel) error
    UpdateHotel(ctx context.Context, h *Hotel) error
    DeleteHotel(ctx context.Context, id string) error
    SearchHotels(ctx context.Context, q SearchQuery) ([]*Hotel, error)
    GetHotelImages(ctx context.Context, id string) ([]Image, error)
    UploadImage(ctx context.Context, id string, img Image) error
    GetAmenities(ctx context.Context, id string) ([]Amenity, error)
    // ... 20 more methods
}

// ✅ Fixed — split into role-specific interfaces (Go idiom: small interfaces)
type HotelReader interface {
    GetHotel(ctx context.Context, id string) (*Hotel, error)
    SearchHotels(ctx context.Context, q SearchQuery) ([]*Hotel, error)
}

type HotelWriter interface {
    CreateHotel(ctx context.Context, h *Hotel) error
    UpdateHotel(ctx context.Context, h *Hotel) error
    DeleteHotel(ctx context.Context, id string) error
}

type HotelImageStore interface {
    GetHotelImages(ctx context.Context, id string) ([]Image, error)
    UploadImage(ctx context.Context, id string, img Image) error
}

// Search service only needs reader — not forced to know about writes
type HotelSearchService struct {
    reader HotelReader // not HotelRepository!
}
```

---

## D — Dependency Inversion Principle

*High-level modules should not depend on low-level modules. Both should depend on abstractions.*

```go
// ❌ Violation — high-level service depends on concrete low-level implementation
type BookingService struct {
    db *sql.DB // concrete dependency on PostgreSQL!
}

// Can't swap DB, can't unit test without real DB

// ✅ Fixed — depend on interface (abstraction)
type BookingRepository interface {
    Create(ctx context.Context, b *Booking) error
    Get(ctx context.Context, id string) (*Booking, error)
}

type BookingService struct {
    repo BookingRepository // interface — can be SQL, Redis, mock, etc.
}

// Constructor injection
func NewBookingService(repo BookingRepository) *BookingService {
    return &BookingService{repo: repo}
}

// In production:
svc := NewBookingService(NewPostgresBookingRepo(db))

// In tests:
svc := NewBookingService(&MockBookingRepository{})
```

---

## Interview Q&A

**Q: Spot the SOLID violations in this code:**
```go
type OrderService struct{}
func (s *OrderService) PlaceOrder(o Order) {
    // validate
    // save to DB (direct sql.DB)
    // send email
    // update inventory
    // generate invoice PDF
}
```
> A: Multiple violations: (1) **SRP** — doing validation, persistence, notification, inventory, PDF generation in one method; (2) **DIP** — directly using sql.DB instead of a repository interface; (3) **OCP** — if we add a new notification channel, we modify this code. Fix: inject dependencies via interfaces (BookingRepository, NotificationService, InventoryService), and split responsibilities into focused services.

**Q: What does "program to interfaces, not implementations" mean in Go?**
> A: In Go, interfaces are implicit — a type satisfies an interface just by having the required methods. "Program to interfaces" means define your dependencies as interfaces (like `io.Writer`, `BookingRepository`) rather than concrete types. This enables: (1) unit testing with mocks; (2) swapping implementations without changing callers; (3) DIP — high-level code doesn't depend on low-level details. Go's convention is small, focused interfaces (`io.Reader` has one method) which naturally leads to ISP compliance.
