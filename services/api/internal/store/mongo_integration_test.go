//go:build integration

package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/domain"
)

func TestMongoStoreEndToEnd(t *testing.T) {
	uri := os.Getenv("MONGO_URI")
	if uri == "" { t.Skip("MONGO_URI is not set") }
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second); defer cancel()
	s, err := NewMongoStore(ctx, uri, "enterprise_workflow_test"); if err != nil { t.Fatal(err) }; defer s.Close(context.Background())
	if err := s.db.Drop(ctx); err != nil { t.Fatal(err) }; if err := s.ensureIndexes(ctx); err != nil { t.Fatal(err) }
	wf, err := s.CreateWorkflow("tenant-a", domain.WorkflowDefinition{Name:"Equipment Request",States:[]string{"SUBMITTED","APPROVED"}}); if err != nil { t.Fatal(err) }
	first, replay, err := s.CreateRecord("tenant-a", "request-1", domain.Record{WorkflowID:wf.ID,Data:map[string]any{"device":"laptop"}}); if err != nil || replay { t.Fatalf("create record: replay=%v err=%v", replay, err) }
	second, replay, err := s.CreateRecord("tenant-a", "request-1", domain.Record{WorkflowID:wf.ID}); if err != nil || !replay || second.ID != first.ID { t.Fatalf("idempotent replay failed: replay=%v err=%v", replay, err) }
	if _, err := s.GetRecord("tenant-b", first.ID); !errors.Is(err, ErrNotFound) { t.Fatalf("tenant isolation failed: %v", err) }
	updated, err := s.UpdateRecord("tenant-a", first.ID, 1, "APPROVED", first.Data); if err != nil || updated.Version != 2 { t.Fatalf("update record: %#v err=%v", updated, err) }
	if _, err := s.UpdateRecord("tenant-a", first.ID, 1, "SUBMITTED", nil); !errors.Is(err, ErrConflict) { t.Fatalf("expected version conflict, got %v", err) }
	outbox, err := s.ListPendingOutbox(ctx, 10); if err != nil { t.Fatal(err) }; if len(outbox) != 2 { t.Fatalf("expected 2 outbox events, got %d", len(outbox)) }
	audit, err := s.ListAuditEvents("tenant-a"); if err != nil { t.Fatal(err) }; if len(audit) != 2 { t.Fatalf("expected 2 audit events, got %d", len(audit)) }
}
