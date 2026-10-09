package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
)

func grantDrawPermissions(t *testing.T, f managementHTTP) {
	t.Helper()
	ctx := context.Background()
	for _, key := range []string{"draw_source.view.brand", "draw_source.write.brand", "draw.view.brand", "draw.manual_create.brand"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, key); err != nil {
			t.Fatal(err)
		}
	}
}

func waitingDrawPeriod(t *testing.T, f managementHTTP, game rulebook.Game) (string, int64) {
	t.Helper()
	ctx := context.Background()
	periodID := ids.New()
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := f.pool.Exec(ctx, `INSERT INTO periods(id,brand_id,game_id,period_no,sequence,bet_start_at,bet_end_at,draw_at,status,version) VALUES($1,$2,$3,'manual-001',1,$4,$5,$6,'pending',1)`, periodID, managedBrand, game.ID, now.Add(-3*time.Hour), now.Add(-2*time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	for i, status := range []string{"betting", "closed", "waiting_draw"} {
		if _, err := f.pool.Exec(ctx, `UPDATE periods SET status=$2,version=version+1,state_reason=$3 WHERE id=$1`, periodID, status, "test transition "+status); err != nil {
			t.Fatalf("period transition %d (%s): %v", i, status, err)
		}
	}
	return periodID, 4
}

func validDrawSource() map[string]any {
	return map[string]any{"id": ids.New(), "name": "Official result", "type": "api", "priority": 1, "enabled": true, "endpoint": "https://results.example.org/draw", "selector": "", "credential_ref": ""}
}

func TestDrawSourceHTTPReadWriteReplayAndConflicts(t *testing.T) {
	f, game := periodFixture(t)
	grantDrawPermissions(t, f)
	base := "/api/v1/admin/games/" + game.ID + "/draw-sources"
	mustStatus(t, f.call("GET", base, "", f.token, managedBrand, nil), 404)
	body := map[string]any{"version": game.Version, "sources": []any{validDrawSource()}, "reason": "configure official source"}
	r := f.call("PUT", base, "draw-source-save-001", f.token, managedBrand, body)
	mustStatus(t, r, 200)
	var saved struct {
		ID      string `json:"id"`
		Sources []any  `json:"sources"`
	}
	managedData(t, r, &saved)
	if saved.ID == "" || len(saved.Sources) != 1 {
		t.Fatal(saved)
	}
	replay := f.call("PUT", base, "draw-source-save-001", f.token, managedBrand, body)
	mustStatus(t, replay, 200)
	var repeated struct {
		ID string `json:"id"`
	}
	managedData(t, replay, &repeated)
	if repeated.ID != saved.ID {
		t.Fatal("source set replay created another revision", saved.ID, repeated.ID)
	}
	changed := map[string]any{"version": game.Version, "sources": []any{validDrawSource()}, "reason": "changed body"}
	mustStatus(t, f.call("PUT", base, "draw-source-save-001", f.token, managedBrand, changed), 409)
	stale := f.call("PUT", base, "draw-source-stale-version", f.token, managedBrand, body)
	mustStatus(t, stale, 409)
	var failureBody struct {
		Error apiError `json:"error"`
	}
	if err := json.Unmarshal(stale.Body.Bytes(), &failureBody); err != nil || failureBody.Error.Code != "DRAW_VERSION_CONFLICT" {
		t.Fatal(stale.Body.String(), err)
	}
	invalid := map[string]any{"version": game.Version + 1, "sources": []any{map[string]any{"id": ids.New(), "name": "bad", "type": "api", "priority": 1, "enabled": true, "endpoint": "http://127.0.0.1/private", "selector": "", "credential_ref": ""}}, "reason": "reject unsafe endpoint"}
	mustStatus(t, f.call("PUT", base, "draw-source-invalid-config", f.token, managedBrand, invalid), 400)
	get := f.call("GET", base, "", f.token, managedBrand, nil)
	mustStatus(t, get, 200)
	var readback struct {
		ID string `json:"id"`
	}
	managedData(t, get, &readback)
	if readback.ID != saved.ID {
		t.Fatal("invalid or stale update changed source set", readback.ID)
	}
	var audits int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action='draw_source.write'`).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("expected one source write audit", audits, err)
	}
}

func TestManualDrawHTTPReplayValidationAndReadHistory(t *testing.T) {
	f, game := periodFixture(t)
	grantDrawPermissions(t, f)
	periodID, version := waitingDrawPeriod(t, f, game)
	base := "/api/v1/admin/periods/" + periodID
	drawnAt := time.Now().UTC().Truncate(time.Second)
	body := map[string]any{"version": version, "period_no": "manual-001", "result": rules.Draw{Digits: []int{1, 2, 3}}, "drawn_at": drawnAt, "reason": "operator confirmed result"}
	badResult := map[string]any{"version": version, "period_no": "manual-001", "result": rules.Draw{Digits: []int{1, 2, 10}}, "drawn_at": drawnAt, "reason": "invalid digits"}
	mustStatus(t, f.call("POST", base+"/manual-draw", "manual-draw-invalid-result", f.token, managedBrand, badResult), 400)
	r := f.call("POST", base+"/manual-draw", "manual-draw-once", f.token, managedBrand, body)
	mustStatus(t, r, 201)
	var saved struct {
		ID      string    `json:"id"`
		Kind    string    `json:"kind"`
		DrawnAt time.Time `json:"drawn_at"`
	}
	managedData(t, r, &saved)
	if saved.ID == "" || saved.Kind != "manual" || !saved.DrawnAt.Equal(drawnAt) {
		t.Fatal(saved)
	}
	replay := f.call("POST", base+"/manual-draw", "manual-draw-once", f.token, managedBrand, body)
	mustStatus(t, replay, 201)
	var repeated struct {
		ID string `json:"id"`
	}
	managedData(t, replay, &repeated)
	if repeated.ID != saved.ID {
		t.Fatal("manual draw replay returned another result", saved.ID, repeated.ID)
	}
	changed := map[string]any{"version": version, "period_no": "manual-001", "result": rules.Draw{Digits: []int{1, 2, 4}}, "drawn_at": drawnAt, "reason": "changed result"}
	mustStatus(t, f.call("POST", base+"/manual-draw", "manual-draw-once", f.token, managedBrand, changed), 409)
	stale := map[string]any{"version": version - 1, "period_no": "manual-001", "result": rules.Draw{Digits: []int{1, 2, 4}}, "drawn_at": drawnAt, "reason": "stale period"}
	r = f.call("POST", base+"/manual-draw", "manual-draw-stale", f.token, managedBrand, stale)
	mustStatus(t, r, 409)
	var staleFailure struct {
		Error apiError `json:"error"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &staleFailure); err != nil || staleFailure.Error.Code != "DRAW_VERSION_CONFLICT" {
		t.Fatal(r.Body.String(), err)
	}
	badTime := `{"version":4,"period_no":"manual-001","result":{"digits":[1,2,3]},"drawn_at":"not-a-time","reason":"bad timestamp"}`
	mustStatus(t, f.rawCall("POST", base+"/manual-draw", "manual-draw-invalid-time", f.token, managedBrand, badTime), 400)
	history := f.call("GET", base+"/draw?limit=10&offset=0", "", f.token, managedBrand, nil)
	mustStatus(t, history, 200)
	var out struct {
		Current *struct {
			ID string `json:"id"`
		} `json:"current"`
		History  []any `json:"history"`
		Attempts []any `json:"attempts"`
		Limit    int   `json:"limit"`
		Offset   int   `json:"offset"`
	}
	managedData(t, history, &out)
	if out.Current == nil || out.Current.ID != saved.ID || len(out.History) != 1 || out.Limit != 10 || out.Offset != 0 {
		t.Fatal(out)
	}
	if r = f.call("GET", base+"/draw?limit=101", "", f.token, managedBrand, nil); r.Code != 400 {
		t.Fatal("invalid draw history pagination accepted", r.Code)
	}
	var results, audits int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM draw_results WHERE period_id=$1`, periodID).Scan(&results); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action='draw.lock'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if results != 1 || audits != 1 {
		t.Fatal("manual draw replay duplicated durable effects", results, audits)
	}
}

func TestDrawHTTPPermissionsBrandOriginAndSession(t *testing.T) {
	f, game := periodFixture(t)
	grantDrawPermissions(t, f)
	sourcePath := "/api/v1/admin/games/" + game.ID + "/draw-sources"
	periodID, version := waitingDrawPeriod(t, f, game)
	drawPath := "/api/v1/admin/periods/" + periodID + "/manual-draw"
	mustStatus(t, f.call("GET", sourcePath, "", "", managedBrand, nil), 401)
	mustStatus(t, f.call("GET", sourcePath, "", f.token, pointsBrandB, nil), 403)
	mustStatus(t, f.call("GET", "/api/v1/admin/periods/"+ids.New()+"/draw", "", f.token, managedBrand, nil), 404)
	body := map[string]any{"version": version, "period_no": "manual-001", "result": rules.Draw{Digits: []int{1, 2, 3}}, "drawn_at": time.Now().UTC(), "reason": "permission test"}
	mustStatus(t, pointsCall(f, "POST", drawPath, "draw-forged-origin", f.token, managedBrand, "https://evil.invalid", body), 403)
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", sourcePath, "", f.token, managedBrand, nil), 403)
	platformToken := platformAdminToken(t, f)
	platformSourcePath := strings.Replace(sourcePath, "/api/v1/admin", "/api/v1/platform", 1)
	mustStatus(t, f.call("GET", platformSourcePath, "", platformToken, managedBrand, nil), 403)
	grantPlatformPermission(t, f, "draw_source.view.platform")
	mustStatus(t, f.call("GET", platformSourcePath, "", platformToken, managedBrand, nil), 404)
	mustStatus(t, f.call("POST", drawPath, "draw-super-readonly", f.token, managedBrand, body), 403)
	var results int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM draw_results WHERE period_id=$1`, periodID).Scan(&results); err != nil || results != 0 {
		t.Fatal("super admin wrote a manual result", results, err)
	}
}

func (f managementHTTP) rawCall(method, path, key, token, brand, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	r.RemoteAddr = "192.0.2.55:12345"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", key)
	r.Header.Set("X-Brand-ID", brand)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	return w
}
