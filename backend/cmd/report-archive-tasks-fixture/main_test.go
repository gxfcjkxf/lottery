//go:build browserfixture

package main

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/reportarchive"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	testOwnedURL = "postgres://lottery_test@127.0.0.1:55432/lottery_archive_tasks_browser_desktop?sslmode=disable"
	testAdmin    = "archive_tasks_browser_desktop"
	testHarbor   = "0199a000-0000-7000-8000-000000000002"
)

func TestFixtureDatabaseURLGuardAllowsOnlyExactOwnedLoopbackTargets(t *testing.T) {
	for _, viewport := range []string{"desktop", "mobile"} {
		for _, host := range []string{"127.0.0.1", "localhost"} {
			for _, port := range []string{"5432", "55432"} {
				for _, scheme := range []string{"postgres", "postgresql"} {
					raw := scheme + "://lottery_test@" + host + ":" + port + "/lottery_archive_tasks_browser_" + viewport + "?sslmode=disable"
					got, err := readFixtureConfig(raw, "test", fixtureAck, viewport, "", "", fixtureAdminPrefix+viewport)
					if err != nil || got.Database != "lottery_archive_tasks_browser_"+viewport || got.Viewport != viewport {
						t.Fatalf("owned URL rejected: %v", err)
					}
				}
			}
		}
	}
	bad := []struct {
		dsn, env, confirm, viewport, readURL, readURLs, username string
	}{
		{testOwnedURL, "production", fixtureAck, "desktop", "", "", testAdmin},
		{testOwnedURL, "test", "", "desktop", "", "", testAdmin},
		{testOwnedURL, "test", fixtureAck, "mobile", "", "", "archive_tasks_browser_mobile"},
		{"postgres://lottery_test@127.0.0.1:55432/postgres?sslmode=disable", "test", fixtureAck, "desktop", "", "", testAdmin},
		{"postgres://lottery_test@remote.example:55432/lottery_archive_tasks_browser_desktop?sslmode=disable", "test", fixtureAck, "desktop", "", "", testAdmin},
		{"postgres://lottery_test@127.0.0.1:5433/lottery_archive_tasks_browser_desktop?sslmode=disable", "test", fixtureAck, "desktop", "", "", testAdmin},
		{"postgres://postgres@127.0.0.1:55432/lottery_archive_tasks_browser_desktop?sslmode=disable", "test", fixtureAck, "desktop", "", "", testAdmin},
		{"postgres://lottery_test:pw@127.0.0.1:55432/lottery_archive_tasks_browser_desktop?sslmode=disable", "test", fixtureAck, "desktop", "", "", testAdmin},
		{testOwnedURL + "#fragment", "test", fixtureAck, "desktop", "", "", testAdmin},
		{testOwnedURL + "&sslmode=disable", "test", fixtureAck, "desktop", "", "", testAdmin},
		{"postgres://lottery_test@127.0.0.1:55432/lottery_archive_tasks_browser_%64esktop?sslmode=disable", "test", fixtureAck, "desktop", "", "", testAdmin},
		{testOwnedURL, "test", fixtureAck, "desktop", "postgres://read.example/db", "", testAdmin},
		{testOwnedURL, "test", fixtureAck, "desktop", "", "[]", testAdmin},
	}
	for _, tc := range bad {
		if _, err := readFixtureConfig(tc.dsn, tc.env, tc.confirm, tc.viewport, tc.readURL, tc.readURLs, tc.username); err == nil {
			t.Errorf("unsafe fixture configuration accepted: %s", tc.dsn)
		}
	}
}

func TestFixtureRejectsUnsafeTargetBeforeOpeningOrWriting(t *testing.T) {
	for key, value := range map[string]string{
		"APP_ENV": "test", "REPORT_ARCHIVE_TASKS_FIXTURE_CONFIRM": fixtureAck,
		"REPORT_ARCHIVE_TASKS_VIEWPORT": "desktop", "TEST_ARCHIVE_TASKS_ADMIN_USERNAME": testAdmin,
		"DATABASE_URL": "postgres://lottery_test@127.0.0.1:55432/postgres?sslmode=disable",
	} {
		t.Setenv(key, value)
	}
	original := os.Args
	os.Args = []string{"report-archive-tasks-fixture", "prepare"}
	t.Cleanup(func() { os.Args = original })
	if err := run(); err == nil || err.Error() != "exact owned report archive task PostgreSQL URL required" {
		t.Fatalf("unsafe database target was not rejected before connect: %v", err)
	}
}

func TestOwnedSchemaPrepareProducesFailedTaskAndRealRetryWorkerCompletesWithoutMoneyChanges(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	seedFixtureAdmin(t, ctx, db)
	before := moneySnapshot(t, ctx, db)
	if !reflect.DeepEqual(before, [3]int64{}) {
		t.Fatalf("seeded isolated schema unexpectedly has wallet activity: %v", before)
	}
	prepared, err := prepare(ctx, db, testAdmin, "desktop")
	if err != nil {
		t.Fatal(err)
	}
	if prepared.State != "failed" || prepared.TaskVersion != 2 || prepared.AttemptCount != 1 || prepared.ArchiveID != nil || prepared.ArchiveCount != "0" {
		t.Fatalf("prepare did not exercise the genuine automatic failure: %+v", prepared)
	}
	if _, err = advance(ctx, db); err == nil {
		t.Fatal("worker advanced a failed task without an explicit admin retry")
	}
	retryFixtureTask(t, ctx, db, prepared.TaskID, prepared.TaskVersion)
	advanced, err := advance(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if advanced.TaskID != prepared.TaskID || advanced.State != "completed" || advanced.TaskVersion != 4 || advanced.AttemptCount != 2 || advanced.ArchiveID == nil || advanced.ArchiveCount != "1" {
		t.Fatalf("explicit retry and real worker did not complete original task: %+v", advanced)
	}
	verified, err := verify(ctx, db)
	if err != nil || !reflect.DeepEqual(verified, advanced) {
		t.Fatalf("verify mismatch: output=%+v error=%v", verified, err)
	}
	after := moneySnapshot(t, ctx, db)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("report archive fixture changed wallet data: before=%v after=%v", before, after)
	}
	var retryAudits int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action='report_archive.task.retry' AND resource_id=$2`, testHarbor, prepared.TaskID).Scan(&retryAudits); err != nil || retryAudits != 1 {
		t.Fatalf("core retry did not write exactly one genuine audit: count=%d error=%v", retryAudits, err)
	}
}

func seedFixtureAdmin(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
	t.Helper()
	const adminID = "0199a000-0000-7000-8000-000000000091"
	const roleID = "0199a000-0000-7000-8000-000000000092"
	if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'not-used-by-test')`, adminID, testAdmin); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name,is_bootstrap) VALUES($1,$2,'archive_tasks_test','Archive task test administrator',true)`, roleID, testHarbor); err != nil {
		t.Fatal(err)
	}
	for _, permission := range []string{"report_archive.view.brand", "report_archive_policy.write.brand", "report_archive_task.retry.brand"} {
		if _, err := db.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, permission); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, roleID, permission); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, adminID, testHarbor); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, adminID, roleID); err != nil {
		t.Fatal(err)
	}
}

func retryFixtureTask(t *testing.T, ctx context.Context, db *pgxpool.Pool, taskID string, version int64) {
	t.Helper()
	adminID, err := activeAdminID(ctx, db, testAdmin)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	actor, err := (adminsys.Store{DB: db}).LockAdminAccess(ctx, tx, adminID, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (reportarchive.Service{DB: db}).RetryAutomaticTaskTx(ctx, tx, testHarbor, taskID, actor, version, "Retry the owned browser fixture task", points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: ids.New()})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func moneySnapshot(t *testing.T, ctx context.Context, db *pgxpool.Pool) [3]int64 {
	t.Helper()
	var got [3]int64
	err := db.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM point_accounts WHERE brand_id=$1),
	 (SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1),
	 (SELECT coalesce(sum(points),0) FROM point_buckets WHERE brand_id=$1)`, testHarbor).Scan(&got[0], &got[1], &got[2])
	if err != nil {
		t.Fatal(err)
	}
	return got
}
