package betting

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/attribution"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

// This fixture sets automatic payout before creating any orders, so their
// immutable commission-policy snapshots—not a later policy edit—authorize it.
func newAutomaticCommissionBatchFixture(t *testing.T) commissionBatchFixture {
	t.Helper()
	f := newBettingFixtureWithWindow(t, storeTestBrand, 20*time.Second, 22*time.Second)
	ctx := context.Background()
	actor := access.Account{
		ID: f.version.CreatedBy, Type: access.AccountAdmin, BrandIDs: []string{f.brand},
		Roles: []access.Role{{BrandID: f.brand, Permissions: []access.Permission{
			{Resource: "agent_policy", Action: "write", Scope: access.ScopeBrand},
			{Resource: "agent", Action: "write", Scope: access.ScopeBrand},
			{Resource: "join_code", Action: "write", Scope: access.ScopeBrand},
			{Resource: "commission_policy", Action: "write", Scope: access.ScopeBrand},
			{Resource: "commission", Action: "view", Scope: access.ScopeBrand},
			{Resource: "commission", Action: "run", Scope: access.ScopeBrand},
			{Resource: "commission", Action: "retry", Scope: access.ScopeBrand},
		}}},
	}
	agents := agency.Service{DB: f.db}
	financial := commission.Service{DB: f.db}
	codes := attribution.Service{DB: f.db}
	var agentPolicy agency.Policy
	var root agency.Node
	var code attribution.Code
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		agentPolicy, err = agents.SavePolicy(ctx, tx, f.brand, actor, agency.PolicyInput{
			Version: 1,
			Config:  agency.PolicyConfig{Enabled: true, MaxDepth: 3, RatioCap: "0.3", Mode: "loss", Cycle: "weekly"},
			Reason:  "automatic commission payment fixture agent policy",
		}, points.Metadata{RequestID: ids.New()})
		return err
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		root, err = agents.Create(ctx, tx, f.brand, actor, agency.CreateInput{
			PolicyVersion: agentPolicy.Version, MemberID: f.member,
			Config: agency.NodeConfig{Ratio: "0.3", Status: "active", CanCreateChildren: true},
			Reason: "automatic commission payment fixture beneficiary",
		}, points.Metadata{RequestID: ids.New()})
		return err
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		code, err = codes.Create(ctx, tx, f.brand, actor, attribution.CreateInput{Kind: "agent", OwnerMemberID: f.member, AgentID: &root.ID, Reason: "automatic commission fixture registration code"}, points.Metadata{RequestID: ids.New()})
		return err
	})
	users, err := identity.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	var session identity.Authentication
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		out, err := users.Register(ctx, tx, f.brand, identity.RegisterInput{
			Username: "bet_auto_" + strings.ReplaceAll(ids.New(), "-", "")[:12],
			Password: "test-commission-batch-password", Privacy: "dev-1", Terms: "dev-1", AgentCode: code.Code,
		}, identity.Metadata{Domain: "aurora.localhost"})
		if err == nil {
			if out.Status != 201 {
				t.Fatalf("automatic commission register status=%d error=%+v", out.Status, out.Error)
			}
			err = json.Unmarshal(out.Data, &session)
		}
		return err
	})
	f.user, err = users.Authenticate(ctx, f.brand, session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	f.member, f.token = session.Member.ID, session.AccessToken

	boundary := f.period.BetEndAt.UTC().Truncate(time.Second).Add(-3 * time.Second)
	weekday := int(boundary.Weekday())
	calendar := commission.Calendar{Timezone: "UTC", Cycle: "weekly", BoundaryTime: boundary.Format("15:04:05"), Weekday: &weekday}
	current, err := financial.Policy(ctx, f.brand)
	if err != nil {
		t.Fatal(err)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, err := financial.Update(ctx, tx, f.brand, actor, commission.PolicyInput{
			Version: current.Version,
			Config:  commission.PolicyConfig{Enabled: true, Calendar: &calendar, PayoutMode: commission.PayoutAutomatic},
			Reason:  "capture automatic payout in actual bet-time snapshots",
		}, points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()})
		return err
	})
	f.service = Service{DB: f.db}
	fundBettingWallet(t, f, 10)
	in := f.input
	in.Selection = rules.Selection{Digits: [][]int{{0}, {0}, {0}}}
	first, err := placeBettingOrder(t, f, in, "commission-auto-lost-one")
	if err != nil {
		t.Fatal(err)
	}
	second, err := placeBettingOrder(t, f, in, "commission-auto-lost-two")
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := placeBettingOrder(t, f, in, "commission-auto-cancelled")
	if err != nil {
		t.Fatal(err)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, err := f.service.CancelAdmin(ctx, tx, f.brand, judgeActor(f), cancelled.ID, cancelled.Version, "cancelled stake excluded from automatic payout", policyMeta(actor.ID))
		return err
	})
	return commissionBatchFixture{betting: f, service: financial, actor: actor, node: root, orders: []Order{first, second, cancelled}, boundary: boundary}
}

func commissionPaymentActor(f commissionBatchFixture) access.Account {
	a := f.actor
	for i := range a.Roles {
		if a.Roles[i].BrandID != f.betting.brand {
			continue
		}
		a.Roles[i].Permissions = append(a.Roles[i].Permissions,
			access.Permission{Resource: "commission_payment_policy", Action: "write", Scope: access.ScopeBrand},
			access.Permission{Resource: "commission_payment", Action: "approve", Scope: access.ScopeBrand},
			access.Permission{Resource: "commission_payment", Action: "retry", Scope: access.ScopeBrand},
		)
	}
	return a
}

func commissionPaymentMeta(a access.Account) points.Metadata {
	return points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()}
}

func commissionPaymentCallTx(t *testing.T, f commissionBatchFixture, call func(pgx.Tx) error) error {
	t.Helper()
	tx, err := f.betting.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err = call(tx); err != nil {
		return err
	}
	return tx.Commit(context.Background())
}

func commissionPaymentPolicy(t *testing.T, f commissionBatchFixture) commission.PaymentPolicy {
	t.Helper()
	tx, err := f.betting.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	policy, err := f.service.PaymentPolicyTx(context.Background(), tx, f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func commissionPaymentRead(t *testing.T, f commissionBatchFixture, id string) commission.Payment {
	t.Helper()
	tx, err := f.betting.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	payment, err := f.service.PaymentTx(context.Background(), tx, f.betting.brand, id)
	if err != nil {
		t.Fatal(err)
	}
	return payment
}

func commissionPayments(t *testing.T, f commissionBatchFixture) commission.PaymentPage {
	t.Helper()
	tx, err := f.betting.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	page, err := f.service.PaymentsTx(context.Background(), tx, f.betting.brand, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func updateCommissionPaymentPolicy(t *testing.T, f commissionBatchFixture, a access.Account, enabled bool, version int64) error {
	t.Helper()
	return commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.UpdatePaymentPolicyTx(context.Background(), tx, f.betting.brand, a,
			commission.PaymentPolicyInput{Version: version, Enabled: enabled, Reason: "integration test payment gate change"}, commissionPaymentMeta(a))
		return err
	})
}

func prepareManualCommissionPayment(t *testing.T, f commissionBatchFixture) commission.Payment {
	t.Helper()
	ctx := context.Background()
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 30)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || cycle.TotalPoints != "1" || cycle.EarningCount != "1" {
		t.Fatalf("fixture did not calculate exact 0.6 commission to 1 point: %+v", cycle)
	}
	actor := commissionPaymentActor(f)
	policy := commissionPaymentPolicy(t, f)
	if policy.Enabled {
		t.Fatalf("new brand payment gate unexpectedly enabled: %+v", policy)
	}
	if steps, err := f.service.ProcessPayments(ctx, 20); err != nil || steps != 0 {
		t.Fatalf("closed payment gate processed=%d err=%v", steps, err)
	}
	if got := commissionPayments(t, f); len(got.Items) != 0 {
		t.Fatalf("closed payment gate registered payments: %+v", got.Items)
	}
	if err := updateCommissionPaymentPolicy(t, f, actor, true, policy.Version); err != nil {
		t.Fatal(err)
	}
	if steps, err := f.service.ProcessPayments(ctx, 20); err != nil || steps == 0 {
		t.Fatalf("open payment gate failed to register manual payment: steps=%d err=%v", steps, err)
	}
	page := commissionPayments(t, f)
	if len(page.Items) != 1 {
		t.Fatalf("registered payment page=%+v; want one payment", page)
	}
	payment := page.Items[0]
	if payment.State != "awaiting_approval" || payment.PayoutMode != commission.PayoutManual || payment.RunID != *cycle.CurrentRunID || payment.TotalPoints != "1" || payment.PaidPoints != "0" || payment.PaidCount != "0" || payment.TargetCount != "1" {
		t.Fatalf("manual payment did not preserve ready run and one-point payout: %+v", payment)
	}
	return payment
}

func TestCommissionPaymentManualApprovalIsAtomicAndRetryable(t *testing.T) {
	f := newCommissionBatchFixture(t)
	ctx := context.Background()
	actor := commissionPaymentActor(f)
	payment := prepareManualCommissionPayment(t, f)
	if wallet := walletBySource(t, f.betting); wallet[3][0] != 0 {
		t.Fatalf("manual payment credited before approval: %+v", wallet)
	}
	var ledgerBefore int
	if err := f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission'`).Scan(&ledgerBefore); err != nil || ledgerBefore != 0 {
		t.Fatalf("manual payment ledger before approval=%d err=%v", ledgerBefore, err)
	}

	viewer := actor
	viewer.Roles = []access.Role{{BrandID: f.betting.brand, Permissions: []access.Permission{{Resource: "commission", Action: "view", Scope: access.ScopeBrand}}}}
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.ApprovePaymentTx(ctx, tx, f.betting.brand, payment.ID, viewer,
			commission.RetryCycleInput{Version: payment.Version, Reason: "unauthorized approval attempt"}, commissionPaymentMeta(viewer))
		return err
	}); !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("view-only actor approval error=%v; want denied", err)
	}
	unauthorized := actor
	unauthorized.SuperAdmin = true
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.ApprovePaymentTx(ctx, tx, f.betting.brand, payment.ID, unauthorized,
			commission.RetryCycleInput{Version: payment.Version, Reason: "super-admin approval attempt"}, commissionPaymentMeta(unauthorized))
		return err
	}); !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("super-admin approval error=%v; want denied", err)
	}
	foreignActor := actor
	foreignActor.BrandIDs = append(append([]string(nil), actor.BrandIDs...), storeTestOtherBrand)
	foreignActor.Roles = append(append([]access.Role(nil), actor.Roles...), access.Role{BrandID: storeTestOtherBrand, Permissions: []access.Permission{
		{Resource: "commission", Action: "view", Scope: access.ScopeBrand},
		{Resource: "commission_payment", Action: "approve", Scope: access.ScopeBrand},
		{Resource: "commission_payment", Action: "retry", Scope: access.ScopeBrand},
		{Resource: "commission_payment_policy", Action: "write", Scope: access.ScopeBrand},
	}})
	var foreignPolicy commission.PaymentPolicy
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		var err error
		foreignPolicy, err = f.service.PaymentPolicyTx(ctx, tx, storeTestOtherBrand)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !foreignPolicy.Enabled {
		if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
			_, err := f.service.UpdatePaymentPolicyTx(ctx, tx, storeTestOtherBrand, foreignActor,
				commission.PaymentPolicyInput{Version: foreignPolicy.Version, Enabled: true, Reason: "authorize isolated cross-brand not-found assertion"}, commissionPaymentMeta(foreignActor))
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.ApprovePaymentTx(ctx, tx, storeTestOtherBrand, payment.ID, foreignActor,
			commission.RetryCycleInput{Version: payment.Version, Reason: "cross-brand approval attempt"}, commissionPaymentMeta(foreignActor))
		return err
	}); !errors.Is(err, commission.ErrNotFound) {
		t.Fatalf("authorized foreign-brand payment lookup error=%v; want not found", err)
	}
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.ApprovePaymentTx(ctx, tx, f.betting.brand, payment.ID, actor,
			commission.RetryCycleInput{Version: payment.Version + 1, Reason: "stale approval version"}, commissionPaymentMeta(actor))
		return err
	}); !errors.Is(err, commission.ErrPaymentVersion) {
		t.Fatalf("stale approval version error=%v; want version conflict", err)
	}
	if got := commissionPaymentRead(t, f, payment.ID); got.State != "awaiting_approval" || got.PaidPoints != "0" {
		t.Fatalf("rejected approval changed payment: %+v", got)
	}

	// A database audit outage must roll back the approval transition completely.
	if _, err := f.betting.db.Exec(ctx, `CREATE FUNCTION test_reject_commission_payment_approval() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='commission.payment.approve' THEN RAISE EXCEPTION 'test approval audit outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_reject_commission_payment_approval BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION test_reject_commission_payment_approval()`); err != nil {
		t.Fatal(err)
	}
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.ApprovePaymentTx(ctx, tx, f.betting.brand, payment.ID, actor,
			commission.RetryCycleInput{Version: payment.Version, Reason: "approval audit rollback"}, commissionPaymentMeta(actor))
		return err
	}); err == nil {
		t.Fatal("approval succeeded despite injected audit failure")
	}
	if _, err := f.betting.db.Exec(ctx, `DROP TRIGGER test_reject_commission_payment_approval ON audit_logs; DROP FUNCTION test_reject_commission_payment_approval()`); err != nil {
		t.Fatal(err)
	}
	if got := commissionPaymentRead(t, f, payment.ID); got.State != "awaiting_approval" || got.PaidPoints != "0" || walletBySource(t, f.betting)[3][0] != 0 {
		t.Fatalf("failed approval left payment/credit mutation: payment=%+v wallet=%+v", got, walletBySource(t, f.betting))
	}

	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.ApprovePaymentTx(ctx, tx, f.betting.brand, payment.ID, actor,
			commission.RetryCycleInput{Version: payment.Version, Reason: "approve exact ready commission run"}, commissionPaymentMeta(actor))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	paying := commissionPaymentRead(t, f, payment.ID)
	if paying.State != "paying" || paying.Version != payment.Version+1 {
		t.Fatalf("approved payment state=%+v; want paying", paying)
	}
	policy := commissionPaymentPolicy(t, f)
	if err := updateCommissionPaymentPolicy(t, f, actor, false, policy.Version); err != nil {
		t.Fatal(err)
	}
	if steps, err := f.service.ProcessPayments(ctx, 20); err != nil || steps != 0 {
		t.Fatalf("closed gate did not pause paying job: steps=%d err=%v", steps, err)
	}
	if got := commissionPaymentRead(t, f, payment.ID); got.State != "paying" || got.PaidCount != "0" || walletBySource(t, f.betting)[3][0] != 0 {
		t.Fatalf("closed gate advanced payment: %+v wallet=%+v", got, walletBySource(t, f.betting))
	}
	policy = commissionPaymentPolicy(t, f)
	if err := updateCommissionPaymentPolicy(t, f, actor, true, policy.Version); err != nil {
		t.Fatal(err)
	}

	// Force one ledger write to fail. The failed worker transaction may not leave
	// an orphan entry or a partially incremented paid total.
	if _, err := f.betting.db.Exec(ctx, `CREATE FUNCTION test_reject_commission_payment_ledger() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.entry_type='commission' THEN RAISE EXCEPTION 'test commission ledger outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_reject_commission_payment_ledger BEFORE INSERT ON point_ledger_entries FOR EACH ROW EXECUTE FUNCTION test_reject_commission_payment_ledger()`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.betting.db.Exec(ctx, `UPDATE commission_payments SET next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, f.betting.brand, payment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if _, err := f.betting.db.Exec(ctx, `DROP TRIGGER test_reject_commission_payment_ledger ON point_ledger_entries; DROP FUNCTION test_reject_commission_payment_ledger()`); err != nil {
		t.Fatal(err)
	}
	failed := commissionPaymentRead(t, f, payment.ID)
	if failed.State != "failed" || failed.PaidPoints != "0" || failed.PaidCount != "0" {
		t.Fatalf("ledger fault did not persist clean failed payment: %+v", failed)
	}
	var orphanEntries int
	if err := f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission'`).Scan(&orphanEntries); err != nil || orphanEntries != ledgerBefore {
		t.Fatalf("ledger fault left orphan commission entries=%d before=%d err=%v", orphanEntries, ledgerBefore, err)
	}
	if walletBySource(t, f.betting)[3][0] != 0 {
		t.Fatalf("ledger fault left orphan commission balance: %+v", walletBySource(t, f.betting))
	}
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.RetryPaymentTx(ctx, tx, f.betting.brand, payment.ID, actor,
			commission.RetryCycleInput{Version: failed.Version, Reason: "retry after ledger recovery"}, commissionPaymentMeta(actor))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	retrying := commissionPaymentRead(t, f, payment.ID)
	if retrying.State != "paying" || retrying.Version != failed.Version+1 {
		t.Fatalf("retry did not restore paying with new version: %+v", retrying)
	}
	if _, err := f.betting.db.Exec(ctx, `UPDATE commission_payments SET next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, f.betting.brand, payment.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := f.service.ProcessPayments(ctx, 20); err != nil {
			t.Fatal(err)
		}
		got := commissionPaymentRead(t, f, payment.ID)
		if got.State == "paid" {
			break
		}
		if _, err := f.betting.db.Exec(ctx, `UPDATE commission_payments SET next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, f.betting.brand, payment.ID); err != nil {
			t.Fatal(err)
		}
	}
	paid := commissionPaymentRead(t, f, payment.ID)
	if paid.State != "paid" || paid.PaidPoints != "1" || paid.PaidCount != "1" || paid.TotalPoints != "1" || paid.TargetCount != "1" {
		t.Fatalf("retry did not settle exactly one target: %+v", paid)
	}
	var entryCount int
	var referenceType, operationKey, deltaText, entryBrand, entryMember string
	if err := f.betting.db.QueryRow(ctx, `SELECT count(*),min(reference_type),min(operation_key),min(delta_snapshot->'commission'->>'available'),min(brand_id::text),min(member_id::text)
	 FROM point_ledger_entries WHERE entry_type='commission' AND reference_id IN(SELECT id FROM commission_payment_targets WHERE payment_id=$1)`, payment.ID).
		Scan(&entryCount, &referenceType, &operationKey, &deltaText, &entryBrand, &entryMember); err != nil {
		t.Fatal(err)
	}
	if entryCount != 1 || referenceType != "commission_payment_target" || deltaText != "1" || entryBrand != f.betting.brand || entryMember != f.node.MemberID || !strings.HasPrefix(operationKey, "commission-payment:") {
		t.Fatalf("commission ledger evidence count=%d ref=%s key=%s delta=%s brand=%s member=%s", entryCount, referenceType, operationKey, deltaText, entryBrand, entryMember)
	}
	var targetID string
	if err := f.betting.db.QueryRow(ctx, `SELECT id::text FROM commission_payment_targets WHERE payment_id=$1`, payment.ID).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if operationKey != "commission-payment:"+targetID {
		t.Fatalf("commission ledger operation key=%q, target=%s", operationKey, targetID)
	}
	wallet, err := (points.Store{DB: f.betting.db}).Read(ctx, f.betting.brand, f.node.MemberID)
	if err != nil || wallet.BySource[3][0] != 1 {
		t.Fatalf("agent beneficiary wallet=%+v err=%v", wallet.BySource, err)
	}
	for i := 0; i < 4; i++ {
		if _, err := f.betting.db.Exec(ctx, `UPDATE commission_payments SET next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, f.betting.brand, payment.ID); err != nil {
			t.Fatal(err)
		}
		if steps, err := f.service.ProcessPayments(ctx, 20); err != nil || steps != 0 {
			t.Fatalf("repeat payment worker %d processed=%d err=%v", i, steps, err)
		}
	}
	if err := f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission'`).Scan(&entryCount); err != nil || entryCount != 1 {
		t.Fatalf("repeat worker duplicated commission credit: entries=%d err=%v", entryCount, err)
	}

	// A correction after credit must invalidate the old evidence without creating
	// another full-value target payment from the changed epoch.
	var cycleID string
	if err := f.betting.db.QueryRow(ctx, `SELECT cycle_id::text FROM commission_payments WHERE id=$1`, payment.ID).Scan(&cycleID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.betting.db.Exec(ctx, `UPDATE commission_cycles SET evidence_epoch=evidence_epoch+1 WHERE brand_id=$1 AND id=$2`, f.betting.brand, cycleID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.betting.db.Exec(ctx, `UPDATE commission_payments SET next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, f.betting.brand, payment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	blocked := commissionPaymentRead(t, f, payment.ID)
	if blocked.State != "blocked" || blocked.PaidPoints != "1" || blocked.PaidCount != "1" {
		t.Fatalf("credited payment was not blocked after evidence epoch changed: %+v", blocked)
	}
	beneficiaryWallet, walletErr := (points.Store{DB: f.betting.db}).Read(ctx, f.betting.brand, f.node.MemberID)
	if err := f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission'`).Scan(&entryCount); err != nil || entryCount != 1 || walletErr != nil || beneficiaryWallet.BySource[3][0] != 1 {
		t.Fatalf("post-credit stale evidence caused second full dispatch: entries=%d beneficiary_wallet=%+v wallet_err=%v query_err=%v", entryCount, beneficiaryWallet.BySource, walletErr, err)
	}
	if page := commissionPayments(t, f); len(page.Items) != 1 || page.Items[0].PaidPoints != "1" {
		t.Fatalf("brand payment list lost paid projection: %+v", page)
	}
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.PaymentTx(ctx, tx, storeTestOtherBrand, payment.ID)
		return err
	}); !errors.Is(err, commission.ErrNotFound) {
		t.Fatalf("payment read crossed brand scope: %v", err)
	}
}

func TestCommissionPaymentListIsBrandScoped(t *testing.T) {
	f := newCommissionBatchFixture(t)
	page := commissionPayments(t, f)
	if page.BrandID != f.betting.brand || len(page.Items) != 0 {
		t.Fatalf("empty brand payment page=%+v", page)
	}
	tx, err := f.betting.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = f.service.PaymentsTx(context.Background(), tx, storeTestOtherBrand, 100, 0); err != nil {
		t.Fatalf("brand-scoped empty payment list failed: %v", err)
	}
}

func TestCommissionPaymentAutomaticRequiresExplicitGateAndSavedAutomaticSnapshot(t *testing.T) {
	f := newAutomaticCommissionBatchFixture(t)
	ctx := context.Background()
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 30)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || cycle.TotalPoints != "1" || cycle.EarningCount != "1" {
		t.Fatalf("automatic snapshot cycle did not calculate 0.6 to one point: %+v", cycle)
	}
	policy := commissionPaymentPolicy(t, f)
	if policy.Enabled {
		t.Fatalf("new brand payment gate unexpectedly enabled: %+v", policy)
	}
	if steps, err := f.service.ProcessPayments(ctx, 20); err != nil || steps != 0 {
		t.Fatalf("closed gate processed automatic payout: steps=%d err=%v", steps, err)
	}
	if page := commissionPayments(t, f); len(page.Items) != 0 {
		t.Fatalf("closed gate registered automatic payout: %+v", page.Items)
	}
	actor := commissionPaymentActor(f)
	if err := updateCommissionPaymentPolicy(t, f, actor, true, policy.Version); err != nil {
		t.Fatal(err)
	}
	// The first bounded step registers the job from the immutable bet-time
	// automatic snapshot. A later step posts the target and is independently
	// fenced by the current payment gate.
	if steps, err := f.service.ProcessPayments(ctx, 1); err != nil || steps != 1 {
		t.Fatalf("automatic payment registration steps=%d err=%v", steps, err)
	}
	page := commissionPayments(t, f)
	if len(page.Items) != 1 {
		t.Fatalf("automatic payment page=%+v", page)
	}
	payment := page.Items[0]
	if payment.State != "paying" || payment.PayoutMode != commission.PayoutAutomatic || payment.RunID != *cycle.CurrentRunID || payment.PaidPoints != "0" {
		t.Fatalf("automatic payment did not use saved mode/current run: %+v", payment)
	}
	var targetCount int
	if err := f.betting.db.QueryRow(ctx, `SELECT count(*) FROM commission_payment_targets WHERE payment_id=$1`, payment.ID).Scan(&targetCount); err != nil || targetCount != 0 {
		t.Fatalf("job registration eagerly created targets: count=%d err=%v", targetCount, err)
	}
	var approvalActor string
	if err := f.betting.db.QueryRow(ctx, `SELECT approval_actor_type FROM commission_payments WHERE id=$1`, payment.ID).Scan(&approvalActor); err != nil || approvalActor != "system" {
		t.Fatalf("automatic payment approval provenance=%q err=%v", approvalActor, err)
	}
	policy = commissionPaymentPolicy(t, f)
	if err := updateCommissionPaymentPolicy(t, f, actor, false, policy.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := f.betting.db.Exec(ctx, `UPDATE commission_payments SET next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, f.betting.brand, payment.ID); err != nil {
		t.Fatal(err)
	}
	if steps, err := f.service.ProcessPayments(ctx, 10); err != nil || steps != 0 {
		t.Fatalf("closed gate did not pause automatic payment: steps=%d err=%v", steps, err)
	}
	if got := commissionPaymentRead(t, f, payment.ID); got.State != "paying" || got.PaidPoints != "0" || walletBySource(t, f.betting)[3][0] != 0 {
		t.Fatalf("closed gate credited automatic payment: %+v wallet=%+v", got, walletBySource(t, f.betting))
	}
	policy = commissionPaymentPolicy(t, f)
	if err := updateCommissionPaymentPolicy(t, f, actor, true, policy.Version); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := f.betting.db.Exec(ctx, `UPDATE commission_payments SET next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, f.betting.brand, payment.ID); err != nil {
			t.Fatal(err)
		}
		if steps, err := f.service.ProcessPayments(ctx, 1); err != nil || steps > 1 {
			t.Fatalf("bounded automatic worker steps=%d err=%v", steps, err)
		}
		if err := f.betting.db.QueryRow(ctx, `SELECT count(*) FROM commission_payment_targets WHERE payment_id=$1`, payment.ID).Scan(&targetCount); err != nil || targetCount > 1 {
			t.Fatalf("one worker step created more than one target: count=%d err=%v", targetCount, err)
		}
		if commissionPaymentRead(t, f, payment.ID).State == "paid" {
			break
		}
	}
	paid := commissionPaymentRead(t, f, payment.ID)
	if paid.State != "paid" || paid.PaidPoints != "1" || paid.PaidCount != "1" {
		t.Fatalf("reopened gate did not resume automatic payment: %+v", paid)
	}
}

func TestCommissionPaymentStaleUncreditedRunCanBeSupersededAfterCorrection(t *testing.T) {
	f := newCommissionBatchFixture(t)
	ctx := context.Background()
	oldPayment := prepareManualCommissionPayment(t, f)
	oldRun := oldPayment.RunID
	actor := commissionPaymentActor(f)
	policy := commissionPaymentPolicy(t, f)
	if err := updateCommissionPaymentPolicy(t, f, actor, false, policy.Version); err != nil {
		t.Fatal(err)
	}
	correction := createFixtureCorrection(t, f.betting, correctedDigits(0, 0, 0))
	if correction.State != "reversing" {
		t.Fatalf("correction started in state %q", correction.State)
	}
	if _, err := f.betting.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	correction, err := f.betting.service.Correction(ctx, f.betting.brand, correction.ID)
	if err != nil || correction.State != "resettling" || correction.NewJobID == nil {
		t.Fatalf("correction did not create replacement settlement job: %+v err=%v", correction, err)
	}
	if _, err := f.betting.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	advanceCommissionWorker(t, f, 40)
	cycle := readCommissionCycle(t, f, oldPayment.CycleID)
	if cycle.State != "ready" || !cycle.EvidenceCurrent || cycle.CurrentRunID == nil || *cycle.CurrentRunID == oldRun {
		t.Fatalf("correction did not produce a current replacement run: %+v old_run=%s", cycle, oldRun)
	}
	if _, err := f.service.ProcessPayments(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if got := commissionPaymentRead(t, f, oldPayment.ID); got.State != "stale" || got.PaidPoints != "0" || got.PaidCount != "0" {
		t.Fatalf("uncredited old payment was not safely invalidated: %+v", got)
	}
	if steps, err := f.service.ProcessPayments(ctx, 10); err != nil || steps != 0 {
		t.Fatalf("closed gate registered replacement payment: steps=%d err=%v", steps, err)
	}
	if page := commissionPayments(t, f); len(page.Items) != 1 || page.Items[0].State != "stale" {
		t.Fatalf("closed gate should retain only the invalidated payment: %+v", page.Items)
	}
	policy = commissionPaymentPolicy(t, f)
	if err := updateCommissionPaymentPolicy(t, f, actor, true, policy.Version); err != nil {
		t.Fatal(err)
	}
	if steps, err := f.service.ProcessPayments(ctx, 1); err != nil || steps != 1 {
		t.Fatalf("replacement run payment registration steps=%d err=%v", steps, err)
	}
	page := commissionPayments(t, f)
	if len(page.Items) != 2 {
		t.Fatalf("expected preserved stale payment plus replacement payment: %+v", page.Items)
	}
	var replacement *commission.Payment
	for i := range page.Items {
		if page.Items[i].RunID == *cycle.CurrentRunID {
			replacement = &page.Items[i]
		}
	}
	if replacement == nil || replacement.ID == oldPayment.ID || replacement.State != "awaiting_approval" || replacement.PaidPoints != "0" {
		t.Fatalf("new evidence run did not receive a fresh uncredited manual payment: %+v", page.Items)
	}
	var ledgerCount int
	if err := f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission'`).Scan(&ledgerCount); err != nil || ledgerCount != 0 {
		t.Fatalf("stale run correction posted commission credit: entries=%d err=%v", ledgerCount, err)
	}
}
