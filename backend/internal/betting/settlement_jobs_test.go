package betting

import (
	"context"
	"errors"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/jackc/pgx/v5"
)

func settlementActor(f bettingFixture) access.Account {
	a := previewActor(f)
	a.Roles[0].Permissions = append(a.Roles[0].Permissions,
		access.Permission{Resource: "settlement_policy", Action: "write", Scope: access.ScopeBrand},
		access.Permission{Resource: "settlement", Action: "run", Scope: access.ScopeBrand},
		access.Permission{Resource: "settlement", Action: "approve", Scope: access.ScopeBrand},
		access.Permission{Resource: "settlement", Action: "retry", Scope: access.ScopeBrand},
	)
	return a
}

func setSettlementMode(t *testing.T, f bettingFixture, actor access.Account, version int64, mode *string) SettlementPolicy {
	t.Helper()
	var policy SettlementPolicy
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		policy, err = f.service.SaveSettlementPolicy(context.Background(), tx, f.brand, actor, SettlementPolicyInput{
			Version: version,
			Mode:    mode,
			Reason:  "configure integration settlement mode",
		}, policyMeta(actor.ID))
		return err
	})
	return policy
}

func startFixtureSettlement(t *testing.T, f bettingFixture, actor access.Account, policyVersion int64) SettlementJob {
	t.Helper()
	ctx := context.Background()
	c, err := f.service.PeriodSettlementContext(ctx, f.brand, f.period.ID)
	if err != nil {
		t.Fatal(err)
	}
	var job SettlementJob
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		job, err = f.service.StartSettlement(ctx, tx, f.brand, actor, f.period.ID, SettlementStartInput{
			Version:       c.PeriodVersion,
			PolicyVersion: policyVersion,
			DrawResultID:  *c.DrawResultID,
			Reason:        "start integration settlement",
		}, policyMeta(actor.ID))
		return err
	})
	return job
}

func actOnFixtureSettlement(t *testing.T, f bettingFixture, actor access.Account, job SettlementJob, action string, version int64) (SettlementJob, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := f.service.ActOnSettlement(ctx, tx, f.brand, actor, job.ID, action, SettlementActionInput{
		Version: version,
		Reason:  "integration settlement action",
	}, policyMeta(actor.ID))
	if err != nil {
		_ = tx.Rollback(ctx)
		return updated, err
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return updated, nil
}

func TestSettlementJobAutomaticPaysFromDrawnOrderSnapshot(t *testing.T) {
	f, order := drawnFixtureOrder(t)
	ctx := context.Background()
	actor := settlementActor(f)
	automatic := "automatic"
	policy := setSettlementMode(t, f, actor, 1, &automatic)
	job := startFixtureSettlement(t, f, actor, policy.Version)
	if job.Mode != automatic || job.State != "processing" || job.TargetCount != 1 || job.PolicyVersion != policy.Version {
		t.Fatalf("unexpected initial settlement job: %+v", job)
	}

	if processed, err := f.service.ProcessSettlements(ctx, 20); err != nil || processed == 0 {
		t.Fatalf("process automatic settlement: processed=%d err=%v", processed, err)
	}
	job, err := f.service.SettlementJob(ctx, f.brand, job.ID)
	if err != nil || job.State != "completed" || job.PaidCount != 1 || job.PrizePoints != "10" || job.PaidPoints != "10" {
		t.Fatalf("automatic job did not complete with one 10 point payout: %+v %v", job, err)
	}
	settled, err := f.service.Order(ctx, f.brand, f.member, order.ID)
	if err != nil || settled.Status != "won" || settled.PrizePoints != 10 {
		t.Fatalf("order not settled as winner: %+v %v", settled, err)
	}
	wallet := walletBySource(t, f)
	if wallet[0][0] != 99 || wallet[1][0] != 10 {
		t.Fatalf("unexpected recharge/winning balances after payout: %+v", wallet)
	}
	var prizes int
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='prize'`).Scan(&prizes); err != nil || prizes != 1 {
		t.Fatalf("prize ledger entries=%d err=%v", prizes, err)
	}
	var periodStatus string
	if err = f.db.QueryRow(ctx, `SELECT status FROM periods WHERE id=$1`, f.period.ID).Scan(&periodStatus); err != nil || periodStatus != "settled" {
		t.Fatalf("period status=%q err=%v", periodStatus, err)
	}
}

func TestSettlementJobManualApprovalAndPolicyAreSnapshotted(t *testing.T) {
	f, order := drawnFixtureOrder(t)
	ctx := context.Background()
	actor := settlementActor(f)
	manual := "manual"
	policy := setSettlementMode(t, f, actor, 1, &manual)
	job := startFixtureSettlement(t, f, actor, policy.Version)
	before := walletBySource(t, f)
	automatic := "automatic"
	changed := setSettlementMode(t, f, actor, policy.Version, &automatic)
	if changed.Version != policy.Version+1 {
		t.Fatalf("policy version did not advance: before=%+v after=%+v", policy, changed)
	}

	if processed, err := f.service.ProcessSettlements(ctx, 20); err != nil || processed == 0 {
		t.Fatalf("calculate manual settlement: processed=%d err=%v", processed, err)
	}
	job, err := f.service.SettlementJob(ctx, f.brand, job.ID)
	if err != nil || job.Mode != manual || job.PolicyVersion != policy.Version || job.State != "awaiting_approval" || job.ReadyCount != 1 {
		t.Fatalf("started job did not retain manual policy: %+v %v", job, err)
	}
	if got := walletBySource(t, f); got != before {
		t.Fatalf("manual calculation changed wallet: before=%+v after=%+v", before, got)
	}
	approved, err := actOnFixtureSettlement(t, f, actor, job, "approve", job.Version)
	if err != nil || approved.State != "paying" {
		t.Fatalf("approve manual settlement: %+v %v", approved, err)
	}
	if processed, err := f.service.ProcessSettlements(ctx, 20); err != nil || processed == 0 {
		t.Fatalf("pay approved settlement: processed=%d err=%v", processed, err)
	}
	completed, err := f.service.SettlementJob(ctx, f.brand, job.ID)
	settled, orderErr := f.service.Order(ctx, f.brand, f.member, order.ID)
	if err != nil || orderErr != nil || completed.State != "completed" || settled.Status != "won" || walletBySource(t, f)[1][0] != 10 {
		t.Fatalf("manual approval did not pay: job=%+v order=%+v err=%v orderErr=%v", completed, settled, err, orderErr)
	}
}

func TestSettlementJobCancellationInvalidatesApprovalAndExcludesTarget(t *testing.T) {
	f, order := drawnFixtureOrder(t)
	ctx := context.Background()
	actor := settlementActor(f)
	manual := "manual"
	policy := setSettlementMode(t, f, actor, 1, &manual)
	job := startFixtureSettlement(t, f, actor, policy.Version)
	if _, err := f.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	job, err := f.service.SettlementJob(ctx, f.brand, job.ID)
	if err != nil || job.State != "awaiting_approval" || job.ReadyCount != 1 {
		t.Fatalf("manual job did not reach approval: %+v %v", job, err)
	}

	canceller := judgeActor(f)
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, err := f.service.CancelAdmin(ctx, tx, f.brand, canceller, order.ID, order.Version, "cancel while settlement awaits approval", policyMeta(canceller.ID))
		return err
	})
	cancelled, err := f.service.Order(ctx, f.brand, f.member, order.ID)
	if err != nil || cancelled.Status != "bet_cancelled" {
		t.Fatalf("target order was not cancelled: %+v %v", cancelled, err)
	}
	if _, err = actOnFixtureSettlement(t, f, actor, job, "approve", job.Version); !errors.Is(err, ErrVersion) {
		t.Fatalf("approval with pre-cancellation job version=%v, want ErrVersion", err)
	}
	fresh, err := f.service.SettlementJob(ctx, f.brand, job.ID)
	if err != nil || fresh.Version <= job.Version || fresh.State != "awaiting_approval" {
		t.Fatalf("cancellation did not bump the awaiting job version: before=%+v after=%+v err=%v", job, fresh, err)
	}
	approved, err := actOnFixtureSettlement(t, f, actor, fresh, "approve", fresh.Version)
	if err != nil || approved.State != "paying" {
		t.Fatalf("approve after cancellation with fresh version: %+v %v", approved, err)
	}
	if processed, err := f.service.ProcessSettlements(ctx, 20); err != nil || processed == 0 {
		t.Fatalf("complete settlement with excluded target: processed=%d err=%v", processed, err)
	}
	completed, err := f.service.SettlementJob(ctx, f.brand, job.ID)
	if err != nil || completed.State != "completed" || completed.PaidCount != 0 || completed.ExcludedCount != 1 {
		t.Fatalf("cancelled target was not excluded from completed job: %+v %v", completed, err)
	}
	if wallet := walletBySource(t, f); wallet[0][0] != 100 || wallet[1][0] != 0 {
		t.Fatalf("cancelled target was paid or not fully refunded: %+v", wallet)
	}
	var prizes int
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='prize'`).Scan(&prizes); err != nil || prizes != 0 {
		t.Fatalf("cancelled target prize ledger entries=%d err=%v", prizes, err)
	}
}

func TestSettlementJobAuthorizationVersionsAndStateGuards(t *testing.T) {
	f, _ := drawnFixtureOrder(t)
	ctx := context.Background()
	actor := settlementActor(f)
	automatic := "automatic"
	policy := setSettlementMode(t, f, actor, 1, &automatic)
	c, err := f.service.PeriodSettlementContext(ctx, f.brand, f.period.ID)
	if err != nil {
		t.Fatal(err)
	}

	denied := access.Account{ID: actor.ID, Type: access.AccountAdmin, BrandIDs: actor.BrandIDs}
	denied.Roles = []access.Role{{BrandID: f.brand}}
	super := actor
	super.SuperAdmin = true
	for _, attempt := range []struct {
		name  string
		brand string
		user  access.Account
		want  error
	}{
		{"missing policy permission", f.brand, denied, ErrDenied},
		{"super admin policy write", f.brand, super, ErrDenied},
		{"cross brand policy write", storeTestOtherBrand, actor, ErrDenied},
	} {
		t.Run(attempt.name, func(t *testing.T) {
			tx, beginErr := f.db.Begin(ctx)
			if beginErr != nil {
				t.Fatal(beginErr)
			}
			_, callErr := f.service.SaveSettlementPolicy(ctx, tx, attempt.brand, attempt.user, SettlementPolicyInput{Version: 1, Mode: &automatic, Reason: "denial check"}, policyMeta(actor.ID))
			_ = tx.Rollback(ctx)
			if !errors.Is(callErr, attempt.want) {
				t.Fatalf("SaveSettlementPolicy error=%v, want %v", callErr, attempt.want)
			}
		})
	}

	startInput := SettlementStartInput{Version: c.PeriodVersion, PolicyVersion: policy.Version, DrawResultID: *c.DrawResultID, Reason: "authorization and version checks"}
	for _, attempt := range []struct {
		name  string
		brand string
		user  access.Account
		input SettlementStartInput
		want  error
	}{
		{"missing run permission", f.brand, denied, startInput, ErrDenied},
		{"super admin run", f.brand, super, startInput, ErrDenied},
		{"cross brand run", storeTestOtherBrand, actor, startInput, ErrDenied},
		{"stale policy version", f.brand, actor, SettlementStartInput{Version: c.PeriodVersion, PolicyVersion: policy.Version + 1, DrawResultID: *c.DrawResultID, Reason: startInput.Reason}, ErrVersion},
	} {
		t.Run(attempt.name, func(t *testing.T) {
			tx, beginErr := f.db.Begin(ctx)
			if beginErr != nil {
				t.Fatal(beginErr)
			}
			_, callErr := f.service.StartSettlement(ctx, tx, attempt.brand, attempt.user, f.period.ID, attempt.input, policyMeta(actor.ID))
			_ = tx.Rollback(ctx)
			if !errors.Is(callErr, attempt.want) {
				t.Fatalf("StartSettlement error=%v, want %v", callErr, attempt.want)
			}
		})
	}

	job := startFixtureSettlement(t, f, actor, policy.Version)
	for _, attempt := range []struct {
		name   string
		brand  string
		user   access.Account
		action string
		want   error
	}{
		{"missing approve permission", f.brand, denied, "approve", ErrDenied},
		{"super admin approve", f.brand, super, "approve", ErrDenied},
		{"cross brand approve", storeTestOtherBrand, actor, "approve", ErrDenied},
		{"approve automatic job", f.brand, actor, "approve", ErrState},
		{"retry active job", f.brand, actor, "retry", ErrState},
	} {
		t.Run(attempt.name, func(t *testing.T) {
			actionJob := job
			if attempt.brand != f.brand {
				actionJob.ID = "0199a000-0000-7000-8000-000000000002"
			}
			tx, beginErr := f.db.Begin(ctx)
			if beginErr != nil {
				t.Fatal(beginErr)
			}
			_, callErr := f.service.ActOnSettlement(ctx, tx, attempt.brand, attempt.user, actionJob.ID, attempt.action, SettlementActionInput{Version: job.Version, Reason: "guard check"}, policyMeta(actor.ID))
			_ = tx.Rollback(ctx)
			if !errors.Is(callErr, attempt.want) {
				t.Fatalf("ActOnSettlement error=%v, want %v", callErr, attempt.want)
			}
		})
	}

	if _, err = f.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	completed, err := f.service.SettlementJob(ctx, f.brand, job.ID)
	if err != nil || completed.State != "completed" {
		t.Fatalf("authorization checks changed worker result: %+v %v", completed, err)
	}
}
