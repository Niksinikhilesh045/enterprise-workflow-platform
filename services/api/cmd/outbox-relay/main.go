package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/events"
	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/store"
)

const (
	outboxBatchSize = 250
	idlePollDelay   = 250 * time.Millisecond
	errorBackoff    = 500 * time.Millisecond
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	mongoStore, err := store.NewMongoStore(
		ctx,
		envOr("MONGO_URI", "mongodb://localhost:27017/?replicaSet=rs0"),
		envOr("MONGO_DB", "enterprise_workflow"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer mongoStore.Close(context.Background())

	publisher, err := events.NewKafkaPublisher(strings.Split(envOr("KAFKA_BROKERS", "localhost:9092"), ","))
	if err != nil {
		log.Fatal(err)
	}
	defer publisher.Close()

	for {
		drained, err := drainOutbox(ctx, mongoStore, publisher, outboxBatchSize)
		if err != nil {
			log.Printf("drain outbox: %v", err)
			if !sleepContext(ctx, errorBackoff) {
				return
			}
			continue
		}
		if drained == 0 && !sleepContext(ctx, idlePollDelay) {
			return
		}
	}
}

func drainOutbox(ctx context.Context, outbox store.OutboxStore, publisher events.Publisher, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = outboxBatchSize
	}

	totalPublished := 0
	for {
		items, err := outbox.ListPendingOutbox(ctx, batchSize)
		if err != nil {
			return totalPublished, fmt.Errorf("list pending outbox: %w", err)
		}
		if len(items) == 0 {
			return totalPublished, nil
		}

		var batchErr error
		for _, event := range items {
			if err := publisher.Publish(ctx, event); err != nil {
				_ = outbox.MarkOutboxFailed(ctx, event.ID, err.Error())
				batchErr = errors.Join(batchErr, fmt.Errorf("publish %s: %w", event.ID, err))
				continue
			}
			if err := outbox.MarkOutboxPublished(ctx, event.ID); err != nil {
				batchErr = errors.Join(batchErr, fmt.Errorf("mark %s published: %w", event.ID, err))
				continue
			}
			totalPublished++
		}

		if batchErr != nil {
			return totalPublished, batchErr
		}
		if len(items) < batchSize {
			return totalPublished, nil
		}
	}
}

func sleepContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
