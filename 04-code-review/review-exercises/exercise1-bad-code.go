// exercise1-bad-code.go — Review this code and identify all issues
// This simulates what you might see in an Agoda code review round
package main

import (
	"database/sql"
	"fmt"
	"time"
)

// BookingService handles hotel bookings
type BookingService struct {
	db   *sql.DB
	smtp SMTPClient
}

type SMTPClient interface {
	Send(to, subject, body string) error
}

// Book creates a hotel booking
// TODO: Review this code for issues
func (s *BookingService) Book(userId string, hotelId string, checkIn string, checkOut string, guests int, promoCode string) (string, bool, error) {
	// Get user
	var userEmail string
	var userStatus int
	rows, err := s.db.Query("SELECT email, status FROM users WHERE id = " + userId)
	if err != nil {
		fmt.Println("error getting user:", err)
		return "", false, err
	}
	rows.Scan(&userEmail, &userStatus)

	// Check if user is active
	if userStatus == 1 {
		// Get hotel
		var hotelName string
		var hotelPrice float64
		rows2, err2 := s.db.Query("SELECT name, price FROM hotels WHERE id = " + hotelId)
		if err2 != nil {
			fmt.Println("error getting hotel:", err2)
			return "", false, err2
		}
		rows2.Scan(&hotelName, &hotelPrice)

		// Calculate price
		ci, _ := time.Parse("2006-01-02", checkIn)
		co, _ := time.Parse("2006-01-02", checkOut)
		nights := int(co.Sub(ci).Hours() / 24)
		total := hotelPrice * float64(nights)
		discounted := false
		if promoCode == "SAVE10" {
			total = total * 0.9
			discounted = true
		} else if promoCode == "SAVE20" {
			total = total * 0.8
			discounted = true
		} else if promoCode == "SAVE30" {
			total = total * 0.7
			discounted = true
		}

		// Create booking
		bookingId := fmt.Sprintf("%d", time.Now().UnixNano())
		s.db.Exec("INSERT INTO bookings (id, user_id, hotel_id, check_in, check_out, total_price) VALUES ('" + bookingId + "','" + userId + "','" + hotelId + "','" + checkIn + "','" + checkOut + "','" + fmt.Sprintf("%f", total) + "')")

		// Send email
		s.smtp.Send(userEmail, "Booking Confirmed", "Your booking at "+hotelName+" is confirmed. Total: "+fmt.Sprintf("%.2f", total))

		return bookingId, discounted, nil
	}

	return "", false, nil
}
