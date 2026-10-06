package betting

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSettlementJobFailureRollsBackLedgerAndOnlyManualRetryResumes(t *testing.T) {
	f, o := drawnFixtureOrder(t)
	ctx := context.Background()
	a := settlementActor(f)
	mode := "automatic"
	p := setSettlementMode(t, f, a, 1, &mode)
	j := startFixtureSettlement(t, f, a, p.Version)
	// Fault injection only in this disposable schema; simulate storage refusing a
	// prize insert after the worker has locked the wallet. Never fake a receipt.
	if _, e := f.db.Exec(ctx, `CREATE FUNCTION test_reject_prize() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.entry_type='prize' THEN RAISE EXCEPTION 'test payout outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_reject_prize BEFORE INSERT ON point_ledger_entries FOR EACH ROW EXECUTE FUNCTION test_reject_prize()`); e != nil {
		t.Fatal(e)
	}
	before := walletBySource(t, f)
	if _, e := f.service.ProcessSettlements(ctx, 20); e == nil {
		t.Fatal("storage failure swallowed")
	}
	j, e := f.service.SettlementJob(ctx, f.brand, j.ID)
	if e != nil || j.State != "failed" || j.FailedCount != 1 || j.PaidPoints != "0" || walletBySource(t, f) != before {
		t.Fatal(j, e)
	}
	var count int
	if e = f.db.QueryRow(ctx, `SELECT count(*) FROM settlement_failures WHERE job_id=$1`, j.ID).Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
	if _, e = f.db.Exec(ctx, `DROP TRIGGER test_reject_prize ON point_ledger_entries`); e != nil {
		t.Fatal(e)
	}
	if n, e := f.service.ProcessSettlements(ctx, 20); e != nil || n != 0 {
		t.Fatal("unapproved auto retry", n, e)
	}
	if _, e = actOnFixtureSettlement(t, f, a, j, "retry", j.Version); e != nil {
		t.Fatal(e)
	}
	// Two independent worker instances contend on the same durable job.
	results := make(chan error, 2)
	for range 2 {
		go func() { _, e := (Service{DB: f.db}).ProcessSettlements(ctx, 20); results <- e }()
	}
	for range 2 {
		if e = <-results; e != nil {
			t.Fatal(e)
		}
	}
	if _, e = f.service.ProcessSettlements(ctx, 20); e != nil {
		t.Fatal(e)
	}
	fresh, e := f.service.Order(ctx, f.brand, f.member, o.ID)
	if e != nil || fresh.Status != "won" || fresh.PrizePoints != 10 || fresh.PayoutEntryID == nil || walletBySource(t, f)[1][0] != 10 {
		t.Fatal(fresh, e)
	}
	if e = f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='prize'`).Scan(&count); e != nil || count != 1 {
		t.Fatal("duplicate payout", count, e)
	}
	if _, e = f.db.Exec(ctx, `DELETE FROM settlement_failures WHERE job_id=$1`, j.ID); e == nil {
		t.Fatal("mutable failure history")
	}
}
func TestSettlementJobCorruptSnapshotIsWholeOrderAbnormalAndExcluded(t *testing.T) {
	f, o := drawnFixtureOrder(t)
	ctx := context.Background()
	a := settlementActor(f)
	mode := "automatic"
	if _, e := f.db.Exec(ctx, `ALTER TABLE bet_orders DISABLE TRIGGER immutable_bet_order; UPDATE bet_orders SET selection_normalized='{}'; ALTER TABLE bet_orders ENABLE TRIGGER immutable_bet_order`); e != nil {
		t.Fatal(e)
	}
	p := setSettlementMode(t, f, a, 1, &mode)
	j := startFixtureSettlement(t, f, a, p.Version)
	before := walletBySource(t, f)
	if _, e := f.service.ProcessSettlements(ctx, 20); e != nil {
		t.Fatal(e)
	}
	j, e := f.service.SettlementJob(ctx, f.brand, j.ID)
	if e != nil || j.State != "completed" || j.ExcludedCount != 1 || j.PaidCount != 0 || walletBySource(t, f) != before {
		t.Fatal(j, e)
	}
	x, e := f.service.Exception(ctx, f.brand, o.ID)
	if e != nil || x == nil || x.Source != "system" || x.MarkedBy != "" || x.JobID == nil || x.ErrorCode == nil {
		t.Fatal(x, e)
	}
	// Terminal targets cannot be admitted to ordinary retry or payout.
	tx, e := f.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `UPDATE settlement_targets SET state='pending',version=version+1 WHERE job_id=$1`, j.ID); e == nil {
		t.Fatal("excluded target restarted")
	}
}
func TestSettlementWriteInputsRejectMissingDuplicateAndNullFields(t *testing.T) {
	for _, raw := range []string{`{}`, `{"version":1,"reason":"x"}`, `{"version":1,"mode":null,"reason":null}`, `{"version":1,"mode":null,"reason":"x","mode":"manual"}`, `{"version":1,"mode":null,"reason":"x","extra":true}`} {
		var in SettlementPolicyInput
		if json.Unmarshal([]byte(raw), &in) == nil {
			t.Fatal("accepted malformed input", raw)
		}
	}
	var in SettlementPolicyInput
	if e := json.Unmarshal([]byte(`{"version":1,"mode":null,"reason":"explicit disable"}`), &in); e != nil {
		t.Fatal(e)
	}
}

func TestSettlementJobCompletionStorageFailureNeedsManualRetryWithoutRePayout(t *testing.T) {
	f, _ := drawnFixtureOrder(t)
	ctx := context.Background()
	a := settlementActor(f)
	mode := "automatic"
	p := setSettlementMode(t, f, a, 1, &mode)
	j := startFixtureSettlement(t, f, a, p.Version)
	if _, e := f.db.Exec(ctx, `CREATE FUNCTION test_reject_settled() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.status='settled' THEN RAISE EXCEPTION 'test finalization outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_reject_settled BEFORE UPDATE ON periods FOR EACH ROW EXECUTE FUNCTION test_reject_settled()`); e != nil {
		t.Fatal(e)
	}
	if _, e := f.service.ProcessSettlements(ctx, 20); e == nil {
		t.Fatal("finalization failure swallowed")
	}
	j, e := f.service.SettlementJob(ctx, f.brand, j.ID)
	if e != nil || j.State != "failed" || j.FailedCount != 0 || j.PaidCount != 1 || j.PaidPoints != "10" {
		t.Fatal(j, e)
	}
	var n int
	if e = f.db.QueryRow(ctx, `SELECT count(*) FROM settlement_failures WHERE job_id=$1 AND order_id IS NULL AND phase='paying'`, j.ID).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if _, e = f.db.Exec(ctx, `DROP TRIGGER test_reject_settled ON periods`); e != nil {
		t.Fatal(e)
	}
	if n, e := f.service.ProcessSettlements(ctx, 20); e != nil || n != 0 {
		t.Fatal("auto finalization retry", n, e)
	}
	if _, e = actOnFixtureSettlement(t, f, a, j, "retry", j.Version); e != nil {
		t.Fatal(e)
	}
	if _, e = f.service.ProcessSettlements(ctx, 20); e != nil {
		t.Fatal(e)
	}
	j, e = f.service.SettlementJob(ctx, f.brand, j.ID)
	if e != nil || j.State != "completed" || walletBySource(t, f)[1][0] != 10 {
		t.Fatal(j, e)
	}
	if e = f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='prize'`).Scan(&n); e != nil || n != 1 {
		t.Fatal("repayout on phase retry", n, e)
	}
}
