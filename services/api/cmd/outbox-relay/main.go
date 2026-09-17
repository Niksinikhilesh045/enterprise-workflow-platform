package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/events"
	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM); defer stop()
	mongoStore, err := store.NewMongoStore(ctx, envOr("MONGO_URI", "mongodb://localhost:27017/?replicaSet=rs0"), envOr("MONGO_DB", "enterprise_workflow")); if err != nil { log.Fatal(err) }
	defer mongoStore.Close(context.Background())
	publisher, err := events.NewKafkaPublisher(strings.Split(envOr("KAFKA_BROKERS", "localhost:9092"), ",")); if err != nil { log.Fatal(err) }; defer publisher.Close()
	ticker := time.NewTicker(500*time.Millisecond); defer ticker.Stop()
	for { select { case <-ctx.Done(): return; case <-ticker.C: flush(ctx, mongoStore, publisher) } }
}

func flush(ctx context.Context, outbox store.OutboxStore, publisher events.Publisher) {
	items, err := outbox.ListPendingOutbox(ctx, 100); if err != nil { log.Printf("list outbox: %v", err); return }
	for _, event := range items { if err := publisher.Publish(ctx, event); err != nil { _ = outbox.MarkOutboxFailed(ctx, event.ID, err.Error()); continue }; _ = outbox.MarkOutboxPublished(ctx, event.ID) }
}

func envOr(key, fallback string) string { if v := os.Getenv(key); v != "" { return v }; return fallback }
