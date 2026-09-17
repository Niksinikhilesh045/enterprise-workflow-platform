package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/events"
	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/store"
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

	brokers := strings.Split(envOr("KAFKA_BROKERS", "localhost:9092"), ",")
	dlqPublisher, err := events.NewKafkaPublisher(brokers)
	if err != nil {
		log.Fatal(err)
	}
	defer dlqPublisher.Close()

	handler := events.NewNotificationHandler(mongoStore)
	consumer, err := events.NewKafkaConsumer(
		brokers,
		envOr("KAFKA_GROUP_ID", "workflow-notifications-v1"),
		store.RecordEventsTopic,
		handler,
		dlqPublisher,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer consumer.Close()

	log.Printf("event worker consuming %s", store.RecordEventsTopic)
	if err := consumer.Run(ctx); err != nil {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
