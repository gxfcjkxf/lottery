package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/gxfcjkxf/lottery/backend/internal/schedule"
)

func periodFixture(t *testing.T) (managementHTTP, rulebook.Game) {
	t.Helper()
	f, _ := bookFixture(t)
	ctx := context.Background()
	for _, key := range []string{"schedule.view.brand", "schedule.write.brand", "period.view.brand", "period.generate.brand"} {
		if _, e := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1`, f.root, key); e != nil {
			t.Fatal(e)
		}
	}
	r := f.call("POST", "/api/v1/admin/games", "period-game-create", f.token, managedBrand, map[string]any{"code": "period_three", "name": "Period Three", "model": rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true}, "timezone": "UTC", "reason": "calendar test game"})
	mustStatus(t, r, 201)
	var game rulebook.Game
	managedData(t, r, &game)
	return f, game
}
func intervalSpec() schedule.Spec {
	return schedule.Spec{Timezone: "UTC", Mode: "interval", IntervalSeconds: 1800, BetOpenBeforeSeconds: 300, BetCloseBeforeSeconds: 30, Weekdays: []int{0, 1, 2, 3, 4, 5, 6}, HolidayPolicy: "normal"}
}
func TestPeriodHTTPImmutableScheduleGenerationReplayAndBoundaries(t *testing.T) {
	f, g := periodFixture(t)
	base := "/api/v1/admin/games/" + g.ID
	mustStatus(t, f.call("GET", base+"/schedule", "", f.token, managedBrand, nil), 404)
	bad := intervalSpec()
	bad.Timezone = "Asia/Manila"
	mustStatus(t, f.call("PUT", base+"/schedule", "calendar-wrong-zone", f.token, managedBrand, map[string]any{"version": g.Version, "spec": bad, "reason": "zone mismatch"}), 400)
	body := map[string]any{"version": g.Version, "spec": intervalSpec(), "reason": "validated calendar"}
	r := f.call("PUT", base+"/schedule", "calendar-save-once", f.token, managedBrand, body)
	mustStatus(t, r, 200)
	var saved rulebook.ScheduleRecord
	managedData(t, r, &saved)
	if saved.Revision != 1 || saved.GameVersion != g.Version+1 {
		t.Fatal(saved)
	}
	replay := f.call("PUT", base+"/schedule", "calendar-save-once", f.token, managedBrand, body)
	mustStatus(t, replay, 200)
	var again rulebook.ScheduleRecord
	managedData(t, replay, &again)
	if again.ID != saved.ID {
		t.Fatal("duplicate schedule")
	}
	mustStatus(t, f.call("PUT", base+"/schedule", "calendar-stale-version", f.token, managedBrand, body), 409)
	var now time.Time
	if e := f.pool.QueryRow(context.Background(), `SELECT clock_timestamp()`).Scan(&now); e != nil {
		t.Fatal(e)
	}
	from := now.UTC().Add(time.Hour).Truncate(time.Second)
	generation := map[string]any{"from": from, "to": from.Add(3 * time.Hour), "reason": "future reservations"}
	r = f.call("POST", base+"/periods/generate", "calendar-generate-once", f.token, managedBrand, generation)
	mustStatus(t, r, 200)
	var out rulebook.Generation
	managedData(t, r, &out)
	if out.Created != 6 || out.Existing != 0 || len(out.Periods) != 6 {
		t.Fatal(out)
	}
	for _, p := range out.Periods {
		if p.Status != "pending" || p.Version != 1 || p.ScheduleID != saved.ID {
			t.Fatal(p)
		}
	}
	mustStatus(t, f.call("POST", base+"/periods/generate", "calendar-generate-once", f.token, managedBrand, generation), 200)
	r = f.call("POST", base+"/periods/generate", "calendar-generate-new-key", f.token, managedBrand, generation)
	mustStatus(t, r, 200)
	managedData(t, r, &out)
	if out.Created != 0 || out.Existing != 6 {
		t.Fatal(out)
	}
	r = f.call("GET", base+"/periods?limit=2&offset=1", "", f.token, managedBrand, nil)
	mustStatus(t, r, 200)
	var listing struct {
		Periods []rulebook.Period `json:"periods"`
	}
	managedData(t, r, &listing)
	if len(listing.Periods) != 2 {
		t.Fatal(listing)
	}
	mustStatus(t, f.call("GET", base+"/periods?limit=101", "", f.token, managedBrand, nil), 400)
	mustStatus(t, f.call("POST", base+"/periods/generate", "calendar-over-range", f.token, managedBrand, map[string]any{"from": from, "to": from.Add(8 * 24 * time.Hour), "reason": "too long"}), 400)
	mustStatus(t, f.call("PUT", base+"/schedule", "calendar-script-unknown", f.token, managedBrand, map[string]any{"version": saved.GameVersion, "spec": intervalSpec(), "script": "eval()", "reason": "unknown field"}), 400)
	mustStatus(t, pointsCall(f, "PUT", base+"/schedule", "calendar-forged-source", f.token, managedBrand, "https://evil.invalid", body), 403)
	mustStatus(t, f.call("GET", base+"/periods", "", f.token, pointsBrandB, nil), 403)
	var schedules, periods, opened, ledger, audits int
	for _, q := range []struct {
		sql string
		out *int
	}{
		{`SELECT count(*) FROM game_schedules`, &schedules}, {`SELECT count(*) FROM periods`, &periods}, {`SELECT started_sequence FROM games WHERE code='period_three'`, &opened}, {`SELECT count(*) FROM point_ledger_entries`, &ledger}, {`SELECT count(*) FROM audit_logs WHERE action='schedule.write'`, &audits},
	} {
		if e := f.pool.QueryRow(context.Background(), q.sql).Scan(q.out); e != nil {
			t.Fatal(e)
		}
	}
	if schedules != 1 || periods != 6 || opened != 0 || ledger != 0 || audits != 1 {
		t.Fatal(schedules, periods, opened, ledger, audits)
	}
}
func TestPeriodHTTPFreshRevocationAndSuperReadOnly(t *testing.T) {
	f, g := periodFixture(t)
	ctx := context.Background()
	base := "/api/v1/admin/games/" + g.ID
	body := map[string]any{"version": g.Version, "spec": intervalSpec(), "reason": "role recheck"}
	if _, e := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE permission_key='schedule.write.brand'`); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("PUT", base+"/schedule", "calendar-revoked-role", f.token, managedBrand, body), 403)
	if _, e := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,'schedule.write.brand' FROM admin_account_roles WHERE account_id=$1`, f.root); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("PUT", base+"/schedule", "calendar-before-super", f.token, managedBrand, body), 200)
	if _, e := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("GET", base+"/schedule", "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("GET", base+"/periods", "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("PUT", base+"/schedule", "calendar-super-write", f.token, managedBrand, body), 403)
	mustStatus(t, f.call("POST", base+"/periods/generate", "calendar-super-generate", f.token, managedBrand, map[string]any{"from": time.Now(), "to": time.Now().Add(time.Hour), "reason": "denied"}), 403)
}
