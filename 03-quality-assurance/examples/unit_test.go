// unit_test.go — Example table-driven tests in Go
package booking_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ============================================================
// Domain types (normally in separate package)
// ============================================================

type Booking struct {
	ID         string
	HotelID    string
	UserID     string
	CheckIn    time.Time
	CheckOut   time.Time
	TotalPrice float64
	Status     string
}

var ErrNotFound = errors.New("not found")
var ErrInvalidDates = errors.New("invalid dates: checkout must be after checkin")

// ============================================================
// Repository interface (what we mock)
// ============================================================

type BookingRepository interface {
	Get(ctx context.Context, id string) (*Booking, error)
	Create(ctx context.Context, b *Booking) error
	Cancel(ctx context.Context, id string) error
}

// ============================================================
// Service (what we test)
// ============================================================

type BookingService struct {
	repo BookingRepository
}

func NewBookingService(repo BookingRepository) *BookingService {
	return &BookingService{repo: repo}
}

func (s *BookingService) CreateBooking(ctx context.Context, hotelID, userID string, checkIn, checkOut time.Time) (*Booking, error) {
	if !checkOut.After(checkIn) {
		return nil, ErrInvalidDates
	}
	nights := int(checkOut.Sub(checkIn).Hours() / 24)
	if nights < 1 {
		return nil, ErrInvalidDates
	}

	b := &Booking{
		ID:         "new-booking-id",
		HotelID:    hotelID,
		UserID:     userID,
		CheckIn:    checkIn,
		CheckOut:   checkOut,
		TotalPrice: float64(nights) * 100, // simplified pricing
		Status:     "PENDING",
	}

	if err := s.repo.Create(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *BookingService) GetBooking(ctx context.Context, id string) (*Booking, error) {
	return s.repo.Get(ctx, id)
}

func (s *BookingService) CancelBooking(ctx context.Context, id string) error {
	booking, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if booking.Status == "CANCELLED" {
		return errors.New("booking already cancelled")
	}
	return s.repo.Cancel(ctx, id)
}

// ============================================================
// Mock repository (generated or manual)
// ============================================================

type MockBookingRepository struct {
	mock.Mock
}

func (m *MockBookingRepository) Get(ctx context.Context, id string) (*Booking, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Booking), args.Error(1)
}

func (m *MockBookingRepository) Create(ctx context.Context, b *Booking) error {
	args := m.Called(ctx, b)
	return args.Error(0)
}

func (m *MockBookingRepository) Cancel(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// ============================================================
// Tests
// ============================================================

func TestCreateBooking(t *testing.T) {
	baseCheckIn := time.Now().Add(24 * time.Hour).Truncate(time.Hour)

	tests := []struct {
		name      string
		hotelID   string
		userID    string
		checkIn   time.Time
		checkOut  time.Time
		setupMock func(*MockBookingRepository)
		wantErr   error
		wantNights int
	}{
		{
			name:     "successful 3-night booking",
			hotelID:  "hotel-123",
			userID:   "user-456",
			checkIn:  baseCheckIn,
			checkOut: baseCheckIn.Add(72 * time.Hour),
			setupMock: func(m *MockBookingRepository) {
				m.On("Create", mock.Anything, mock.MatchedBy(func(b *Booking) bool {
					return b.HotelID == "hotel-123" && b.Status == "PENDING"
				})).Return(nil)
			},
			wantNights: 3,
		},
		{
			name:     "checkout before checkin should fail",
			hotelID:  "hotel-123",
			userID:   "user-456",
			checkIn:  baseCheckIn,
			checkOut: baseCheckIn.Add(-24 * time.Hour), // BEFORE check-in
			setupMock: func(m *MockBookingRepository) {
				// No repo calls expected
			},
			wantErr: ErrInvalidDates,
		},
		{
			name:     "same day checkout should fail",
			hotelID:  "hotel-123",
			userID:   "user-456",
			checkIn:  baseCheckIn,
			checkOut: baseCheckIn, // same time
			setupMock: func(m *MockBookingRepository) {
				// No repo calls expected
			},
			wantErr: ErrInvalidDates,
		},
		{
			name:     "repository error propagates",
			hotelID:  "hotel-123",
			userID:   "user-456",
			checkIn:  baseCheckIn,
			checkOut: baseCheckIn.Add(48 * time.Hour),
			setupMock: func(m *MockBookingRepository) {
				m.On("Create", mock.Anything, mock.Anything).Return(errors.New("db connection failed"))
			},
			wantErr: errors.New("db connection failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &MockBookingRepository{}
			tt.setupMock(mockRepo)
			svc := NewBookingService(mockRepo)

			booking, err := svc.CreateBooking(context.Background(), tt.hotelID, tt.userID, tt.checkIn, tt.checkOut)

			if tt.wantErr != nil {
				require.Error(t, err)
				if errors.Is(tt.wantErr, ErrInvalidDates) {
					assert.ErrorIs(t, err, ErrInvalidDates)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, booking)
			assert.Equal(t, "PENDING", booking.Status)
			assert.Equal(t, float64(tt.wantNights)*100, booking.TotalPrice)
			mockRepo.AssertExpectations(t)
		})
	}
}

func TestCancelBooking(t *testing.T) {
	tests := []struct {
		name      string
		bookingID string
		setupMock func(*MockBookingRepository)
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "successfully cancel active booking",
			bookingID: "booking-1",
			setupMock: func(m *MockBookingRepository) {
				m.On("Get", mock.Anything, "booking-1").Return(&Booking{ID: "booking-1", Status: "CONFIRMED"}, nil)
				m.On("Cancel", mock.Anything, "booking-1").Return(nil)
			},
		},
		{
			name:      "cancel already-cancelled booking fails",
			bookingID: "booking-2",
			setupMock: func(m *MockBookingRepository) {
				m.On("Get", mock.Anything, "booking-2").Return(&Booking{ID: "booking-2", Status: "CANCELLED"}, nil)
			},
			wantErr: true,
			errMsg:  "already cancelled",
		},
		{
			name:      "cancel non-existent booking fails",
			bookingID: "booking-999",
			setupMock: func(m *MockBookingRepository) {
				m.On("Get", mock.Anything, "booking-999").Return(nil, ErrNotFound)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &MockBookingRepository{}
			tt.setupMock(mockRepo)
			svc := NewBookingService(mockRepo)

			err := svc.CancelBooking(context.Background(), tt.bookingID)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}
			assert.NoError(t, err)
			mockRepo.AssertExpectations(t)
		})
	}
}
