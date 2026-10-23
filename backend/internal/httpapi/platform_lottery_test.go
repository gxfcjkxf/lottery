package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"strings"
	"testing"
	"time"
)

func TestPlatformLotteryHTTPPermissionsReplayAndSharedRead(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	var brandRole string
	if e := f.pool.QueryRow(ctx, `SELECT role_id::text FROM admin_account_roles WHERE account_id=$1`, f.root).Scan(&brandRole); e != nil {
		t.Fatal(e)
	}
	for _, permission := range []string{"platform_game.view.brand", "platform_game.connect.brand", "platform_game.sell.brand"} {
		if _, e := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, brandRole, permission); e != nil {
			t.Fatal(e)
		}
	}
	platformID := ids.New()
	if _, e := f.pool.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) SELECT $1,'platform_lottery_operator',password_hash,true FROM admin_accounts WHERE id=$2`, platformID, f.root); e != nil {
		t.Fatal(e)
	}
	pf := f
	pf.root = platformID
	role := grantPlatformPermission(t, pf, "platform_game.view.platform", "platform_game.write.platform")
	login := f.call("POST", "/api/v1/platform/auth/login", "platform-lottery-login", "", "", map[string]string{"identifier": "platform_lottery_operator", "password": "root-test-password-2026"})
	mustStatus(t, login, 200)
	var auth identity.AdminAuthentication
	managedData(t, login, &auth)
	body := map[string]any{"code": "platform_shared", "name": "Platform shared", "timezone": "UTC", "model": rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true}, "reason": "Create shared game"}
	created := f.call("POST", "/api/v1/platform/platform-games", "platform-game-create", auth.AccessToken, "", body)
	mustStatus(t, created, 201)
	var game platformlottery.Game
	managedData(t, created, &game)
	replay := f.call("POST", "/api/v1/platform/platform-games", "platform-game-create", auth.AccessToken, "", body)
	mustStatus(t, replay, 201)
	var same platformlottery.Game
	managedData(t, replay, &same)
	if same.ID != game.ID {
		t.Fatal("creation replay duplicated game")
	}
	extraBody := map[string]any{"code": "delete_unused", "name": "Delete unused", "timezone": "UTC", "model": rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true}}
	extraResponse := f.call("POST", "/api/v1/platform/platform-games", "lifecycle-extra", auth.AccessToken, "", extraBody)
	mustStatus(t, extraResponse, 201)
	var extra platformlottery.Game
	managedData(t, extraResponse, &extra)
	path := "/api/v1/platform/platform-games/" + extra.ID + "/lifecycle"
	offline := platformlottery.LifecycleInput{ExpectedVersion: 1, Status: "offline", Reason: "test offline"}
	mustStatus(t, f.call("POST", path, "offline-lifecycle", auth.AccessToken, "", offline), 200)
	mustStatus(t, f.call("POST", path, "offline-lifecycle", auth.AccessToken, "", offline), 200)
	mustStatus(t, f.call("POST", path, "offline-stale", auth.AccessToken, "", offline), 409)
	mustStatus(t, f.call("POST", path, "brand-offline", f.token, "", offline), 403)
	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games/"+extra.ID+"/lifecycle", "wrong-entry", f.token, managedBrand, offline), 404)
	deleted := platformlottery.LifecycleInput{ExpectedVersion: 2, Status: "deleted", Reason: "test delete"}
	mustStatus(t, f.call("POST", path, "delete-unused", auth.AccessToken, "", deleted), 200)
	mustStatus(t, f.call("POST", path, "delete-unused", auth.AccessToken, "", deleted), 200)
	mustStatus(t, f.call("GET", "/api/v1/platform/platform-games/"+extra.ID, "", auth.AccessToken, "", nil), 200) // Historical detail remains readable after soft deletion.
	modelBody := platformlottery.ModelInput{Model: rules.Model{Type: "DIGITS_0_9", Length: 3, Ordered: true}, ExpectedVersion: 1, Reason: "Visual model edit"}
	updated := f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/model", "platform-model-update", auth.AccessToken, "", modelBody)
	mustStatus(t, updated, 200)
	var modelGame platformlottery.Game
	managedData(t, updated, &modelGame)
	if modelGame.Version != 2 || modelGame.Model.AllowRepeat {
		t.Fatal("model update did not persist")
	}
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/model", "platform-model-update", auth.AccessToken, "", modelBody), 200)
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/model", "platform-model-stale", auth.AccessToken, "", modelBody), 409)
	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games/"+game.ID+"/model", "brand-cannot-update-model", f.token, managedBrand, modelBody), 404)
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/model", "brand-wrong-model-surface", f.token, "", modelBody), 403)
	mustStatus(t, f.call("GET", "/api/v1/admin/platform-games/"+game.ID, "", f.token, managedBrand, nil), 404)
	connect := map[string]string{"reason": "Connect brand to shared game"}
	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games/"+game.ID+"/connect", "brand-connect-game", f.token, managedBrand, connect), 201)
	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games/"+game.ID+"/connect", "brand-connect-game", f.token, managedBrand, connect), 201)
	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games/"+game.ID+"/connect", "brand-connect-again", f.token, managedBrand, connect), 409)
	no := false
	yes := true
	availabilityPath := "/api/v1/admin/platform-games/" + game.ID + "/availability"
	availability := platformlottery.AvailabilityInput{ExpectedVersion: 1, Enabled: &no, Reason: "test disable"}
	mustStatus(t, f.call("POST", availabilityPath, "disable-brand", f.token, managedBrand, availability), 200)
	mustStatus(t, f.call("POST", availabilityPath, "disable-brand", f.token, managedBrand, availability), 200)
	mustStatus(t, f.call("POST", availabilityPath, "stale-brand", f.token, managedBrand, availability), 409)
	availability.ExpectedVersion = 2
	availability.Enabled = &yes
	mustStatus(t, f.call("POST", availabilityPath, "enable-brand", f.token, managedBrand, availability), 200)

	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games", "brand-cannot-create", f.token, managedBrand, body), 404)
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-games", "brand-wrong-surface", f.token, "", body), 403)
	now := time.Now().UTC()
	periodBody := platformlottery.PeriodInput{PeriodNo: "shared-001", BetStartAt: now.Add(-3 * time.Hour), BetEndAt: now.Add(-2 * time.Hour), DrawAt: now.Add(-time.Hour), Reason: "Create shared period"}
	createdPeriod := f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/periods", "platform-create-period", auth.AccessToken, "", periodBody)
	mustStatus(t, createdPeriod, 201)
	var period platformlottery.Period
	managedData(t, createdPeriod, &period)
	resultBody := map[string]any{"result": rules.Draw{Digits: []int{1, 2, 3}}, "reason": "Publish shared result"}
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-games/periods/"+period.ID+"/result", "platform-publish-result", auth.AccessToken, "", resultBody), 201)
	modelBody.ExpectedVersion = 2
	locked := f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/model", "platform-used-model", auth.AccessToken, "", modelBody)
	mustStatus(t, locked, 409)
	if !strings.Contains(locked.Body.String(), "PLATFORM_GAME_MODEL_LOCKED") {
		t.Fatal("missing explicit used-model rejection")
	}
	read := f.call("GET", "/api/v1/admin/platform-games/"+game.ID, "", f.token, managedBrand, nil)
	mustStatus(t, read, 200)
	if !strings.Contains(read.Body.String(), period.ID) {
		t.Fatal("connected brand did not read canonical period")
	}
	mustStatus(t, f.call("GET", "/api/v1/admin/platform-games/"+game.ID, "", f.token, "0199a000-0000-7000-8000-000000000002", nil), 403)
	if _, e := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='platform_game.write.platform'`, role); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-games", "platform-game-create", auth.AccessToken, "", body), 403)
	modelBody.ExpectedVersion = 1
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/model", "platform-model-update", auth.AccessToken, "", modelBody), 403)
}

func TestPlatformGameRenamePermissionsVersionAndReplay(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	platformID := ids.New()
	if _, e := f.pool.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) SELECT $1,'rename_operator',password_hash,true FROM admin_accounts WHERE id=$2`, platformID, f.root); e != nil {
		t.Fatal(e)
	}
	pf := f
	pf.root = platformID
	grantPlatformPermission(t, pf, "platform_game.view.platform", "platform_game.write.platform")
	login := f.call("POST", "/api/v1/platform/auth/login", "rename-login", "", "", map[string]string{"identifier": "rename_operator", "password": "root-test-password-2026"})
	mustStatus(t, login, 200)
	var auth identity.AdminAuthentication
	managedData(t, login, &auth)
	created := f.call("POST", "/api/v1/platform/platform-games", "rename-create", auth.AccessToken, "", platformlottery.GameInput{Code: "rename_game", Name: "Original", Model: rules.Model{Type: "DIGITS_0_9", Length: 3, Ordered: true}, Timezone: "UTC"})
	mustStatus(t, created, 201)
	var game platformlottery.Game
	managedData(t, created, &game)
	path := "/api/v1/platform/platform-games/" + game.ID + "/name"
	in := platformlottery.NameInput{Name: "Renamed", ExpectedVersion: 1}
	renamed := f.call("POST", path, "rename-once", auth.AccessToken, "", in)
	mustStatus(t, renamed, 200)
	var out platformlottery.Game
	managedData(t, renamed, &out)
	if out.Name != "Renamed" || out.Code != game.Code || out.Version != 2 || out.Model.Length != 3 {
		t.Fatalf("rename changed identity/model: %+v", out)
	}
	mustStatus(t, f.call("POST", path, "rename-once", auth.AccessToken, "", in), 200)
	mustStatus(t, f.call("POST", path, "rename-stale", auth.AccessToken, "", in), 409)
	mustStatus(t, f.call("POST", path, "rename-brand", f.token, "", in), 403)
	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games/"+game.ID+"/name", "rename-wrong-entry", f.token, managedBrand, in), 404)
	mustStatus(t, f.call("POST", path, "rename-empty", auth.AccessToken, "", platformlottery.NameInput{Name: " ", ExpectedVersion: 2}), 400)
	var count int
	if e := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE resource_id=$1 AND action='platform_game.name.update'`, game.ID).Scan(&count); e != nil {
		t.Fatal(e)
	}
	if count != 1 {
		t.Fatalf("rename audit replayed: %d", count)
	}
}
