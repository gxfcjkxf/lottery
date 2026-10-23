package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/finance"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"testing"
	"time"
)

func TestOperatorWritesWithoutReasonRetainAuditAndWalletGuards(t *testing.T) {
	f := safetyFixture(t)
	ctx := context.Background()
	mustStatus(t, f.call("PATCH", "/api/v1/admin/users/"+f.memberID, "optional-member-edit-001", f.token, managedBrand, map[string]any{"status": "normal", "notes": "updated without an operator reason"}), 200)
	mustStatus(t, f.call("PUT", "/api/v1/admin/point-policy", "optional-point-policy-001", f.token, managedBrand, map[string]any{"version": 1, "max_balance_points": nil, "max_recharge_points": nil, "max_adjustment_points": nil}), 200)
	create := f.call("POST", "/api/v1/admin/recharges", "optional-recharge-create-001", f.token, managedBrand, map[string]string{"member_id": f.memberID, "points": "10"})
	mustStatus(t, create, 201)
	var recharge finance.Recharge
	managedData(t, create, &recharge)
	confirmPath := "/api/v1/admin/recharges/" + recharge.ID + "/confirm"
	confirmBody := map[string]any{"version": recharge.Version}
	confirm := f.call("POST", confirmPath, "optional-recharge-confirm-001", f.token, managedBrand, confirmBody)
	mustStatus(t, confirm, 200)
	var confirmed, replayed finance.Recharge
	managedData(t, confirm, &confirmed)
	replay := f.call("POST", confirmPath, "optional-recharge-confirm-001", f.token, managedBrand, confirmBody)
	mustStatus(t, replay, 200)
	managedData(t, replay, &replayed)
	if confirmed.LedgerEntryID == "" || replayed.LedgerEntryID != confirmed.LedgerEntryID {
		t.Fatal("reason-free replay lost its original ledger receipt")
	}
	mustStatus(t, f.call("POST", confirmPath, "optional-recharge-duplicate-001", f.token, managedBrand, confirmBody), 409)
	var actor, reason string
	var at time.Time
	var snapshots bool
	if err := f.pool.QueryRow(ctx, `SELECT actor_id::text,reason,created_at,before_json<>after_json FROM audit_logs WHERE id=$1`, confirmed.AuditLogID).Scan(&actor, &reason, &at, &snapshots); err != nil {
		t.Fatal(err)
	}
	if actor != f.root || reason != "" || at.IsZero() || !snapshots {
		t.Fatalf("audit lost actor, time or before/after evidence: %s %q %s %v", actor, reason, at, snapshots)
	}
	walletPath := "/api/v1/admin/wallets/" + f.memberID
	freeze := f.call("POST", walletPath+"/freeze", "optional-freeze-001", f.token, managedBrand, map[string]string{"points": "3"})
	mustStatus(t, freeze, 200)
	var frozen points.Entry
	managedData(t, freeze, &frozen)
	mustStatus(t, f.call("POST", walletPath+"/unfreeze", "optional-unfreeze-001", f.token, managedBrand, map[string]string{"entry_id": frozen.ID}), 200)
	mustStatus(t, f.call("POST", walletPath+"/adjust", "optional-adjust-001", f.token, managedBrand, map[string]string{"source": "gift", "delta": "2"}), 200)
	mustStatus(t, f.call("POST", walletPath+"/adjust", "optional-underflow-001", f.token, managedBrand, map[string]string{"source": "gift", "delta": "-3"}), 409)
	mustStatus(t, f.call("POST", "/api/v1/admin/recharges", "optional-cross-brand-001", f.token, pointsBrandB, map[string]string{"member_id": f.memberID, "points": "1"}), 403)
	wallet := pointWallet(t, f)
	if wallet.DisplayPoints != 12 || wallet.AvailablePoints != 12 || wallet.FrozenPoints != 0 {
		t.Fatalf("wallet protection changed: %+v", wallet)
	}
	var count int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE reference_id=$1 AND entry_type='recharge'`, recharge.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("reason-free confirm produced %d ledger entries", count)
	}
}
