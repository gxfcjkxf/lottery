package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
)

func TestDrawScheduleHTTPPermissionsVersionsAndReplay(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	operator := ids.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) SELECT $1,'draw_schedule_operator',password_hash,true FROM admin_accounts WHERE id=$2`, operator, f.root); err != nil {
		t.Fatal(err)
	}
	pf := f
	pf.root = operator
	grantPlatformPermission(t, pf, "platform_game.view.platform", "platform_game.write.platform")
	login := f.call("POST", "/api/v1/platform/auth/login", "schedule-login", "", "", map[string]string{"identifier": "draw_schedule_operator", "password": "root-test-password-2026"})
	mustStatus(t, login, 200)
	var auth identity.AdminAuthentication
	managedData(t, login, &auth)
	raw, err := os.ReadFile("../rules/config/examples/lucky_group_preview.json")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot rules.ConfiguredSnapshot
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	created := f.call("POST", "/api/v1/platform/platform-games", "schedule-game", auth.AccessToken, "", platformlottery.GameInput{Code: "schedule_http", Name: "Schedule HTTP", Timezone: "Asia/Macau", Model: snapshot.Catalog.Model})
	mustStatus(t, created, 201)
	var game platformlottery.Game
	managedData(t, created, &game)
	path := "/api/v1/platform/platform-games/" + game.ID + "/draw-schedule"
	body := platformlottery.DrawSchedule{Enabled: true, Mode: "daily", DrawTime: "21:30:00", OpenBeforeHours: 24.5, CloseBeforeSeconds: 60, CollectAfterSeconds: 90}
	saved := f.call("POST", path, "save-schedule", auth.AccessToken, "", body)
	mustStatus(t, saved, 200)
	var schedule platformlottery.DrawSchedule
	managedData(t, saved, &schedule)
	if schedule.Version != 1 || schedule.CollectAfterSeconds != 90 || schedule.OpenBeforeHours != 24.5 {
		t.Fatal("schedule not saved", schedule)
	}
	replay := f.call("POST", path, "save-schedule", auth.AccessToken, "", body)
	mustStatus(t, replay, 200)
	var same platformlottery.DrawSchedule
	managedData(t, replay, &same)
	if same != schedule {
		t.Fatal("replay differs")
	}
	mustStatus(t, f.call("POST", path, "stale-schedule", auth.AccessToken, "", body), 409)
	mustStatus(t, f.call("POST", path, "brand-schedule", f.token, "", body), 403)
	mustStatus(t, f.call("POST", strings.Replace(path, "/platform/", "/admin/", 1), "brand-entry-schedule", f.token, managedBrand, body), 404)
	mustStatus(t, f.call("POST", path, "anonymous-schedule", "", "", body), 401)
	body = schedule
	body.DrawTime = "25:00:00"
	mustStatus(t, f.call("POST", path, "invalid-schedule", auth.AccessToken, "", body), 400)
	read := f.call("GET", "/api/v1/platform/platform-games/"+game.ID+"/automation", "", auth.AccessToken, "", nil)
	mustStatus(t, read, 200)
	var config platformlottery.AutomationConfig
	managedData(t, read, &config)
	if config.Schedule == nil || *config.Schedule != schedule {
		t.Fatal("read differs", config.Schedule)
	}
	var versions int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM platform_draw_schedule_versions WHERE game_id=$1`, game.ID).Scan(&versions); err != nil || versions != 1 {
		t.Fatal("duplicate version", versions, err)
	}
}
