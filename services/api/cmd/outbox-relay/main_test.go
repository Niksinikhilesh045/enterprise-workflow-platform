package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/domain"
)

type fakeOutboxStore struct {
	pending  []domain.OutboxEvent
	published map[string]bool
	failed    map[string]int
}

func newFakeOutboxStore(count int) *fakeOutboxStore {
	events := make([]domain.OutboxEvent, 0, count)
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("event-%03d", i)
		events = append(events, domain.OutboxEvent{ID: id, Topic: "workflow.record-events", Key: id})
	}
	return &fakeOutboxStore{
		pending: events,
		published: make(map[string]bool),
		failed: make(map[string]int),
	}
}

func (s *fakeOutboxStore) ListPendingOutbox(_ context.Context, limit int) ([]domain.OutboxEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	out := make([]domain.OutboxEvent, 0, limit)
	for _, event := range s.pending {
		if s.published[event.ID] {
			continue
		}
		out = append(out, event)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (s *fakeOutboxStore) MarkOutboxPublished(_ context.Context, id string) error {
	s.published[id] = true
	return nil
}

func (s *fakeOutboxStore) MarkOutboxFailed(_ context.Context, id, _ string) error {
	s.failed[id]++
	return nil
}

type fakePublisher struct {
	calls   int
	failOn  string
}

func (p *fakePublisher) Publish(_ context.Context, event domain.OutboxEvent) error {
	p.calls++
	if event.ID == p.failOn {
		return errors.New("synthetic publish failure")
	}
	return nil
}

func (p *fakePublisher) Close() error { return nil }

func TestDrainOutboxProcessesMultipleBatches(t *testing.T) {
	store := newFakeOutboxStore(725)
	publisher := &fakePublisher{}

	count, err := drainOutbox(context.Background(), store, publisher, 250)
	if err != nil {
		t.Fatalf("drainOutbox returned error: %v", err)
	}
	if count != 725 {
		t.Fatalf("expected 725 published events, got %d", count)
	}
	if publisher.calls != 725 {
		t.Fatalf("expected 725 publish calls, got %d", publisher.calls)
	}
	if len(store.published) != 725 {
		t.Fatalf("expected 725 published markers, got %d", len(store.published))
	}
}

func TestDrainOutboxStopsAfterBatchWithPublishError(t *testing.T) {
	store := newFakeOutboxStore(300)
	publisher := &fakePublisher{failOn: "event-010"}

	count, err := drainOutbox(context.Background(), store, publisher, 100)
	if err == nil {
		t.Fatal("expected drainOutbox to return a publish error")
	}
	if count != 99 {
		t.Fatalf("expected 99 successfully published events from first batch, got %d", count)
	}
	if store.failed["event-010"] != 1 {
		t.Fatalf("expected failed event to be marked once, got %d", store.failed["event-010"])
	}
	if publisher.calls != 100 {
		t.Fatalf("expected exactly one batch to be attempted after failure, got %d publish calls", publisher.calls)
	}
}
