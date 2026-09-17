package events

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/domain"
	"github.com/segmentio/kafka-go"
)

type Publisher interface {
	Publish(context.Context, domain.OutboxEvent) error
	Close() error
}

type KafkaPublisher struct{ writer *kafka.Writer }

func NewKafkaPublisher(brokers []string) (*KafkaPublisher, error) {
	clean := make([]string, 0, len(brokers))
	for _, broker := range brokers {
		if broker = strings.TrimSpace(broker); broker != "" {
			clean = append(clean, broker)
		}
	}
	if len(clean) == 0 {
		return nil, errors.New("at least one kafka broker is required")
	}
	return &KafkaPublisher{writer: &kafka.Writer{
		Addr:         kafka.TCP(clean...),
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
		Async:        false,
		BatchTimeout: 20 * time.Millisecond,
	}}, nil
}

func (p *KafkaPublisher) Publish(ctx context.Context, event domain.OutboxEvent) error {
	message, err := kafkaMessage(event)
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, message)
}

func (p *KafkaPublisher) PublishBatch(ctx context.Context, batch []domain.OutboxEvent) error {
	if len(batch) == 0 {
		return nil
	}
	messages := make([]kafka.Message, 0, len(batch))
	for _, event := range batch {
		message, err := kafkaMessage(event)
		if err != nil {
			return err
		}
		messages = append(messages, message)
	}
	return p.writer.WriteMessages(ctx, messages...)
}

func kafkaMessage(event domain.OutboxEvent) (kafka.Message, error) {
	payload, err := json.Marshal(event)
	if err != nil {
		return kafka.Message{}, err
	}
	return kafka.Message{
		Topic: event.Topic,
		Key:   []byte(event.Key),
		Value: payload,
		Headers: []kafka.Header{
			{Key: "event-id", Value: []byte(event.ID)},
			{Key: "event-type", Value: []byte(event.Type)},
			{Key: "tenant-id", Value: []byte(event.TenantID)},
		},
	}, nil
}

func (p *KafkaPublisher) Close() error { return p.writer.Close() }
