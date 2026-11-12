package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/dataretention"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
)

func retentionRequest(f managementHTTP, method, path, key, actor, brand, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	r.RemoteAddr = "198.51.100.17:23456"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+f.token)
	r.Header.Set("Idempotency-Key", key)
	r.Header.Set("X-Data-Retention-Actor-ID", actor)
	if brand != "" {
		r.Header.Set("X-Brand-ID", brand)
	}
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	return w
}

func TestDataRetentionPolicyAndOriginalReceipts(t *testing.T) {
	base := managedFixture(t)
	op := securityOperator(t, base)
	grantPlatformPermission(t, op, "platform_settings.view.platform", "platform_settings.write.platform")
	path := "/api/v1/platform/data-retention-policy"
	mustStatus(t, retentionRequest(base, "GET", path, "", base.root, "", ""), 403)
	mustStatus(t, retentionRequest(op, "GET", path, "", op.root, managedBrand, ""), 400)
	read := retentionRequest(op, "GET", path, "", op.root, "", "")
	mustStatus(t, read, 200)
	var initial dataretention.Policy
	managedData(t, read, &initial)
	if initial.Enabled || initial.NextRun != nil || initial.Version != 1 {
		t.Fatal(initial)
	}
	for _, body := range []string{`{"version":1,"enabled":true,"execution_day":29,"execution_time":"02:00"}`, `{"version":1,"enabled":true,"execution_day":0,"execution_time":"02:00"}`, `{"version":1,"enabled":true,"execution_day":10,"execution_time":"2:00"}`, `{"version":1,"enabled":null,"execution_day":10,"execution_time":"02:00"}`, `{"version":1,"enabled":true,"execution_day":10}`, `{"version":1,"enabled":true,"execution_day":10,"execution_time":"02:00","daily_enabled":true}`} {
		mustStatus(t, retentionRequest(op, "PUT", path, ids.New(), op.root, "", body), 400)
	}
	body := `{"version":1,"enabled":true,"execution_day":10,"execution_time":"02:00"}`
	key := ids.New()
	accepted := retentionRequest(op, "PUT", path, key, op.root, "", body)
	mustStatus(t, accepted, 200)
	var original dataretention.Policy
	managedData(t, accepted, &original)
	if !original.Enabled || original.Version != 2 || original.NextRun == nil || !original.NextRun.ExecuteAt.After(time.Now()) {
		t.Fatal(original)
	}
	replay := retentionRequest(op, "PUT", path, key, op.root, "", body)
	mustStatus(t, replay, 200)
	var ack dataretention.Policy
	managedData(t, replay, &ack)
	a, _ := json.Marshal(original)
	b, _ := json.Marshal(ack)
	if string(a) != string(b) {
		t.Fatal("original receipt changed")
	}
	changed := `{"version":2,"enabled":true,"execution_day":20,"execution_time":"04:15"}`
	mustStatus(t, retentionRequest(op, "PUT", path, ids.New(), op.root, "", changed), 200)
	replay = retentionRequest(op, "PUT", path, key, op.root, "", body)
	mustStatus(t, replay, 200)
	managedData(t, replay, &ack)
	if ack.Version != 2 || ack.ExecutionDay != 10 {
		t.Fatal("replay became latest state", ack)
	}
	mustStatus(t, retentionRequest(op, "PUT", path, ids.New(), op.root, "", body), 409)
	mustStatus(t, retentionRequest(op, "PUT", path, ids.New(), base.root, "", changed), 401)
	mustStatus(t, retentionRequest(op, "PUT", path+"?unknown=1", ids.New(), op.root, "", changed), 400)
}

func TestDataRetentionTasksScopeRetryAndRetiredRoutes(t *testing.T) {
	base := managedFixture(t)
	op := securityOperator(t, base)
	grantPlatformPermission(t, op, "platform_settings.view.platform", "platform_settings.write.platform")
	ctx := context.Background()
	brand, task := ids.New(), ids.New()
	if _, err := op.pool.Exec(ctx, `INSERT INTO brands(id,code,name,status) VALUES($1,$2,'Retention API isolated test','active')`, brand, "retention_"+brand); err != nil {
		t.Fatal(err)
	}
	if _, err := op.pool.Exec(ctx, `INSERT INTO data_retention_tasks(id,brand_id,policy_version,timezone,execute_at,cutoff_at,state,error_code) VALUES($1,$2,1,'UTC','2026-11-10T02:00:00Z','2026-10-01T00:00:00Z','failed','DATA_RETENTION_BATCH_FAILED')`, task, brand); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/platform/data-retention-tasks"
	res := retentionRequest(op, "GET", path+"?brand_id="+brand+"&limit=20&offset=0", "", op.root, "", "")
	mustStatus(t, res, 200)
	var page dataretention.Page
	managedData(t, res, &page)
	if page.TotalCount != "1" || len(page.Items) != 1 || page.Items[0].BrandName != "Retention API isolated test" {
		t.Fatal(page)
	}
	res = retentionRequest(op, "GET", path+"?brand_id="+managedBrand, "", op.root, "", "")
	mustStatus(t, res, 200)
	managedData(t, res, &page)
	if page.TotalCount != "0" {
		t.Fatal("cross-brand task leaked", page)
	}
	mustStatus(t, retentionRequest(base, "GET", path, "", base.root, "", ""), 403)
	key := ids.New()
	mustStatus(t, retentionRequest(op, "POST", path+"/"+task+"/retry", key, op.root, "", "{}"), 200)
	mustStatus(t, retentionRequest(op, "POST", path+"/"+task+"/retry", key, op.root, "", "{}"), 200)
	mustStatus(t, retentionRequest(op, "POST", path+"/"+task+"/retry", ids.New(), op.root, "", "{}"), 409)
	for _, retired := range []string{"/api/v1/platform/report-archive-policy", "/api/v1/platform/report-archives", "/api/v1/admin/report-archive-tasks"} {
		mustStatus(t, retentionRequest(op, "GET", retired, "", op.root, "", ""), 404)
	}
}
