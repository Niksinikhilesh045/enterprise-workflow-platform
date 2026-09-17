package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/domain"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("version conflict")
	ErrInvalidWorkflow = errors.New("invalid workflow")
)

const RecordEventsTopic = "workflow.record-events"

type Store interface {
	CreateWorkflow(tenantID string, wf domain.WorkflowDefinition) (domain.WorkflowDefinition, error)
	ListWorkflows(tenantID string) ([]domain.WorkflowDefinition, error)
	CreateRecord(tenantID, idempotencyKey string, r domain.Record) (domain.Record, bool, error)
	GetRecord(tenantID, id string) (domain.Record, error)
	UpdateRecord(tenantID, id string, expectedVersion int64, state string, data map[string]any) (domain.Record, error)
	ListAuditEvents(tenantID string) ([]domain.AuditEvent, error)
}

type OutboxStore interface {
	ListPendingOutbox(ctx context.Context, limit int) ([]domain.OutboxEvent, error)
	MarkOutboxPublished(ctx context.Context, id string) error
	MarkOutboxFailed(ctx context.Context, id, reason string) error
}

type MemoryStore struct {
	mu sync.RWMutex
	workflows map[string]domain.WorkflowDefinition
	records map[string]domain.Record
	idempotency map[string]string
	audit []domain.AuditEvent
	outbox []domain.OutboxEvent
	counter int64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{workflows: map[string]domain.WorkflowDefinition{}, records: map[string]domain.Record{}, idempotency: map[string]string{}}
}

func (s *MemoryStore) nextID(prefix string) string { s.counter++; return fmt.Sprintf("%s_%06d", prefix, s.counter) }

func (s *MemoryStore) CreateWorkflow(tenantID string, wf domain.WorkflowDefinition) (domain.WorkflowDefinition, error) {
	s.mu.Lock(); defer s.mu.Unlock()
	if tenantID == "" || len(wf.States) == 0 || wf.Name == "" { return domain.WorkflowDefinition{}, ErrInvalidWorkflow }
	now := time.Now().UTC(); wf.ID = s.nextID("wf"); wf.TenantID = tenantID; wf.Version = 1; wf.CreatedAt, wf.UpdatedAt = now, now; s.workflows[wf.ID] = wf
	return wf, nil
}

func (s *MemoryStore) ListWorkflows(tenantID string) ([]domain.WorkflowDefinition, error) {
	s.mu.RLock(); defer s.mu.RUnlock(); out := make([]domain.WorkflowDefinition, 0)
	for _, wf := range s.workflows { if wf.TenantID == tenantID { out = append(out, wf) } }
	return out, nil
}

func (s *MemoryStore) CreateRecord(tenantID, key string, r domain.Record) (domain.Record, bool, error) {
	s.mu.Lock(); defer s.mu.Unlock()
	wf, ok := s.workflows[r.WorkflowID]; if !ok || wf.TenantID != tenantID { return domain.Record{}, false, ErrInvalidWorkflow }
	if key != "" { if existing, ok := s.idempotency[tenantID+":"+key]; ok { return s.records[existing], true, nil } }
	now := time.Now().UTC(); r.ID = s.nextID("rec"); r.TenantID = tenantID; r.State = wf.States[0]; r.Version = 1; r.Idempotency = key; r.CreatedAt, r.UpdatedAt = now, now; s.records[r.ID] = r
	if key != "" { s.idempotency[tenantID+":"+key] = r.ID }
	s.appendAuditLocked(r, "record.created"); s.appendOutboxLocked(r, "record.created")
	return r, false, nil
}

func (s *MemoryStore) GetRecord(tenantID, id string) (domain.Record, error) {
	s.mu.RLock(); defer s.mu.RUnlock(); r, ok := s.records[id]; if !ok || r.TenantID != tenantID { return domain.Record{}, ErrNotFound }; return r, nil
}

func (s *MemoryStore) UpdateRecord(tenantID, id string, expectedVersion int64, state string, data map[string]any) (domain.Record, error) {
	s.mu.Lock(); defer s.mu.Unlock(); r, ok := s.records[id]; if !ok || r.TenantID != tenantID { return domain.Record{}, ErrNotFound }; if r.Version != expectedVersion { return domain.Record{}, ErrConflict }
	wf := s.workflows[r.WorkflowID]; if state != "" && !contains(wf.States, state) { return domain.Record{}, ErrInvalidWorkflow }; if state != "" { r.State = state }; if data != nil { r.Data = data }; r.Version++; r.UpdatedAt = time.Now().UTC(); s.records[id] = r
	s.appendAuditLocked(r, "record.updated"); s.appendOutboxLocked(r, "record.updated"); return r, nil
}

func (s *MemoryStore) ListAuditEvents(tenantID string) ([]domain.AuditEvent, error) {
	s.mu.RLock(); defer s.mu.RUnlock(); out := make([]domain.AuditEvent, 0)
	for _, e := range s.audit { if e.TenantID == tenantID { out = append(out, e) } }; return out, nil
}

func (s *MemoryStore) ListPendingOutbox(_ context.Context, limit int) ([]domain.OutboxEvent, error) {
	s.mu.RLock(); defer s.mu.RUnlock(); if limit <= 0 { limit = 100 }; out := make([]domain.OutboxEvent, 0, limit)
	for _, e := range s.outbox { if e.PublishedAt == nil { out = append(out, e); if len(out) == limit { break } } }; return out, nil
}

func (s *MemoryStore) MarkOutboxPublished(_ context.Context, id string) error {
	s.mu.Lock(); defer s.mu.Unlock(); for i := range s.outbox { if s.outbox[i].ID == id { now := time.Now().UTC(); s.outbox[i].PublishedAt = &now; s.outbox[i].LastError = ""; return nil } }; return ErrNotFound
}

func (s *MemoryStore) MarkOutboxFailed(_ context.Context, id, reason string) error {
	s.mu.Lock(); defer s.mu.Unlock(); for i := range s.outbox { if s.outbox[i].ID == id { s.outbox[i].Attempts++; s.outbox[i].LastError = reason; return nil } }; return ErrNotFound
}

func (s *MemoryStore) appendAuditLocked(r domain.Record, typ string) { s.audit = append(s.audit, domain.AuditEvent{ID:s.nextID("evt"),TenantID:r.TenantID,RecordID:r.ID,Type:typ,OccurredAt:time.Now().UTC(),Payload:map[string]any{"state":r.State,"version":r.Version}}) }
func (s *MemoryStore) appendOutboxLocked(r domain.Record, typ string) { s.outbox = append(s.outbox, domain.OutboxEvent{ID:s.nextID("out"),TenantID:r.TenantID,AggregateID:r.ID,Topic:RecordEventsTopic,Key:r.ID,Type:typ,Payload:map[string]any{"recordId":r.ID,"workflowId":r.WorkflowID,"state":r.State,"version":r.Version},CreatedAt:time.Now().UTC()}) }
func contains(items []string, wanted string) bool { for _, item := range items { if item == wanted { return true } }; return false }
