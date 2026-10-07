package betting

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

func TestCommissionDiscoveryUpgradeQueuesActualHistoricalSnapshotsOnly(t *testing.T) {
	db := testdb.NewAtVersion(t, 48)
	f := newCommissionBatchFixtureFromBetting(t, newBettingFixtureWithDBWindow(t, db, storeTestBrand, 20*time.Second, 22*time.Second))
	ctx := context.Background()
	// Place a real policy-disabled order under the old schema as well. It
	// must not be assigned the preceding enabled policy during migration.
	current, err := f.service.Policy(ctx, f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	bettingTx(t, db, func(tx pgx.Tx) error {
		_, e := f.service.Update(ctx, tx, f.betting.brand, f.actor, commission.PolicyInput{Version: current.Version, Config: commission.PolicyConfig{Enabled: false, PayoutMode: commission.PayoutManual}, Reason: "disable new commission participation before real upgrade"}, points.Metadata{ActorType: "admin", ActorID: f.actor.ID, RequestID: ids.New()})
		return e
	})
	disabled, err := placeBettingOrder(t, f.betting, f.betting.input, "discovery-upgrade-disabled-bet")
	if err != nil {
		t.Fatal(err)
	}
	manual := func() commission.Cycle { waitCommissionBoundary(t, f.boundary); return createCommissionCycle(t, f) }()
	fingerprint := func() string {
		var raw string
		err := db.QueryRow(ctx, `SELECT jsonb_build_object('orders',(SELECT jsonb_agg(to_jsonb(o) ORDER BY id) FROM bet_orders o),
		 'wallets',(SELECT jsonb_agg(to_jsonb(b) ORDER BY account_id,source,state) FROM point_buckets b),
		 'ledger',(SELECT jsonb_agg(to_jsonb(l) ORDER BY id) FROM point_ledger_entries l),
		 'audits',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM audit_logs a))::text`).Scan(&raw)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	before := fingerprint()
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if fingerprint() != before {
		t.Fatal("discovery migration rewrote economic or audit history")
	}
	var count int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM commission_discovery`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("enabled historical queues=%d err=%v", count, err)
	}
	wrongFrom := manual.WindowFrom.Add(time.Second)
	// Both wrong bounds contain the real placement time: these specifically
	// verify calendar equality, not just the older containment constraint.
	guardTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, guardErr := guardTx.Exec(ctx, `UPDATE commission_discovery SET window_from=$2,window_to=$3 WHERE id=$1`, f.orders[0].ID, wrongFrom, manual.WindowTo)
	_ = guardTx.Rollback(ctx)
	if guardErr == nil || !strings.Contains(guardErr.Error(), "calendar window mismatch") {
		t.Fatalf("wrong discovery calendar bounds accepted: %v", guardErr)
	}
	guardTx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wrongCycle := ids.New()
	log, err := audit.Append(ctx, guardTx, audit.Record{BrandID: f.betting.brand, ActorType: "admin", ActorID: f.actor.ID, Action: "commission.cycle.create", ResourceType: "commission_cycle", ResourceID: wrongCycle, Reason: "test incorrect historical window", RequestID: ids.New(), After: map[string]any{"version": 1, "state": "enumerating", "anchor_order_id": f.orders[0].ID, "window_from": wrongFrom, "window_to": manual.WindowTo, "calendar": manual.Calendar}})
	if err != nil {
		_ = guardTx.Rollback(ctx)
		t.Fatal(err)
	}
	calendar, _ := json.Marshal(manual.Calendar)
	_, guardErr = guardTx.Exec(ctx, `INSERT INTO commission_cycles(id,brand_id,window_from,window_to,anchor_order_id,calendar,created_by,reason,creation_audit_log_id) VALUES($1,$2,$3,$4,$5,$6,$7,'test incorrect historical window',$8)`, wrongCycle, f.betting.brand, wrongFrom, manual.WindowTo, f.orders[0].ID, calendar, f.actor.ID, log)
	_ = guardTx.Rollback(ctx)
	if guardErr == nil || !strings.Contains(guardErr.Error(), "calendar window mismatch") {
		t.Fatalf("wrong cycle calendar bounds accepted: %v", guardErr)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cycle, err := f.service.CycleTx(ctx, tx, f.betting.brand, manual.ID)
	_ = tx.Rollback(ctx)
	if err != nil || cycle.CreationActorType != "admin" || cycle.CreatedBy == nil || *cycle.CreatedBy != f.actor.ID {
		t.Fatalf("historical creator was changed: %+v err=%v", cycle, err)
	}
	if n, e := f.service.ProcessDiscovery(ctx, 100); e != nil || n != 3 {
		t.Fatalf("historical discovery processed=%d err=%v", n, e)
	}
	for _, order := range f.orders {
		d := commissionDiscoveryRead(t, f, order.ID)
		if d.State != "registered" || d.CycleID == nil || *d.CycleID != manual.ID {
			t.Fatalf("historical manual cycle not reused: %+v", d)
		}
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM commission_discovery WHERE id=$1`, disabled.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("disabled order was retroactively authorized: %d err=%v", count, err)
	}
	// The public projection keeps nullable creator provenance exact without
	// exposing private source snapshots to the management history reader.
	raw, err := json.Marshal(cycle)
	if err != nil || !json.Valid(raw) {
		t.Fatal("historical creator DTO cannot serialize")
	}
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM commission_discovery`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("migration replay duplicated queue: %d err=%v", count, err)
	}
}
