package httpapi

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
)

const commissionCyclesPath = "/api/v1/admin/commission-cycles"

func grantCommissionCycle(t *testing.T, f managementHTTP, actions ...string) {
	t.Helper()
	ctx := context.Background()
	for _, action := range actions {
		key := "commission." + action + ".brand"
		if _, err := f.pool.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, key); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCommissionCyclesHTTPReadAuthorizationPaginationAndRevocation(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	path := commissionCyclesPath
	if got := f.call("GET", path, "", "", managedBrand, nil); got.Code != 401 {
		t.Fatalf("missing session status=%d body=%s", got.Code, got.Body.String())
	}
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	grantCommissionCycle(t, f, "view")
	for _, query := range []string{"?limit=0", "?limit=101", "?offset=1000001", "?limit=20&limit=30", "?other=1", "?offset="} {
		mustStatus(t, f.call("GET", path+query, "", f.token, managedBrand, nil), 400)
	}
	pageResponse := f.call("GET", path, "", f.token, managedBrand, nil)
	mustStatus(t, pageResponse, 200)
	var page commission.CycleListPage
	managedData(t, pageResponse, &page)
	if page.BrandID != managedBrand || page.Limit != 20 || page.Offset != 0 || page.TotalCount != "0" || len(page.Items) != 0 || page.Items == nil {
		t.Fatalf("expected a real empty cycle page, got %+v", page)
	}
	var cycles int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM commission_cycles WHERE brand_id=$1`, managedBrand).Scan(&cycles); err != nil || cycles != 0 {
		t.Fatalf("empty read created a cycle: count=%d err=%v", cycles, err)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN (SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='commission.view.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	grantPlatformPermission(t, f, "commission.view.platform")
	// Platform view grants visibility, but never substitutes for a brand-scoped run.
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	mustStatus(t, f.call("POST", path, "cycle-platform-run-denied-01", f.token, managedBrand, map[string]string{
		"anchor_order_id": "0199a000-0000-7000-8000-000000000099", "reason": "platform scope cannot run",
	}), 403)
	if _, err := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	platformToken := platformAdminToken(t, f)
	mustStatus(t, f.call("GET", "/api/v1/platform/commission-cycles", "", platformToken, managedBrand, nil), 200)
	mustStatus(t, f.call("POST", "/api/v1/platform/commission-cycles", "cycle-platform-entry-run-denied-01", platformToken, managedBrand, map[string]string{
		"anchor_order_id": "0199a000-0000-7000-8000-000000000099", "reason": "platform entry has no run route",
	}), 404)
}

func TestCommissionCyclesHTTPDetailAndCreateNotFoundAndClosedBodies(t *testing.T) {
	f := managedFixture(t)
	grantCommissionCycle(t, f, "view", "run", "retry")
	missing := "0199a000-0000-7000-8000-000000000099"
	mustStatus(t, f.call("GET", commissionCyclesPath+"/"+missing, "", f.token, managedBrand, nil), 404)
	mustStatus(t, f.call("GET", commissionCyclesPath+"/"+missing+"?limit=20", "", f.token, managedBrand, nil), 400)
	for _, body := range []string{
		`{"anchor_order_id":"` + missing + `","reason":"valid reason","extra":true}`,
		`{"anchor_order_id":"` + missing + `","anchor_order_id":"` + missing + `","reason":"valid reason"}`,
		`{"anchor_order_id":"` + missing + `","reason":""}`,
	} {
		mustStatus(t, f.rawCall("POST", commissionCyclesPath, "cycle-invalid-body-001", f.token, managedBrand, body), 400)
	}
	mustStatus(t, f.call("POST", commissionCyclesPath, "cycle-nonexistent-anchor-01", f.token, managedBrand, map[string]string{
		"anchor_order_id": missing, "reason": "nonexistent real anchor",
	}), 404)
	mustStatus(t, f.call("POST", commissionCyclesPath+"?limit=20", "cycle-query-rejected-01", f.token, managedBrand, map[string]string{
		"anchor_order_id": missing, "reason": "query is forbidden",
	}), 400)
}

func TestCommissionCyclesHTTPRejectsOrderWithoutEnabledFinancialPolicy(t *testing.T) {
	// settlementHTTPFixture creates and settles a genuine order using the normal
	// betting path. Its saved financial commission policy is disabled, so it is
	// a real anchor rejection, not fabricated future evidence.
	f, order := settlementHTTPFixture(t)
	grantCommissionCycle(t, f.managementHTTP, "view", "run", "retry")
	response := f.call("POST", commissionCyclesPath, "cycle-disabled-anchor-001", f.token, managedBrand, map[string]string{
		"anchor_order_id": order.ID, "reason": "reject genuine order with disabled commission policy",
	})
	mustStatus(t, response, 409)
	var envelope struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error == nil || envelope.Error.Code != "COMMISSION_CYCLE_STATE_CONFLICT" {
		t.Fatalf("unexpected disabled-policy rejection: %s", response.Body.String())
	}
	var cycles int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM commission_cycles WHERE brand_id=$1`, managedBrand).Scan(&cycles); err != nil || cycles != 0 {
		t.Fatalf("rejected anchor created a cycle: count=%d err=%v", cycles, err)
	}
}

func TestCommissionCyclesHTTPCreateReceiptSurvivesWorkerAndAuthRevocation(t *testing.T) {
	var boundary time.Time
	f, order := settlementHTTPFixtureBeforeBet(t, func(f pointsHTTPFixture) {
		grantCommissionCycle(t, f.managementHTTP, "view", "run", "retry")
		agentGrantRoot(t, f.managementHTTP)
		if got := f.call("PUT", "/api/v1/admin/agent-policy", "cycle-http-agent-policy-01", f.token, managedBrand, agentPolicyBody(true, 3, "0.1")); got.Code != 200 {
			t.Fatalf("enable genuine agency policy: %d %s", got.Code, got.Body.String())
		}
		grantCommissionPolicy(t, f.managementHTTP)
		boundary = time.Now().UTC().Add(4 * time.Second).Truncate(time.Second)
		weekday := int(boundary.Weekday())
		config := enabledCommissionPolicyConfig()
		config.Calendar.Weekday = &weekday
		config.Calendar.BoundaryTime = boundary.Format("15:04:05")
		currentResponse := f.call("GET", "/api/v1/admin/commission-policy", "", f.token, managedBrand, nil)
		mustStatus(t, currentResponse, 200)
		var current commission.Policy
		managedData(t, currentResponse, &current)
		updated := f.call("PUT", "/api/v1/admin/commission-policy", "cycle-http-financial-policy-01", f.token, managedBrand,
			commission.PolicyInput{Version: current.Version, Config: config, Reason: "enable genuine weekly cycle fixture"})
		mustStatus(t, updated, 200)
	})
	if !order.PlacedAt.Before(boundary) {
		t.Fatalf("real anchor was placed outside the configured window: placed=%s boundary=%s", order.PlacedAt, boundary)
	}
	if wait := time.Until(boundary); wait > 0 {
		time.Sleep(wait)
	}

	body := `{"anchor_order_id":"` + order.ID + `","reason":"create genuine enabled closed cycle"}`
	first := f.rawCall("POST", commissionCyclesPath, "cycle-http-create-receipt-01", f.token, managedBrand, body)
	mustStatus(t, first, 201)
	var created commission.Cycle
	managedData(t, first, &created)
	if created.State != "enumerating" || created.Version != 1 || created.AnchorOrderID != order.ID || created.CreationAuditLogID == "" {
		t.Fatalf("unexpected original create receipt: %+v", created)
	}

	service := commission.Service{DB: f.pool}
	committed, err := service.ProcessCycles(context.Background(), 1)
	if err != nil || committed != 1 {
		t.Fatalf("advance real cycle worker once: committed=%d err=%v", committed, err)
	}
	var state string
	if err := f.pool.QueryRow(context.Background(), `SELECT state FROM commission_cycles WHERE brand_id=$1 AND id=$2`, managedBrand, created.ID).Scan(&state); err != nil || state != "waiting" {
		t.Fatalf("worker did not advance the created cycle: state=%q err=%v", state, err)
	}

	replay := f.rawCall("POST", commissionCyclesPath, "cycle-http-create-receipt-01", f.token, managedBrand, body)
	mustStatus(t, replay, 201)
	var replayed commission.Cycle
	managedData(t, replay, &replayed)
	if replayed.ID != created.ID || replayed.State != "enumerating" || replayed.Version != 1 || replayed.CreationAuditLogID != created.CreationAuditLogID {
		t.Fatalf("replay did not preserve original create receipt: replay=%+v original=%+v", replayed, created)
	}
	if changed := f.rawCall("POST", commissionCyclesPath, "cycle-http-create-receipt-01", f.token, managedBrand, "{ "+body[1:]); changed.Code != 409 {
		t.Fatalf("byte-different request under same key status=%d body=%s", changed.Code, changed.Body.String())
	}

	runsResponse := f.call("GET", commissionCyclesPath+"/"+created.ID+"/runs", "", f.token, managedBrand, nil)
	mustStatus(t, runsResponse, 200)
	var runs commission.RunPage
	managedData(t, runsResponse, &runs)
	if runs.CycleID != created.ID || runs.TotalCount != "0" || len(runs.Items) != 0 || runs.Items == nil {
		t.Fatalf("unexpected real pre-run history: %+v", runs)
	}
	mustStatus(t, f.call("GET", commissionCyclesPath+"/"+created.ID+"/runs?limit=20&limit=30", "", f.token, managedBrand, nil), 400)
	literalRunID := "0199a000-0000-7000-8000-000000000099"
	mustStatus(t, f.call("GET", commissionCyclesPath+"/"+created.ID+"/runs/"+literalRunID+"/calculations", "", f.token, managedBrand, nil), 404)
	mustStatus(t, f.call("GET", commissionCyclesPath+"/"+created.ID+"/runs/not-a-uuid/calculations", "", f.token, managedBrand, nil), 400)
	mustStatus(t, f.call("GET", commissionCyclesPath+"/"+created.ID+"/runs/"+literalRunID+"/calculations?unexpected=1", "", f.token, managedBrand, nil), 400)

	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id IN (SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='commission.run.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.rawCall("POST", commissionCyclesPath, "cycle-http-create-receipt-01", f.token, managedBrand, body), 403)
	var createAudits int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action='commission.cycle.create' AND resource_id=$2`, managedBrand, created.ID).Scan(&createAudits); err != nil || createAudits != 1 {
		t.Fatalf("replays changed creation audit count: count=%d err=%v", createAudits, err)
	}
}
