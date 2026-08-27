# Code Smells & Clean Code in Go

## Common Code Smells

### 1. Long Functions
```go
// ❌ 100-line function doing everything
func (s *Service) ProcessOrder(order Order) error {
    // 20 lines of validation
    // 30 lines of pricing
    // 20 lines of inventory check
    // 30 lines of payment processing
    ...
}

// ✅ Decomposed — each function does ONE thing
func (s *Service) ProcessOrder(ctx context.Context, order Order) error {
    if err := s.validateOrder(ctx, order); err != nil {
        return fmt.Errorf("validation: %w", err)
    }
    price, err := s.calculatePrice(ctx, order)
    if err != nil {
        return fmt.Errorf("pricing: %w", err)
    }
    if err := s.checkInventory(ctx, order); err != nil {
        return fmt.Errorf("inventory: %w", err)
    }
    return s.processPayment(ctx, order, price)
}
```

### 2. Too Many Parameters
```go
// ❌ Too many parameters — hard to read, easy to mix up order
func CreateHotel(name, city, country, address, phone, email string, 
                  rating float64, stars int, hasPool, hasGym, hasSpa bool) *Hotel { ... }

// ✅ Use a struct
type CreateHotelInput struct {
    Name      string
    City      string
    Country   string
    Address   string
    Phone     string
    Email     string
    Rating    float64
    Stars     int
    Amenities []string
}

func CreateHotel(input CreateHotelInput) (*Hotel, error) { ... }
```

### 3. Primitive Obsession
```go
// ❌ Using raw strings for everything
func CreateBooking(hotelID string, userID string, currency string, amount float64) { ... }

// ✅ Use domain types
type HotelID string
type UserID string
type Currency string
type Money struct {
    Amount   float64
    Currency Currency
}

func CreateBooking(hotelID HotelID, userID UserID, price Money) { ... }
// Now you can't accidentally pass userID where hotelID is expected
```

### 4. Magic Numbers
```go
// ❌ Magic numbers
if retries > 3 { ... }
time.Sleep(500 * time.Millisecond)
if rating > 8.5 { ... }

// ✅ Named constants
const (
    MaxRetries          = 3
    RetryDelay          = 500 * time.Millisecond
    PremiumRatingThreshold = 8.5
)
```

### 5. Error Swallowing
```go
// ❌ Silently ignoring errors
result, _ := json.Marshal(data)  // what if Marshal fails?
db.Exec(query)                   // what if the query fails?

// ✅ Always handle errors
result, err := json.Marshal(data)
if err != nil {
    return fmt.Errorf("marshal response: %w", err)
}
```

### 6. Deep Nesting
```go
// ❌ Arrow code — hard to follow
func processRequest(r Request) error {
    if r.IsValid() {
        user, err := getUser(r.UserID)
        if err == nil {
            if user.IsActive() {
                hotel, err := getHotel(r.HotelID)
                if err == nil {
                    // actual logic buried 4 levels deep
                }
            }
        }
    }
    return nil
}

// ✅ Early returns (guard clauses)
func processRequest(ctx context.Context, r Request) error {
    if !r.IsValid() {
        return ErrInvalidRequest
    }
    user, err := getUser(ctx, r.UserID)
    if err != nil {
        return fmt.Errorf("get user: %w", err)
    }
    if !user.IsActive() {
        return ErrUserInactive
    }
    hotel, err := getHotel(ctx, r.HotelID)
    if err != nil {
        return fmt.Errorf("get hotel: %w", err)
    }
    // actual logic here, at level 1
    return process(user, hotel)
}
```

### 7. Boolean Traps
```go
// ❌ What do true/false mean?
hotel.SetProperties(true, false, true)
sendEmail(user, true)

// ✅ Use named parameters or constants
hotel.SetPool(true).SetGym(false).SetSpa(true)

type EmailType string
const (
    EmailTypeConfirmation EmailType = "confirmation"
    EmailTypePromo        EmailType = "promotional"
)
sendEmail(user, EmailTypeConfirmation)
```

---

## Go-Specific Clean Code

### Error Handling
```go
// ✅ Wrap errors with context at each layer
func (r *Repo) GetHotel(ctx context.Context, id string) (*Hotel, error) {
    hotel, err := r.db.QueryRow(ctx, query, id)
    if err != nil {
        return nil, fmt.Errorf("hotel repo get %s: %w", id, err) // wraps with context
    }
    return hotel, nil
}

// Use errors.Is/As to check, not string comparison
if errors.Is(err, ErrNotFound) { ... }

var validErr *ValidationError
if errors.As(err, &validErr) { ... }
```

### Interface Design
```go
// ✅ Accept interfaces, return structs
func ProcessPayment(p PaymentProcessor, amount Money) (*Receipt, error) { ... }
// PaymentProcessor is an interface — flexible input

// ✅ Keep interfaces small (Go idiom)
type Reader interface { Read(p []byte) (n int, err error) } // just ONE method!
```

### defer
```go
// ✅ Always use defer for cleanup — prevents resource leaks
func readFile(path string) ([]byte, error) {
    f, err := os.Open(path)
    if err != nil { return nil, err }
    defer f.Close() // always runs, even if error occurs below
    return io.ReadAll(f)
}

// ✅ defer with named return values for error enrichment
func getUser(ctx context.Context, id string) (user *User, err error) {
    defer func() {
        if err != nil {
            err = fmt.Errorf("getUser %s: %w", id, err)
        }
    }()
    return repo.Get(ctx, id)
}
```

---

## Code Review Exercise

Review this code and identify all issues:

```go
func (s *Service) Book(uid string, hid string, ci string, co string, g int) (string, error) {
    u, e := s.db.Query("SELECT * FROM users WHERE id = " + uid)
    if e != nil {
        fmt.Println(e)
        return "", e
    }
    h, e2 := s.db.Query("SELECT * FROM hotels WHERE id = " + hid)
    if e2 != nil {
        fmt.Println(e2)
        return "", e2
    }
    id := uuid.New()
    s.db.Exec("INSERT INTO bookings VALUES ('" + id.String() + "','" + uid + "','" + hid + "')")
    s.SendEmail(u.Email, "Your booking " + id.String() + " is confirmed")
    return id.String(), nil
}
```

**Issues to identify**:
1. **SQL Injection** — string concatenation in queries (use parameterized queries!)
2. **SRP violation** — booking + email sending in one function
3. **Error handling** — `fmt.Println(e)` instead of logging, errors not wrapped
4. **No context propagation** — no `ctx context.Context` parameter
5. **Poor naming** — `uid`, `hid`, `ci`, `co`, `g` are not descriptive
6. **Dates as strings** — `ci`, `co` should be `time.Time`
7. **No input validation** — no check on dates, guest count
8. **No transaction** — booking insert and email are not atomic
9. **`SELECT *`** — select only needed columns
