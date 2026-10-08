package rewards

import (
	"context"
	"testing"
)

func TestFinanceBusinessWitnessesIncludeRealGrantAndRevocation(t *testing.T) {
	f := newRewardFixture(t)
	order := rewardGrant(t, f, 25)
	if _, err := rewardRevoke(t, f, order, f.actor, "full approved reversal"); err != nil {
		t.Fatal(err)
	}
	rows, err := f.db.Query(context.Background(), `SELECT family,source_id::text,ledger_id::text,source_valid,expected_entry_type
	 FROM point_finance_business_witnesses($1,$2) ORDER BY expected_entry_type`, f.brand, f.account)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := 0
	for rows.Next() {
		var family, sourceID, ledgerID, entryType string
		var valid bool
		if err := rows.Scan(&family, &sourceID, &ledgerID, &valid, &entryType); err != nil {
			t.Fatal(err)
		}
		if family != "reward" || !valid || ledgerID == "" || entryType != "reward_grant" && entryType != "reward_reversal" {
			t.Fatalf("invalid reward witness: family=%s source=%s ledger=%s valid=%t entry=%s", family, sourceID, ledgerID, valid, entryType)
		}
		got++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Fatalf("got %d reward financial witnesses, want grant and full revocation", got)
	}
	rows.Close()
	var consistent bool
	if err = f.db.QueryRow(context.Background(), `SELECT (point_business_preview($1,$2)->>'consistent')::boolean`, f.brand, f.account).Scan(&consistent); err != nil || !consistent {
		t.Fatalf("full kernel rejected actual reward grant/revocation: consistent=%v err=%v", consistent, err)
	}
}

func TestFinanceBusinessWitnessesRetainMissingRequiredRewardPointer(t *testing.T) {
	f := newRewardFixture(t)
	order := rewardGrant(t, f, 11)
	if _, err := rewardRevoke(t, f, order, f.actor, "full approved reversal"); err != nil {
		t.Fatal(err)
	}
	orderID := rewardID(t, order)
	ctx := context.Background()
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `ALTER TABLE reward_orders DISABLE TRIGGER guarded_reward_order;
		ALTER TABLE reward_order_actions DISABLE TRIGGER guarded_reward_action`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `DO $$ DECLARE constraint_name text; BEGIN
		 SELECT conname INTO constraint_name FROM pg_constraint
		 WHERE conrelid='reward_orders'::regclass AND contype='c'
		 AND pg_get_constraintdef(oid) LIKE '%revoke_ledger_entry_id%';
		 IF constraint_name IS NULL THEN RAISE EXCEPTION 'revocation pointer check constraint not found'; END IF;
		 EXECUTE format('ALTER TABLE reward_orders DROP CONSTRAINT %I',constraint_name);
		END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE reward_orders SET grant_ledger_entry_id=NULL WHERE id=$1`, orderID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE reward_order_actions SET ledger_entry_id=NULL WHERE order_id=$1 AND operation='grant'`, orderID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE reward_orders SET revoke_ledger_entry_id=NULL WHERE id=$1`, orderID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM reward_order_actions WHERE order_id=$1 AND operation IN('revoke','retry')`, orderID); err != nil {
		t.Fatal(err)
	}
	var grants, reversals, missingGrantLedger, validCount int
	if err = tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE expected_entry_type='reward_grant'),
	 count(*) FILTER(WHERE expected_entry_type='reward_reversal'),
		 count(*) FILTER(WHERE ledger_id IS NULL),count(*) FILTER(WHERE source_valid)
	 FROM point_finance_business_witnesses($1,$2) WHERE family='reward' AND source_id=$3`,
		f.brand, f.account, orderID).Scan(&grants, &reversals, &missingGrantLedger, &validCount); err != nil {
		t.Fatal(err)
	}
	if grants != 1 || reversals != 1 || missingGrantLedger != 2 || validCount != 0 {
		t.Fatalf("missing required reward evidence witnesses: grants=%d reversals=%d missing_ledgers=%d valid=%d; want both invalid rows", grants, reversals, missingGrantLedger, validCount)
	}
	// Rolling back restores both the corrupted pointer and the explicitly
	// disabled guard without committing the deliberately invalid row.
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var guardEnabled bool
	if err = f.db.QueryRow(ctx, `SELECT count(*)=2 FROM pg_trigger WHERE tgenabled='O' AND
	 (tgrelid,tgname) IN (('reward_orders'::regclass,'guarded_reward_order'),('reward_order_actions'::regclass,'guarded_reward_action'))`).Scan(&guardEnabled); err != nil || !guardEnabled {
		t.Fatalf("reward guards not restored after rollback: enabled=%t err=%v", guardEnabled, err)
	}
}
