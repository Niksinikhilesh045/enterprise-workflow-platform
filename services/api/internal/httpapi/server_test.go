package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/store"
)

func TestEndToEndWorkflow(t *testing.T) {
	h := New(store.NewMemoryStore())
	wfBody := bytes.NewBufferString(`{"name":"Equipment Request","states":["SUBMITTED","APPROVED"],"fields":[{"key":"device","label":"Device","type":"text","required":true}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workflows", wfBody)
	req.Header.Set("X-Tenant-ID", "acme")
	res := httptest.NewRecorder(); h.ServeHTTP(res, req)
	if res.Code != 201 { t.Fatalf("workflow create status=%d body=%s", res.Code, res.Body.String()) }
	var wf map[string]any; _ = json.Unmarshal(res.Body.Bytes(), &wf)
	workflowID := wf["id"].(string)

	recordBody := bytes.NewBufferString(`{"workflowId":"`+workflowID+`","data":{"device":"MacBook Pro"}}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/records", recordBody)
	req.Header.Set("X-Tenant-ID", "acme"); req.Header.Set("Idempotency-Key", "demo-1")
	res = httptest.NewRecorder(); h.ServeHTTP(res, req)
	if res.Code != 201 { t.Fatalf("record create status=%d body=%s", res.Code, res.Body.String()) }
}
