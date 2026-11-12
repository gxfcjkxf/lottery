package dataretention_test

import (
	"context"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/dataretention"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestRetentionDiscoveryCatchupAndTimezoneFreeze(t *testing.T) {
	db := testdb.NewUnseeded(t)
	ctx := context.Background()
	brand := ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO brands(id,code,name,status) VALUES($1,$2,'Retention scheduler isolated test','active')`, brand, "retention_"+brand); err != nil {
		t.Fatal(err)
	}
	var now time.Time
	var zone string
	if err := db.QueryRow(ctx, `SELECT statement_timestamp(),timezone FROM platform_settings WHERE id`).Scan(&now, &zone); err != nil {
		t.Fatal(err)
	}
	schedule := dataretention.Schedule{ExecutionDay: 1, ExecutionTime: "00:00"}
	first, err := schedule.NextRun(now.AddDate(0, -2, 0), zone)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE data_retention_policy SET enabled=true,execution_day=1,execution_time='00:00',next_execute_at=$1,next_cutoff_at=$2,next_timezone=$3 WHERE id`, first.ExecuteAt, first.CutoffAt, zone); err != nil {
		t.Fatal(err)
	}
	s := dataretention.Service{DB: db}
	for i := 0; i < 2; i++ {
		n, e := s.Discover(ctx)
		if e != nil || n != 2 {
			t.Fatal(n, e)
		}
	}
	if n, e := s.Discover(ctx); e != nil || n != 0 {
		t.Fatal("future cursor generated duplicate tasks", n, e)
	}
	var tasks, platformTasks int
	if err = db.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE brand_id IS NULL) FROM data_retention_tasks`).Scan(&tasks, &platformTasks); err != nil || tasks != 4 || platformTasks != 2 {
		t.Fatal(tasks, platformTasks, err)
	}
	var savedZone string
	var savedCutoff time.Time
	if err = db.QueryRow(ctx, `SELECT timezone,cutoff_at FROM data_retention_tasks WHERE brand_id=$1 AND execute_at=$2`, brand, first.ExecuteAt).Scan(&savedZone, &savedCutoff); err != nil || savedZone != zone || !savedCutoff.Equal(first.CutoffAt) {
		t.Fatal(savedZone, savedCutoff, err)
	}
	newZone := "UTC"
	if zone == newZone {
		newZone = "Asia/Singapore"
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE platform_settings SET timezone=$1 WHERE id`, newZone); err != nil {
		t.Fatal(err)
	}
	if err = s.SyncTimezoneTx(ctx, tx, zone, newZone, points.Metadata{ActorType: "system", RequestID: ids.New()}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var version int
	var nextZone string
	var next time.Time
	if err = db.QueryRow(ctx, `SELECT version,next_timezone,next_execute_at FROM data_retention_policy WHERE id`).Scan(&version, &nextZone, &next); err != nil || version != 2 || nextZone != newZone || !next.After(now) {
		t.Fatal(version, nextZone, next, err)
	}
	if err = db.QueryRow(ctx, `SELECT timezone,cutoff_at FROM data_retention_tasks WHERE brand_id=$1 AND execute_at=$2`, brand, first.ExecuteAt).Scan(&savedZone, &savedCutoff); err != nil || savedZone != zone || !savedCutoff.Equal(first.CutoffAt) {
		t.Fatal("created task range changed", savedZone, savedCutoff, err)
	}
	// Global audits have no brand, but must still be handled by the platform task.
	auditID := ids.New()
	if _, err = db.Exec(ctx, `INSERT INTO audit_logs(id,actor_type,action,resource_type,request_id,created_at) VALUES($1,'system','retention.test.read','query',$2,$3)`, auditID, ids.New(), first.CutoffAt.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		worked, e := s.Process(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if !worked {
			break
		}
	}
	var raw, receipts int
	if err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_logs WHERE id=$1),(SELECT count(*) FROM data_retention_receipts WHERE id=$1)`, auditID).Scan(&raw, &receipts); err != nil || raw != 0 || receipts != 0 {
		t.Fatal("global query audit not discarded", raw, receipts, err)
	}
}
