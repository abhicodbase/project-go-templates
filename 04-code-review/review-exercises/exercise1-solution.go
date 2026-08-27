// exercise1-solution.go — Refactored version of exercise1-bad-code.go
// Issues found and fixed:
// 1. SQL INJECTION — string concatenation in queries
// 2. Missing context.Context propagation
// 3. No input validation (dates, guests, promo code)
// 4. SRP violation — email sending mixed with booking creation
// 5. Rows not closed — resource leak
// 6. Error silently ignored (rows.Scan return)
// 7. No database transaction — booking + email not atomic
// 8. Magic strings for promo codes — use table/strategy
// 9. Poor naming — userId, hotelId, checkIn as string
// 10. Boolean return for "discounted" — use a richer result type
// 11. fmt.Println for errors — use structured logging
// 12. Non-deterministic booking ID (time.Now().UnixNano()) — use UUID
// 13. Ignoring error from db.Exec
// 14. Nested if creates deep indentation — use early returns

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Domain types (type safety)
type UserID string
type HotelID string

// Domain errors
var (
	ErrUserNotFound    = errors.New("user not found")
	ErrUserInactive    = errors.New("user is not active")
	ErrHotelNotFound   = errors.New("hotel not found")
	ErrInvalidDates    = errors.New("invalid dates: checkout must be after checkin")
	ErrInvalidGuests   = errors.New("invalid guest count: must be between 1 and 20")
)

// Rich result type — no boolean return smell
type BookingResult struct {
	BookingID  string
	TotalPrice float64
	Discounted bool
	PromoCode  string
}

// CreateBookingInput — clean parameter grouping
type CreateBookingInput struct {
	UserID    UserID
	HotelID   HotelID
	CheckIn   time.Time
	CheckOut  time.Time
	Guests    int
	PromoCode string
}

// Interfaces for dependencies (DIP)
type UserRepository interface {
	GetByID(ctx context.Context, id UserID) (*User, error)
}

type HotelRepository interface {
	GetByID(ctx context.Context, id HotelID) (*Hotel, error)
}

type BookingRepository interface {
	Create(ctx context.Context, tx *sql.Tx, b *Booking) error
}

type NotificationService interface {
	SendBookingConfirmation(ctx context.Context, userEmail, hotelName string, result BookingResult) error
}

type PricingEngine interface {
	Calculate(basePrice float64, nights int, promoCode string) (float64, bool)
}

// Domain models
type User struct {
	ID     UserID
	Email  string
	Active bool
}

type Hotel struct {
	ID    HotelID
	Name  string
	Price float64 // per night
}

type Booking struct {
	ID         string
	UserID     UserID
	HotelID    HotelID
	CheckIn    time.Time
	CheckOut   time.Time
	TotalPrice float64
	PromoCode  string
}

// BookingService — single responsibility: orchestrate booking creation
type BookingService struct {
	db           *sql.DB
	userRepo     UserRepository
	hotelRepo    HotelRepository
	bookingRepo  BookingRepository
	notifier     NotificationService
	pricing      PricingEngine
}

func NewBookingService(
	db *sql.DB,
	userRepo UserRepository,
	hotelRepo HotelRepository,
	bookingRepo BookingRepository,
	notifier NotificationService,
	pricing PricingEngine,
) *BookingService {
	return &BookingService{
		db:          db,
		userRepo:    userRepo,
		hotelRepo:   hotelRepo,
		bookingRepo: bookingRepo,
		notifier:    notifier,
		pricing:     pricing,
	}
}

// CreateBooking — clean, validated, transactional booking creation
func (s *BookingService) CreateBooking(ctx context.Context, input CreateBookingInput) (*BookingResult, error) {
	// 1. Validate input early (guard clauses)
	if err := s.validateInput(input); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	// 2. Fetch user and verify they're active
	user, err := s.userRepo.GetByID(ctx, input.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if !user.Active {
		return nil, ErrUserInactive
	}

	// 3. Fetch hotel
	hotel, err := s.hotelRepo.GetByID(ctx, input.HotelID)
	if err != nil {
		return nil, fmt.Errorf("get hotel: %w", err)
	}

	// 4. Calculate price
	nights := int(input.CheckOut.Sub(input.CheckIn).Hours() / 24)
	totalPrice, discounted := s.pricing.Calculate(hotel.Price, nights, input.PromoCode)

	// 5. Create booking in a transaction
	booking := &Booking{
		ID:         uuid.New().String(),
		UserID:     input.UserID,
		HotelID:    input.HotelID,
		CheckIn:    input.CheckIn,
		CheckOut:   input.CheckOut,
		TotalPrice: totalPrice,
		PromoCode:  input.PromoCode,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() // Safe: no-op if committed

	if err := s.bookingRepo.Create(ctx, tx, booking); err != nil {
		return nil, fmt.Errorf("create booking: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	// 6. Send notification AFTER successful commit (async in production)
	result := &BookingResult{
		BookingID:  booking.ID,
		TotalPrice: totalPrice,
		Discounted: discounted,
		PromoCode:  input.PromoCode,
	}

	// Fire-and-forget notification (don't fail the booking if email fails)
	go func() {
		if err := s.notifier.SendBookingConfirmation(
			context.Background(), // new context — original may be cancelled
			user.Email,
			hotel.Name,
			*result,
		); err != nil {
			// Log error but don't fail — booking already committed
			fmt.Printf("notification failed for booking %s: %v\n", booking.ID, err)
		}
	}()

	return result, nil
}

// validateInput contains all input validation (single responsibility)
func (s *BookingService) validateInput(input CreateBookingInput) error {
	if !input.CheckOut.After(input.CheckIn) {
		return ErrInvalidDates
	}
	if input.Guests < 1 || input.Guests > 20 {
		return ErrInvalidGuests
	}
	if input.CheckIn.Before(time.Now().Truncate(24 * time.Hour)) {
		return fmt.Errorf("check-in date must be today or in the future")
	}
	return nil
}

// StandardPricingEngine — Open/Closed: add new promo codes without modifying engine
type StandardPricingEngine struct {
	promoCodes map[string]float64 // promoCode -> discount fraction
}

func NewStandardPricingEngine() *StandardPricingEngine {
	return &StandardPricingEngine{
		promoCodes: map[string]float64{
			"SAVE10": 0.10,
			"SAVE20": 0.20,
			"SAVE30": 0.30,
		},
	}
}

func (e *StandardPricingEngine) Calculate(basePrice float64, nights int, promoCode string) (float64, bool) {
	total := basePrice * float64(nights)
	if discount, ok := e.promoCodes[promoCode]; ok {
		return total * (1 - discount), true
	}
	return total, false
}
