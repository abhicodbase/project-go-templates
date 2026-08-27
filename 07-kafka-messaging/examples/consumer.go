// consumer.go — Kafka consumer in Go with manual offset commit + DLQ
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// ============================================================
// Consumer with at-least-once semantics
// ============================================================

type BookingEventHandler interface {
	Handle(ctx context.Context, event BookingEvent) error
}

type KafkaConsumer struct {
	consumer *kafka.Consumer
	producer *kafka.Producer // for DLQ
	topic    string
	dlqTopic string
	handler  BookingEventHandler
	maxRetries int
}

func NewKafkaConsumer(bootstrapServers, groupID, topic, dlqTopic string, handler BookingEventHandler) (*KafkaConsumer, error) {
	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":       bootstrapServers,
		"group.id":                groupID,
		"auto.offset.reset":       "earliest",     // start from beginning for new group
		"enable.auto.commit":      false,           // MANUAL commit (at-least-once)
		"max.poll.interval.ms":    300000,          // 5 min before considered dead
		"session.timeout.ms":      45000,
		"heartbeat.interval.ms":   3000,
		"partition.assignment.strategy": "cooperative-sticky", // minimize rebalance disruption
	})
	if err != nil {
		return nil, fmt.Errorf("create kafka consumer: %w", err)
	}

	p, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": bootstrapServers,
	})
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("create dlq producer: %w", err)
	}

	return &KafkaConsumer{
		consumer:   c,
		producer:   p,
		topic:      topic,
		dlqTopic:   dlqTopic,
		handler:    handler,
		maxRetries: 3,
	}, nil
}

// Start begins consuming messages until context is cancelled
func (kc *KafkaConsumer) Start(ctx context.Context) error {
	if err := kc.consumer.SubscribeTopics([]string{kc.topic}, nil); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}

	log.Printf("Consumer started: topic=%s group=%s", kc.topic, "notification-service")

	for {
		select {
		case <-ctx.Done():
			log.Println("Shutting down consumer...")
			// Commit final offsets
			kc.consumer.Commit()
			return nil
		default:
		}

		msg, err := kc.consumer.ReadMessage(100 * time.Millisecond)
		if err != nil {
			var kafkaErr kafka.Error
			if errors.As(err, &kafkaErr) && kafkaErr.Code() == kafka.ErrTimedOut {
				continue // No message in 100ms — normal
			}
			log.Printf("Consumer error: %v", err)
			continue
		}

		kc.processMessage(ctx, msg)
	}
}

func (kc *KafkaConsumer) processMessage(ctx context.Context, msg *kafka.Message) {
	// Parse event
	var event BookingEvent
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		log.Printf("Failed to unmarshal message: %v, sending to DLQ", err)
		kc.sendToDLQ(msg, err)
		kc.commitOffset(msg) // Commit even on parse error — can't fix this
		return
	}

	log.Printf("Processing: event_id=%s type=%s booking_id=%s offset=%d",
		event.EventID, event.Type, event.BookingID, msg.TopicPartition.Offset)

	// Retry with backoff
	var lastErr error
	for attempt := 0; attempt < kc.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt*attempt) * 100 * time.Millisecond
			log.Printf("Retry %d/%d for event %s after %v", attempt+1, kc.maxRetries, event.EventID, backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
		}

		if err := kc.handler.Handle(ctx, event); err != nil {
			lastErr = err
			log.Printf("Handler error (attempt %d): %v", attempt+1, err)
			continue
		}

		// Success — commit offset
		kc.commitOffset(msg)
		return
	}

	// All retries exhausted → DLQ
	log.Printf("All retries failed for event %s, sending to DLQ: %v", event.EventID, lastErr)
	kc.sendToDLQ(msg, lastErr)
	kc.commitOffset(msg) // Commit to avoid infinite loop
}

func (kc *KafkaConsumer) sendToDLQ(original *kafka.Message, err error) {
	dlqMsg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &kc.dlqTopic,
			Partition: kafka.PartitionAny,
		},
		Key:   original.Key,
		Value: original.Value,
		Headers: append(original.Headers,
			kafka.Header{Key: "dlq-error", Value: []byte(err.Error())},
			kafka.Header{Key: "dlq-timestamp", Value: []byte(time.Now().Format(time.RFC3339))},
			kafka.Header{Key: "original-topic", Value: []byte(*original.TopicPartition.Topic)},
			kafka.Header{Key: "original-partition", Value: []byte(fmt.Sprintf("%d", original.TopicPartition.Partition))},
		),
	}

	if err := kc.producer.Produce(dlqMsg, nil); err != nil {
		log.Printf("Failed to send to DLQ: %v", err)
	}
}

func (kc *KafkaConsumer) commitOffset(msg *kafka.Message) {
	if _, err := kc.consumer.CommitMessage(msg); err != nil {
		log.Printf("Failed to commit offset: %v", err)
	}
}

func (kc *KafkaConsumer) Close() {
	kc.consumer.Close()
	kc.producer.Close()
}

// ============================================================
// Example handler — Idempotent!
// ============================================================

type NotificationHandler struct {
	processedIDs map[string]bool // in-memory dedup (use Redis in production)
}

func (h *NotificationHandler) Handle(ctx context.Context, event BookingEvent) error {
	// Idempotency check — if already processed, skip
	if h.processedIDs[event.EventID] {
		log.Printf("Duplicate event %s, skipping", event.EventID)
		return nil
	}

	switch event.Type {
	case EventBookingCreated:
		log.Printf("Sending booking confirmation email for booking %s", event.BookingID)
		// sendEmail(...)
	case EventBookingCancelled:
		log.Printf("Sending cancellation notification for booking %s", event.BookingID)
		// sendEmail(...)
	default:
		log.Printf("Unknown event type: %s", event.Type)
	}

	h.processedIDs[event.EventID] = true
	return nil
}

// ============================================================
// Demo
// ============================================================

func main() {
	bootstrapServers := getEnv("KAFKA_BROKERS", "localhost:9092")
	topic := getEnv("KAFKA_TOPIC", "booking-events")
	dlqTopic := topic + "-dlq"
	groupID := "notification-service"

	handler := &NotificationHandler{processedIDs: make(map[string]bool)}
	consumer, err := NewKafkaConsumer(bootstrapServers, groupID, topic, dlqTopic, handler)
	if err != nil {
		log.Fatalf("Failed to create consumer: %v", err)
	}
	defer consumer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown on signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Received shutdown signal")
		cancel()
	}()

	if err := consumer.Start(ctx); err != nil {
		log.Fatalf("Consumer error: %v", err)
	}
	log.Println("Consumer stopped")
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
