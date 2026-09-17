package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/domain"
	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/store"
)

type Server struct{ store store.Store }

func New(s store.Store) http.Handler {
	srv := &Server{store: s}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", srv.health)
	mux.HandleFunc("POST /api/v1/workflows", srv.createWorkflow)
	mux.HandleFunc("GET /api/v1/workflows", srv.listWorkflows)
	mux.HandleFunc("POST /api/v1/records", srv.createRecord)
	mux.HandleFunc("GET /api/v1/records/{id}", srv.getRecord)
	mux.HandleFunc("PUT /api/v1/records/{id}", srv.updateRecord)
	mux.HandleFunc("GET /api/v1/audit-events", srv.listAudit)
	return withCORS(withJSON(mux))
}

func tenantID(r *http.Request) (string, bool) {
	tenant := strings.TrimSpace(r.Header.Get("X-Tenant-ID"))
	return tenant, tenant != ""
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, map[string]string{"status": "ok"}) }

func (s *Server) createWorkflow(w http.ResponseWriter, r *http.Request) {
	tenant, ok := tenantID(r)
	if !ok {
		writeErr(w, 400, "X-Tenant-ID is required")
		return
	}
	var wf domain.WorkflowDefinition
	if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	created, err := s.store.CreateWorkflow(tenant, wf)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) listWorkflows(w http.ResponseWriter, r *http.Request) {
	tenant, ok := tenantID(r)
	if !ok {
		writeErr(w, 400, "X-Tenant-ID is required")
		return
	}
	writeJSON(w, 200, s.store.ListWorkflows(tenant))
}

func (s *Server) createRecord(w http.ResponseWriter, r *http.Request) {
	tenant, ok := tenantID(r)
	if !ok {
		writeErr(w, 400, "X-Tenant-ID is required")
		return
	}
	var rec domain.Record
	if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	created, replay, err := s.store.CreateRecord(tenant, r.Header.Get("Idempotency-Key"), rec)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	if replay {
		w.Header().Set("Idempotent-Replay", "true")
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) getRecord(w http.ResponseWriter, r *http.Request) {
	tenant, ok := tenantID(r)
	if !ok {
		writeErr(w, 400, "X-Tenant-ID is required")
		return
	}
	rec, err := s.store.GetRecord(tenant, r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "record not found")
		return
	}
	writeJSON(w, 200, rec)
}

type updateRecordRequest struct {
	State           string         `json:"state"`
	Data            map[string]any `json:"data"`
	ExpectedVersion int64          `json:"expectedVersion"`
}

func (s *Server) updateRecord(w http.ResponseWriter, r *http.Request) {
	tenant, ok := tenantID(r)
	if !ok {
		writeErr(w, 400, "X-Tenant-ID is required")
		return
	}
	var req updateRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if h := r.Header.Get("If-Match"); h != "" {
		if v, err := strconv.ParseInt(strings.Trim(h, "\""), 10, 64); err == nil {
			req.ExpectedVersion = v
		}
	}
	rec, err := s.store.UpdateRecord(tenant, r.PathValue("id"), req.ExpectedVersion, req.State, req.Data)
	if errors.Is(err, store.ErrConflict) {
		writeErr(w, 409, "version conflict")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "record not found")
		return
	}
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	w.Header().Set("ETag", strconv.FormatInt(rec.Version, 10))
	writeJSON(w, 200, rec)
}

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	tenant, ok := tenantID(r)
	if !ok {
		writeErr(w, 400, "X-Tenant-ID is required")
		return
	}
	writeJSON(w, 200, s.store.ListAuditEvents(tenant))
}

func withJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Tenant-ID, Idempotency-Key, If-Match")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
