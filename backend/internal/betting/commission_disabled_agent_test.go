package betting

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

// Unlike a synthetic disabled-node evaluator fixture, this exercises the real
// registration attribution, placement trigger, final settlement, cycle and
// ledger posting. Stopping agent operations is not stopping its saved earnings.
func TestCommissionDisabledAgentPreservesNewBetSnapshotsAndActualPayout(t *testing.T) {
	for _, mode := range []string{commission.PayoutAutomatic, commission.PayoutManual} {
		t.Run(mode, func(t *testing.T) { assertDisabledAgentActualPayout(t, mode) })
	}
}

func assertDisabledAgentActualPayout(t *testing.T, mode string) {
	t.Helper()
	var f commissionBatchFixture
	if mode == commission.PayoutManual {
		f = newCommissionBatchFixture(t)
	} else {
		f = newAutomaticCommissionBatchFixture(t)
	}
	ctx := context.Background()
	agents := agency.Service{DB: f.betting.db}
	policy, err := agents.Policy(ctx, f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	var child agency.Node
	bettingTx(t, f.betting.db, func(tx pgx.Tx) error {
		parentVersion := f.node.Version
		child, err = agents.Create(ctx, tx, f.betting.brand, f.actor, agency.CreateInput{
			PolicyVersion: policy.Version, MemberID: f.betting.member,
			ParentID: &f.node.ID, ParentVersion: &parentVersion,
			Config: agency.NodeConfig{Ratio: "0.1", Status: "active"},
			Reason: "actual child for disabled-agent operation boundary",
		}, policyMeta(f.actor.ID))
		return err
	})

	// Authenticate a real persisted session for the original agent member. Its
	// normal user identity remains valid after only the agent node is disabled.
	var userID string
	if err = f.betting.db.QueryRow(ctx, `SELECT global_user_id::text FROM brand_members WHERE brand_id=$1 AND id=$2`, f.betting.brand, f.node.MemberID).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	token, err := authcrypto.NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	digest := authcrypto.DigestSessionToken(token)
	if _, err = f.betting.db.Exec(ctx, `INSERT INTO sessions(id,token_hash,user_id,member_id,brand_id,expires_at) VALUES($1,$2,$3,$4,$5,clock_timestamp()+interval '1 hour')`, ids.New(), hex.EncodeToString(digest[:]), userID, f.node.MemberID, f.betting.brand); err != nil {
		t.Fatal(err)
	}
	users, err := identity.New(f.betting.db)
	if err != nil {
		t.Fatal(err)
	}
	agentSession, err := users.Authenticate(ctx, f.betting.brand, token)
	if err != nil {
		t.Fatal(err)
	}
	bettingTx(t, f.betting.db, func(tx pgx.Tx) error {
		_, e := agents.CheckUserWrite(ctx, tx, f.betting.brand, agentSession, child.ID)
		return e
	})

	update := func(ratio string) {
		t.Helper()
		bettingTx(t, f.betting.db, func(tx pgx.Tx) error {
			f.node, err = agents.Update(ctx, tx, f.betting.brand, f.actor, f.node.ID, agency.UpdateInput{
				Version: f.node.Version, PolicyVersion: policy.Version,
				Config: agency.NodeConfig{Ratio: ratio, Status: "disabled"},
				Reason: "disable agent operations without suppressing saved earnings",
			}, policyMeta(f.actor.ID))
			return err
		})
	}
	update("0.2")
	agentSession, err = users.Authenticate(ctx, f.betting.brand, token)
	if err != nil || agentSession.Member.Status != "normal" {
		t.Fatalf("agent disable changed ordinary member authentication: session=%+v err=%v", agentSession.Member, err)
	}
	err = commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, e := agents.UpdateChild(ctx, tx, f.betting.brand, agentSession, child.ID, agency.ChildInput{
			Version: child.Version, PolicyVersion: policy.Version, ParentVersion: f.node.Version,
			Ratio: "0.05", Reason: "disabled agent must not edit its child",
		}, points.Metadata{RequestID: ids.New()})
		return e
	})
	if !errors.Is(err, agency.ErrDenied) {
		t.Fatalf("disabled agent child update=%v, want denied", err)
	}
	var childVersion int64
	var childRatio string
	if err = f.betting.db.QueryRow(ctx, `SELECT version,config->>'ratio' FROM agent_nodes WHERE id=$1`, child.ID).Scan(&childVersion, &childRatio); err != nil || childVersion != child.Version || childRatio != "0.1" {
		t.Fatalf("denied operation changed child: version=%d ratio=%s err=%v", childVersion, childRatio, err)
	}

	in := f.betting.input
	in.Selection = rules.Selection{Digits: [][]int{{0}, {0}, {0}}}
	in.Multiplier = 7
	newOrder, err := placeBettingOrder(t, f.betting, in, "commission-new-wager-after-agent-disable")
	if err != nil || !newOrder.PlacedAt.Before(f.boundary) {
		t.Fatalf("new bet after agent disable: order=%+v err=%v boundary=%v", newOrder, err, f.boundary)
	}
	f.orders = append(f.orders, newOrder)
	// This later version is deliberately different. Neither earlier active
	// wagers nor the disabled-node wager may use today's 0.1 ratio instead.
	update("0.1")
	for _, expected := range []struct{ id, version, ratio, state string }{
		{f.orders[0].ID, "1", "0.3", "active"},
		{newOrder.ID, "2", "0.2", "disabled"},
	} {
		var version, ratio, state string
		if err = f.betting.db.QueryRow(ctx, `SELECT commission_rule_snapshot->'agent_path'->0->>'version',commission_rule_snapshot->'agent_path'->0->'config'->>'ratio',commission_rule_snapshot->'agent_path'->0->'config'->>'status' FROM bet_orders WHERE id=$1`, expected.id).Scan(&version, &ratio, &state); err != nil || version != expected.version || ratio != expected.ratio || state != expected.state {
			t.Fatalf("bet %s snapshot version=%s ratio=%s state=%s, want=%+v err=%v", expected.id, version, ratio, state, expected, err)
		}
	}

	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 30)
	cycle = readCommissionCycle(t, f, cycle.ID)
	// 1*0.3 + 1*0.3 + 7*0.2 = 2 exactly. Ignoring the disabled wager,
	// or rereading the current 0.1 ratio, would incorrectly round to 1.
	if cycle.State != "ready" || cycle.TotalPoints != "2" || cycle.TargetCount != "4" || cycle.EarningCount != "1" {
		t.Fatalf("disabled-agent cycle=%+v, want one beneficiary and two points", cycle)
	}
	var numerator, denominator string
	var exactPoints points.Amount
	if err = f.betting.db.QueryRow(ctx, `SELECT numerator,denominator,points FROM commission_earnings WHERE run_id=$1 AND agent_id=$2`, *cycle.CurrentRunID, f.node.ID).Scan(&numerator, &denominator, &exactPoints); err != nil || numerator != "2" || denominator != "1" || exactPoints != 2 {
		t.Fatalf("saved exact earning=%s/%s points=%d err=%v", numerator, denominator, exactPoints, err)
	}
	var eligible int
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM commission_calculations WHERE run_id=$1 AND reason='eligible'`, *cycle.CurrentRunID).Scan(&eligible); err != nil || eligible != 3 {
		t.Fatalf("eligible wagers=%d err=%v, want both pre-disable and new wager", eligible, err)
	}
	actor := commissionPaymentActor(f)
	gate := commissionPaymentPolicy(t, f)
	if err = updateCommissionPaymentPolicy(t, f, actor, true, gate.Version); err != nil {
		t.Fatal(err)
	}
	if steps, e := f.service.ProcessPayments(ctx, 1); e != nil || steps != 1 {
		t.Fatalf("payment registration steps=%d err=%v", steps, e)
	}
	registered := commissionPayments(t, f)
	if len(registered.Items) != 1 || registered.Items[0].PayoutMode != mode || registered.Items[0].PaidPoints != "0" {
		t.Fatalf("registration did not preserve original payout mode: %+v", registered.Items)
	}
	if mode == commission.PayoutManual {
		payment := registered.Items[0]
		if payment.State != "awaiting_approval" {
			t.Fatalf("disabled agent bypassed manual approval: %+v", payment)
		}
		if steps, e := f.service.ProcessPayments(ctx, 20); e != nil || steps != 0 {
			t.Fatalf("unapproved manual payment steps=%d err=%v", steps, e)
		}
		beforeApproval, e := (points.Store{DB: f.betting.db}).Read(ctx, f.betting.brand, f.node.MemberID)
		if e != nil || beforeApproval.DisplayPoints != 0 {
			t.Fatalf("manual payment credited before approval: wallet=%+v err=%v", beforeApproval, e)
		}
		if err = commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
			_, e := f.service.ApprovePaymentTx(ctx, tx, f.betting.brand, payment.ID, actor,
				commission.RetryCycleInput{Version: payment.Version, Reason: "approve saved earnings for disabled agent"}, commissionPaymentMeta(actor))
			return e
		}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 5; i++ {
		if _, err = f.service.ProcessPayments(ctx, 20); err != nil {
			t.Fatal(err)
		}
		if payments := commissionPayments(t, f); len(payments.Items) == 1 && payments.Items[0].State == "paid" {
			break
		}
		if _, err = f.betting.db.Exec(ctx, `UPDATE commission_payments SET next_work_at=clock_timestamp() WHERE cycle_id=$1`, cycle.ID); err != nil {
			t.Fatal(err)
		}
	}
	payments := commissionPayments(t, f)
	if len(payments.Items) != 1 || payments.Items[0].State != "paid" || payments.Items[0].PaidPoints != "2" || payments.Items[0].PayoutMode != mode {
		t.Fatalf("disabled-agent actual payment=%+v", payments.Items)
	}
	wallet, err := (points.Store{DB: f.betting.db}).Read(ctx, f.betting.brand, f.node.MemberID)
	if err != nil || wallet.BySource[3][0] != 2 || wallet.DisplayPoints != 2 {
		t.Fatalf("disabled agent actual wallet=%+v err=%v", wallet, err)
	}
	for i := 0; i < 3; i++ {
		if steps, e := f.service.ProcessPayments(ctx, 20); e != nil || steps != 0 {
			t.Fatalf("repeated payment steps=%d err=%v", steps, e)
		}
	}
	var ledgerCount int
	var ledgerPoints string
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*),min(delta_snapshot->'commission'->>'available') FROM point_ledger_entries WHERE entry_type='commission' AND member_id=$1`, f.node.MemberID).Scan(&ledgerCount, &ledgerPoints); err != nil || ledgerCount != 1 || ledgerPoints != "2" {
		t.Fatalf("commission ledger count=%d amount=%s err=%v", ledgerCount, ledgerPoints, err)
	}
}
