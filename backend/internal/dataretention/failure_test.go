package dataretention_test

import (
	"context"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/dataretention"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestBatchFailureRollsBackAndRetryKeepsBrandScope(t *testing.T) {
	db := testdb.NewUnseeded(t)
	ctx := context.Background()
	brand, other := ids.New(), ids.New()
	for _, b := range []string{brand, other} {
		if _, err := db.Exec(ctx, `INSERT INTO brands(id,code,name,status) VALUES($1,$2,'Retention failure isolated test','active')`, b, "retention_"+b); err != nil {
			t.Fatal(err)
		}
	}
	var target string
	for _, b := range []string{brand, other} {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		id, e := audit.Append(ctx, tx, audit.Record{BrandID: b, ActorType: "system", Action: "retention.test.read", ResourceType: "query", RequestID: ids.New()})
		if e != nil {
			tx.Rollback(ctx)
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		if b == brand {
			target = id
		}
	}
	task := ids.New()
	cutoff := time.Now().UTC().AddDate(0, 0, 1)
	if _, err := db.Exec(ctx, `INSERT INTO data_retention_tasks(id,brand_id,policy_version,timezone,execute_at,cutoff_at) VALUES($1,$2,1,'UTC',$3,$4)`, task, brand, cutoff.AddDate(0, 2, 0), cutoff); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `CREATE FUNCTION retention_test_fail_summary() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'owned test summary failure'; END $$; CREATE TRIGGER retention_test_fail_summary BEFORE INSERT ON data_retention_daily FOR EACH ROW EXECUTE FUNCTION retention_test_fail_summary()`); err != nil {
		t.Fatal(err)
	}
	s := dataretention.Service{DB: db}
	if _, err := s.Process(ctx); err == nil {
		t.Fatal("injected failure did not fail")
	}
	var state string
	var raw, receipts, summaries int
	if err := db.QueryRow(ctx, `SELECT state,(SELECT count(*) FROM audit_logs WHERE id=$2),(SELECT count(*) FROM data_retention_receipts),(SELECT count(*) FROM data_retention_daily) FROM data_retention_tasks WHERE id=$1`, task, target).Scan(&state, &raw, &receipts, &summaries); err != nil || state != "failed" || raw != 1 || receipts != 0 || summaries != 0 {
		t.Fatal(state, raw, receipts, summaries, err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM audit_logs WHERE id=$1`, target); err == nil {
		t.Fatal("ordinary deletion bypassed immutable guard")
	}
	if _, err := db.Exec(ctx, `DROP TRIGGER retention_test_fail_summary ON data_retention_daily;DROP FUNCTION retention_test_fail_summary()`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = s.RetryTx(ctx, tx, task, points.Metadata{ActorType: "system", RequestID: ids.New()}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO data_retention_receipts(family,id,brand_id,task_id,payload,original_sha256) SELECT 'audit_logs',id,$2,$3,retention_payload('audit_logs',to_jsonb(a)),retention_json_hash(to_jsonb(a)) FROM audit_logs a WHERE id=$1`, target, other, task); err == nil {
		t.Fatal("cross-brand receipt was accepted")
	}
	oldChallenge, activeChallenge := ids.New(), ids.New()
	if _, err = db.Exec(ctx, `INSERT INTO auth_challenges(id,brand_id,kind,proof_hash,expires_at) VALUES($1,$3,'captcha','owned-expired',now()-interval '2 days'),($2,$3,'captcha','owned-active',now()+interval '3 days')`, oldChallenge, activeChallenge, brand); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		worked, e := s.Process(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if !worked {
			break
		}
		if i == 11 {
			t.Fatal("retry did not finish")
		}
	}
	var otherRows, activeRows, oldRows int
	if err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_logs WHERE brand_id=$1),(SELECT count(*) FROM auth_challenges WHERE id=$2),(SELECT count(*) FROM auth_challenges WHERE id=$3),(SELECT count(*) FROM data_retention_receipts WHERE family='audit_logs' AND id=$4)`, other, activeChallenge, oldChallenge, target).Scan(&otherRows, &activeRows, &oldRows, &receipts); err != nil || otherRows != 1 || activeRows != 1 || oldRows != 0 || receipts != 0 {
		t.Fatal(otherRows, activeRows, oldRows, receipts, err)
	}
	var count string
	if err = db.QueryRow(ctx, `SELECT totals->>'record_count' FROM data_retention_daily WHERE brand_id=$1 AND family='audit_logs'`, brand).Scan(&count); err != nil || count != "1" {
		t.Fatal("retry counted the failed batch twice", count, err)
	}
}
