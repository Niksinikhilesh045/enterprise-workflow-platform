package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/domain"
	"github.com/segmentio/kafka-go"
)

type Handler interface {
	Handle(context.Context, domain.OutboxEvent) error
}

type NotificationSink interface {
	StoreNotification(context.Context, domain.Notification) (bool, error)
}

type NotificationHandler struct {
	sink NotificationSink
	now  func() time.Time
}

func NewNotificationHandler(sink NotificationSink) *NotificationHandler {
	return &NotificationHandler{sink: sink, now: time.Now}
}

func (h *NotificationHandler) Handle(ctx context.Context, event domain.OutboxEvent) error {
	state, _ := event.Payload["state"].(string)
	_, err := h.sink.StoreNotification(ctx, domain.Notification{
		ID:        event.ID,
		TenantID:  event.TenantID,
		RecordID:  event.AggregateID,
		EventType: event.Type,
		State:     state,
		CreatedAt: h.now().UTC(),
	})
	return err
}

type KafkaConsumer struct {
	reader      *kafka.Reader
	handler     Handler
	dlq         Publisher
	dlqTopic    string
	maxAttempts int
	retryDelay  time.Duration
}

func NewKafkaConsumer(brokers []string, groupID, topic string, handler Handler, dlq Publisher) (*KafkaConsumer, error) {
	clean := make([]string, 0, len(brokers))
	for _, broker := range brokers {
		broker = strings.TrimSpace(broker)
		if broker != "" {
			clean = append(clean, broker)
		}
	}
	if len(clean) == 0 || groupID == "" || topic == "" || handler == nil || dlq == nil {
		return nil, errors.New("brokers, group ID, topic, handler, and DLQ publisher are required")
	}
	return &KafkaConsumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:        clean,
			GroupID:        groupID,
			Topic:          topic,
			MinBytes:       1,
			MaxBytes:       10e6,
			CommitInterval: 0,
		}),
		handler:     handler,
		dlq:         dlq,
		dlqTopic:    topic + ".dlq",
		maxAttempts: 3,
		retryDelay:  150 * time.Millisecond,
	}, nil
}

func (c *KafkaConsumer) Run(ctx context.Context) error {
	for {
		message, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			return fmt.Errorf("fetch kafka message: %w", err)
		}

		if err := c.consumeMessage(ctx, message); err != nil {
			return err
		}
		if err := c.reader.CommitMessages(ctx, message); err != nil {
			return fmt.Errorf("commit kafka message: %w", err)
		}
	}
}

func (c *KafkaConsumer) consumeMessage(ctx context.Context, message kafka.Message) error {
	var event domain.OutboxEvent
	if err := json.Unmarshal(message.Value, &event); err != nil {
		return c.publishDLQ(ctx, domain.OutboxEvent{
			ID:        headerValue(message.Headers, "event-id"),
			TenantID:  headerValue(message.Headers, "tenant-id"),
			AggregateID: string(message.Key),
			Topic:     c.dlqTopic,
			Key:       string(message.Key),
			Type:      "consumer.decode_failed",
			Payload:   map[string]any{"raw": string(message.Value), "error": err.Error()},
			CreatedAt: time.Now().UTC(),
		})
	}

	err := retry(ctx, c.maxAttempts, c.retryDelay, func() error {
		return c.handler.Handle(ctx, event)
	})
	if err == nil {
		return nil
	}

	failed := event
	failed.Topic = c.dlqTopic
	failed.Type = event.Type + ".failed"
	failed.LastError = err.Error()
	failed.Attempts = c.maxAttempts
	if failed.Payload == nil {
		failed.Payload = map[string]any{}
	}
	failed.Payload["consumerError"] = err.Error()
	failed.Payload["consumerAttempts"] = c.maxAttempts
	return c.publishDLQ(ctx, failed)
}

func (c *KafkaConsumer) publishDLQ(ctx context.Context, event domain.OutboxEvent) error {
	if event.ID == "" {
		event.ID = fmt.Sprintf("dlq-%d", time.Now().UnixNano())
	}
	if event.Key == "" {
		event.Key = event.AggregateID
	}
	if err := c.dlq.Publish(ctx, event); err != nil {
		return fmt.Errorf("publish DLQ event: %w", err)
	}
	return nil
}

func (c *KafkaConsumer) Close() error {
	return c.reader.Close()
}

func retry(ctx context.Context, attempts int, delay time.Duration, fn func() error) error {
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := fn(); err != nil {
			lastErr = err
			if attempt == attempts {
				break
			}
			timer := time.NewTimer(delay * time.Duration(attempt))
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		return nil
	}
	return lastErr
}

func headerValue(headers []kafka.Header, key string) string {
	for _, header := range headers {
		if header.Key == key {
			return string(header.Value)
		}
	}
	return ""
}
