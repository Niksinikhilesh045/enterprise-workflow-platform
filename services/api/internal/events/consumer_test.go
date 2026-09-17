package events

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/domain"
	"github.com/segmentio/kafka-go"
)

type fakeHandler struct {
	failures int
	calls    int
}

func (h *fakeHandler) Handle(_ context.Context, _ domain.OutboxEvent) error {
	h.calls++
	if h.calls <= h.failures {
		return errors.New("temporary failure")
	}
	return nil
}

type fakePublisher struct {
	events []domain.OutboxEvent
	err    error
}

func (p *fakePublisher) Publish(_ context.Context, event domain.OutboxEvent) error {
	if p.err != nil {
		return p.err
	}
	p.events = append(p.events, event)
	return nil
}

func (p *fakePublisher) Close() error { return nil }

type fakeNotificationSink struct {
	items []domain.Notification
}

func (s *fakeNotificationSink) StoreNotification(_ context.Context, notification domain.Notification) (bool, error) {
	s.items = append(s.items, notification)
	return true, nil
}

func kafkaEventMessage(t *testing.T) kafka.Message {
	t.Helper()
	event := domain.OutboxEvent{
		ID:          "event-1",
		TenantID:    "tenant-a",
		AggregateID: "record-1",
		Topic:       "workflow.record.events",
		Key:         "record-1",
		Type:        "record.updated",
		Payload:     map[string]any{"state": "APPROVED"},
		CreatedAt:   time.Now().UTC(),
	}
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	return kafka.Message{Key: []byte(event.Key), Value: body}
}

func TestConsumerRetriesAndRecoversWithoutDLQ(t *testing.T) {
	handler := &fakeHandler{failures: 2}
	dlq := &fakePublisher{}
	consumer := &KafkaConsumer{handler: handler, dlq: dlq, dlqTopic: "workflow.record.events.dlq", maxAttempts: 3, retryDelay: time.Millisecond}

	if err := consumer.consumeMessage(context.Background(), kafkaEventMessage(t)); err != nil {
		t.Fatal(err)
	}
	if handler.calls != 3 {
		t.Fatalf("expected 3 handler calls, got %d", handler.calls)
	}
	if len(dlq.events) != 0 {
		t.Fatalf("expected no DLQ events, got %d", len(dlq.events))
	}
}

func TestConsumerRoutesExhaustedFailureToDLQ(t *testing.T) {
	handler := &fakeHandler{failures: 5}
	dlq := &fakePublisher{}
	consumer := &KafkaConsumer{handler: handler, dlq: dlq, dlqTopic: "workflow.record.events.dlq", maxAttempts: 3, retryDelay: time.Millisecond}

	if err := consumer.consumeMessage(context.Background(), kafkaEventMessage(t)); err != nil {
		t.Fatal(err)
	}
	if handler.calls != 3 {
		t.Fatalf("expected 3 handler calls, got %d", handler.calls)
	}
	if len(dlq.events) != 1 {
		t.Fatalf("expected one DLQ event, got %d", len(dlq.events))
	}
	failed := dlq.events[0]
	if failed.Topic != "workflow.record.events.dlq" || failed.Type != "record.updated.failed" || failed.Attempts != 3 {
		t.Fatalf("unexpected DLQ event: %#v", failed)
	}
}

func TestConsumerRoutesMalformedPayloadToDLQ(t *testing.T) {
	dlq := &fakePublisher{}
	consumer := &KafkaConsumer{handler: &fakeHandler{}, dlq: dlq, dlqTopic: "workflow.record.events.dlq", maxAttempts: 3, retryDelay: time.Millisecond}
	message := kafka.Message{
		Key:   []byte("record-1"),
		Value: []byte("not-json"),
		Headers: []kafka.Header{
			{Key: "event-id", Value: []byte("event-bad")},
			{Key: "tenant-id", Value: []byte("tenant-a")},
		},
	}

	if err := consumer.consumeMessage(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if len(dlq.events) != 1 || dlq.events[0].Type != "consumer.decode_failed" {
		t.Fatalf("malformed event was not sent to DLQ: %#v", dlq.events)
	}
}

func TestNotificationHandlerProjectsSourceEventID(t *testing.T) {
	sink := &fakeNotificationSink{}
	handler := NewNotificationHandler(sink)
	handler.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	event := domain.OutboxEvent{
		ID:          "event-1",
		TenantID:    "tenant-a",
		AggregateID: "record-1",
		Type:        "record.updated",
		Payload:     map[string]any{"state": "APPROVED"},
	}

	if err := handler.Handle(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if len(sink.items) != 1 {
		t.Fatalf("expected one notification, got %d", len(sink.items))
	}
	item := sink.items[0]
	if item.ID != event.ID || item.TenantID != event.TenantID || item.RecordID != event.AggregateID || item.State != "APPROVED" {
		t.Fatalf("unexpected notification: %#v", item)
	}
}
