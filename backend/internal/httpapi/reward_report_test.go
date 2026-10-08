package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	rewardReportJSONPath      = "/api/v1/admin/reports/rewards"
	rewardReportCSVPath       = "/api/v1/admin/reports/rewards.csv"
	rewardOrdersReportPath    = "/api/v1/admin/reports/reward-orders"
	rewardOrdersReportCSVPath = "/api/v1/admin/reports/reward-orders.csv"
)

func rewardReportQueryURL(path, from, to, groupBy string) string {
	return reportURL(path, url.Values{"from": {from}, "to": {to}, "group_by": {groupBy}})
}

func rewardReportCall(f managementHTTP, method, path, token, brand, body string) *httptest.ResponseRecorder {
	return rewardRawCall(f, method, path, "", token, brand, "", body)
}

func grantRewardReportPermission(t *testing.T, f managementHTTP, permission string) {
	t.Helper()
	grantReportPermission(t, f, permission)
}

type rewardReportFixture struct {
	pointsHTTPFixture
	grantedID  string
	revokedID  string
	pendingID  string
	reversalAt time.Time
}

func newRewardReportFixture(t *testing.T) rewardReportFixture {
	t.Helper()
	f := rewardFixture(t)
	grantRewardPermission(t, f.managementHTTP, "reward.grant.brand", "reward.revoke.brand")
	granted := rewardCreate(t, f, "reward-report-grant-001", "11")
	revoked := rewardCreate(t, f, "reward-report-revoke-grant-01", "13")
	revoke := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath+"/"+revoked["id"].(string)+"/revoke", "reward-report-revoke-01", f.token, managedBrand, f.root, `{"version":1,"reason":"report fixture reversal"}`)
	if revoke.Code != 200 || !strings.Contains(revoke.Body.String(), `"state":"revoked"`) {
		t.Fatalf("fixture revoke status=%d body=%s", revoke.Code, revoke.Body.String())
	}
	pending := rewardCreate(t, f, "reward-report-pending-grant-01", "17")
	freeze := f.call("POST", "/api/v1/admin/wallets/"+f.memberID+"/freeze", "reward-report-freeze-001", f.token, managedBrand, map[string]string{"points": "28", "reason": "freeze all available gift points for report fixture"})
	mustStatus(t, freeze, 200)
	var freezeLedger struct {
		ID        string `json:"id"`
		EntryType string `json:"entry_type"`
	}
	managedData(t, freeze, &freezeLedger)
	if freezeLedger.ID == "" || freezeLedger.EntryType != "freeze" {
		t.Fatalf("wallet freeze did not persist a normal freeze ledger entry: %+v", freezeLedger)
	}
	pendingRevoke := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath+"/"+pending["id"].(string)+"/revoke", "reward-report-pending-revoke-01", f.token, managedBrand, f.root, `{"version":1,"reason":"report fixture pending reversal"}`)
	if pendingRevoke.Code != 200 || !strings.Contains(pendingRevoke.Body.String(), `"state":"revocation_pending"`) {
		t.Fatalf("fixture pending revoke status=%d body=%s", pendingRevoke.Code, pendingRevoke.Body.String())
	}
	var reversalAt time.Time
	if err := f.pool.QueryRow(context.Background(), `SELECT created_at FROM point_ledger_entries WHERE brand_id=$1 AND entry_type='reward_reversal' AND reference_id=$2`, managedBrand, revoked["id"]).Scan(&reversalAt); err != nil {
		t.Fatal(err)
	}
	var pendingReversals, genericFreezes int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1 AND reference_type='reward_order' AND reference_id=$2 AND entry_type='reward_reversal'`, managedBrand, pending["id"]).Scan(&pendingReversals); err != nil || pendingReversals != 0 {
		t.Fatalf("pending order must not have a reversal posting: count=%d err=%v", pendingReversals, err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM point_ledger_entries WHERE id=$1 AND entry_type='freeze'`, freezeLedger.ID).Scan(&genericFreezes); err != nil || genericFreezes != 1 {
		t.Fatalf("real wallet freeze ledger entry missing: count=%d err=%v", genericFreezes, err)
	}
	return rewardReportFixture{pointsHTTPFixture: f, grantedID: granted["id"].(string), revokedID: revoked["id"].(string), pendingID: pending["id"].(string), reversalAt: reversalAt.UTC()}
}

func TestRewardReportsHTTPRealTotalsStrictQueriesAndCompleteCSV(t *testing.T) {
	f := newRewardReportFixture(t)
	grantRewardReportPermission(t, f.managementHTTP, "report_reward.view.brand")
	grantRewardReportPermission(t, f.managementHTTP, "report_reward.export.brand")
	from, to := reportWindow()
	ctx := context.Background()
	var ledgerBefore, ordersBefore, actionsBefore int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1`, managedBrand).Scan(&ledgerBefore); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM reward_orders WHERE brand_id=$1`, managedBrand).Scan(&ordersBefore); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM reward_order_actions WHERE brand_id=$1`, managedBrand).Scan(&actionsBefore); err != nil {
		t.Fatal(err)
	}

	jsonURL := rewardReportQueryURL(rewardReportJSONPath, from, to, "order") + "&member_id=" + f.memberID
	response := rewardReportCall(f.managementHTTP, "GET", jsonURL, f.token, managedBrand, "")
	mustStatus(t, response, 200)
	var rewardEnvelope struct {
		Data struct {
			BrandID  string `json:"brand_id"`
			Timezone string `json:"timezone"`
			Summary  struct {
				EntryCount         string `json:"entry_count"`
				GrantEntryCount    string `json:"grant_entry_count"`
				GrantPoints        string `json:"grant_points"`
				ReversalEntryCount string `json:"reversal_entry_count"`
				ReversalPoints     string `json:"reversal_points"`
				NetPoints          string `json:"net_points"`
			} `json:"summary"`
			Items []json.RawMessage `json:"items"`
			Query struct {
				GroupBy  string  `json:"group_by"`
				MemberID *string `json:"member_id"`
				Limit    int     `json:"limit"`
				Offset   int     `json:"offset"`
			} `json:"query"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &rewardEnvelope); err != nil {
		t.Fatal(err)
	}
	got := rewardEnvelope.Data
	if got.BrandID != managedBrand || got.Timezone == "" || got.Summary.EntryCount != "4" || got.Summary.GrantEntryCount != "3" || got.Summary.GrantPoints != "41" || got.Summary.ReversalEntryCount != "1" || got.Summary.ReversalPoints != "13" || got.Summary.NetPoints != "28" || len(got.Items) != 3 || got.Query.GroupBy != "order" || got.Query.MemberID == nil || *got.Query.MemberID != f.memberID || got.Query.Limit != 20 || got.Query.Offset != 0 {
		t.Fatalf("wrong real reward posting report: %+v", got)
	}

	ordersURL := rewardReportQueryURL(rewardOrdersReportPath, from, to, "state")
	ordersResponse := rewardReportCall(f.managementHTTP, "GET", ordersURL, f.token, managedBrand, "")
	mustStatus(t, ordersResponse, 200)
	var orderEnvelope struct {
		Data struct {
			Summary struct {
				OrderCount     string `json:"order_count"`
				OriginalPoints string `json:"original_points"`
				GrantedCount   string `json:"granted_count"`
				GrantedPoints  string `json:"granted_points"`
				PendingCount   string `json:"pending_count"`
				PendingPoints  string `json:"pending_points"`
				RevokedCount   string `json:"revoked_count"`
				RevokedPoints  string `json:"revoked_points"`
			} `json:"summary"`
			Items []struct {
				Key    string            `json:"key"`
				Totals map[string]string `json:"totals"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(ordersResponse.Body.Bytes(), &orderEnvelope); err != nil {
		t.Fatal(err)
	}
	os := orderEnvelope.Data.Summary
	if os.OrderCount != "3" || os.OriginalPoints != "41" || os.GrantedCount != "1" || os.GrantedPoints != "11" || os.PendingCount != "1" || os.PendingPoints != "17" || os.RevokedCount != "1" || os.RevokedPoints != "13" {
		t.Fatalf("wrong current order-state report: %+v", os)
	}
	if len(orderEnvelope.Data.Items) != 3 {
		t.Fatalf("state report groups=%+v", orderEnvelope.Data.Items)
	}
	states := map[string]bool{}
	for _, item := range orderEnvelope.Data.Items {
		states[item.Key] = true
	}
	if !states["granted"] || !states["revoked"] || !states["revocation_pending"] {
		t.Fatalf("state groups do not represent granted/revoked/pending: %+v", states)
	}

	// A window containing only the reversal proves posting totals are signed and
	// independent of the order's creation cohort.
	windowFrom := f.reversalAt.Format(time.RFC3339Nano)
	var nextPostingAt time.Time
	if err := f.pool.QueryRow(context.Background(), `SELECT min(created_at) FROM point_ledger_entries WHERE brand_id=$1 AND created_at>$2`, managedBrand, f.reversalAt).Scan(&nextPostingAt); err != nil {
		t.Fatal(err)
	}
	windowTo := nextPostingAt.UTC().Format(time.RFC3339Nano)
	negative := rewardReportCall(f.managementHTTP, "GET", rewardReportQueryURL(rewardReportJSONPath, windowFrom, windowTo, "order"), f.token, managedBrand, "")
	mustStatus(t, negative, 200)
	var negativeEnvelope struct {
		Data struct {
			Summary struct {
				EntryCount     string `json:"entry_count"`
				GrantPoints    string `json:"grant_points"`
				ReversalPoints string `json:"reversal_points"`
				NetPoints      string `json:"net_points"`
			} `json:"summary"`
		} `json:"data"`
	}
	if err := json.Unmarshal(negative.Body.Bytes(), &negativeEnvelope); err != nil {
		t.Fatal(err)
	}
	if negativeEnvelope.Data.Summary.EntryCount != "1" || negativeEnvelope.Data.Summary.GrantPoints != "0" || negativeEnvelope.Data.Summary.ReversalPoints != "13" || negativeEnvelope.Data.Summary.NetPoints != "-13" {
		t.Fatalf("reversal-only window must be negative: %+v", negativeEnvelope.Data.Summary)
	}

	for _, tc := range []struct {
		path, kind string
		headers    []string
	}{
		{rewardReportCSVPath, "rewards", []string{"record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "member_id", "order_id", "key", "label", "entry_count", "grant_entry_count", "grant_points", "reversal_entry_count", "reversal_points", "net_points"}},
		{rewardOrdersReportCSVPath, "reward_orders", []string{"record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "member_id", "order_id", "key", "label", "order_count", "original_points", "granted_count", "granted_points", "pending_count", "pending_points", "revoked_count", "revoked_points"}},
	} {
		csvURL := rewardReportQueryURL(tc.path, from, to, "day") + "&order_id=" + f.grantedID
		out := rewardReportCall(f.managementHTTP, "GET", csvURL, f.token, managedBrand, "")
		mustStatus(t, out, 200)
		h := out.Header()
		if h.Get("Content-Type") != "text/csv; charset=utf-8" || h.Get("Cache-Control") != "no-store" || h.Get("X-Report-Brand-ID") != managedBrand || h.Get("X-Report-Kind") != tc.kind || h.Get("X-Report-Format-Version") != "1" || !uuidPattern.MatchString(h.Get("X-Report-Audit-ID")) || h.Get("X-Report-From") != from || h.Get("X-Report-To") != to || h.Get("X-Report-Group-By") != "day" || h.Get("X-Report-Order-ID") != f.grantedID || h.Get("X-Report-Timezone") == "" || h.Get("Content-Length") != h.Get("X-Report-Byte-Count") || h.Get("Content-Length") != strconv.Itoa(out.Body.Len()) {
			t.Fatalf("incomplete export metadata for %s: %v", tc.kind, h)
		}
		sum := sha256.Sum256(out.Body.Bytes())
		if h.Get("X-Report-SHA256") != hex.EncodeToString(sum[:]) {
			t.Fatalf("wrong %s digest", tc.kind)
		}
		if !strings.HasPrefix(out.Body.String(), "\xef\xbb\xbf") {
			t.Fatalf("%s CSV missing BOM", tc.kind)
		}
		rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(out.Body.String(), "\xef\xbb\xbf"))).ReadAll()
		if err != nil || len(rows) != 3 || strings.Join(rows[0], ",") != strings.Join(tc.headers, ",") || rows[1][0] != "summary" || rows[1][8] != f.grantedID || rows[2][0] != "group" || rows[2][8] != f.grantedID {
			t.Fatalf("%s CSV is not a complete filtered export: rows=%v err=%v", tc.kind, rows, err)
		}
		var audits int
		if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE id=$1 AND actor_id=$2 AND action=$3 AND resource_type='report_export'`, h.Get("X-Report-Audit-ID"), f.root, "report."+tc.kind+".export").Scan(&audits); err != nil || audits != 1 {
			t.Fatalf("%s audit witness missing/duplicated count=%d err=%v", tc.kind, audits, err)
		}
	}

	for _, path := range []string{rewardReportJSONPath, rewardReportCSVPath, rewardOrdersReportPath, rewardOrdersReportCSVPath} {
		base := rewardReportQueryURL(path, from, to, "day")
		for _, suffix := range []string{"&from=" + url.QueryEscape(from), "&unknown=1", "&group_by=day&group_by=state", "&member_id=", "&order_id=bad", "&limit=+20", "&offset=001"} {
			out := rewardReportCall(f.managementHTTP, "GET", base+suffix, f.token, managedBrand, "")
			mustStatus(t, out, 400)
			if strings.Contains(out.Header().Get("Content-Type"), "text/csv") || out.Header().Get("X-Report-Audit-ID") != "" {
				t.Fatalf("invalid query produced export/audit metadata: %s %s", suffix, out.Header())
			}
		}
	}
	for _, path := range []string{rewardReportJSONPath, rewardReportCSVPath, rewardOrdersReportPath, rewardOrdersReportCSVPath} {
		out := rewardReportCall(f.managementHTTP, "GET", rewardReportQueryURL(path, from, to, "day"), f.token, managedBrand, `{"ignored":true}`)
		mustStatus(t, out, 400)
	}
	for _, path := range []string{rewardReportCSVPath, rewardOrdersReportCSVPath} {
		out := rewardReportCall(f.managementHTTP, "GET", rewardReportQueryURL(path, from, to, "day")+"&limit=20", f.token, managedBrand, "")
		mustStatus(t, out, 400)
		if strings.Contains(out.Header().Get("Content-Type"), "text/csv") {
			t.Fatal("pagination query emitted CSV")
		}
	}
	var ledgerAfter, ordersAfter, actionsAfter int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1`, managedBrand).Scan(&ledgerAfter); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM reward_orders WHERE brand_id=$1`, managedBrand).Scan(&ordersAfter); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM reward_order_actions WHERE brand_id=$1`, managedBrand).Scan(&actionsAfter); err != nil {
		t.Fatal(err)
	}
	if ledgerAfter != ledgerBefore || ordersAfter != ordersBefore || actionsAfter != actionsBefore {
		t.Fatalf("report GET changed business records: ledger %d->%d orders %d->%d actions %d->%d", ledgerBefore, ledgerAfter, ordersBefore, ordersAfter, actionsBefore, actionsAfter)
	}
}

func TestRewardReportPermissionIsIndependentAndExplicitlyScoped(t *testing.T) {
	f := rewardFixture(t)
	from, to := reportWindow()
	jsonURL := rewardReportQueryURL(rewardReportJSONPath, from, to, "day")
	csvURL := rewardReportQueryURL(rewardReportCSVPath, from, to, "day")
	grantRewardReportPermission(t, f.managementHTTP, "report_reward.view.brand")
	for _, path := range []string{jsonURL, rewardReportQueryURL(rewardOrdersReportPath, from, to, "state")} {
		mustStatus(t, rewardReportCall(f.managementHTTP, "GET", path, f.token, managedBrand, ""), 200)
	}
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='reward.view.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, rewardReportCall(f.managementHTTP, "GET", jsonURL, f.token, managedBrand, ""), 200)
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='report_reward.view.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, rewardReportCall(f.managementHTTP, "GET", jsonURL, f.token, managedBrand, ""), 403)
	grantRewardReportPermission(t, f.managementHTTP, "reward.view.brand")
	mustStatus(t, rewardReportCall(f.managementHTTP, "GET", jsonURL, f.token, managedBrand, ""), 403)
	grantRewardReportPermission(t, f.managementHTTP, "report_reward.view.brand")
	mustStatus(t, rewardReportCall(f.managementHTTP, "GET", jsonURL, f.token, pointsBrandB, ""), 403)
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, rewardReportCall(f.managementHTTP, "GET", jsonURL, f.token, pointsBrandB, ""), 403)
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=false WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	grantRewardReportPermission(t, f.managementHTTP, "report_reward.export.brand")
	mustStatus(t, rewardReportCall(f.managementHTTP, "GET", csvURL, f.token, managedBrand, ""), 200)
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='report_reward.export.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, rewardReportCall(f.managementHTTP, "GET", csvURL, f.token, managedBrand, ""), 403)
	grantRewardReportPermission(t, f.managementHTTP, "report_reward.view.platform")
	grantRewardReportPermission(t, f.managementHTTP, "report_reward.export.platform")
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key IN('report_reward.view.brand','report_reward.export.brand')`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, rewardReportCall(f.managementHTTP, "GET", jsonURL, f.token, pointsBrandB, ""), 200)
	mustStatus(t, rewardReportCall(f.managementHTTP, "GET", rewardReportQueryURL(rewardReportCSVPath, from, to, "day"), f.token, pointsBrandB, ""), 200)
}

func TestRewardReportStrictRFC3339AndNanosecondUTCEcho(t *testing.T) {
	f := managedFixture(t)
	grantRewardReportPermission(t, f, "report_reward.view.brand")
	grantRewardReportPermission(t, f, "report_reward.export.brand")
	goodTo := "2026-10-08T10:11:13.987654321+08:00"
	for _, invalidFrom := range []string{
		"2026-10-08 10:11:12Z",            // RFC3339 requires T.
		"2026-10-08T10:11Z",               // seconds are required.
		"2026-10-08T10:11:12,123456789Z",  // Go accepts comma fractions; RFC3339 does not.
		"2026-10-08T10:11:12.1234567890Z", // Go truncates over-precision; reject it.
		"2026-10-08T10:11:12.Z",           // A decimal point must have 1-9 digits.
		"2026-10-08T10:11:12.123456789",   // A timezone is required.
	} {
		for _, tc := range []struct{ path, groupBy string }{
			{rewardReportJSONPath, "day"},
			{rewardReportCSVPath, "day"},
			{rewardOrdersReportPath, "state"},
			{rewardOrdersReportCSVPath, "state"},
		} {
			out := rewardReportCall(f, "GET", rewardReportQueryURL(tc.path, invalidFrom, goodTo, tc.groupBy), f.token, managedBrand, "")
			mustStatus(t, out, 400)
			if strings.Contains(out.Header().Get("Content-Type"), "text/csv") || out.Header().Get("X-Report-Audit-ID") != "" {
				t.Fatalf("invalid timestamp emitted report data/metadata: path=%s from=%q headers=%v", tc.path, invalidFrom, out.Header())
			}
		}
	}

	from, to := "2026-10-08T10:11:12.123456789+08:00", goodTo
	wantFrom, wantTo := "2026-10-08T02:11:12.123456789Z", "2026-10-08T02:11:13.987654321Z"
	jsonResponse := rewardReportCall(f, "GET", rewardReportQueryURL(rewardReportJSONPath, from, to, "day"), f.token, managedBrand, "")
	mustStatus(t, jsonResponse, 200)
	var envelope struct {
		Data struct {
			Query struct {
				From time.Time `json:"from"`
				To   time.Time `json:"to"`
			} `json:"query"`
		} `json:"data"`
	}
	if err := json.Unmarshal(jsonResponse.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Query.From.Format(time.RFC3339Nano) != wantFrom || envelope.Data.Query.To.Format(time.RFC3339Nano) != wantTo || envelope.Data.Query.From.Nanosecond() != 123456789 || envelope.Data.Query.To.Nanosecond() != 987654321 {
		t.Fatalf("JSON query did not preserve nanoseconds and normalize to UTC: from=%s to=%s", envelope.Data.Query.From.Format(time.RFC3339Nano), envelope.Data.Query.To.Format(time.RFC3339Nano))
	}
	csvResponse := rewardReportCall(f, "GET", rewardReportQueryURL(rewardReportCSVPath, from, to, "day"), f.token, managedBrand, "")
	mustStatus(t, csvResponse, 200)
	if csvResponse.Header().Get("X-Report-From") != wantFrom || csvResponse.Header().Get("X-Report-To") != wantTo {
		t.Fatalf("CSV query headers did not preserve nanoseconds and normalize to UTC: from=%q to=%q", csvResponse.Header().Get("X-Report-From"), csvResponse.Header().Get("X-Report-To"))
	}
}
