package httpapi

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPlatformAuditListOmitsSnapshotsAndKeepsScope(t *testing.T) {
	f := managedFixture(t)
	id := insertAuditExportFact(t, f, managedBrand, "fixture.audit.summary", "summary record")
	insertAuditExportFact(t, f, "0199a000-0000-7000-8000-000000000002", "fixture.audit.summary", "foreign record")
	brandRead := f.call("GET", "/api/v1/admin/audit?action=fixture.audit.summary", "", f.token, managedBrand, nil)
	mustStatus(t, brandRead, 200)
	var brand struct {
		Items []auditItem `json:"items"`
	}
	managedData(t, brandRead, &brand)
	if len(brand.Items) != 1 || string(brand.Items[0].Before) != `{"status":"normal"}` || string(brand.Items[0].After) != `{"status":"frozen"}` {
		t.Fatal("brand change details missing", brandRead.Body.String())
	}
	grantPlatformPermission(t, f, "audit.view.platform")
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	token := platformAdminToken(t, f)
	response := f.call("GET", "/api/v1/platform/audit?action=fixture.audit.summary", "", token, managedBrand, nil)
	mustStatus(t, response, 200)
	var platform struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	managedData(t, response, &platform)
	if len(platform.Items) != 1 || string(platform.Items[0]["id"]) != `"`+id+`"` {
		t.Fatal("platform summary scope", response.Body.String())
	}
	if _, ok := platform.Items[0]["before_json"]; ok {
		t.Fatal("platform list contains before snapshot")
	}
	if _, ok := platform.Items[0]["after_json"]; ok {
		t.Fatal("platform list contains after snapshot")
	}
	var rawBefore, rawAfter []byte
	if err := f.pool.QueryRow(context.Background(), `SELECT before_json,after_json FROM audit_logs WHERE id=$1`, id).Scan(&rawBefore, &rawAfter); err != nil || len(rawBefore) == 0 || len(rawAfter) == 0 {
		t.Fatal("stored evidence changed", err)
	}
}
