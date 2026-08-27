# Design Patterns in Go

## Creational Patterns

### Singleton
```go
var (
    dbInstance *Database
    once       sync.Once
)

func GetDatabase() *Database {
    once.Do(func() {
        dbInstance = &Database{}
        dbInstance.Connect(os.Getenv("DB_URL"))
    })
    return dbInstance
}
```

### Factory Method
```go
type NotificationSender interface {
    Send(ctx context.Context, to, message string) error
}

func NewNotificationSender(channel string) (NotificationSender, error) {
    switch channel {
    case "email":
        return NewEmailSender(smtpConfig)
    case "sms":
        return NewSMSSender(twilioConfig)
    case "push":
        return NewPushSender(fcmConfig)
    default:
        return nil, fmt.Errorf("unsupported channel: %s", channel)
    }
}
```

### Builder
```go
type HotelSearchQuery struct {
    City      string
    CheckIn   time.Time
    CheckOut  time.Time
    MinRating float64
    MaxPrice  float64
    Amenities []string
    PageSize  int
    PageToken string
}

type HotelSearchQueryBuilder struct {
    query HotelSearchQuery
}

func NewHotelSearch(city string) *HotelSearchQueryBuilder {
    return &HotelSearchQueryBuilder{
        query: HotelSearchQuery{City: city, PageSize: 20},
    }
}

func (b *HotelSearchQueryBuilder) WithDates(checkIn, checkOut time.Time) *HotelSearchQueryBuilder {
    b.query.CheckIn = checkIn
    b.query.CheckOut = checkOut
    return b
}

func (b *HotelSearchQueryBuilder) WithMinRating(r float64) *HotelSearchQueryBuilder {
    b.query.MinRating = r; return b
}

func (b *HotelSearchQueryBuilder) WithMaxPrice(p float64) *HotelSearchQueryBuilder {
    b.query.MaxPrice = p; return b
}

func (b *HotelSearchQueryBuilder) WithAmenities(a ...string) *HotelSearchQueryBuilder {
    b.query.Amenities = a; return b
}

func (b *HotelSearchQueryBuilder) Build() HotelSearchQuery {
    return b.query
}

// Usage
query := NewHotelSearch("Bangkok").
    WithDates(checkIn, checkOut).
    WithMinRating(4.0).
    WithMaxPrice(200).
    WithAmenities("pool", "gym").
    Build()
```

---

## Structural Patterns

### Decorator / Middleware
```go
// Wrap a handler with additional behavior (logging, auth, metrics)
type HotelService interface {
    GetHotel(ctx context.Context, id string) (*Hotel, error)
}

// Logging decorator
type LoggingHotelService struct {
    next   HotelService
    logger *slog.Logger
}

func (s *LoggingHotelService) GetHotel(ctx context.Context, id string) (*Hotel, error) {
    start := time.Now()
    hotel, err := s.next.GetHotel(ctx, id)
    s.logger.Info("GetHotel",
        "id", id,
        "duration", time.Since(start),
        "error", err,
    )
    return hotel, err
}

// Caching decorator
type CachingHotelService struct {
    next  HotelService
    cache Cache
    ttl   time.Duration
}

func (s *CachingHotelService) GetHotel(ctx context.Context, id string) (*Hotel, error) {
    if hotel, found := s.cache.Get("hotel:" + id); found {
        return hotel.(*Hotel), nil
    }
    hotel, err := s.next.GetHotel(ctx, id)
    if err == nil {
        s.cache.Set("hotel:"+id, hotel, s.ttl)
    }
    return hotel, err
}

// Compose decorators
svc := &LoggingHotelService{
    next: &CachingHotelService{
        next: &RealHotelService{repo: repo},
        cache: redisCache,
        ttl: 5 * time.Minute,
    },
    logger: logger,
}
```

### Adapter
```go
// Adapt a third-party hotel supplier API to your domain interface

// Third-party interface (external, can't modify)
type SupplierAPI interface {
    FetchHotelData(hotelCode string, lang string) (SupplierHotelResponse, error)
}

// Your domain interface
type HotelDataProvider interface {
    GetHotel(ctx context.Context, id string) (*Hotel, error)
}

// Adapter bridges the gap
type SupplierAdapter struct {
    api SupplierAPI
}

func (a *SupplierAdapter) GetHotel(ctx context.Context, id string) (*Hotel, error) {
    raw, err := a.api.FetchHotelData(id, "en")
    if err != nil {
        return nil, fmt.Errorf("supplier error: %w", err)
    }
    // Transform supplier response to domain model
    return &Hotel{
        ID:     raw.HotelCode,
        Name:   raw.PropertyName,
        Rating: raw.StarRating / 2, // supplier uses 10-star scale
        City:   raw.Location.CityName,
    }, nil
}
```

---

## Behavioral Patterns

### Strategy
```go
// Swap algorithms at runtime
type PricingStrategy interface {
    Calculate(basePrice float64, nights int, userTier string) float64
}

type StandardPricing struct{}
func (p *StandardPricing) Calculate(base float64, nights int, _ string) float64 {
    return base * float64(nights)
}

type MemberPricing struct{}
func (p *MemberPricing) Calculate(base float64, nights int, tier string) float64 {
    discount := map[string]float64{"silver": 0.05, "gold": 0.10, "platinum": 0.20}
    d := discount[tier]
    return base * float64(nights) * (1 - d)
}

type EarlyBirdPricing struct{}
func (p *EarlyBirdPricing) Calculate(base float64, nights int, _ string) float64 {
    return base * float64(nights) * 0.85 // 15% off
}

type BookingCalculator struct {
    strategy PricingStrategy
}

func (c *BookingCalculator) SetStrategy(s PricingStrategy) { c.strategy = s }
func (c *BookingCalculator) Calculate(base float64, nights int, tier string) float64 {
    return c.strategy.Calculate(base, nights, tier)
}
```

### Observer
```go
// Event bus — notify multiple handlers of events
type EventHandler func(ctx context.Context, event interface{}) error

type EventBus struct {
    mu       sync.RWMutex
    handlers map[string][]EventHandler
}

func (b *EventBus) Subscribe(eventType string, handler EventHandler) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.handlers[eventType] = append(b.handlers[eventType], handler)
}

func (b *EventBus) Publish(ctx context.Context, eventType string, event interface{}) {
    b.mu.RLock()
    handlers := b.handlers[eventType]
    b.mu.RUnlock()

    for _, h := range handlers {
        go func(handler EventHandler) { // async
            if err := handler(ctx, event); err != nil {
                log.Error("event handler error", "type", eventType, "err", err)
            }
        }(h)
    }
}

// Usage
bus := &EventBus{handlers: make(map[string][]EventHandler)}
bus.Subscribe("booking.confirmed", sendConfirmationEmail)
bus.Subscribe("booking.confirmed", updateAnalytics)
bus.Subscribe("booking.confirmed", chargePayment)

bus.Publish(ctx, "booking.confirmed", BookingConfirmedEvent{BookingID: "123"})
```

### Functional Options (Go idiom — replaces constructor overloading)
```go
type ServerConfig struct {
    addr    string
    timeout time.Duration
    maxConn int
    tls     bool
}

type Option func(*ServerConfig)

func WithAddr(addr string) Option {
    return func(c *ServerConfig) { c.addr = addr }
}

func WithTimeout(d time.Duration) Option {
    return func(c *ServerConfig) { c.timeout = d }
}

func WithMaxConnections(n int) Option {
    return func(c *ServerConfig) { c.maxConn = n }
}

func WithTLS() Option {
    return func(c *ServerConfig) { c.tls = true }
}

func NewServer(opts ...Option) *Server {
    cfg := &ServerConfig{
        addr:    ":8080",          // defaults
        timeout: 30 * time.Second,
        maxConn: 1000,
    }
    for _, opt := range opts {
        opt(cfg)
    }
    return &Server{config: cfg}
}

// Usage — only specify what you need
srv := NewServer(
    WithAddr(":9090"),
    WithTLS(),
    WithMaxConnections(5000),
)
```

---

## Interview Q&A

**Q: How does the Decorator pattern differ from inheritance?**
> A: Go doesn't have inheritance — it uses composition and interfaces. The Decorator pattern wraps a value with the same interface, adding behavior without modifying the original. Benefits: (1) compose multiple behaviors (logging + caching + metrics) independently; (2) single responsibility — each decorator does one thing; (3) open/closed — add behavior by wrapping, not modifying. This is why Go middleware chains for HTTP handlers work so well — each middleware is a decorator.

**Q: When would you use Strategy vs Factory Method?**
> A: Factory Method is about *creation* — how to create objects without specifying concrete classes. Strategy is about *behavior* — swapping algorithms at runtime. Use Factory Method when you want to centralize object creation logic. Use Strategy when you need to select between different algorithms based on runtime conditions (user tier, A/B test, feature flag). They often work together: a Factory creates the right Strategy.
