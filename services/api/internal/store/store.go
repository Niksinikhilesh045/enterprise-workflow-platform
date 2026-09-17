package store

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/domain"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrConflict        = errors.New("version conflict")
	ErrInvalidWorkflow = errors.New("invalid workflow")
)

type Store interface {
	CreateWorkflow(tenantID string, wf domain.WorkflowDefinition) (domain.WorkflowDefinition, error)
	ListWorkflows(tenantID string) []domain.WorkflowDefinition
	CreateRecord(tenantID, idempotencyKey string, r domain.Record) (domain.Record, bool, error)
	GetRecord(tenantID, id string) (domain.Record, error)
	UpdateRecord(tenantID, id string, expectedVersion int64, state string, data map[string]any) (domain.Record, error)
	ListAuditEvents(tenantID string) []domain.AuditEvent
}

type MemoryStore struct {
	mu          sync.RWMutex
	workflows   map[string]domain.WorkflowDefinition
	records     map[string]domain.Record
	idempotency map[string]string
	audit       []domain.AuditEvent
	counter     int64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{workflows: map[string]domain.WorkflowDefinition{}, records: map[string]domain.Record{}, idempotency: map[string]string{}}
}

func (s *MemoryStore) nextID(prefix string) string {
	s.counter++
	return fmt.Sprintf("%s_%06d", prefix, s.counter)
}

func (s *MemoryStore) CreateWorkflow(tenantID string, wf domain.WorkflowDefinition) (domain.WorkflowDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tenantID == "" || len(wf.States) == 0 || wf.Name == "" {
		return domain.WorkflowDefinition{}, ErrInvalidWorkflow
	}
	now := time.Now().UTC()
	wf.ID = s.nextID("wf")
	wf.TenantID = tenantID
	wf.Version = 1
	wf.CreatedAt, wf.UpdatedAt = now, now
	s.workflows[wf.ID] = wf
	return wf, nil
}

func (s *MemoryStore) ListWorkflows(tenantID string) []domain.WorkflowDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.WorkflowDefinition, 0)
	for _, wf := range s.workflows {
		if wf.TenantID == tenantID {
			out = append(out, wf)
		}
	}
	return out
}

func (s *MemoryStore) CreateRecord(tenantID, key string, r domain.Record) (domain.Record, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	wf, ok := s.workflows[r.WorkflowID]
	if !ok || wf.TenantID != tenantID {
		return domain.Record{}, false, ErrInvalidWorkflow
	}
	if key != "" {
		if existing, ok := s.idempotency[tenantID+":"+key]; ok {
			return s.records[existing], true, nil
		}
	}
	now := time.Now().UTC()
	r.ID = s.nextID("rec")
	r.TenantID = tenantID
	r.State = wf.States[0]
	r.Version = 1
	r.Idempotency = key
	r.CreatedAt, r.UpdatedAt = now, now
	s.records[r.ID] = r
	if key != "" {
		s.idempotency[tenantID+":"+key] = r.ID
	}
	s.appendAuditLocked(r, "record.created", map[string]any{"state": r.State})
	return r, false, nil
}

func (s *MemoryStore) GetRecord(tenantID, id string) (domain.Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.records[id]
	if !ok || r.TenantID != tenantID {
		return domain.Record{}, ErrNotFound
	}
	return r, nil
}

func (s *MemoryStore) UpdateRecord(tenantID, id string, expectedVersion int64, state string, data map[string]any) (domain.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[id]
	if !ok || r.TenantID != tenantID {
		return domain.Record{}, ErrNotFound
	}
	if r.Version != expectedVersion {
		return domain.Record{}, ErrConflict
	}
	wf := s.workflows[r.WorkflowID]
	if state != "" && !contains(wf.States, state) {
		return domain.Record{}, ErrInvalidWorkflow
	}
	if state != "" {
		r.State = state
	}
	if data != nil {
		r.Data = data
	}
	r.Version++
	r.UpdatedAt = time.Now().UTC()
	s.records[id] = r
	s.appendAuditLocked(r, "record.updated", map[string]any{"state": r.State, "version": r.Version})
	return r, nil
}

func (s *MemoryStore) ListAuditEvents(tenantID string) []domain.AuditEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.AuditEvent, 0)
	for _, event := range s.audit {
		if event.TenantID == tenantID {
			out = append(out, event)
		}
	}
	return out
}

func (s *MemoryStore) appendAuditLocked(r domain.Record, eventType string, payload map[string]any) {
	s.audit = append(s.audit, domain.AuditEvent{ID: s.nextID("evt"), TenantID: r.TenantID, RecordID: r.ID, Type: eventType, OccurredAt: time.Now().UTC(), Payload: payload})
}

func contains(items []string, wanted string) bool {
	for _, item := range items {
		if item == wanted {
			return true
		}
	}
	return false
}
