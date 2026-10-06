package httpapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
)

func TestPublicDrawHTTPUsesRealCurrentResultWithoutLoginOrMoneyWrites(t *testing.T) {
	f, g := periodFixture(t)
	grantDrawPermissions(t, f)
	id, v := waitingDrawPeriod(t, f, g)
	base := "/api/v1/admin/periods/" + id
	r := f.call("POST", base+"/manual-draw", "public-draw-real-fixture", f.token, managedBrand, map[string]any{"version": v, "period_no": "manual-001", "result": rules.Draw{Digits: []int{0, 1, 0}}, "drawn_at": time.Now().UTC().Truncate(time.Second), "reason": "verified public archive fixture"})
	mustStatus(t, r, 201)
	var saved struct {
		ID string `json:"id"`
	}
	managedData(t, r, &saved)
	ctx := context.Background()
	var before int
	if e := f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_logs)+(SELECT count(*) FROM point_ledger_entries)+(SELECT count(*) FROM bet_orders)`).Scan(&before); e != nil {
		t.Fatal(e)
	}
	for _, prefix := range []string{"/api/v1", "/api/v1/b/aurora"} {
		read := f.call("GET", prefix+"/draw-results?game_id="+g.ID+"&period_no=manual-001&limit=1", "", "", managedBrand, nil)
		mustStatus(t, read, 200)
		var page betting.PublicDrawPage
		managedData(t, read, &page)
		if len(page.Items) != 1 || page.Items[0].ID != saved.ID || page.HasMore || page.Items[0].Result.Digits[0] != 0 {
			t.Fatal(page)
		}
		for _, private := range []string{"source_id", "created_by", "corrected_from_id", "result_hash", "claim", "endpoint", "credential", "state_reason"} {
			if strings.Contains(read.Body.String(), private) {
				t.Fatalf("public leak %s", private)
			}
		}
		detail := f.call("GET", prefix+"/draw-results/"+saved.ID, "", "", managedBrand, nil)
		mustStatus(t, detail, 200)
		var item betting.PublicDrawDetail
		managedData(t, detail, &item)
		if item.Item.ID != saved.ID {
			t.Fatal(item)
		}
		history := f.call("GET", prefix+"/games/"+g.ID+"/periods?period_no=manual-001", "", "", managedBrand, nil)
		mustStatus(t, history, 200)
		var periods betting.PublicPeriodPage
		managedData(t, history, &periods)
		if len(periods.Items) != 1 || periods.Items[0].Draw == nil || periods.Items[0].Draw.ID != saved.ID {
			t.Fatal(periods)
		}
	}
	for _, path := range []string{"/draw-results?limit=101", "/draw-results?offset=-1", "/draw-results?game_id=bad", "/draw-results?period_no=%20", "/draw-results?limit=1&limit=2", "/draw-results?game_id=" + g.ID + "&game_id=" + g.ID, "/draw-results/bad", "/games/bad/periods"} {
		mustStatus(t, f.call("GET", "/api/v1"+path, "", "", managedBrand, nil), 400)
	}
	// The public domain remains authoritative even if another brand is requested
	// in a spoofed administrative header.
	mustStatus(t, f.call("GET", "/api/v1/draw-results/"+saved.ID, "", "", "0199a000-0000-7000-8000-000000000002", nil), 200)
	for _, path := range []string{"/draw-results/" + saved.ID, "/draw-results?game_id=" + g.ID, "/games/" + g.ID + "/periods"} {
		mustStatus(t, f.call("GET", "/api/v1/b/harbor"+path, "", "", managedBrand, nil), 404)
	}
	var after int
	if e := f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_logs)+(SELECT count(*) FROM point_ledger_entries)+(SELECT count(*) FROM bet_orders)`).Scan(&after); e != nil || before != after {
		t.Fatalf("public GET wrote data %d -> %d %v", before, after, e)
	}
	h := New(Dependencies{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/draw-results", nil))
	if rec.Code != 503 {
		t.Fatal(rec.Code)
	}
}
