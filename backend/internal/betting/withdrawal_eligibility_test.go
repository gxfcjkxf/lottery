package betting

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/gxfcjkxf/lottery/backend/internal/withdrawal"
	"github.com/jackc/pgx/v5"
)

func eligibilityAdmin(t *testing.T, f bettingFixture) access.Account {
	t.Helper()
	ctx := context.Background()
	role := ids.New()
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, f.version.CreatedBy, f.brand); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name) VALUES($1,$2,$3,'Withdrawal eligibility tests')`, role, f.brand, "eligibility_"+role[:8]); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, f.version.CreatedBy, role); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT $1,key FROM permissions WHERE key IN ('withdrawal_policy.write.brand','withdrawal.view.brand','withdrawal.approve.brand','withdrawal.cancel.brand','withdrawal.mark_paid.brand')`, role)
		return err
	})
	a, err := (adminsys.Store{DB: f.db}).Account(ctx, f.version.CreatedBy)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func eligibilityBrandPolicy(t *testing.T, f bettingFixture, a access.Account, n string) {
	t.Helper()
	s := withdrawal.Service{DB: f.db}
	p, err := s.BrandPolicy(context.Background(), f.brand)
	if err != nil {
		t.Fatal(err)
	}
	c := p.Config
	c.Enabled = true
	c.TurnoverMultiple = n
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, err := s.UpdateBrand(context.Background(), tx, f.brand, a, withdrawal.BrandInput{Version: p.Version, Config: c, Reason: "explicit test turnover policy"}, policyMeta(a.ID))
		return err
	})
}

func eligibilityGamePolicy(t *testing.T, f bettingFixture, a access.Account, n string) {
	t.Helper()
	s := withdrawal.Service{DB: f.db}
	p, err := s.GamePolicy(context.Background(), f.brand, f.game.ID)
	if err != nil {
		t.Fatal(err)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, err := s.UpdateGame(context.Background(), tx, f.brand, f.game.ID, a, withdrawal.GameInput{Version: p.Version, Config: withdrawal.GameConfig{TurnoverMultiple: &n}, Reason: "explicit game turnover test policy"}, policyMeta(a.ID))
		return err
	})
}

func eligibilitySecondGame(t *testing.T, first bettingFixture) bettingFixture {
	t.Helper()
	f := first
	ctx := context.Background()
	s := rulebook.Store{DB: f.db}
	a := bettingRuleActor(first.version.CreatedBy, f.brand, "write", "validate", "submit")
	r := bettingRuleActor(first.version.ReviewedBy, f.brand, "review")
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		f.game, err = s.CreateGame(ctx, tx, f.brand, a, "eligibility_"+ids.New()[:8], "Second turnover game", bettingDefinition().Model, "UTC", "test game", points.Metadata{})
		return err
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		f.play, err = s.CreatePlay(ctx, tx, f.brand, a, f.game.ID, "exact", "Exact", "test play", points.Metadata{})
		return err
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		f.version, err = s.CreateVersion(ctx, tx, f.brand, a, f.play.ID, bettingDefinition(), "immediate", "test version", points.Metadata{})
		return err
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		f.version, err = s.Validate(ctx, tx, f.brand, a, f.version.ID, f.version.Version, []rules.ValidationCase{{Name: "exact", Selection: rules.Selection{Digits: [][]int{{1}, {2}, {1}}}, Draw: rules.Draw{Digits: []int{1, 2, 1}}, Multiplier: 1, ExpectedBetPoints: amountPtr(1), ExpectedPrizePoints: amountPtr(10), ExpectedWon: boolPtr(true)}}, "test validate", points.Metadata{})
		return err
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		f.version, err = s.Submit(ctx, tx, f.brand, a, f.version.ID, f.version.Version, "test submit", points.Metadata{})
		return err
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		f.version, err = s.Review(ctx, tx, f.brand, r, f.version.ID, f.version.Version, true, true, "test review", points.Metadata{})
		return err
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		f.period, err = s.OpenPeriod(ctx, tx, f.brand, f.game.ID, "eligibility-second", time.Now().UTC().Add(-time.Minute), first.period.BetEndAt, first.period.DrawAt)
		return err
	})
	v := readBettingPolicyVersions(t, f.service, f.brand, f.game.ID)
	f.input = Input{PeriodID: f.period.ID, PlayID: f.play.ID, RuleVersionID: f.version.ID, Selection: rules.Selection{Digits: [][]int{{0}, {0}, {0}}}, Multiplier: 1, PolicyVersions: &v}
	return f
}

func eligibilityFund(t *testing.T, f bettingFixture, amounts ...points.Amount) {
	t.Helper()
	for i, n := range amounts {
		if n == 0 {
			continue
		}
		var delta points.Balance
		delta[i][0] = n
		bettingTx(t, f.db, func(tx pgx.Tx) error {
			_, err := (points.Store{DB: f.db}).Post(context.Background(), tx, points.Change{BrandID: f.brand, MemberID: f.member, EntryType: "adjustment", ReferenceType: "eligibility_test", OperationKey: "eligibility-fund:" + ids.New(), Reason: "explicit source funding", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: []points.Allocation{{Source: []string{"recharge", "winning", "gift", "commission"}[i], State: "available", Points: n}}})
			return err
		})
	}
}

func eligibilitySettle(t *testing.T, fixtures ...bettingFixture) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := (rulebook.Store{DB: fixtures[0].db}).Tick(ctx); err != nil {
			t.Fatal(err)
		}
		ready := true
		for _, f := range fixtures {
			var state string
			if err := f.db.QueryRow(ctx, `SELECT status FROM periods WHERE id=$1`, f.period.ID).Scan(&state); err != nil {
				t.Fatal(err)
			}
			ready = ready && state == "waiting_draw"
		}
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("test periods did not reach natural draw time")
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, f := range fixtures {
		var version int64
		if err := f.db.QueryRow(ctx, `SELECT version FROM periods WHERE id=$1`, f.period.ID).Scan(&version); err != nil {
			t.Fatal(err)
		}
		draw := rules.Draw{Digits: []int{1, 2, 1}}
		var previous []byte
		err := f.db.QueryRow(ctx, `SELECT d.result FROM periods p JOIN draw_results d ON d.id=p.draw_result_id WHERE p.brand_id=$1 AND p.game_id=$2 AND p.draw_at<$3 ORDER BY p.draw_at DESC,p.id DESC LIMIT 1`, f.brand, f.game.ID, f.period.DrawAt).Scan(&previous)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal(err)
		}
		if previous != nil {
			var last rules.Draw
			if err = json.Unmarshal(previous, &last); err != nil {
				t.Fatal(err)
			}
			if len(last.Digits) == 3 && last.Digits[0] == 1 && last.Digits[1] == 2 && last.Digits[2] == 1 {
				draw.Digits = []int{1, 1, 1}
			}
		}
		publicManual(t, f, f.period.ID, version, f.period.PeriodNo, draw, f.period.DrawAt)
	}
	f := fixtures[0]
	actor := settlementActor(f)
	automatic := "automatic"
	policy, err := f.service.SettlementPolicy(ctx, f.brand)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Mode == nil || *policy.Mode != automatic {
		policy = setSettlementMode(t, f, actor, policy.Version, &automatic)
	}
	for _, f := range fixtures {
		startFixtureSettlement(t, f, settlementActor(f), policy.Version)
	}
	if _, err := f.service.ProcessSettlements(ctx, 100); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		var status string
		if err := f.db.QueryRow(ctx, `SELECT status FROM periods WHERE id=$1`, f.period.ID).Scan(&status); err != nil || status != "settled" {
			t.Fatalf("period not final: %s %v", status, err)
		}
	}
}

func eligibilityCreate(t *testing.T, f bettingFixture, in withdrawal.OrderInput) (withdrawal.Order, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	o, err := (withdrawal.OrderService{DB: f.db, Points: points.Store{DB: f.db}, Eligibility: withdrawal.TurnoverChecker{}}).Create(ctx, tx, f.brand, f.member, in, points.Metadata{ActorType: "user", ActorID: f.user.User.ID, RequestID: ids.New()})
	if err == nil {
		err = tx.Commit(ctx)
	}
	return o, err
}

func eligibilityAdvance(t *testing.T, f bettingFixture, a access.Account, o withdrawal.Order, action string) withdrawal.Order {
	t.Helper()
	ctx := context.Background()
	var next withdrawal.Order
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		next, err = (withdrawal.OrderService{DB: f.db, Points: points.Store{DB: f.db}, Eligibility: withdrawal.TurnoverChecker{}}).Advance(ctx, tx, f.brand, o.ID, action, a, withdrawal.ActionInput{Version: o.Version, ClientKey: "eligibility-action:" + ids.New(), Reason: "audited test state transition"}, policyMeta(a.ID))
		return err
	})
	return next
}

func TestRealWithdrawalTurnoverAcrossGamesSnapshotsAndSuccessCutoff(t *testing.T) {
	f := newBettingFixtureWithWindow(t, storeTestBrand, 5*time.Second, 5100*time.Millisecond)
	g := eligibilitySecondGame(t, f)
	a := eligibilityAdmin(t, f)
	eligibilityBrandPolicy(t, f, a, "2")
	eligibilityGamePolicy(t, g, a, "5")
	eligibilityFund(t, f, 100, 250, 100)
	x := f.input
	x.Multiplier = 100
	first, err := placeBettingOrder(t, f, x, "eligibility-first-recharge")
	if err != nil {
		t.Fatal(err)
	}
	x = g.input
	x.Multiplier = 250
	second, err := placeBettingOrder(t, g, x, "eligibility-second-winning")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Allocation) != 1 || first.Allocation[0].Source != "recharge" || len(second.Allocation) != 1 || second.Allocation[0].Source != "winning" {
		t.Fatal("source fixture did not fund actual stakes as intended")
	}
	eligibilityBrandPolicy(t, f, a, "1000")
	eligibilityGamePolicy(t, g, a, "1000")
	eligibilitySettle(t, f, g)
	in := withdrawal.OrderInput{Points: 30, ClientKey: "real-eligibility-request", SourceAllocation: []points.Allocation{{Source: "gift", State: "available", Points: 30}}}
	o, err := eligibilityCreate(t, f, in)
	if err != nil {
		t.Fatal(err)
	}
	var evidence struct {
		Qualification struct {
			Count       string `json:"valid_order_count"`
			Points      string `json:"valid_points"`
			Numerator   string `json:"credit_numerator"`
			Denominator string `json:"credit_denominator"`
			Digest      string `json:"order_snapshot_digest"`
		} `json:"turnover_qualification"`
		Base struct {
			Points string `json:"points"`
		} `json:"turnover_base_snapshot"`
	}
	if err = json.Unmarshal(o.EligibilityEvidence, &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence.Qualification.Count != "2" || evidence.Qualification.Points != "350" || evidence.Qualification.Numerator != "100" || evidence.Qualification.Denominator != "1" || len(evidence.Qualification.Digest) != 64 || evidence.Base.Points != "100" {
		t.Fatalf("wrong real turnover evidence: %s", o.EligibilityEvidence)
	}
	replayed, err := eligibilityCreate(t, f, in)
	if err != nil || replayed.ID != o.ID {
		t.Fatal(replayed, err)
	}
	o = eligibilityAdvance(t, f, a, o, "cancel")
	if walletBySource(t, f)[2][0] != 100 {
		t.Fatal("cancel did not restore gift source")
	}
	in.ClientKey = "real-eligibility-after-cancel"
	o, err = eligibilityCreate(t, f, in)
	if err != nil {
		t.Fatal("failed/cancelled applications advanced turnover cycle", err)
	}
	o = eligibilityAdvance(t, f, a, o, "approve")
	o = eligibilityAdvance(t, f, a, o, "mark_paid")
	if o.State != "paid" || walletBySource(t, f)[2][0] != 70 {
		t.Fatal("actual paid transition changed wrong source")
	}
	in = withdrawal.OrderInput{Points: 1, ClientKey: "real-eligibility-next-cycle", SourceAllocation: []points.Allocation{{Source: "winning", State: "available", Points: 1}}}
	before := walletBySource(t, f)
	if _, err = eligibilityCreate(t, f, in); !errors.Is(err, withdrawal.ErrIneligible) {
		t.Fatalf("old bets reused after successful cutoff: %v", err)
	}
	if walletBySource(t, f) != before {
		t.Fatal("ineligible next cycle reserved points")
	}
}

func TestRealWithdrawalTurnoverCountsAllFundingSources(t *testing.T) {
	for source, amounts := range map[string][]points.Amount{"recharge": {3, 0, 0, 0}, "winning": {0, 2, 1, 0}, "gift": {0, 0, 3, 0}, "commission": {0, 0, 0, 3}} {
		t.Run(source, func(t *testing.T) {
			f := newBettingFixtureWithWindow(t, storeTestBrand, 2*time.Second, 2100*time.Millisecond)
			a := eligibilityAdmin(t, f)
			eligibilityBrandPolicy(t, f, a, "1")
			if source == "commission" {
				service := withdrawal.Service{DB: f.db}
				policy, err := service.BrandPolicy(context.Background(), f.brand)
				if err != nil {
					t.Fatal(err)
				}
				config := policy.Config
				config.AllowedSources = []string{"commission"}
				bettingTx(t, f.db, func(tx pgx.Tx) error {
					_, err := service.UpdateBrand(context.Background(), tx, f.brand, a, withdrawal.BrandInput{Version: policy.Version, Config: config, Reason: "explicitly authorize commission withdrawal test"}, policyMeta(a.ID))
					return err
				})
			}
			eligibilityFund(t, f, amounts...)
			x := f.input
			x.Multiplier = 2
			o, err := placeBettingOrder(t, f, x, "all-sources-"+source)
			if err != nil {
				t.Fatal(err)
			}
			if len(o.Allocation) != 1 || o.Allocation[0].Source != source {
				t.Fatal(o.Allocation)
			}
			eligibilitySettle(t, f)
			availableSource := "gift"
			if source == "recharge" {
				availableSource = "recharge"
			} else if source == "commission" {
				availableSource = "commission"
			}
			withdrawalOrder, createErr := eligibilityCreate(t, f, withdrawal.OrderInput{Points: 1, ClientKey: "all-sources-withdraw-" + source, SourceAllocation: []points.Allocation{{Source: availableSource, State: "available", Points: 1}}})
			err = createErr
			if err != nil {
				t.Fatal("valid source stake was not counted", err)
			}
			if source == "commission" {
				var evidence struct {
					Qualification struct {
						Points string `json:"valid_points"`
					} `json:"turnover_qualification"`
					Base struct {
						Points string `json:"points"`
					} `json:"turnover_base_snapshot"`
				}
				if err = json.Unmarshal(withdrawalOrder.EligibilityEvidence, &evidence); err != nil || evidence.Qualification.Points != "2" || evidence.Base.Points != "0" {
					t.Fatalf("commission stake must count for turnover while its balance stays outside the base: evidence=%s err=%v", withdrawalOrder.EligibilityEvidence, err)
				}
			}
		})
	}
}

func TestRealWithdrawalTurnoverRejectsPeriodLockWithoutReservation(t *testing.T) {
	f, _ := drawnFixtureOrder(t)
	a := eligibilityAdmin(t, f)
	eligibilityBrandPolicy(t, f, a, "0.000001")
	mode := "automatic"
	p := setSettlementMode(t, f, settlementActor(f), 1, &mode)
	startFixtureSettlement(t, f, settlementActor(f), p.Version)
	if _, err := f.service.ProcessSettlements(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	blocker, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(ctx)
	if _, err = blocker.Exec(ctx, `SELECT id FROM periods WHERE id=$1 FOR UPDATE`, f.period.ID); err != nil {
		t.Fatal(err)
	}
	before := walletBySource(t, f)
	started := time.Now()
	in := withdrawal.OrderInput{Points: 1, ClientKey: "turnover-period-lock-busy", SourceAllocation: []points.Allocation{{Source: "recharge", State: "available", Points: 1}}}
	if _, err = eligibilityCreate(t, f, in); !errors.Is(err, withdrawal.ErrTurnoverBusy) {
		t.Fatalf("expected period contention, not waiting or reserving: %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("wallet-held checker waited for period lock")
	}
	if walletBySource(t, f) != before {
		t.Fatal("busy checker occupied funds")
	}
	if err = blocker.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = eligibilityCreate(t, f, in); err == nil {
		t.Fatal("post-lock low turnover unexpectedly met current balance")
	} else if !errors.Is(err, withdrawal.ErrIneligible) {
		t.Fatalf("period release did not restore eligibility checking: %v", err)
	}
}

func eligibilityInspect(t *testing.T, f bettingFixture) (withdrawal.EligibilityDecision, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	wallet, err := (points.Store{DB: f.db}).LockedSnapshot(ctx, tx, f.brand, f.member)
	if err != nil {
		t.Fatal(err)
	}
	var at time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
		t.Fatal(err)
	}
	base := withdrawal.TurnoverBaseSnapshot{RechargeAvailable: wallet.BySource[0][0], GiftAvailable: wallet.BySource[2][0], Points: wallet.BySource[0][0] + wallet.BySource[2][0], WalletVersion: wallet.Version}
	return (withdrawal.TurnoverChecker{}).Check(ctx, tx, withdrawal.EligibilityInput{BrandID: f.brand, MemberID: f.member, Wallet: wallet, TurnoverBase: base, CutoffVersion: wallet.Version, CutoffAt: at})
}

func eligibilityExpectCredit(t *testing.T, decision withdrawal.EligibilityDecision, count, amount, numerator string) {
	t.Helper()
	var evidence struct {
		Qualification struct {
			Count     string `json:"valid_order_count"`
			Amount    string `json:"valid_points"`
			Numerator string `json:"credit_numerator"`
		} `json:"turnover_qualification"`
	}
	if err := json.Unmarshal(decision.Evidence, &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence.Qualification.Count != count || evidence.Qualification.Amount != amount || evidence.Qualification.Numerator != numerator {
		t.Fatalf("unexpected credit evidence: %s", decision.Evidence)
	}
}

func TestRealWithdrawalTurnoverExcludesCancelledAbnormalAndUnfinished(t *testing.T) {
	f := newBettingFixtureWithWindow(t, storeTestBrand, 3*time.Second, 3100*time.Millisecond)
	g := eligibilitySecondGame(t, f)
	eligibilityFund(t, f, 20)
	x := f.input
	x.Selection = rules.Selection{Digits: [][]int{{0}, {0}, {0}}}
	if _, err := placeBettingOrder(t, f, x, "turnover-valid-loss"); err != nil {
		t.Fatal(err)
	}
	o, err := placeBettingOrder(t, f, f.input, "turnover-cancelled-stake")
	if err != nil {
		t.Fatal(err)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, err := f.service.CancelAdmin(context.Background(), tx, f.brand, judgeActor(f), o.ID, o.Version, "exclude refunded stake", policyMeta(f.version.CreatedBy))
		return err
	})
	o, err = placeBettingOrder(t, f, f.input, "turnover-abnormal-stake")
	if err != nil {
		t.Fatal(err)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, err := f.service.MarkAbnormal(context.Background(), tx, f.brand, exceptionActor(f), o.ID, o.Version, "exclude abnormal stake", policyMeta(f.version.CreatedBy))
		return err
	})
	x = g.input
	x.Multiplier = 7
	if _, err = placeBettingOrder(t, g, x, "turnover-unfinished-stake"); err != nil {
		t.Fatal(err)
	}
	before := walletBySource(t, f)
	decision, err := eligibilityInspect(t, f)
	if err != nil {
		t.Fatal(err)
	}
	eligibilityExpectCredit(t, decision, "0", "0", "0")
	eligibilitySettle(t, f)
	decision, err = eligibilityInspect(t, f)
	if err != nil {
		t.Fatal(err)
	}
	eligibilityExpectCredit(t, decision, "1", "1", "1")
	if walletBySource(t, f) != before {
		t.Fatal("read-only credit checker changed balances or zero-prize settlement paid")
	}
}

func TestRealWithdrawalTurnoverMissingLegacySnapshotFailsEvenWithZeroBase(t *testing.T) {
	f := newBettingFixtureWithWindow(t, storeTestBrand, 2*time.Second, 2100*time.Millisecond)
	a := eligibilityAdmin(t, f)
	eligibilityBrandPolicy(t, f, a, "1")
	eligibilityFund(t, f, 0, 1, 0)
	// Simulate a pre-0043 order in this disposable schema without inventing a
	// historical N. All other original bet, ledger, audit and settlement guards
	// remain active; the capture trigger is restored before settlement.
	if _, err := f.db.Exec(context.Background(), `ALTER TABLE bet_orders DISABLE TRIGGER capture_bet_withdrawal_snapshot`); err != nil {
		t.Fatal(err)
	}
	o, err := placeBettingOrder(t, f, f.input, "legacy-snapshot-missing")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(context.Background(), `ALTER TABLE bet_orders ENABLE TRIGGER capture_bet_withdrawal_snapshot`); err != nil {
		t.Fatal(err)
	}
	var missing bool
	if err = f.db.QueryRow(context.Background(), `SELECT withdrawal_rule_snapshot IS NULL FROM bet_orders WHERE id=$1`, o.ID).Scan(&missing); err != nil || !missing {
		t.Fatal(missing, err)
	}
	eligibilitySettle(t, f)
	before := walletBySource(t, f)
	_, err = eligibilityCreate(t, f, withdrawal.OrderInput{Points: 1, ClientKey: "legacy-not-inferred", SourceAllocation: []points.Allocation{{Source: "winning", State: "available", Points: 1}}})
	if !errors.Is(err, withdrawal.ErrTurnoverEvidence) {
		t.Fatalf("legacy stake without its N must not be guessed or skipped at zero base: %v", err)
	}
	if walletBySource(t, f) != before {
		t.Fatal("missing snapshot check reserved points")
	}
}

func TestRealWithdrawalTurnoverCorrectionUsesOnlyCompletedCurrentGeneration(t *testing.T) {
	f, _, _ := settledCorrectionFixture(t)
	before, err := eligibilityInspect(t, f)
	if err != nil {
		t.Fatal(err)
	}
	eligibilityExpectCredit(t, before, "1", "1", "1")
	createFixtureCorrection(t, f, rules.Draw{Digits: []int{1, 1, 1}})
	during, err := eligibilityInspect(t, f)
	if err != nil {
		t.Fatal(err)
	}
	eligibilityExpectCredit(t, during, "0", "0", "0")
	if _, err = f.service.ProcessCorrections(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ProcessSettlements(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	after, err := eligibilityInspect(t, f)
	if err != nil {
		t.Fatal(err)
	}
	eligibilityExpectCredit(t, after, "1", "1", "1")
	readDigest := func(decision withdrawal.EligibilityDecision) string {
		var evidence struct {
			Qualification struct {
				Digest string `json:"order_snapshot_digest"`
			} `json:"turnover_qualification"`
		}
		if err := json.Unmarshal(decision.Evidence, &evidence); err != nil {
			t.Fatal(err)
		}
		return evidence.Qualification.Digest
	}
	if readDigest(before) == readDigest(after) {
		t.Fatal("credit digest failed to bind the changed immutable settlement generation")
	}
}

func TestRealWithdrawalTurnoverAfterSubmissionCountsInNextSuccessfulCycle(t *testing.T) {
	f := newBettingFixtureWithWindow(t, storeTestBrand, 2*time.Second, 2100*time.Millisecond)
	a := eligibilityAdmin(t, f)
	eligibilityBrandPolicy(t, f, a, "0.000001")
	eligibilityFund(t, f, 100)
	if _, err := placeBettingOrder(t, f, f.input, "cycle-first-qualified-stake"); err != nil {
		t.Fatal(err)
	}
	eligibilitySettle(t, f)
	in := withdrawal.OrderInput{Points: 1, ClientKey: "cycle-first-request", SourceAllocation: []points.Allocation{{Source: "recharge", State: "available", Points: 1}}}
	o, err := eligibilityCreate(t, f, in)
	if err != nil {
		t.Fatal(err)
	}
	// New betting is possible only after the prior lottery period is final.
	// This stake happens during review, after submission but before payment.
	next := f
	now := time.Now().UTC()
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		next.period, err = (rulebook.Store{DB: f.db}).OpenPeriod(context.Background(), tx, f.brand, f.game.ID, "next-withdrawal-cycle", now.Add(-time.Second), now.Add(2*time.Second), now.Add(2100*time.Millisecond))
		return err
	})
	next.input.PeriodID = next.period.ID
	bet, err := placeBettingOrder(t, next, next.input, "cycle-during-review-stake")
	if err != nil {
		t.Fatal(err)
	}
	eligibilitySettle(t, next)
	o = eligibilityAdvance(t, f, a, o, "approve")
	o = eligibilityAdvance(t, f, a, o, "mark_paid")
	in.ClientKey = "cycle-second-qualified-request"
	second, err := eligibilityCreate(t, next, in)
	if err != nil {
		t.Fatal("post-submission stake was incorrectly erased at payment", err)
	}
	eligibilityExpectCredit(t, withdrawal.EligibilityDecision{Evidence: second.EligibilityEvidence}, "1", "1", "1000000")
	if second.CycleFromVersion != o.ReserveVersion || second.CycleFromAt == nil || !second.CycleFromAt.Equal(o.CreatedAt) {
		t.Fatal("successful cycle used completion rather than submission boundary")
	}
	var betVersion int64
	if err = f.db.QueryRow(context.Background(), `SELECT version FROM point_ledger_entries WHERE id=$1`, bet.DebitEntryID).Scan(&betVersion); err != nil || betVersion <= second.CycleFromVersion {
		t.Fatal("review-period stake was not after reserve cursor", err)
	}
	public, err := json.Marshal(withdrawal.ToOrderView(second))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(public, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"eligibility_evidence", "turnover_qualification", "withdrawal_rule_snapshot", "order_snapshot_digest"} {
		if _, ok := fields[key]; ok {
			t.Fatalf("private qualification evidence leaked in order DTO: %s", key)
		}
	}
}
