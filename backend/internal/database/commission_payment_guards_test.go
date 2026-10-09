package database_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

func TestCommissionPaymentPolicyStartsOffAndCannotBeToggledWithoutAudit(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	var enabled bool
	var version int64
	if err := db.QueryRow(ctx, `SELECT enabled,version FROM brand_commission_payment_policies WHERE brand_id=$1`, upgradeBrand).Scan(&enabled, &version); err != nil {
		t.Fatal(err)
	}
	if enabled || version != 1 {
		t.Fatalf("payment rollout default=(%t,%d), want (false,1)", enabled, version)
	}

	if _, err := db.Exec(ctx, `UPDATE brand_commission_payment_policies SET enabled=true WHERE brand_id=$1`, upgradeBrand); err == nil {
		t.Fatal("unaudited SQL toggle enabled commission payments")
	}
	if err := db.QueryRow(ctx, `SELECT enabled,version FROM brand_commission_payment_policies WHERE brand_id=$1`, upgradeBrand).Scan(&enabled, &version); err != nil || enabled || version != 1 {
		t.Fatalf("rejected SQL toggle changed policy=(%t,%d), err=%v", enabled, version, err)
	}

	for _, query := range []string{
		`UPDATE commission_payment_policy_revisions SET enabled=true WHERE brand_id=$1 AND version=1`,
		`DELETE FROM commission_payment_policy_revisions WHERE brand_id=$1 AND version=1`,
	} {
		if _, err := db.Exec(ctx, query, upgradeBrand); err == nil {
			t.Errorf("mutable policy revision accepted: %s", query)
		}
	}
	var revisions int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM commission_payment_policy_revisions WHERE brand_id=$1 AND version=1 AND enabled=false AND audit_log_id IS NULL`, upgradeBrand).Scan(&revisions); err != nil || revisions != 1 {
		t.Fatalf("initial immutable revision count=%d, err=%v", revisions, err)
	}
}

func TestStandaloneCommissionCreditNeedsPaymentTargetAndGenericAdjustmentRemainsAvailable(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	member, account := currentWallet(t, db)
	store := points.Store{DB: db}
	before, err := store.Read(ctx, upgradeBrand, member)
	if err != nil {
		t.Fatal(err)
	}

	targetID := ids.New()
	var commissionDelta points.Balance
	commissionDelta[3][0] = 7
	commissionAllocation := []points.Allocation{{Source: "commission", State: "available", Points: 7}}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Post(ctx, tx, points.Change{
		BrandID: upgradeBrand, MemberID: member, EntryType: "commission", ReferenceType: "commission_payment_target",
		ReferenceID: targetID, OperationKey: "commission-payment:" + targetID,
		Reason: "attempt standalone business commission credit", ActorType: "system", RequestID: ids.New(),
		Delta: commissionDelta, Allocation: commissionAllocation,
	})
	if err == nil {
		_ = tx.Rollback(ctx)
		t.Fatal("commission business credit without a real payment target was accepted")
	}
	_ = tx.Rollback(ctx)
	afterRejected, err := store.Read(ctx, upgradeBrand, member)
	if err != nil || afterRejected != before {
		t.Fatalf("rejected commission credit changed wallet: before=%+v after=%+v err=%v", before, afterRejected, err)
	}
	var commissionEntries int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1 AND entry_type='commission'`, upgradeBrand).Scan(&commissionEntries); err != nil || commissionEntries != 0 {
		t.Fatalf("rejected commission credit left ledger rows=%d err=%v", commissionEntries, err)
	}
	var accountID string
	if err := db.QueryRow(ctx, `SELECT id::text FROM point_accounts WHERE brand_id=$1 AND brand_member_id=$2`, upgradeBrand, member).Scan(&accountID); err != nil || accountID != account {
		t.Fatalf("fixture account=%q want=%q err=%v", accountID, account, err)
	}

	// The source bucket is still available to an ordinary, explicitly typed
	// wallet adjustment. Only business commission entries require a payment.
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Post(ctx, tx, points.Change{
		BrandID: upgradeBrand, MemberID: member, EntryType: "adjustment", ReferenceType: "commission_adjustment_test",
		OperationKey: "manual-adjustment:" + ids.New(), Reason: "authorized generic wallet adjustment",
		ActorType: "system", RequestID: ids.New(), Delta: commissionDelta, Allocation: commissionAllocation,
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("generic wallet adjustment into commission source was rejected: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	afterAdjustment, err := store.Read(ctx, upgradeBrand, member)
	if err != nil || afterAdjustment.BySource[3][0] != 7 || afterAdjustment.Version != before.Version+1 {
		t.Fatalf("generic adjustment did not post exactly once: wallet=%+v err=%v", afterAdjustment, err)
	}
}

func TestCommissionCreditGuardSearchPathResistsTemporaryTargetAndFunctionSpoofing(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	member, _ := currentWallet(t, db)
	var schema string
	if err := db.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	var pathPinned bool
	if err := db.QueryRow(ctx, `SELECT coalesce(proconfig @> ARRAY[$1]::text[],false) FROM pg_proc WHERE oid=to_regprocedure('guard_commission_payment_credit()')`, "search_path=pg_catalog, "+schema+", pg_temp").Scan(&pathPinned); err != nil || !pathPinned {
		t.Fatalf("commission credit guard does not pin search_path ahead of pg_temp: pinned=%t err=%v", pathPinned, err)
	}

	// Populate a complete counterfeit authorization chain in pg_temp. If the
	// trigger trusted temp relations/functions, this would authorize the credit;
	// the real schema's empty target set must remain authoritative.
	for _, table := range []string{"commission_payment_targets", "commission_payments", "commission_cycles", "brand_commission_payment_policies", "brands"} {
		if _, err := db.Exec(ctx, fmt.Sprintf(`CREATE TEMP TABLE %s (LIKE %s.%s INCLUDING DEFAULTS)`, pgx.Identifier{table}.Sanitize(), quotedSchema, pgx.Identifier{table}.Sanitize())); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, fmt.Sprintf(`INSERT INTO pg_temp.brands SELECT * FROM %s.brands WHERE id=$1`, quotedSchema), upgradeBrand); err != nil {
		t.Fatal(err)
	}
	cycleID, anchorID, creatorID, auditID := ids.New(), ids.New(), ids.New(), ids.New()
	paymentID, runID, targetID, earningID, agentID := ids.New(), ids.New(), ids.New(), ids.New(), ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO pg_temp.brand_commission_payment_policies(brand_id,version,enabled) VALUES($1,1,true)`, upgradeBrand); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO pg_temp.commission_cycles(id,brand_id,window_from,window_to,anchor_order_id,calendar,created_by,reason,creation_audit_log_id)
 VALUES($2,$1,clock_timestamp()-interval '1 day',clock_timestamp()+interval '1 day',$3,'{}',$4,'spoofed temp cycle',$5)`, upgradeBrand, cycleID, anchorID, creatorID, auditID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO pg_temp.commission_payments(id,brand_id,cycle_id,run_id,evidence_epoch,payout_mode,state,total_points,target_count,approval_actor_type,approval_audit_log_id,creation_audit_log_id,last_audit_log_id)
	 VALUES($3,$1,$2,$4,0,'automatic','paying',1,1,'system',$5,$5,$5)`, upgradeBrand, cycleID, paymentID, runID, auditID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO pg_temp.commission_payment_targets(id,brand_id,payment_id,earning_id,points,member_id,agent_id,payment_version,state)
	 VALUES($2,$1,$3,$4,1,$5,$6,1,'pending')`, upgradeBrand, targetID, paymentID, earningID, member, agentID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `CREATE FUNCTION pg_temp.commission_payment_evidence_current(uuid,uuid,uuid,bigint) RETURNS boolean LANGUAGE sql AS $$ SELECT true $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION pg_temp.point_zero_snapshot() RETURNS jsonb LANGUAGE sql AS $$ SELECT %s.point_zero_snapshot() $$`, quotedSchema)); err != nil {
		t.Fatal(err)
	}

	var delta points.Balance
	delta[3][0] = 1
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	missingTargetID := ids.New()
	_, err = (points.Store{DB: db}).Post(ctx, tx, points.Change{
		BrandID: upgradeBrand, MemberID: member, EntryType: "commission", ReferenceType: "commission_payment_target",
		ReferenceID: missingTargetID, OperationKey: "commission-payment:" + missingTargetID,
		Reason: "reject temporary authorization spoof", ActorType: "system", RequestID: ids.New(), Delta: delta,
		Allocation: []points.Allocation{{Source: "commission", State: "available", Points: 1}},
	})
	_ = tx.Rollback(ctx)
	if err == nil || !strings.Contains(err.Error(), "commission credit missing cycle") {
		t.Fatalf("temporary fake target/function authorized a business credit: err=%v", err)
	}
}
