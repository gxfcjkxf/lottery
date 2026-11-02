package httpapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/hkjccalendar"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
)

type calendarFetchFunc func(context.Context) ([]hkjccalendar.Month, error)

func (f calendarFetchFunc) Fetch(ctx context.Context) ([]hkjccalendar.Month, error) { return f(ctx) }

func TestOfficialCalendarManualSyncPermissionsReplayAndFailure(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	calls := 0
	var fetchErr error
	source := calendarFetchFunc(func(context.Context) ([]hkjccalendar.Month, error) {
		calls++
		return []hkjccalendar.Month{{Month: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), DrawDates: []string{"2026-11-03"}, SnowballDates: []string{}, PresaleDates: []string{}}}, fetchErr
	})
	users, err := identity.New(f.pool)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := mutation.New(f.pool, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	f.http = New(Dependencies{Brands: tenant.Store{DB: f.pool}, Ready: f.pool.Ping, Identity: users, Mutations: engine, Admins: adminsys.Store{DB: f.pool}, OfficialCalendarSource: source})
	id := ids.New()
	if _, err = f.pool.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) SELECT $1,'calendar_sync_tester',password_hash,true FROM admin_accounts WHERE id=$2`, id, f.root); err != nil {
		t.Fatal(err)
	}
	pf := f
	pf.root = id
	role := grantPlatformPermission(t, pf, "platform_game.view.platform")
	login := f.call("POST", "/api/v1/platform/auth/login", "calendar-login", "", "", map[string]string{"identifier": "calendar_sync_tester", "password": "root-test-password-2026"})
	mustStatus(t, login, 200)
	var auth identity.AdminAuthentication
	managedData(t, login, &auth)
	path := "/api/v1/platform/platform-games/official-calendar/sync"
	mustStatus(t, f.call("POST", path, "calendar-denied", auth.AccessToken, "", map[string]any{}), 403)
	mustStatus(t, f.call("POST", path, "calendar-brand", f.token, "", map[string]any{}), 403)
	mustStatus(t, f.call("POST", path, "calendar-anon", "", "", map[string]any{}), 401)
	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games/official-calendar/sync", "calendar-wrong-entry", f.token, managedBrand, map[string]any{}), 404)
	if calls != 0 {
		t.Fatal("unauthorized collection")
	}
	if _, err = f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'platform_game.write.platform')`, role); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("POST", path, "", auth.AccessToken, "", map[string]any{}), 400)
	mustStatus(t, f.call("POST", path, "calendar-bad-body", auth.AccessToken, "", map[string]any{"source": "untrusted"}), 400)
	sync := func(key, status string) {
		t.Helper()
		r := f.call("POST", path, key, auth.AccessToken, "", map[string]any{})
		mustStatus(t, r, 200)
		var out hkjccalendar.SyncResult
		managedData(t, r, &out)
		if out.Status != status {
			t.Fatalf("outcome %+v", out)
		}
	}
	sync("calendar-success", "synced")
	sync("calendar-success", "synced")
	if calls != 1 {
		t.Fatal("replay fetched again", calls)
	}
	fetchErr = errors.New("upstream unavailable")
	sync("calendar-failed", "failed")
	sync("calendar-failed", "failed")
	if calls != 2 {
		t.Fatal("failed replay fetched again", calls)
	}
	var code string
	var dates int
	if err = f.pool.QueryRow(ctx, `SELECT last_error_code,(SELECT sum(cardinality(draw_dates)) FROM hkjc_calendar_months) FROM hkjc_calendar_sync`).Scan(&code, &dates); err != nil || code != "HKJC_CALENDAR_FETCH_FAILED" || dates != 1 {
		t.Fatalf("failure persistence %s %d %v", code, dates, err)
	}
	fetchErr = nil
	sync("calendar-recover", "synced")
	if calls != 3 {
		t.Fatal("manual sync did not bypass future schedule")
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(79001034)`); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("POST", path, "calendar-lock-busy", auth.AccessToken, "", map[string]any{}), 409)
	if calls != 3 {
		t.Fatal("concurrent collection")
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='platform_game.calendar.sync'`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("audit count %d %v", count, err)
	}
	if _, err = f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='platform_game.write.platform'`, role); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("POST", path, "calendar-success", auth.AccessToken, "", map[string]any{}), 403)
	if calls != 3 {
		t.Fatal("revoked replay collected")
	}
}
