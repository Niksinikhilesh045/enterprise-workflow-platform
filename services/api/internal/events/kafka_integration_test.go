//go:build integration

package events

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/domain"
	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/store"
)

type countingNotificationSink struct {
	store *store.MongoStore
	calls atomic.Int32
}

func (s *countingNotificationSink) StoreNotification(ctx context.Context, notification domain.Notification) (bool, error) {
	s.calls.Add(1)
	return s.store.StoreNotification(ctx, notification)
}

func TestKafkaMongoNotificationIdempotency(t *testing.T) {
	mongoURI := os.Getenv("MONGO_URI")
	brokersValue := os.Getenv("KAFKA_BROKERS")
	if mongoURI == "" || brokersValue == "" {
		t.Skip("MONGO_URI and KAFKA_BROKERS are required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	database := fmt.Sprintf("enterprise_workflow_events_%d", time.Now().UnixNano())
	mongoStore, err := store.NewMongoStore(ctx, mongoURI, database)
	if err != nil {
		t.Fatal(err)
	}
	defer mongoStore.Close(context.Background())

	brokers := strings.Split(brokersValue, ",")
	publisher, err := NewKafkaPublisher(brokers)
	if err != nil {
		t.Fatal(err)
	}
	defer publisher.Close()

	const topic = "workflow.record.events.integration"
	sink := &countingNotificationSink{store: mongoStore}
	handler := NewNotificationHandler(sink)
	consumer, err := NewKafkaConsumer(brokers, "integration-consumer-"+fmt.Sprint(time.Now().UnixNano()), topic, handler, publisher)
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()

	consumerCtx, stopConsumer := context.WithCancel(ctx)
	defer stopConsumer()
	errCh := make(chan error, 1)
	go func() { errCh <- consumer.Run(consumerCtx) }()

	event := domain.OutboxEvent{
		ID:          "integration-event-1",
		TenantID:    "tenant-a",
		AggregateID: "record-1",
		Topic:       topic,
		Key:         "record-1",
		Type:        "record.updated",
		Payload:     map[string]any{"state": "APPROVED", "version": 2},
		CreatedAt:   time.Now().UTC(),
	}
	if err := publisher.Publish(ctx, event); err != nil {
		t.Fatal(err)
	}
	if err := publisher.Publish(ctx, event); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if sink.calls.Load() >= 2 {
			break
		}
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("consumer stopped early: %v", err)
			}
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	if sink.calls.Load() < 2 {
		t.Fatalf("expected duplicate Kafka delivery to be processed twice, got %d calls", sink.calls.Load())
	}

	count, err := mongoStore.CountNotifications(ctx, "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected exactly one idempotent notification, got %d", count)
	}

	stopConsumer()
}
