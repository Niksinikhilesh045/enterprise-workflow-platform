package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/auth"
	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/domain"
	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/store"
)

type contextKey string

const claimsKey contextKey = "auth-claims"

type Server struct {
	store store.Store
	auth  *auth.Manager
}

func New(s store.Store) http.Handler {
	return newHandler(&Server{store: s})
}

func NewWithAuth(s store.Store, manager *auth.Manager) http.Handler {
	return newHandler(&Server{store: s, auth: manager})
}

func newHandler(srv *Server) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", srv.health)
	mux.Handle("POST /api/v1/workflows", srv.authorize([]auth.Role{auth.RoleAdmin, auth.RoleBuilder}, http.HandlerFunc(srv.createWorkflow)))
	mux.Handle("GET /api/v1/workflows", srv.authorize(allRoles(), http.HandlerFunc(srv.listWorkflows)))
	mux.Handle("POST /api/v1/records", srv.authorize([]auth.Role{auth.RoleAdmin, auth.RoleBuilder, auth.RoleRequester}, http.HandlerFunc(srv.createRecord)))
	mux.Handle("GET /api/v1/records/{id}", srv.authorize(allRoles(), http.HandlerFunc(srv.getRecord)))
	mux.Handle("PUT /api/v1/records/{id}", srv.authorize([]auth.Role{auth.RoleAdmin, auth.RoleApprover}, http.HandlerFunc(srv.updateRecord)))
	mux.Handle("GET /api/v1/audit-events", srv.authorize([]auth.Role{auth.RoleAdmin, auth.RoleAuditor}, http.HandlerFunc(srv.listAudit)))
	return withCORS(withJSON(mux))
}

func allRoles() []auth.Role {
	return []auth.Role{auth.RoleAdmin, auth.RoleBuilder, auth.RoleRequester, auth.RoleApprover, auth.RoleAuditor}
}

func tenantID(r *http.Request) (string, bool) {
	if claims, ok := r.Context().Value(claimsKey).(auth.Claims); ok {
		return claims.TenantID, claims.TenantID != ""
	}
	tenant := strings.TrimSpace(r.Header.Get("X-Tenant-ID"))
	return tenant, tenant != ""
}

func (s *Server) authorize(roles []auth.Role, next http.Handler) http.Handler {
	if s.auth == nil {
		return next
	}
	allowed := make(map[auth.Role]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := auth.Bearer(r.Header.Get("Authorization"))
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "authentication required")
			return
		}
		claims, err := s.auth.Parse(token)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		if requested := strings.TrimSpace(r.Header.Get("X-Tenant-ID")); requested != "" && requested != claims.TenantID {
			writeErr(w, http.StatusForbidden, "tenant does not match token")
			return
		}
		if _, ok := allowed[claims.Role]; !ok {
			writeErr(w, http.StatusForbidden, "role is not allowed for this operation")
			return
		}
		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) createWorkflow(w http.ResponseWriter, r *http.Request) {
	tenant, ok := tenantID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "tenant is required")
		return
	}
	var wf domain.WorkflowDefinition
	if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	created, err := s.store.CreateWorkflow(tenant, wf)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) listWorkflows(w http.ResponseWriter, r *http.Request) {
	tenant, ok := tenantID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "tenant is required")
		return
	}
	items, err := s.store.ListWorkflows(tenant)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "unable to list workflows")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) createRecord(w http.ResponseWriter, r *http.Request) {
	tenant, ok := tenantID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "tenant is required")
		return
	}
	var rec domain.Record
	if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	created, replay, err := s.store.CreateRecord(tenant, r.Header.Get("Idempotency-Key"), rec)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
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
		writeErr(w, http.StatusBadRequest, "tenant is required")
		return
	}
	rec, err := s.store.GetRecord(tenant, r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "record not found")
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

type updateRecordRequest struct {
	State           string         `json:"state"`
	Data            map[string]any `json:"data"`
	ExpectedVersion int64          `json:"expectedVersion"`
}

func (s *Server) updateRecord(w http.ResponseWriter, r *http.Request) {
	tenant, ok := tenantID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "tenant is required")
		return
	}
	var req updateRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if h := r.Header.Get("If-Match"); h != "" {
		if v, err := strconv.ParseInt(strings.Trim(h, "\""), 10, 64); err == nil {
			req.ExpectedVersion = v
		}
	}
	rec, err := s.store.UpdateRecord(tenant, r.PathValue("id"), req.ExpectedVersion, req.State, req.Data)
	if errors.Is(err, store.ErrConflict) {
		writeErr(w, http.StatusConflict, "version conflict")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "record not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.Header().Set("ETag", strconv.FormatInt(rec.Version, 10))
	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	tenant, ok := tenantID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "tenant is required")
		return
	}
	items, err := s.store.ListAuditEvents(tenant)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "unable to list audit events")
		return
	}
	writeJSON(w, http.StatusOK, items)
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
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Tenant-ID, Idempotency-Key, If-Match")
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
