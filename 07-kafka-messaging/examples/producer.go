// producer.go — Kafka producer in Go using confluent-kafka-go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// ============================================================
// Domain event types
// ============================================================

type EventType string

const (
	EventBookingCreated   EventType = "BookingCreated"
	EventBookingConfirmed EventType = "BookingConfirmed"
	EventBookingCancelled EventType = "BookingCancelled"
)

type BookingEvent struct {
	EventID   string    `json:"event_id"`
	Type      EventType `json:"type"`
	BookingID string    `json:"booking_id"`
	UserID    string    `json:"user_id"`
	HotelID   string    `json:"hotel_id"`
	Timestamp time.Time `json:"timestamp"`
}

// ============================================================
// KafkaProducer wraps the confluent producer
// ============================================================

type KafkaProducer struct {
	producer *kafka.Producer
	topic    string
}

func NewKafkaProducer(bootstrapServers, topic string) (*KafkaProducer, error) {
	p, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers":  bootstrapServers,
		"acks":               "all",             // wait for all replicas (most durable)
		"retries":            10,
		"enable.idempotence": true,              // exactly-once producer semantics
		"compression.type":   "snappy",          // compress messages
		"linger.ms":          5,                 // batch messages for 5ms for throughput
		"batch.size":         16384,
	})
	if err != nil {
		return nil, fmt.Errorf("create kafka producer: %w", err)
	}

	// Start delivery report goroutine
	go func() {
		for e := range p.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					log.Printf("Delivery failed: key=%s err=%v", ev.Key, ev.TopicPartition.Error)
				} else {
					log.Printf("Delivered: key=%s partition=%d offset=%d",
						ev.Key, ev.TopicPartition.Partition, ev.TopicPartition.Offset)
				}
			case kafka.Error:
				log.Printf("Kafka error: %v", ev)
			}
		}
	}()

	return &KafkaProducer{producer: p, topic: topic}, nil
}

// Publish sends a booking event to Kafka
func (kp *KafkaProducer) Publish(ctx context.Context, event BookingEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &kp.topic,
			Partition: kafka.PartitionAny,
		},
		Key:   []byte(event.BookingID), // same booking_id → same partition → ordered
		Value: payload,
		Headers: []kafka.Header{
			{Key: "event-type", Value: []byte(event.Type)},
			{Key: "event-id", Value: []byte(event.EventID)},
			{Key: "correlation-id", Value: []byte(correlationIDFromContext(ctx))},
		},
		Timestamp: event.Timestamp,
	}

	// Check context before producing
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return kp.producer.Produce(msg, nil)
}

// Flush ensures all messages are delivered before shutdown
func (kp *KafkaProducer) Close() {
	remaining := kp.producer.Flush(15 * 1000) // 15 seconds timeout
	if remaining > 0 {
		log.Printf("Warning: %d messages not delivered before shutdown", remaining)
	}
	kp.producer.Close()
}

func correlationIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value("correlation_id").(string); ok {
		return id
	}
	return ""
}

// ============================================================
// Demo
// ============================================================

func main() {
	bootstrapServers := getEnv("KAFKA_BROKERS", "localhost:9092")
	topic := getEnv("KAFKA_TOPIC", "booking-events")

	producer, err := NewKafkaProducer(bootstrapServers, topic)
	if err != nil {
		log.Fatalf("Failed to create producer: %v", err)
	}
	defer producer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Produce 10 booking events
	for i := 0; i < 10; i++ {
		event := BookingEvent{
			EventID:   fmt.Sprintf("evt-%d", i),
			Type:      EventBookingCreated,
			BookingID: fmt.Sprintf("booking-%d", i%3), // 3 unique bookings
			UserID:    fmt.Sprintf("user-%d", i),
			HotelID:   "hotel-grand-palace",
			Timestamp: time.Now(),
		}

		if err := producer.Publish(ctx, event); err != nil {
			log.Printf("Failed to publish event: %v", err)
			continue
		}
		log.Printf("Published: booking_id=%s type=%s", event.BookingID, event.Type)

		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
