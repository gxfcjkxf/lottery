package betting

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
)

func correctionWithInput(t *testing.T, f bettingFixture, actor access.Account, version int64, policyVersion *int64, result rules.Draw) (Correction, error) {
	t.Helper()
	ctx := context.Background()
	c, err := f.service.CorrectionContext(ctx, f.brand, f.period.ID)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.service.CreateCorrection(ctx, tx, f.brand, actor, *c.DrawResultID, CorrectionInput{
		Version: version, PolicyVersion: policyVersion, Result: result, Reason: "integration correction",
	}, policyMeta(actor.ID))
	if err != nil {
		_ = tx.Rollback(ctx)
		return out, err
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return out, nil
}

func TestCorrectionOnDrawnPeriodCompletesWithoutCreatingSettlementJob(t *testing.T) {
	f, _ := drawnFixtureOrder(t)
	ctx := context.Background()
	a := correctionActor(f)
	mode := "automatic"
	setSettlementMode(t, f, a, 1, &mode)
	beforeWallet := walletBySource(t, f)
	var beforeJobs int
	if err := f.db.QueryRow(ctx, `SELECT count(*) FROM settlement_jobs WHERE period_id=$1`, f.period.ID).Scan(&beforeJobs); err != nil {
		t.Fatal(err)
	}

	c := createFixtureCorrection(t, f, correctedDigits(1, 2, 2))
	if c.State != "completed" || c.NewJobID != nil || c.PreviousJobID != nil || c.DrawResultID == c.PreviousDrawResultID {
		t.Fatalf("drawn-period correction was not immediately completed: %+v", c)
	}
	if c.Result.Digits[0] != 1 || c.Result.Digits[1] != 2 || c.Result.Digits[2] != 2 {
		t.Fatalf("corrected result not published: %+v", c.Result)
	}
	if got := walletBySource(t, f); got != beforeWallet {
		t.Fatalf("correction changed wallet: before=%+v after=%+v", beforeWallet, got)
	}
	var afterJobs int
	if err := f.db.QueryRow(ctx, `SELECT count(*) FROM settlement_jobs WHERE period_id=$1`, f.period.ID).Scan(&afterJobs); err != nil || afterJobs != beforeJobs {
		t.Fatalf("correction created settlement job: before=%d after=%d err=%v", beforeJobs, afterJobs, err)
	}
	period, err := f.service.PeriodSettlementContext(ctx, f.brand, f.period.ID)
	if err != nil || !period.CanStart || period.DrawResultID == nil || *period.DrawResultID != c.DrawResultID {
		t.Fatalf("period not ready for normal settlement: %+v %v", period, err)
	}
}

func TestCorrectionSnapshotsManualPolicyForNewSettlementGeneration(t *testing.T) {
	f, o, old := settledCorrectionFixture(t)
	ctx := context.Background()
	a := settlementActor(f)
	manual := "manual"
	currentPolicy, err := f.service.SettlementPolicy(ctx, f.brand)
	if err != nil {
		t.Fatal(err)
	}
	policy := setSettlementMode(t, f, a, currentPolicy.Version, &manual)
	policyVersion := policy.Version
	c := createFixtureCorrection(t, f, correctedDigits(1, 2, 2))
	if c.State != "reversing" || c.PolicyVersion == nil || *c.PolicyVersion != policy.Version || c.Mode == nil || *c.Mode != manual {
		t.Fatalf("correction did not capture current manual policy: %+v", c)
	}
	if _, err := f.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	c, err = f.service.Correction(ctx, f.brand, c.ID)
	if err != nil || c.State != "resettling" || c.NewJobID == nil {
		t.Fatalf("correction did not publish a new generation: %+v %v", c, err)
	}
	job, err := f.service.SettlementJob(ctx, f.brand, *c.NewJobID)
	if err != nil || job.Mode != manual || job.PolicyVersion != policyVersion || job.PreviousJobID == nil || *job.PreviousJobID != old.ID || job.Generation != old.Generation+1 {
		t.Fatalf("new job lost correction policy snapshot: %+v %v", job, err)
	}
	automatic := "automatic"
	changed := setSettlementMode(t, f, a, policy.Version, &automatic)
	if changed.Version != policy.Version+1 {
		t.Fatalf("policy did not change after correction start: %+v", changed)
	}
	if _, err = f.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	job, err = f.service.SettlementJob(ctx, f.brand, job.ID)
	original, orderErr := f.service.SettlementJob(ctx, f.brand, old.ID)
	if err != nil || orderErr != nil || job.State != "awaiting_approval" || job.Mode != manual || job.Current != true || original.Current || original.State != "completed" {
		t.Fatalf("manual snapshot did not await approval: new=%+v old=%+v errors=%v/%v", job, original, err, orderErr)
	}
	reset, err := f.service.Order(ctx, f.brand, f.member, o.ID)
	if err != nil || reset.Status != "placed" || reset.PayoutEntryID != nil {
		t.Fatalf("original win was not reversed before new approval: %+v %v", reset, err)
	}
	approved, err := actOnFixtureSettlement(t, f, a, job, "approve", job.Version)
	if err != nil || approved.State != "paying" {
		t.Fatalf("approve correction generation: %+v %v", approved, err)
	}
	if _, err = f.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	c, err = f.service.Correction(ctx, f.brand, c.ID)
	job, jobErr := f.service.SettlementJob(ctx, f.brand, *c.NewJobID)
	if err != nil || jobErr != nil || c.State != "completed" || job.State != "completed" {
		t.Fatalf("approved correction did not complete: correction=%+v job=%+v errors=%v/%v", c, job, err, jobErr)
	}
}

func TestCorrectionCanReturnToEarlierResultWithoutRepeatingReversal(t *testing.T) {
	f, o, first := settledCorrectionFixture(t)
	ctx := context.Background()
	a := correctionActor(f)
	firstCorrection := createFixtureCorrection(t, f, correctedDigits(1, 2, 2))
	if _, err := f.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	firstCorrection, err := f.service.Correction(ctx, f.brand, firstCorrection.ID)
	if err != nil || firstCorrection.State != "completed" || firstCorrection.NewJobID == nil {
		t.Fatalf("first correction incomplete: %+v %v", firstCorrection, err)
	}
	secondJob, err := f.service.SettlementJob(ctx, f.brand, *firstCorrection.NewJobID)
	if err != nil || secondJob.Generation != 2 || secondJob.Current != true {
		t.Fatalf("generation two not current: %+v %v", secondJob, err)
	}
	period, err := f.service.CorrectionContext(ctx, f.brand, f.period.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondCorrection, err := correctionWithInput(t, f, a, period.PeriodVersion-1, ptrInt64(period.PolicyVersion), correctedDigits(1, 2, 1))
	if !errors.Is(err, ErrVersion) {
		t.Fatalf("stale correction version error=%v, want ErrVersion", err)
	}
	policyVersion := period.PolicyVersion
	secondCorrection, err = correctionWithInput(t, f, a, period.PeriodVersion, &policyVersion, correctedDigits(1, 2, 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	secondCorrection, err = f.service.Correction(ctx, f.brand, secondCorrection.ID)
	third, jobErr := f.service.SettlementJob(ctx, f.brand, *secondCorrection.NewJobID)
	firstFresh, firstErr := f.service.SettlementJob(ctx, f.brand, first.ID)
	secondFresh, secondErr := f.service.SettlementJob(ctx, f.brand, secondJob.ID)
	freshOrder, orderErr := f.service.Order(ctx, f.brand, f.member, o.ID)
	if err != nil || jobErr != nil || firstErr != nil || secondErr != nil || orderErr != nil || secondCorrection.State != "completed" || third.Generation != 3 || !third.Current || firstFresh.Current || secondFresh.Current || freshOrder.Status != "won" || freshOrder.PrizePoints != 10 {
		t.Fatalf("round-trip correction failed: correction=%+v gen3=%+v old1=%+v old2=%+v order=%+v errors=%v/%v/%v/%v/%v", secondCorrection, third, firstFresh, secondFresh, freshOrder, err, jobErr, firstErr, secondErr, orderErr)
	}
	var debits, prizes, reversals int
	if err = f.db.QueryRow(ctx, `SELECT count(*) FILTER (WHERE entry_type='bet'),count(*) FILTER (WHERE entry_type='prize'),count(*) FILTER (WHERE entry_type='prize_reversal') FROM point_ledger_entries WHERE member_id=$1`, f.member).Scan(&debits, &prizes, &reversals); err != nil || debits != 1 || prizes != 2 || reversals != 1 {
		t.Fatalf("ledger entries debits=%d prizes=%d reversals=%d err=%v", debits, prizes, reversals, err)
	}
	var linkedReversals int
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE reversal_of=$1 AND entry_type='prize_reversal'`, *o.PayoutEntryID).Scan(&linkedReversals); err != nil || linkedReversals != 1 {
		t.Fatalf("initial prize reversal count=%d err=%v", linkedReversals, err)
	}
	if firstFresh.DrawResultID == third.DrawResultID || secondFresh.DrawResultID == third.DrawResultID {
		t.Fatal("historical generations were rewritten")
	}
}

func TestCorrectionFreezesAwaitingOldManualJobAndCreatesNoWinner(t *testing.T) {
	f, o := drawnFixtureOrder(t)
	ctx := context.Background()
	a := correctionActor(f)
	manual := "manual"
	policy := setSettlementMode(t, f, a, 1, &manual)
	old := startFixtureSettlement(t, f, a, policy.Version)
	if _, err := f.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	old, err := f.service.SettlementJob(ctx, f.brand, old.ID)
	if err != nil || old.State != "awaiting_approval" {
		t.Fatalf("old job not awaiting approval: %+v %v", old, err)
	}
	policyVersion := policy.Version
	c := createFixtureCorrection(t, f, correctedDigits(1, 2, 2))
	if c.State != "reversing" {
		t.Fatalf("correction did not freeze period: %+v", c)
	}
	if _, err = actOnFixtureSettlement(t, f, a, old, "approve", old.Version); !errors.Is(err, ErrState) {
		t.Fatalf("old job approval error=%v, want ErrState", err)
	}
	if _, err = f.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	oldAfter, err := f.service.SettlementJob(ctx, f.brand, old.ID)
	orderAfter, orderErr := f.service.Order(ctx, f.brand, f.member, o.ID)
	if err != nil || orderErr != nil || oldAfter.State != "awaiting_approval" || orderAfter.Status != "placed" || walletBySource(t, f)[1][0] != 0 {
		t.Fatalf("old job paid during correction: job=%+v order=%+v errors=%v/%v", oldAfter, orderAfter, err, orderErr)
	}
	if _, err = f.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	c, err = f.service.Correction(ctx, f.brand, c.ID)
	newJob, jobErr := f.service.SettlementJob(ctx, f.brand, *c.NewJobID)
	if err != nil || jobErr != nil || newJob.State != "awaiting_approval" || newJob.Mode != manual || newJob.PolicyVersion != policyVersion {
		t.Fatalf("new correction job did not await manual approval: correction=%+v job=%+v errors=%v/%v", c, newJob, err, jobErr)
	}
	if _, err = actOnFixtureSettlement(t, f, a, old, "approve", old.Version); !errors.Is(err, ErrState) {
		t.Fatalf("old generation became approvable: %v", err)
	}
	if _, err = actOnFixtureSettlement(t, f, a, newJob, "approve", newJob.Version); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	c, err = f.service.Correction(ctx, f.brand, c.ID)
	orderAfter, orderErr = f.service.Order(ctx, f.brand, f.member, o.ID)
	if err != nil || orderErr != nil || c.State != "completed" || orderAfter.Status != "lost" || orderAfter.PrizePoints != 0 || walletBySource(t, f)[1][0] != 0 {
		t.Fatalf("corrected loss produced a payout: correction=%+v order=%+v errors=%v/%v", c, orderAfter, err, orderErr)
	}
}

func TestCorrectionPublicationFailureIsManualRetryableAndDoesNotRepeatReversal(t *testing.T) {
	f, o, old := settledCorrectionFixture(t)
	ctx := context.Background()
	a := correctionActor(f)
	c := createFixtureCorrection(t, f, correctedDigits(1, 2, 2))
	if _, err := f.db.Exec(ctx, `CREATE FUNCTION test_reject_correction_job() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.correction_id IS NOT NULL THEN RAISE EXCEPTION 'test correction publication outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_reject_correction_job BEFORE INSERT ON settlement_jobs FOR EACH ROW EXECUTE FUNCTION test_reject_correction_job()`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessCorrections(ctx, 20); err == nil {
		t.Fatal("publication storage failure swallowed")
	}
	c, err := f.service.Correction(ctx, f.brand, c.ID)
	if err != nil || c.State != "failed" || c.LastErrorCode == nil || *c.LastErrorCode != "CORRECTION_STORAGE_FAILED" || c.NewJobID != nil || c.ReversedCount != 1 || c.ReversedPoints != "10" {
		t.Fatalf("publication failure state unexpected: %+v %v", c, err)
	}
	var failureRows, phaseFailures, linkedReversals int
	if err = f.db.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE order_id IS NULL) FROM draw_correction_failures WHERE correction_id=$1`, c.ID).Scan(&failureRows, &phaseFailures); err != nil || failureRows != 1 || phaseFailures != 1 {
		t.Fatalf("phase failure rows=%d null-order failures=%d err=%v", failureRows, phaseFailures, err)
	}
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE reversal_of=$1 AND entry_type='prize_reversal'`, *o.PayoutEntryID).Scan(&linkedReversals); err != nil || linkedReversals != 1 {
		t.Fatalf("reversal count=%d err=%v", linkedReversals, err)
	}
	if n, processErr := f.service.ProcessCorrections(ctx, 20); processErr != nil || n != 0 {
		t.Fatalf("failed correction auto-retried: n=%d err=%v", n, processErr)
	}
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE reversal_of=$1 AND entry_type='prize_reversal'`, *o.PayoutEntryID).Scan(&linkedReversals); err != nil || linkedReversals != 1 {
		t.Fatalf("automatic retry repeated reversal: count=%d err=%v", linkedReversals, err)
	}
	if _, err = f.db.Exec(ctx, `DROP TRIGGER test_reject_correction_job ON settlement_jobs`); err != nil {
		t.Fatal(err)
	}
	retried, err := func() (Correction, error) {
		tx, beginErr := f.db.Begin(ctx)
		if beginErr != nil {
			t.Fatal(beginErr)
		}
		out, retryErr := f.service.RetryCorrection(ctx, tx, f.brand, a, c.ID, SettlementActionInput{Version: c.Version, Reason: "retry publication after storage recovery"}, policyMeta(a.ID))
		if retryErr != nil {
			_ = tx.Rollback(ctx)
			return out, retryErr
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			t.Fatal(commitErr)
		}
		return out, nil
	}()
	if err != nil || retried.State != "reversing" || retried.ReversedCount != 1 {
		t.Fatalf("manual correction retry failed: %+v %v", retried, err)
	}
	if _, err = f.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	retried, err = f.service.Correction(ctx, f.brand, c.ID)
	if err != nil || retried.State != "resettling" || retried.NewJobID == nil || retried.ReversedCount != 1 {
		t.Fatalf("retry republished incorrectly: %+v %v", retried, err)
	}
	oldJob, err := f.service.SettlementJob(ctx, f.brand, old.ID)
	if err != nil || oldJob.Current {
		t.Fatalf("old generation current=%v err=%v", oldJob.Current, err)
	}
}

func TestCorrectionAuthorizationVersionsAndJSONValidation(t *testing.T) {
	f, _, _ := settledCorrectionFixture(t)
	ctx := context.Background()
	actor := correctionActor(f)
	period, err := f.service.CorrectionContext(ctx, f.brand, f.period.ID)
	if err != nil {
		t.Fatal(err)
	}
	policyVersion := period.PolicyVersion
	deniedRun := actor
	deniedRun.Roles = append([]access.Role(nil), actor.Roles...)
	deniedRun.Roles[0].Permissions = append([]access.Permission(nil), actor.Roles[0].Permissions...)
	filtered := deniedRun.Roles[0].Permissions[:0]
	for _, permission := range deniedRun.Roles[0].Permissions {
		if permission.Resource != "settlement" || permission.Action != "run" {
			filtered = append(filtered, permission)
		}
	}
	deniedRun.Roles[0].Permissions = filtered
	super := actor
	super.SuperAdmin = true
	for _, attempt := range []struct {
		name  string
		brand string
		user  access.Account
		ver   int64
		pv    *int64
		want  error
	}{
		{"cross brand", storeTestOtherBrand, actor, period.PeriodVersion, &policyVersion, ErrDenied},
		{"super admin", f.brand, super, period.PeriodVersion, &policyVersion, ErrDenied},
		{"missing settlement run", f.brand, deniedRun, period.PeriodVersion, &policyVersion, ErrDenied},
		{"stale period version", f.brand, actor, period.PeriodVersion - 1, &policyVersion, ErrVersion},
		{"stale policy version", f.brand, actor, period.PeriodVersion, ptrInt64(policyVersion - 1), ErrVersion},
	} {
		t.Run(attempt.name, func(t *testing.T) {
			tx, beginErr := f.db.Begin(ctx)
			if beginErr != nil {
				t.Fatal(beginErr)
			}
			_, callErr := f.service.CreateCorrection(ctx, tx, attempt.brand, attempt.user, *period.DrawResultID, CorrectionInput{Version: attempt.ver, PolicyVersion: attempt.pv, Result: correctedDigits(1, 2, 2), Reason: "authorization guard"}, policyMeta(actor.ID))
			_ = tx.Rollback(ctx)
			if !errors.Is(callErr, attempt.want) {
				t.Fatalf("CreateCorrection error=%v, want %v", callErr, attempt.want)
			}
		})
	}
	for _, raw := range []string{
		`{"version":1,"policy_version":null,"result":{"regular":[],"special":[],"digits":[1,2,2]},"reason":"x","version":1}`,
		`{"version":1,"policy_version":null,"result":{"regular":[],"special":[],"digits":[1,2,2],"digits":[1,2,1]},"reason":"x"}`,
		`{"version":1,"policy_version":null,"result":{"regular":[],"special":[],"digits":[1,2,2],"extra":[]},"reason":"x"}`,
		`{"version":1,"policy_version":null,"result":{"regular":[],"special":[],"digits":[1,2,2]},"reason":"x","extra":true}`,
	} {
		var input CorrectionInput
		if json.Unmarshal([]byte(raw), &input) == nil {
			t.Fatalf("accepted malformed correction JSON: %s", raw)
		}
	}
}

func ptrInt64(value int64) *int64 { return &value }
