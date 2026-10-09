//go:build browserfixture

// Creates an isolated real-workflow qualification fixture for browser tests.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/finance"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/gxfcjkxf/lottery/backend/internal/withdrawal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	fixtureBrand = "0199a000-0000-7000-8000-000000000001"
	fixtureDB    = "lottery_withdrawal_qualification_s9"
	fixtureAck   = "owned_synthetic_database"
)

func safeFixtureURL(raw, environment, confirmation, adminPassword, userPassword string) error {
	u, err := url.Parse(raw)
	if err != nil || environment != "test" || confirmation != fixtureAck || len(adminPassword) < 16 || len(userPassword) < 16 ||
		(u.Scheme != "postgres" && u.Scheme != "postgresql") ||
		(u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") ||
		(u.Port() != "5432" && u.Port() != "55432") || u.Path != "/"+fixtureDB || u.Fragment != "" ||
		u.User == nil || u.User.Username() != "lottery_test" {
		return errors.New("explicit owned synthetic qualification database required")
	}
	if _, ok := u.User.Password(); !ok || len(u.User.String()) < len(u.User.Username())+2 {
		return errors.New("fixture database password required")
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(query) != 1 || len(query["sslmode"]) != 1 || query.Get("sslmode") != "disable" {
		return errors.New("fixture connection overrides forbidden")
	}
	return nil
}

type fixtureUser struct {
	UserID   string `json:"user_id"`
	MemberID string `json:"member_id"`
}

type fixtureOutput struct {
	AdminID    string        `json:"admin_id"`
	ReviewerID string        `json:"reviewer_id"`
	Users      []fixtureUser `json:"users"`
	GameID     string        `json:"game_id"`
	PlayID     string        `json:"play_id"`
	RuleID     string        `json:"rule_id"`
	PeriodID   string        `json:"period_id"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "qualification fixture failed:", err.Error())
		os.Exit(1)
	}
}

func run() error {
	dsn := os.Getenv("DATABASE_URL")
	adminPassword := os.Getenv("WITHDRAWAL_QUAL_FIXTURE_ADMIN_PASSWORD")
	userPassword := os.Getenv("WITHDRAWAL_QUAL_FIXTURE_USER_PASSWORD")
	if err := safeFixtureURL(dsn, os.Getenv("APP_ENV"), os.Getenv("WITHDRAWAL_QUAL_FIXTURE_CONFIRM"), adminPassword, userPassword); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return errors.New("fixture database unavailable")
	}
	defer db.Close()
	if err = db.Ping(ctx); err != nil {
		return errors.New("fixture database unavailable")
	}
	if err = database.CheckMigrations(ctx, db); err != nil {
		return errors.New("qualification fixture requires the current baseline to be applied")
	}
	var brandExists bool
	if err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brands WHERE id=$1 AND code='aurora' AND status='active')`, fixtureBrand).Scan(&brandExists); err != nil || !brandExists {
		return errors.New("active Aurora test brand required")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return errors.New("fixture transaction unavailable")
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(80401009)`); err != nil {
		return err
	}
	var existing int
	err = tx.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM global_users WHERE username IN ('qual_user','qual_mobile_user'))+
	 +(SELECT count(*) FROM admin_accounts WHERE username IN ('qual_admin','qual_rule_reviewer'))
	 +(SELECT count(*) FROM point_accounts pa JOIN brand_members bm ON bm.id=pa.brand_member_id JOIN global_users u ON u.id=bm.global_user_id WHERE u.username IN ('qual_user','qual_mobile_user'))`).Scan(&existing)
	if err != nil {
		return err
	}
	if existing != 0 {
		return errors.New("named qualification fixture identities already exist")
	}
	var occupied int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM global_users)+(SELECT count(*) FROM admin_accounts)+(SELECT count(*) FROM point_accounts)`).Scan(&occupied); err != nil {
		return err
	}
	if occupied != 0 {
		return errors.New("qualification fixture requires empty identities and wallets")
	}

	adminHash, err := authcrypto.HashPassword(adminPassword)
	if err != nil {
		return errors.New("fixture administrator password rejected")
	}
	adminID, reviewerID := ids.New(), ids.New()
	var reviewerSecret [32]byte
	if _, err = rand.Read(reviewerSecret[:]); err != nil {
		return errors.New("reviewer credential generation failed")
	}
	reviewerHash, err := authcrypto.HashPassword(hex.EncodeToString(reviewerSecret[:]))
	if err != nil {
		return errors.New("reviewer credential generation failed")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,'qual_admin',$2),($3,'qual_rule_reviewer',$4)`, adminID, adminHash, reviewerID, reviewerHash); err != nil {
		return err
	}
	adminPerms := []string{"game.write.brand", "rule.write.brand", "rule.validate.brand", "rule.submit.brand", "draw.manual_create.brand", "settlement_policy.write.brand", "settlement.run.brand", "wallet.adjust.brand", "wallet.view.brand", "recharge.write.brand", "withdrawal_policy.write.brand", "withdrawal.view.brand", "withdrawal.approve.brand", "withdrawal.cancel.brand", "withdrawal.mark_paid.brand"}
	if err = addScopedRole(ctx, tx, adminID, fixtureBrand, "qualification_fixture_admin", "Qualification fixture operator", adminPerms); err != nil {
		return err
	}
	if err = addScopedRole(ctx, tx, reviewerID, fixtureBrand, "qualification_rule_reviewer", "Independent qualification rule reviewer", []string{"rule.review.brand"}); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}

	admin, err := (adminsys.Store{DB: db}).Account(ctx, adminID)
	if err != nil {
		return err
	}
	reviewer, err := (adminsys.Store{DB: db}).Account(ctx, reviewerID)
	if err != nil {
		return err
	}
	identityStore, err := identity.New(db)
	if err != nil {
		return errors.New("identity service unavailable")
	}
	users := make([]fixtureUser, 0, 2)
	sessions := make([]identity.Session, 0, 2)
	for _, username := range []string{"qual_user", "qual_mobile_user"} {
		registered, registerErr := identityMutation(ctx, db, func(tx pgx.Tx) (any, error) {
			result, e := identityStore.Register(ctx, tx, fixtureBrand, identity.RegisterInput{Username: username, Password: userPassword, Privacy: "dev-1", Terms: "dev-1"}, identity.Metadata{RequestID: ids.New(), Domain: "localhost"})
			if e != nil {
				return nil, e
			}
			if result.Error != nil {
				return nil, errors.New("synthetic fixture member registration failed")
			}
			var auth identity.Authentication
			if e = json.Unmarshal(result.Data, &auth); e != nil {
				return nil, e
			}
			return auth, nil
		})
		if registerErr != nil {
			return registerErr
		}
		auth := registered.(identity.Authentication)
		session, authErr := identityStore.Authenticate(ctx, fixtureBrand, auth.AccessToken)
		if authErr != nil {
			return errors.New("fixture member authentication failed")
		}
		users = append(users, fixtureUser{UserID: auth.User.ID, MemberID: auth.Member.ID})
		sessions = append(sessions, session)
	}

	meta := func(actor string) points.Metadata {
		return points.Metadata{ActorType: "admin", ActorID: actor, RequestID: ids.New(), Reason: "owned synthetic qualification browser fixture"}
	}
	withdrawals := withdrawal.Service{DB: db}
	brandPolicy, err := withdrawals.BrandPolicy(ctx, fixtureBrand)
	if err != nil {
		return err
	}
	config := brandPolicy.Config
	config.Enabled = true
	config.TurnoverMultiple = "0.000001"
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		_, e := withdrawals.UpdateBrand(ctx, tx, fixtureBrand, admin, withdrawal.BrandInput{Version: brandPolicy.Version, Config: config, Reason: "explicit qualification browser fixture policy"}, meta(adminID))
		return e
	}); err != nil {
		return err
	}

	pointsStore := points.Store{DB: db}
	financeStore := finance.Service{DB: db, Points: pointsStore}
	for _, user := range users {
		if err = inTx(ctx, db, func(tx pgx.Tx) error {
			r, e := financeStore.CreateRecharge(ctx, tx, fixtureBrand, user.MemberID, 100, "fixture-recharge-"+user.MemberID, "synthetic test recharge", "verified synthetic fixture recharge", meta(adminID))
			if e != nil {
				return e
			}
			_, e = financeStore.ConfirmRecharge(ctx, tx, fixtureBrand, r.ID, r.Version, "confirmed synthetic fixture recharge", meta(adminID))
			return e
		}); err != nil {
			return err
		}
		var giftDelta points.Balance
		giftDelta[2][0] = 20
		if err = inTx(ctx, db, func(tx pgx.Tx) error {
			_, e := pointsStore.Post(ctx, tx, points.Change{BrandID: fixtureBrand, MemberID: user.MemberID, EntryType: "adjust", ReferenceType: "manual", ReferenceID: user.MemberID, OperationKey: "qualification-gift:" + user.MemberID, Reason: "synthetic fixture gift", ActorType: "admin", ActorID: adminID, RequestID: ids.New(), Delta: giftDelta, Allocation: []points.Allocation{{Source: "gift", State: "available", Points: 20}}})
			return e
		}); err != nil {
			return err
		}
	}

	model := rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true}
	position, one, ten, yes := 3, points.Amount(1), points.Amount(10), true
	definition := rules.Definition{SchemaVersion: 1, Model: model, Selection: rules.SelectionRule{Mode: "numbers"}, UnitPoints: 1,
		PrizeTiers: []rules.Tier{{Code: "EXACT", Condition: rules.Condition{Op: "equals", Field: "position_match", Value: &position}, Odds: "10", Exclusive: true}},
		Rounding:   "half_up", RoundingScope: "order", Limits: rules.Limits{MaxCombinations: 100, MaxMultiplier: 1000}}
	rulebookStore := rulebook.Store{DB: db}
	code := "qual_" + strings.ReplaceAll(ids.New()[:13], "-", "")
	var game rulebook.Game
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		game, e = rulebookStore.CreateGame(ctx, tx, fixtureBrand, admin, code, "Qualification fixture", model, "UTC", "synthetic browser qualification workflow", meta(adminID))
		return e
	}); err != nil {
		return err
	}
	var play rulebook.Play
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		play, e = rulebookStore.CreatePlay(ctx, tx, fixtureBrand, admin, game.ID, "exact", "Exact", "qualification fixture", meta(adminID))
		return e
	}); err != nil {
		return err
	}
	var version rulebook.Version
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		version, e = rulebookStore.CreateVersion(ctx, tx, fixtureBrand, admin, play.ID, definition, "immediate", "qualification fixture", meta(adminID))
		return e
	}); err != nil {
		return err
	}
	selection, draw := rules.Selection{Digits: [][]int{{1}, {2}, {1}}}, rules.Draw{Digits: []int{1, 2, 1}}
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		version, e = rulebookStore.Validate(ctx, tx, fixtureBrand, admin, version.ID, version.Version, []rules.ValidationCase{{Name: "winning exact selection", Selection: selection, Draw: draw, Multiplier: 1, ExpectedBetPoints: &one, ExpectedPrizePoints: &ten, ExpectedWon: &yes}}, "validate actual qualification bet", meta(adminID))
		return e
	}); err != nil {
		return err
	}
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		version, e = rulebookStore.Submit(ctx, tx, fixtureBrand, admin, version.ID, version.Version, "submit qualification fixture rule", meta(adminID))
		return e
	}); err != nil {
		return err
	}
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		version, e = rulebookStore.Review(ctx, tx, fixtureBrand, reviewer, version.ID, version.Version, true, true, "independent qualification fixture rule review", meta(reviewerID))
		return e
	}); err != nil {
		return err
	}

	now := time.Now().UTC()
	var period rulebook.Period
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		period, e = rulebookStore.OpenPeriod(ctx, tx, fixtureBrand, game.ID, "qualification-"+ids.New()[:8], now.Add(-time.Second), now.Add(2*time.Second), now.Add(4*time.Second))
		return e
	}); err != nil {
		return err
	}
	betService := betting.Service{DB: db}
	for _, session := range sessions {
		brandBet, e := betService.BrandPolicy(ctx, fixtureBrand)
		if e != nil {
			return e
		}
		gameBet, e := betService.GamePolicy(ctx, fixtureBrand, game.ID)
		if e != nil {
			return e
		}
		input := betting.Input{PeriodID: period.ID, PlayID: play.ID, RuleVersionID: version.ID, Selection: selection, Multiplier: 1, PolicyVersions: &betting.PolicyVersions{Brand: brandBet.Version, Game: gameBet.Version}}
		if err = inTx(ctx, db, func(tx pgx.Tx) error {
			_, e := betService.Place(ctx, tx, fixtureBrand, session, input, "qualification-bet:"+session.Member.ID, points.Metadata{ActorType: "user", ActorID: session.User.ID, RequestID: ids.New()})
			return e
		}); err != nil {
			return err
		}
	}
	settlementPolicy, err := betService.SettlementPolicy(ctx, fixtureBrand)
	if err != nil {
		return err
	}
	mode := "automatic"
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		_, e := betService.SaveSettlementPolicy(ctx, tx, fixtureBrand, admin, betting.SettlementPolicyInput{Version: settlementPolicy.Version, Mode: &mode, Reason: "automatic fixture settlement"}, meta(adminID))
		return e
	}); err != nil {
		return err
	}
	if err = waitForPeriod(ctx, db, rulebookStore, period.ID, period.DrawAt); err != nil {
		return err
	}
	var currentPeriod rulebook.Period
	if err = db.QueryRow(ctx, `SELECT id::text,brand_id::text,game_id::text,period_no,sequence,status,bet_start_at,bet_end_at,draw_at,version FROM periods WHERE id=$1`, period.ID).Scan(&currentPeriod.ID, &currentPeriod.BrandID, &currentPeriod.GameID, &currentPeriod.PeriodNo, &currentPeriod.Sequence, &currentPeriod.Status, &currentPeriod.BetStartAt, &currentPeriod.BetEndAt, &currentPeriod.DrawAt, &currentPeriod.Version); err != nil {
		return err
	}
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		_, e := rulebookStore.ManualDraw(ctx, tx, fixtureBrand, admin, period.ID, currentPeriod.Version, period.PeriodNo, draw, time.Now().UTC(), "actual synthetic fixture draw", meta(adminID))
		return e
	}); err != nil {
		return err
	}
	settlementContext, err := betService.PeriodSettlementContext(ctx, fixtureBrand, period.ID)
	if err != nil || settlementContext.DrawResultID == nil {
		return errors.New("actual fixture draw unavailable for settlement")
	}
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		_, e := betService.StartSettlement(ctx, tx, fixtureBrand, admin, period.ID, betting.SettlementStartInput{Version: settlementContext.PeriodVersion, PolicyVersion: settlementContext.PolicyVersion, DrawResultID: *settlementContext.DrawResultID, Reason: "settle actual qualification fixture bets"}, meta(adminID))
		return e
	}); err != nil {
		return err
	}
	if _, err = betService.ProcessSettlements(ctx, 20); err != nil {
		return err
	}
	for _, user := range users {
		wallet, e := pointsStore.Read(ctx, fixtureBrand, user.MemberID)
		if e != nil {
			return e
		}
		if wallet.RechargePoints != 99 || wallet.WinningPoints != 10 || wallet.GiftPoints != 20 {
			return errors.New("real qualification fixture ledger did not settle to expected source balances")
		}
	}
	out := fixtureOutput{AdminID: adminID, ReviewerID: reviewerID, Users: users, GameID: game.ID, PlayID: play.ID, RuleID: version.ID, PeriodID: period.ID}
	if err = json.NewEncoder(os.Stdout).Encode(out); err != nil {
		return err
	}
	return nil
}

func addScopedRole(ctx context.Context, tx pgx.Tx, account, brand, code, name string, keys []string) error {
	role := ids.New()
	if _, err := tx.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name) VALUES($1,$2,$3,$4)`, role, brand, code, name); err != nil {
		return err
	}
	for _, key := range keys {
		if _, err := tx.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, role, key); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, account, brand); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, account, role)
	return err
}

func inTx(ctx context.Context, db *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func identityMutation(ctx context.Context, db *pgxpool.Pool, fn func(pgx.Tx) (any, error)) (any, error) {
	var result any
	err := inTx(ctx, db, func(tx pgx.Tx) error { var e error; result, e = fn(tx); return e })
	return result, err
}

func waitForPeriod(ctx context.Context, db *pgxpool.Pool, store rulebook.Store, period string, drawAt time.Time) error {
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, err := store.Tick(ctx); err != nil {
			return err
		}
		var status string
		if err := db.QueryRow(ctx, `SELECT status FROM periods WHERE id=$1`, period).Scan(&status); err != nil {
			return err
		}
		var now time.Time
		if err := db.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			return err
		}
		if status == "waiting_draw" && !now.Before(drawAt) {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("natural fixture draw window timed out")
		case <-tick.C:
		}
	}
}
