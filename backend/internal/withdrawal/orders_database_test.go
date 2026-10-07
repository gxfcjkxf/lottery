package withdrawal

import (
	"context"
	"errors"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/compliance"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestWithdrawalDatabaseRejectsRewritingFactsOrUnwitnessedStates(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 25}}
	f.fund(t, allocation)
	order := createOrder(t, f, 25, "database-order-original", allocation)
	cases := []struct {
		name, sql string
		args      []any
	}{
		{"amount mutation", `UPDATE withdrawal_orders SET points=26 WHERE id=$1`, []any{order.ID}},
		{"source mutation", `UPDATE withdrawal_orders SET source_allocation='[]' WHERE id=$1`, []any{order.ID}},
		{"policy mutation", `UPDATE withdrawal_orders SET policy_snapshot='{}' WHERE id=$1`, []any{order.ID}},
		{"qualification mutation", `UPDATE withdrawal_orders SET eligibility_evidence='{}' WHERE id=$1`, []any{order.ID}},
		{"order deletion", `DELETE FROM withdrawal_orders WHERE id=$1`, []any{order.ID}},
		{"history rewrite", `UPDATE withdrawal_order_transitions SET reason='tampered' WHERE order_id=$1`, []any{order.ID}},
		{"receipt deletion", `DELETE FROM withdrawal_operation_receipts WHERE order_id=$1`, []any{order.ID}},
		{"unwitnessed approval", `UPDATE withdrawal_orders SET state='processing',version=version+1,reviewed_at=clock_timestamp(),updated_at=clock_timestamp() WHERE id=$1`, []any{order.ID}},
		{"invented successful cycle", `INSERT INTO withdrawal_turnover_cycles(brand_id,member_id,account_id,cutoff_at,cutoff_version,last_paid_order_id) SELECT brand_id,member_id,account_id,created_at,reserve_version,id FROM withdrawal_orders WHERE id=$1`, []any{order.ID}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			tx, err := f.db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			_, err = tx.Exec(ctx, c.sql, c.args...)
			if err == nil {
				err = tx.Commit(ctx)
			}
			var databaseError *pgconn.PgError
			if !errors.As(err, &databaseError) || databaseError.Code != "P0001" && databaseError.Code != "23514" {
				t.Fatalf("expected evidence/immutability rejection, got %v", err)
			}
		})
	}
	current, err := f.service.Read(context.Background(), orderTestBrand, order.ID)
	if err != nil || current.Version != 1 || current.State != "reviewing" || current.Points != 25 {
		t.Fatalf("rejected tampering changed order: %+v err=%v", current, err)
	}
	wallet, err := f.points.Read(context.Background(), orderTestBrand, f.member)
	if err != nil || wallet.AvailablePoints != 0 || wallet.WithdrawalPoints != 25 {
		t.Fatalf("rejected tampering changed funds: %+v err=%v", wallet, err)
	}
}

func TestWithdrawalConfiguredComplianceCannotBeBypassedByEligibilityChecker(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 25}}
	f.fund(t, allocation)
	ctx := context.Background()
	actor := f.admin
	actor.Roles = append([]access.Role(nil), actor.Roles...)
	actor.Roles = append(actor.Roles, access.Role{BrandID: orderTestBrand, Permissions: []access.Permission{{Resource: "compliance_policy", Action: "write", Scope: access.ScopeBrand}}})
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, err = (compliance.Service{DB: f.db}).Update(ctx, tx, orderTestBrand, actor, compliance.Input{Version: 1, Config: compliance.Config{IdentityEnabled: true, AllowedCountries: []string{}}, Reason: "require verified identity adapter"}, points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = createOrderResult(f, OrderInput{Points: 25, SourceAllocation: allocation, ClientKey: "compliance-disabled-adapter"}); !errors.Is(err, ErrIneligible) {
		t.Fatalf("trusted turnover checker bypassed missing identity verification: %v", err)
	}
	wallet, err := f.points.Read(ctx, orderTestBrand, f.member)
	if err != nil || wallet.AvailablePoints != 25 || wallet.WithdrawalPoints != 0 {
		t.Fatalf("blocked request reserved funds: %+v err=%v", wallet, err)
	}
}

func TestWithdrawalFreshPermissionRevocationAlsoBlocksReceiptReplay(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 25}}
	f.fund(t, allocation)
	order := createOrder(t, f, 25, "fresh-revocation-original", allocation)
	input := ActionInput{Version: 1, ClientKey: "fresh-revocation-approve", Reason: "single reviewer approves"}
	meta := points.Metadata{ActorType: "admin", ActorID: f.admin.ID, RequestID: ids.New()}
	if _, err := orderAction(t, f, order.ID, "approve", input.ClientKey, f.admin, input, meta); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(context.Background(), `DELETE FROM role_permissions WHERE permission_key='withdrawal.approve.brand' AND role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1)`, f.admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := orderAction(t, f, order.ID, "approve", input.ClientKey, f.admin, input, meta); !errors.Is(err, ErrDenied) {
		t.Fatalf("stale permissions replayed success: %v", err)
	}
	history, err := f.service.History(context.Background(), orderTestBrand, order.ID)
	if err != nil || len(history) != 2 || history[0].FromState != "" || history[1].FromState != "reviewing" || history[1].ToState != "processing" {
		t.Fatalf("immutable history incorrect: %+v err=%v", history, err)
	}
	if _, err = f.service.History(context.Background(), orderOtherBrand, order.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("history crossed brands: %v", err)
	}
}

func TestWithdrawalAuditFailureRollsBackReservationAndOriginalSourceRelease(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 30}, {Source: "gift", State: "available", Points: 20}}
	f.fund(t, allocation)
	ctx := context.Background()
	if _, err := f.db.Exec(ctx, `CREATE FUNCTION reject_withdrawal_test_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action IN('withdrawal.reviewing','withdrawal.rejected') THEN RAISE EXCEPTION 'injected withdrawal audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_withdrawal_test_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_withdrawal_test_audit()`); err != nil {
		t.Fatal(err)
	}
	input := OrderInput{Points: 50, SourceAllocation: allocation, ClientKey: "audited-create-original"}
	if _, err := createOrderResult(f, input); err == nil {
		t.Fatal("reservation survived failed mandatory audit")
	}
	wallet, err := f.points.Read(ctx, orderTestBrand, f.member)
	if err != nil || wallet.AvailablePoints != 50 || wallet.WithdrawalPoints != 0 {
		t.Fatalf("failed create audit changed wallet: %+v err=%v", wallet, err)
	}
	var count int
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM withdrawal_orders`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed creation left order: %d (%v)", count, err)
	}
	if _, err = f.db.Exec(ctx, `DROP TRIGGER reject_withdrawal_test_audit ON audit_logs`); err != nil {
		t.Fatal(err)
	}
	order, err := createOrderResult(f, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(ctx, `CREATE TRIGGER reject_withdrawal_test_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_withdrawal_test_audit()`); err != nil {
		t.Fatal(err)
	}
	action := ActionInput{Version: 1, ClientKey: "audited-reject-original", Reason: "declined after review"}
	meta := points.Metadata{ActorType: "admin", ActorID: f.admin.ID, RequestID: ids.New()}
	if _, err = orderAction(t, f, order.ID, "reject", action.ClientKey, f.admin, action, meta); err == nil {
		t.Fatal("release survived failed mandatory audit")
	}
	wallet, err = f.points.Read(ctx, orderTestBrand, f.member)
	if err != nil || wallet.AvailablePoints != 0 || wallet.WithdrawalPoints != 50 {
		t.Fatalf("failed release audit changed wallet: %+v err=%v", wallet, err)
	}
	current, err := f.service.Read(ctx, orderTestBrand, order.ID)
	if err != nil || current.State != "reviewing" || current.Version != 1 {
		t.Fatalf("failed audit advanced order: %+v (%v)", current, err)
	}
	if _, err = f.db.Exec(ctx, `DROP TRIGGER reject_withdrawal_test_audit ON audit_logs`); err != nil {
		t.Fatal(err)
	}
	if _, err = orderAction(t, f, order.ID, "reject", action.ClientKey, f.admin, action, meta); err != nil {
		t.Fatal(err)
	}
	wallet, err = f.points.Read(ctx, orderTestBrand, f.member)
	if err != nil || wallet.BySource[0][0] != 30 || wallet.BySource[2][0] != 20 || wallet.WithdrawalPoints != 0 {
		t.Fatalf("explicit retry failed original-source return: %+v (%v)", wallet, err)
	}
}
