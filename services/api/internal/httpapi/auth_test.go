package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/auth"
	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/store"
)

const authTestSecret = "01234567890123456789012345678901"

func newAuthenticatedHandler(t *testing.T) (http.Handler, *auth.Manager) {
	t.Helper()
	manager, err := auth.NewManager(authTestSecret)
	if err != nil {
		t.Fatal(err)
	}
	return NewWithAuth(store.NewMemoryStore(), manager), manager
}

func issueToken(t *testing.T, manager *auth.Manager, tenant string, role auth.Role) string {
	t.Helper()
	token, err := manager.Issue("user-1", tenant, role, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestAuthenticationRequired(t *testing.T) {
	h, _ := newAuthenticatedHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.Code)
	}
}

func TestBuilderCanCreateWorkflow(t *testing.T) {
	h, manager := newAuthenticatedHandler(t)
	token := issueToken(t, manager, "tenant-a", auth.RoleBuilder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workflows", bytes.NewBufferString(`{"name":"Equipment Request","states":["SUBMITTED","APPROVED"]}`))
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestRequesterCannotCreateWorkflow(t *testing.T) {
	h, manager := newAuthenticatedHandler(t)
	token := issueToken(t, manager, "tenant-a", auth.RoleRequester)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workflows", bytes.NewBufferString(`{"name":"Equipment Request","states":["SUBMITTED"]}`))
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", res.Code)
	}
}

func TestTenantHeaderCannotOverrideTokenTenant(t *testing.T) {
	h, manager := newAuthenticatedHandler(t)
	token := issueToken(t, manager, "tenant-a", auth.RoleBuilder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Tenant-ID", "tenant-b")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", res.Code)
	}
}

func TestAuditorCanReadAuditButCannotUpdateRecord(t *testing.T) {
	h, manager := newAuthenticatedHandler(t)
	token := issueToken(t, manager, "tenant-a", auth.RoleAuditor)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-events", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected audit read 200, got %d", res.Code)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/v1/records/rec_1", bytes.NewBufferString(`{"state":"APPROVED","expectedVersion":1}`))
	req.Header.Set("Authorization", "Bearer "+token)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected update 403, got %d", res.Code)
	}
}
