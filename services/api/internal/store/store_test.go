package store

import (
	"errors"
	"testing"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/domain"
)

func seedWorkflow(t *testing.T, s *MemoryStore, tenant string) domain.WorkflowDefinition {
	t.Helper()
	wf, err := s.CreateWorkflow(tenant, domain.WorkflowDefinition{Name: "Equipment Request", States: []string{"SUBMITTED", "APPROVED"}})
	if err != nil {
		t.Fatal(err)
	}
	return wf
}

func TestTenantIsolation(t *testing.T) {
	s := NewMemoryStore()
	wf := seedWorkflow(t, s, "tenant-a")
	rec, _, err := s.CreateRecord("tenant-a", "", domain.Record{WorkflowID: wf.ID, Data: map[string]any{"device": "laptop"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetRecord("tenant-b", rec.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected tenant isolation, got %v", err)
	}
}

func TestIdempotentCreate(t *testing.T) {
	s := NewMemoryStore()
	wf := seedWorkflow(t, s, "tenant-a")
	first, replay, err := s.CreateRecord("tenant-a", "req-123", domain.Record{WorkflowID: wf.ID})
	if err != nil || replay {
		t.Fatalf("unexpected first create: replay=%v err=%v", replay, err)
	}
	second, replay, err := s.CreateRecord("tenant-a", "req-123", domain.Record{WorkflowID: wf.ID})
	if err != nil || !replay || first.ID != second.ID {
		t.Fatalf("idempotency failed: %#v %#v replay=%v err=%v", first, second, replay, err)
	}
}

func TestOptimisticConcurrency(t *testing.T) {
	s := NewMemoryStore()
	wf := seedWorkflow(t, s, "tenant-a")
	rec, _, _ := s.CreateRecord("tenant-a", "", domain.Record{WorkflowID: wf.ID})
	updated, err := s.UpdateRecord("tenant-a", rec.ID, 1, "APPROVED", nil)
	if err != nil || updated.Version != 2 {
		t.Fatalf("first update failed: %#v %v", updated, err)
	}
	if _, err := s.UpdateRecord("tenant-a", rec.ID, 1, "SUBMITTED", nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestListRecordsFiltersTenantAndState(t *testing.T) {
	s := NewMemoryStore()
	workflowA := seedWorkflow(t, s, "tenant-a")
	workflowB := seedWorkflow(t, s, "tenant-b")

	pending, _, err := s.CreateRecord("tenant-a", "pending", domain.Record{WorkflowID: workflowA.ID, Data: map[string]any{"request": "Laptop"}})
	if err != nil {
		t.Fatal(err)
	}
	approved, _, err := s.CreateRecord("tenant-a", "approved", domain.Record{WorkflowID: workflowA.ID, Data: map[string]any{"request": "Monitor"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateRecord("tenant-a", approved.ID, approved.Version, "APPROVED", nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateRecord("tenant-b", "other", domain.Record{WorkflowID: workflowB.ID}); err != nil {
		t.Fatal(err)
	}

	all, err := s.ListRecords("tenant-a", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 tenant-a records, got %d", len(all))
	}

	submitted, err := s.ListRecords("tenant-a", "SUBMITTED")
	if err != nil {
		t.Fatal(err)
	}
	if len(submitted) != 1 || submitted[0].ID != pending.ID {
		t.Fatalf("unexpected submitted records: %#v", submitted)
	}

	approvedRecords, err := s.ListRecords("tenant-a", "APPROVED")
	if err != nil {
		t.Fatal(err)
	}
	if len(approvedRecords) != 1 || approvedRecords[0].ID != approved.ID {
		t.Fatalf("unexpected approved records: %#v", approvedRecords)
	}
}
