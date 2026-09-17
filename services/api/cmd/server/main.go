package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/auth"
	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/httpapi"
	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	appStore, closeStore := newStore(ctx)
	defer closeStore()

	tokenManager, err := auth.NewManager(os.Getenv("AUTH_SECRET"))
	if err != nil {
		log.Fatalf("AUTH_SECRET configuration error: %v", err)
	}

	addr := envOr("HTTP_ADDR", ":8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewWithAuth(appStore, tokenManager),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		log.Printf("enterprise workflow API listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func newStore(ctx context.Context) (store.Store, func()) {
	if envOr("PERSISTENCE", "memory") != "mongo" {
		return store.NewMemoryStore(), func() {}
	}
	s, err := store.NewMongoStore(ctx, envOr("MONGO_URI", "mongodb://localhost:27017/?replicaSet=rs0"), envOr("MONGO_DB", "enterprise_workflow"))
	if err != nil {
		log.Fatal(err)
	}
	return s, func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Close(closeCtx)
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
