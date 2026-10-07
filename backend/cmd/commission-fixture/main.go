//go:build browserfixture

// commission-fixture creates and advances an explicitly owned synthetic
// commission workflow. It is excluded from normal application builds.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/attribution"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	fixtureBrand = "0199a000-0000-7000-8000-000000000001"
	fixtureAck   = "owned_synthetic_database"
	fixtureAdmin = "commission_admin"
	fixtureOwner = "commission_agent_owner"
	fixtureUser  = "commission_user"
	fixtureDBA   = "/lottery_commission_ui_desktop_s16"
	fixtureDBB   = "/lottery_commission_ui_mobile_s16"
	fixtureDBAV  = "/lottery_commission_ui_desktop_s16_verified"
	fixtureDBBV  = "/lottery_commission_ui_mobile_s16_verified"
	fixtureDBAF  = "/lottery_commission_ui_desktop_s16_final"
	fixtureDBBF  = "/lottery_commission_ui_mobile_s16_final"
	fixtureDBAD  = "/lottery_commission_ui_desktop_s16_distinct"
	fixtureDBBD  = "/lottery_commission_ui_mobile_s16_distinct"
	fixtureDBAP  = "/lottery_commission_payment_desktop_s17"
	fixtureDBBP  = "/lottery_commission_payment_mobile_s17"
	fixtureDBAPV = "/lottery_commission_payment_desktop_s17_verified"
	fixtureDBBPV = "/lottery_commission_payment_mobile_s17_verified"
	fixtureDBAPA = "/lottery_commission_payment_desktop_s17_actor"
	fixtureDBBPA = "/lottery_commission_payment_mobile_s17_actor"
)

func safeFixtureURL(raw, environment, confirmation, adminPassword, userPassword string, requirePasswords bool) error {
	u, err := url.Parse(raw)
	if err != nil || strings.Contains(raw, "#") || environment != "test" || confirmation != fixtureAck ||
		(u.Scheme != "postgres" && u.Scheme != "postgresql") ||
		(u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") ||
		(u.Port() != "5432" && u.Port() != "55432") ||
		(u.Path != fixtureDBA && u.Path != fixtureDBB && u.Path != fixtureDBAV && u.Path != fixtureDBBV && u.Path != fixtureDBAF && u.Path != fixtureDBBF && u.Path != fixtureDBAD && u.Path != fixtureDBBD && u.Path != fixtureDBAP && u.Path != fixtureDBBP && u.Path != fixtureDBAPV && u.Path != fixtureDBBPV && u.Path != fixtureDBAPA && u.Path != fixtureDBBPA) || u.RawPath != "" || u.Fragment != "" || u.Opaque != "" ||
		u.User == nil || u.User.Username() != "lottery_test" || len(adminPassword) < 16 && requirePasswords || len(userPassword) < 16 && requirePasswords {
		return errors.New("explicit owned synthetic commission database required")
	}
	if pw, ok := u.User.Password(); !ok || pw == "" {
		return errors.New("fixture database password required")
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(query) != 1 || len(query["sslmode"]) != 1 || query.Get("sslmode") != "disable" {
		return errors.New("fixture connection overrides forbidden")
	}
	return nil
}

type fixtureOutput struct {
	BrandID             string `json:"brand_id"`
	AdminID             string `json:"admin_id"`
	UserMemberID        string `json:"user_member_id"`
	ReadyCycleID        string `json:"ready_cycle_id"`
	FailedCycleID       string `json:"failed_cycle_id"`
	UnregisteredOrderID string `json:"unregistered_order_id"`
	FailedDiscoveryID   string `json:"failed_discovery_id"`
	RootAgentID         string `json:"root_agent_id"`
}

type fixtureDiagnostic struct {
	stage string
	code  string
}

func (e fixtureDiagnostic) Error() string { return "stage=" + e.stage + " code=" + e.code }

func diagnostic(stage, code string) error { return fixtureDiagnostic{stage: stage, code: code} }

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "commission fixture failed:", err.Error())
		os.Exit(1)
	}
}

func run() error {
	command := "init"
	if len(os.Args) > 2 {
		return errors.New("usage: commission-fixture [init|advance|discover|pay|verify]")
	}
	if len(os.Args) == 2 {
		command = os.Args[1]
	}
	if command != "init" && command != "advance" && command != "discover" && command != "pay" && command != "verify" {
		return errors.New("usage: commission-fixture [init|advance|discover|pay|verify]")
	}
	dsn := os.Getenv("DATABASE_URL")
	if err := safeFixtureURL(dsn, os.Getenv("APP_ENV"), os.Getenv("COMMISSION_FIXTURE_CONFIRM"),
		os.Getenv("COMMISSION_FIXTURE_ADMIN_PASSWORD"), os.Getenv("COMMISSION_FIXTURE_USER_PASSWORD"), command == "init"); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return errors.New("fixture database unavailable")
	}
	defer db.Close()
	if err = db.Ping(ctx); err != nil {
		return errors.New("fixture database unavailable")
	}
	if command == "init" {
		if err = database.Migrate(ctx, db); err != nil {
			return errors.New("fixture migrations failed")
		}
		if err = database.Seed(ctx, db, "test"); err != nil {
			return errors.New("fixture seed failed")
		}
		out, e := initialize(ctx, db, os.Getenv("COMMISSION_FIXTURE_ADMIN_PASSWORD"), os.Getenv("COMMISSION_FIXTURE_USER_PASSWORD"))
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	adminID, err := requireFixtureAdmin(ctx, db)
	if err != nil {
		return err
	}
	if err = requireLatestMigration(ctx, db); err != nil {
		return err
	}
	switch command {
	case "pay":
		processed, e := (commission.Service{DB: db}).ProcessPayments(ctx, 100)
		if e != nil {
			return errors.New("commission payment worker failed")
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"processed": processed})
	case "advance":
		if err = advanceCycles(ctx, commission.Service{DB: db}); err != nil {
			return errors.New("commission cycle worker failed")
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"advanced": true})
	case "discover":
		processed, e := (commission.Service{DB: db}).ProcessDiscovery(ctx, 100)
		if e != nil {
			return errors.New("commission discovery worker failed")
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"processed": processed})
	case "verify":
		out, e := verify(ctx, db, adminID)
		if e != nil {
			return errors.New("fixture verification failed")
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	return nil
}

func initialize(ctx context.Context, db *pgxpool.Pool, adminPassword, userPassword string) (fixtureOutput, error) {
	var out fixtureOutput
	out.BrandID = fixtureBrand
	var latest string
	if err := db.QueryRow(ctx, `SELECT name FROM schema_migrations ORDER BY name DESC LIMIT 1`).Scan(&latest); err != nil || latest != "0050_commission_payments.up.sql" {
		return out, errors.New("commission fixture requires the latest migration")
	}
	var brandOK bool
	if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brands WHERE id=$1 AND code='aurora' AND status='active')`, fixtureBrand).Scan(&brandOK); err != nil || !brandOK {
		return out, errors.New("active seeded test brand required")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return out, errors.New("fixture initialization transaction unavailable")
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(80401016)`); err != nil {
		return out, errors.New("fixture ownership lock unavailable")
	}
	var identities, wallets int
	err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM global_users)+(SELECT count(*) FROM admin_accounts), (SELECT count(*) FROM point_accounts)`).Scan(&identities, &wallets)
	if err != nil || identities != 0 || wallets != 0 {
		return out, errors.New("commission fixture requires empty identities and wallets")
	}
	adminHash, err := authcrypto.HashPassword(adminPassword)
	if err != nil {
		return out, errors.New("fixture administrator password rejected")
	}
	var reviewerSecret [32]byte
	if _, err = rand.Read(reviewerSecret[:]); err != nil {
		return out, errors.New("reviewer credential generation failed")
	}
	reviewerPassword := hex.EncodeToString(reviewerSecret[:])
	reviewerHash, err := authcrypto.HashPassword(reviewerPassword)
	if err != nil {
		return out, errors.New("reviewer credential generation failed")
	}
	adminID, reviewerID := ids.New(), ids.New()
	if _, err = tx.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,$3),($4,'commission_rule_reviewer',$5)`, adminID, fixtureAdmin, adminHash, reviewerID, reviewerHash); err != nil {
		return out, errors.New("fixture admin creation failed")
	}
	adminPerms := []string{
		"game.write.brand", "rule.write.brand", "rule.validate.brand", "rule.submit.brand", "draw.manual_create.brand",
		"settlement_policy.write.brand", "settlement.run.brand", "settlement.approve.brand", "wallet.view.brand", "wallet.adjust.brand", "brand.view.brand",
		"agent_policy.write.brand", "agent.write.brand", "join_code.write.brand", "commission_policy.write.brand",
		"commission.view.brand", "commission.run.brand", "commission.retry.brand",
		"commission_payment.approve.brand", "commission_payment.retry.brand", "commission_payment_policy.write.brand",
	}
	if err = addScopedRole(ctx, tx, adminID, fixtureBrand, "commission_fixture_admin", "Commission fixture operator", adminPerms); err != nil {
		return out, errors.New("fixture operator permissions failed")
	}
	if err = addScopedRole(ctx, tx, reviewerID, fixtureBrand, "commission_fixture_reviewer", "Independent commission rule reviewer", []string{"rule.review.brand"}); err != nil {
		return out, errors.New("fixture reviewer permissions failed")
	}
	if err = tx.Commit(ctx); err != nil {
		return out, errors.New("fixture administrators could not be committed")
	}
	out.AdminID = adminID
	adminStore := adminsys.Store{DB: db}
	admin, err := adminStore.Account(ctx, adminID)
	if err != nil {
		return out, errors.New("fixture operator authorization unavailable")
	}
	reviewer, err := adminStore.Account(ctx, reviewerID)
	if err != nil {
		return out, errors.New("fixture reviewer authorization unavailable")
	}
	meta := func(actor string) points.Metadata {
		return points.Metadata{ActorType: "admin", ActorID: actor, RequestID: ids.New(), Reason: "owned synthetic commission browser fixture"}
	}
	identityStore, err := identity.New(db)
	if err != nil {
		return out, errors.New("fixture identity service unavailable")
	}
	ownerAuth, err := register(ctx, db, identityStore, fixtureOwner, userPassword, "")
	if err != nil {
		return out, errors.New("fixture agent owner registration failed")
	}
	ownerSession, err := identityStore.Authenticate(ctx, fixtureBrand, ownerAuth.AccessToken)
	if err != nil {
		return out, errors.New("fixture agent owner authentication failed")
	}
	agents := agency.Service{DB: db}
	var agentPolicy agency.Policy
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		agentPolicy, e = agents.SavePolicy(ctx, tx, fixtureBrand, admin, agency.PolicyInput{Version: 1, Config: agency.PolicyConfig{Enabled: true, MaxDepth: 3, RatioCap: "0.3", Mode: "loss", Cycle: "weekly"}, Reason: "enable owned synthetic weekly loss commission"}, meta(adminID))
		return e
	}); err != nil {
		return out, errors.New("fixture agent policy failed")
	}
	var root agency.Node
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		root, e = agents.Create(ctx, tx, fixtureBrand, admin, agency.CreateInput{PolicyVersion: agentPolicy.Version, MemberID: ownerSession.Member.ID, Config: agency.NodeConfig{Ratio: "0.3", Status: "active", CanCreateChildren: true}, Reason: "owned synthetic root commission beneficiary"}, meta(adminID))
		return e
	}); err != nil {
		return out, errors.New("fixture root agent creation failed")
	}
	out.RootAgentID = root.ID
	codes := attribution.Service{DB: db}
	var code attribution.Code
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		code, e = codes.Create(ctx, tx, fixtureBrand, admin, attribution.CreateInput{Kind: "agent", OwnerMemberID: ownerSession.Member.ID, AgentID: &root.ID, Reason: "register owned synthetic commission member"}, meta(adminID))
		return e
	}); err != nil {
		return out, errors.New("fixture agent code creation failed")
	}
	userAuth, err := register(ctx, db, identityStore, fixtureUser, userPassword, code.Code)
	if err != nil {
		return out, errors.New("fixture attributed user registration failed")
	}
	userSession, err := identityStore.Authenticate(ctx, fixtureBrand, userAuth.AccessToken)
	if err != nil {
		return out, errors.New("fixture member authentication failed")
	}
	out.UserMemberID = userSession.Member.ID
	var seed points.Balance
	seed[2][0] = 20
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		_, e := (points.Store{DB: db}).Post(ctx, tx, points.Change{BrandID: fixtureBrand, MemberID: userSession.Member.ID, EntryType: "adjustment", ReferenceType: "test_fixture", ReferenceID: userSession.Member.ID, OperationKey: "commission-fixture-fund:" + userSession.Member.ID, Reason: "owned synthetic commission fixture funding", ActorType: "system", RequestID: ids.New(), Delta: seed, Allocation: []points.Allocation{{Source: "gift", State: "available", Points: 20}}})
		return e
	}); err != nil {
		return out, errors.New("fixture audited system wallet funding failed")
	}

	game, play, version, err := createRuleWorkflow(ctx, db, admin, reviewer, meta)
	if err != nil {
		return out, err
	}
	betService := betting.Service{DB: db}
	settleMode := "manual"
	policy, err := betService.SettlementPolicy(ctx, fixtureBrand)
	if err != nil {
		return out, errors.New("fixture settlement policy unavailable")
	}
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		_, e := betService.SaveSettlementPolicy(ctx, tx, fixtureBrand, admin, betting.SettlementPolicyInput{Version: policy.Version, Mode: &settleMode, Reason: "explicit owned synthetic fixture settlement"}, meta(adminID))
		return e
	}); err != nil {
		return out, errors.New("fixture settlement policy update failed")
	}
	finance := commission.Service{DB: db}
	phaseA, err := runPeriod(ctx, db, admin, game, play, version, userSession, betService, finance, 2, "auto")
	if err != nil {
		return out, fmt.Errorf("fixture phase A: %w", err)
	}
	out.ReadyCycleID = phaseA.cycleID
	phaseB, err := runPeriod(ctx, db, admin, game, play, version, userSession, betService, finance, 1, "manual-failure")
	if err != nil {
		return out, fmt.Errorf("fixture phase B: %w", err)
	}
	out.FailedCycleID = phaseB.cycleID
	phaseC, err := runPeriod(ctx, db, admin, game, play, version, userSession, betService, finance, 1, "discovery-failure")
	if err != nil {
		return out, fmt.Errorf("fixture phase C: %w", err)
	}
	out.UnregisteredOrderID = phaseC.orderIDs[0]
	out.FailedDiscoveryID = phaseC.orderIDs[0]
	if err = validateInitOut(ctx, db, out); err != nil {
		return out, errors.New("fixture final invariants failed")
	}
	return out, nil
}

type registerResult struct{ AccessToken string }

func register(ctx context.Context, db *pgxpool.Pool, store *identity.Store, username, password, agentCode string) (registerResult, error) {
	var result registerResult
	err := inTx(ctx, db, func(tx pgx.Tx) error {
		out, e := store.Register(ctx, tx, fixtureBrand, identity.RegisterInput{Username: username, Password: password, Privacy: "dev-1", Terms: "dev-1", AgentCode: agentCode}, identity.Metadata{RequestID: ids.New(), Domain: "localhost"})
		if e != nil || out.Error != nil || out.Status != 201 {
			return errors.New("registration rejected")
		}
		var auth identity.Authentication
		if e = json.Unmarshal(out.Data, &auth); e != nil {
			return e
		}
		result.AccessToken = auth.AccessToken
		return nil
	})
	return result, err
}

func createRuleWorkflow(ctx context.Context, db *pgxpool.Pool, admin, reviewer access.Account, meta func(string) points.Metadata) (rulebook.Game, rulebook.Play, rulebook.Version, error) {
	store := rulebook.Store{DB: db}
	model := rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true}
	position, one, ten, won := 3, points.Amount(1), points.Amount(10), true
	definition := rules.Definition{SchemaVersion: 1, Model: model, Selection: rules.SelectionRule{Mode: "numbers"}, UnitPoints: 1,
		PrizeTiers: []rules.Tier{{Code: "EXACT", Condition: rules.Condition{Op: "equals", Field: "position_match", Value: &position}, Odds: "10", Exclusive: true}},
		Rounding:   "half_up", RoundingScope: "order", Limits: rules.Limits{MaxCombinations: 100, MaxMultiplier: 1000}}
	var game rulebook.Game
	if err := inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		game, e = store.CreateGame(ctx, tx, fixtureBrand, admin, "commission_fixture", "Commission fixture", model, "UTC", "owned synthetic commission browser workflow", meta(admin.ID))
		return e
	}); err != nil {
		return game, rulebook.Play{}, rulebook.Version{}, errors.New("fixture game creation failed")
	}
	var play rulebook.Play
	if err := inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		play, e = store.CreatePlay(ctx, tx, fixtureBrand, admin, game.ID, "exact", "Exact", "commission fixture", meta(admin.ID))
		return e
	}); err != nil {
		return game, play, rulebook.Version{}, errors.New("fixture play creation failed")
	}
	var version rulebook.Version
	if err := inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		version, e = store.CreateVersion(ctx, tx, fixtureBrand, admin, play.ID, definition, "immediate", "commission fixture", meta(admin.ID))
		return e
	}); err != nil {
		return game, play, version, errors.New("fixture rule creation failed")
	}
	caseSelection, draw := rules.Selection{Digits: [][]int{{1}, {2}, {1}}}, rules.Draw{Digits: []int{1, 2, 1}}
	if err := inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		version, e = store.Validate(ctx, tx, fixtureBrand, admin, version.ID, version.Version, []rules.ValidationCase{{Name: "exact selection", Selection: caseSelection, Draw: draw, Multiplier: 1, ExpectedBetPoints: &one, ExpectedPrizePoints: &ten, ExpectedWon: &won}}, "validate commission fixture exact rule", meta(admin.ID))
		return e
	}); err != nil {
		return game, play, version, errors.New("fixture rule validation failed")
	}
	if err := inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		version, e = store.Submit(ctx, tx, fixtureBrand, admin, version.ID, version.Version, "submit commission fixture rule", meta(admin.ID))
		return e
	}); err != nil {
		return game, play, version, errors.New("fixture rule submission failed")
	}
	if err := inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		version, e = store.Review(ctx, tx, fixtureBrand, reviewer, version.ID, version.Version, true, true, "independent commission fixture rule review", meta(reviewer.ID))
		return e
	}); err != nil {
		return game, play, version, errors.New("fixture independent review failed")
	}
	return game, play, version, nil
}

type phaseResult struct {
	cycleID  string
	orderIDs []string
}

func runPeriod(ctx context.Context, db *pgxpool.Pool, admin access.Account, game rulebook.Game, play rulebook.Play, version rulebook.Version, session identity.Session, betService betting.Service, finance commission.Service, betCount int, phase string) (phaseResult, error) {
	result := phaseResult{orderIDs: make([]string, 0, betCount)}
	now := time.Now().UTC()
	betEnd := now.Add(5 * time.Second).Truncate(time.Second)
	if !betEnd.After(now) {
		betEnd = betEnd.Add(time.Second)
	}
	boundary := betEnd.Add(-time.Second).Truncate(time.Second)
	weekday := int(boundary.Weekday())
	calendar := commission.Calendar{Timezone: "UTC", Cycle: "weekly", BoundaryTime: boundary.Format("15:04:05"), Weekday: &weekday}
	payoutMode := commission.PayoutManual
	if phase == "auto" {
		payoutMode = commission.PayoutAutomatic
	}
	policy, err := finance.Policy(ctx, fixtureBrand)
	if err != nil {
		return result, diagnostic("period.commission_policy_read", "FIXTURE_COMMISSION_POLICY_READ_FAILED")
	}
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		_, e := finance.Update(ctx, tx, fixtureBrand, admin, commission.PolicyInput{Version: policy.Version, Config: commission.PolicyConfig{Enabled: true, Calendar: &calendar, PayoutMode: payoutMode}, Reason: "set owned fixture weekly boundary and payout mode before actual bets"}, points.Metadata{ActorType: "admin", ActorID: admin.ID, RequestID: ids.New()})
		return e
	}); err != nil {
		return result, diagnostic("period.commission_policy_update", "FIXTURE_COMMISSION_POLICY_UPDATE_FAILED")
	}
	periodNo := "commission-" + strings.ReplaceAll(ids.New(), "-", "")[:12]
	periodStore := rulebook.Store{DB: db}
	var period rulebook.Period
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		period, e = periodStore.OpenPeriod(ctx, tx, fixtureBrand, game.ID, periodNo, now.Add(-time.Second), betEnd, betEnd.Add(time.Second))
		return e
	}); err != nil {
		return result, diagnostic("period.open", "FIXTURE_PERIOD_OPEN_FAILED")
	}
	brandPolicy, err := betService.BrandPolicy(ctx, fixtureBrand)
	if err != nil {
		return result, diagnostic("period.brand_betting_policy_read", "FIXTURE_BRAND_BETTING_POLICY_READ_FAILED")
	}
	gamePolicy, err := betService.GamePolicy(ctx, fixtureBrand, game.ID)
	if err != nil {
		return result, diagnostic("period.game_betting_policy_read", "FIXTURE_GAME_BETTING_POLICY_READ_FAILED")
	}
	input := betting.Input{PeriodID: period.ID, PlayID: play.ID, RuleVersionID: version.ID, Selection: rules.Selection{Digits: [][]int{{0}, {0}, {0}}}, Multiplier: 1, PolicyVersions: &betting.PolicyVersions{Brand: brandPolicy.Version, Game: gamePolicy.Version}}
	for i := 0; i < betCount; i++ {
		tx, e := db.Begin(ctx)
		if e != nil {
			return result, diagnostic("period.wager_begin", "FIXTURE_WAGER_TRANSACTION_FAILED")
		}
		order, e := betService.Place(ctx, tx, fixtureBrand, session, input, "commission-fixture:"+phase+":"+fmt.Sprint(i), points.Metadata{ActorType: "user", ActorID: session.User.ID, RequestID: ids.New()})
		if e != nil {
			_ = tx.Rollback(ctx)
			return result, diagnostic("period.wager_place", "FIXTURE_WAGER_REJECTED")
		}
		if e = tx.Commit(ctx); e != nil {
			return result, diagnostic("period.wager_commit", "FIXTURE_WAGER_COMMIT_FAILED")
		}
		result.orderIDs = append(result.orderIDs, order.ID)
	}
	// Persist each real order's saved weekly window while its actual boundary
	// is still in the future. Later discovery work only advances these records.
	if _, err = finance.ProcessDiscovery(ctx, 100); err != nil {
		return result, diagnostic("period.discovery_schedule", "FIXTURE_DISCOVERY_SCHEDULE_FAILED")
	}
	if err = waitUntil(ctx, boundary, 5*time.Second); err != nil {
		return result, diagnostic("period.boundary_wait", "FIXTURE_BOUNDARY_WAIT_FAILED")
	}
	if err = waitUntil(ctx, period.DrawAt, 3*time.Second); err != nil {
		return result, diagnostic("period.draw_wait", "FIXTURE_DRAW_WAIT_FAILED")
	}
	if _, err = periodStore.Tick(ctx); err != nil {
		return result, diagnostic("period.tick", "FIXTURE_PERIOD_TICK_FAILED")
	}
	var current rulebook.Period
	err = db.QueryRow(ctx, `SELECT id::text,brand_id::text,game_id::text,period_no,sequence,status,bet_start_at,bet_end_at,draw_at,version FROM periods WHERE id=$1`, period.ID).Scan(&current.ID, &current.BrandID, &current.GameID, &current.PeriodNo, &current.Sequence, &current.Status, &current.BetStartAt, &current.BetEndAt, &current.DrawAt, &current.Version)
	if err != nil || current.Status != "waiting_draw" {
		return result, diagnostic("period.draw_state", "FIXTURE_PERIOD_NOT_WAITING_FOR_DRAW")
	}
	// The real draw guard rejects a repeated previous result. Keep all three
	// outcomes distinct while the selected 000 wagers genuinely lose.
	digits := []int{1, 2, 1}
	if phase == "manual-failure" {
		digits[2] = 2
	}
	if phase == "discovery-failure" {
		digits[2] = 3
	}
	_, err = drawActual(ctx, db, periodStore, admin, current, rules.Draw{Digits: digits})
	if err != nil {
		return result, diagnostic("period.manual_draw", "FIXTURE_MANUAL_DRAW_FAILED")
	}
	if err = settleActual(ctx, db, betService, admin, period.ID); err != nil {
		return result, err
	}
	switch phase {
	case "auto":
		if _, err = finance.ProcessDiscovery(ctx, 100); err != nil {
			return result, diagnostic("phase_a.discovery", "FIXTURE_AUTOMATIC_DISCOVERY_FAILED")
		}
		result.cycleID, err = cycleForOrder(ctx, db, result.orderIDs[0])
		if err != nil {
			return result, diagnostic("phase_a.cycle_lookup", "FIXTURE_READY_CYCLE_LOOKUP_FAILED")
		}
		if err = advanceUntil(ctx, finance, db, fixtureBrand, result.cycleID, "ready"); err != nil {
			return result, diagnostic("phase_a.cycle_worker", "FIXTURE_READY_CYCLE_WORKER_FAILED")
		}
	case "manual-failure":
		result.cycleID, err = manuallyCreateCycle(ctx, db, finance, admin, result.orderIDs[0])
		if err != nil {
			return result, diagnostic("phase_b.manual_cycle", "FIXTURE_MANUAL_CYCLE_CREATE_FAILED")
		}
		if err = failCalculationPage(ctx, finance, db, fixtureBrand, result.cycleID); err != nil {
			return result, diagnostic("phase_b.calculation_failure", "FIXTURE_CALCULATION_PAGE_FAILURE_SETUP_FAILED")
		}
		if _, err = finance.ProcessDiscovery(ctx, 100); err != nil { // Links this real order to the already existing cycle.
			return result, diagnostic("phase_b.discovery_link", "FIXTURE_EXISTING_CYCLE_DISCOVERY_LINK_FAILED")
		}
		if err = assertDiscoveryState(ctx, db, result.orderIDs[0], "registered", result.cycleID); err != nil {
			return result, diagnostic("phase_b.discovery_state", "FIXTURE_DISCOVERY_NOT_LINKED_TO_EXISTING_CYCLE")
		}
	case "discovery-failure":
		if err = failDiscoveryCreation(ctx, finance); err != nil {
			return result, diagnostic("phase_c.discovery_failure", "FIXTURE_DISCOVERY_FAILURE_SETUP_FAILED")
		}
		var count int
		if err = db.QueryRow(ctx, `SELECT count(*) FROM commission_cycles WHERE brand_id=$1 AND anchor_order_id=$2`, fixtureBrand, result.orderIDs[0]).Scan(&count); err != nil || count != 0 {
			return result, diagnostic("phase_c.cycle_state", "FIXTURE_DISCOVERY_FAILURE_CREATED_CYCLE")
		}
		if err = assertDiscoveryState(ctx, db, result.orderIDs[0], "failed", ""); err != nil {
			return result, diagnostic("phase_c.discovery_state", "FIXTURE_DISCOVERY_NOT_FAILED")
		}
	}
	return result, nil
}

func drawActual(ctx context.Context, db *pgxpool.Pool, store rulebook.Store, admin access.Account, period rulebook.Period, draw rules.Draw) (rulebook.DrawResult, error) {
	var out rulebook.DrawResult
	err := inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		out, e = store.ManualDraw(ctx, tx, fixtureBrand, admin, period.ID, period.Version, period.PeriodNo, draw, time.Now().UTC(), "owned synthetic commission fixture draw", points.Metadata{ActorType: "admin", ActorID: admin.ID, RequestID: ids.New()})
		return e
	})
	return out, err
}

func settleActual(ctx context.Context, db *pgxpool.Pool, service betting.Service, admin access.Account, periodID string) error {
	info, err := service.PeriodSettlementContext(ctx, fixtureBrand, periodID)
	if err != nil || info.DrawResultID == nil || !info.CanStart {
		return diagnostic("settlement.precondition", "FIXTURE_DRAW_NOT_SETTLEABLE")
	}
	var job betting.SettlementJob
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		job, e = service.StartSettlement(ctx, tx, fixtureBrand, admin, periodID, betting.SettlementStartInput{Version: info.PeriodVersion, PolicyVersion: info.PolicyVersion, DrawResultID: *info.DrawResultID, Reason: "settle owned synthetic commission fixture wagers"}, points.Metadata{ActorType: "admin", ActorID: admin.ID, RequestID: ids.New()})
		return e
	}); err != nil {
		return diagnostic("settlement.start", "FIXTURE_SETTLEMENT_START_FAILED")
	}
	if job.Mode != "manual" && job.Mode != "automatic" {
		return diagnostic("settlement.start", "FIXTURE_SETTLEMENT_MODE_UNSUPPORTED")
	}
	for i := 0; i < 30; i++ {
		job, err = service.SettlementJob(ctx, fixtureBrand, job.ID)
		if err != nil {
			return diagnostic("settlement.status", "FIXTURE_SETTLEMENT_STATUS_READ_FAILED")
		}
		if job.State == "completed" && job.FailedCount == 0 {
			return nil
		}
		if job.State == "failed" || job.FailedCount != 0 {
			return diagnostic("settlement.status", "FIXTURE_SETTLEMENT_JOB_FAILED")
		}
		if job.Mode == "manual" && job.State == "awaiting_approval" {
			if err = inTx(ctx, db, func(tx pgx.Tx) error {
				var e error
				job, e = service.ActOnSettlement(ctx, tx, fixtureBrand, admin, job.ID, "approve", betting.SettlementActionInput{Version: job.Version, Reason: "approve owned synthetic commission fixture settlement"}, points.Metadata{ActorType: "admin", ActorID: admin.ID, RequestID: ids.New()})
				return e
			}); err != nil {
				return diagnostic("settlement.approval", "FIXTURE_SETTLEMENT_APPROVAL_FAILED")
			}
			continue
		}
		if job.State != "processing" && job.State != "paying" {
			return diagnostic("settlement.status", "FIXTURE_SETTLEMENT_UNEXPECTED_STATE")
		}
		if _, err = service.ProcessSettlements(ctx, 20); err != nil {
			return diagnostic("settlement.process", "FIXTURE_SETTLEMENT_WORKER_FAILED")
		}
	}
	return diagnostic("settlement.process", "FIXTURE_SETTLEMENT_STEP_LIMIT")
}

func manuallyCreateCycle(ctx context.Context, db *pgxpool.Pool, service commission.Service, admin access.Account, anchor string) (string, error) {
	var cycle commission.Cycle
	err := inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		cycle, e = service.CreateCycleTx(ctx, tx, fixtureBrand, admin, commission.CreateCycleInput{AnchorOrderID: anchor, Reason: "manual owned fixture cycle registration"}, points.Metadata{ActorType: "admin", ActorID: admin.ID, RequestID: ids.New()})
		return e
	})
	return cycle.ID, err
}

func failCalculationPage(ctx context.Context, service commission.Service, db *pgxpool.Pool, brand, cycleID string) error {
	const trigger, fn = "commission_fixture_reject_calculation_page", "commission_fixture_reject_calculation_page_fn"
	if err := requireFailureHooksAvailable(ctx, db, trigger, fn); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `CREATE FUNCTION `+fn+`() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='commission.cycle.calculation_page' THEN RAISE EXCEPTION 'owned fixture calculation page outage'; END IF; RETURN NEW; END $$`); err != nil {
		return errors.New("owned calculation failure hook creation failed")
	}
	if _, err := db.Exec(ctx, `CREATE TRIGGER `+trigger+` BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION `+fn+`() `); err != nil {
		_, _ = db.Exec(context.Background(), `DROP FUNCTION IF EXISTS `+fn+`() `)
		return errors.New("owned calculation failure hook activation failed")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = db.Exec(cleanup, `DROP TRIGGER IF EXISTS `+trigger+` ON audit_logs`)
		_, _ = db.Exec(cleanup, `DROP FUNCTION IF EXISTS `+fn+`() `)
	}()
	if err := advanceUntil(ctx, service, db, brand, cycleID, "failed"); err != nil {
		return errors.New("commission cycle did not fail at its audited calculation page")
	}
	return nil
}

func failDiscoveryCreation(ctx context.Context, service commission.Service) error {
	db := service.DB
	const trigger, fn = "commission_fixture_reject_cycle_create", "commission_fixture_reject_cycle_create_fn"
	if err := requireFailureHooksAvailable(ctx, db, trigger, fn); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `CREATE FUNCTION `+fn+`() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='commission.cycle.create' AND NEW.actor_type='system' THEN RAISE EXCEPTION 'owned fixture cycle creation outage'; END IF; RETURN NEW; END $$`); err != nil {
		return errors.New("owned discovery failure hook creation failed")
	}
	if _, err := db.Exec(ctx, `CREATE TRIGGER `+trigger+` BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION `+fn+`() `); err != nil {
		_, _ = db.Exec(context.Background(), `DROP FUNCTION IF EXISTS `+fn+`() `)
		return errors.New("owned discovery failure hook activation failed")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = db.Exec(cleanup, `DROP TRIGGER IF EXISTS `+trigger+` ON audit_logs`)
		_, _ = db.Exec(cleanup, `DROP FUNCTION IF EXISTS `+fn+`() `)
	}()
	if _, err := service.ProcessDiscovery(ctx, 1); err != nil {
		return errors.New("owned discovery failure processing failed")
	}
	return nil
}

func requireFailureHooksAvailable(ctx context.Context, db *pgxpool.Pool, trigger, function string) error {
	var exists bool
	if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname=$1 AND NOT tgisinternal) OR EXISTS(SELECT 1 FROM pg_proc WHERE proname=$2)`, trigger, function).Scan(&exists); err != nil {
		return errors.New("could not validate owned failure hook names")
	}
	if exists {
		return errors.New("owned failure hook name is already occupied")
	}
	return nil
}

func advanceUntil(ctx context.Context, service commission.Service, db *pgxpool.Pool, brand, cycleID, desired string) error {
	for step := 0; step < 12; step++ {
		state, err := cycleState(ctx, db, service, brand, cycleID)
		if err != nil {
			return err
		}
		if state == desired {
			return nil
		}
		if state == "ready" || state == "failed" || state == "cancelled" {
			return errors.New("cycle reached another terminal state")
		}
		if _, err = service.ProcessCycles(ctx, 20); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	return errors.New("cycle worker exceeded bounded steps")
}

func advanceCycles(ctx context.Context, service commission.Service) error {
	for i := 0; i < 10; i++ {
		processed, err := service.ProcessCycles(ctx, 100)
		if err != nil {
			return err
		}
		if processed == 0 {
			return nil
		}
	}
	return errors.New("cycle worker reached command step bound")
}

func cycleState(ctx context.Context, db *pgxpool.Pool, service commission.Service, brand, id string) (string, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	cycle, err := service.CycleTx(ctx, tx, brand, id)
	return cycle.State, err
}

func cycleForOrder(ctx context.Context, db *pgxpool.Pool, order string) (string, error) {
	var id string
	err := db.QueryRow(ctx, `SELECT id::text FROM commission_cycles WHERE brand_id=$1 AND anchor_order_id=$2`, fixtureBrand, order).Scan(&id)
	return id, err
}

func assertDiscoveryState(ctx context.Context, db *pgxpool.Pool, order, state, cycle string) error {
	var gotState string
	var gotCycle *string
	if err := db.QueryRow(ctx, `SELECT state,cycle_id::text FROM commission_discovery WHERE brand_id=$1 AND id=$2`, fixtureBrand, order).Scan(&gotState, &gotCycle); err != nil {
		return err
	}
	if gotState != state || cycle != "" && (gotCycle == nil || *gotCycle != cycle) {
		return errors.New("commission discovery state mismatch")
	}
	return nil
}

func waitUntil(ctx context.Context, target time.Time, max time.Duration) error {
	d := time.Until(target)
	if d < 0 {
		return nil
	}
	if d > max {
		return errors.New("fixture timing boundary exceeded maximum wait")
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func requireFixtureAdmin(ctx context.Context, db *pgxpool.Pool) (string, error) {
	var id string
	err := db.QueryRow(ctx, `SELECT id::text FROM admin_accounts WHERE username=$1 AND status='active'`, fixtureAdmin).Scan(&id)
	if err != nil {
		return "", errors.New("owned commission fixture administrator required")
	}
	return id, nil
}

func requireLatestMigration(ctx context.Context, db *pgxpool.Pool) error {
	var latest string
	if err := db.QueryRow(ctx, `SELECT name FROM schema_migrations ORDER BY name DESC LIMIT 1`).Scan(&latest); err != nil || latest != "0050_commission_payments.up.sql" {
		return errors.New("owned commission fixture requires the latest migration")
	}
	return nil
}

func verify(ctx context.Context, db *pgxpool.Pool, adminID string) (map[string]any, error) {
	var entries, walletPoints, pendingDiscoveries, failedDiscoveries, unresolvedCycles int64
	err := db.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1 AND entry_type='commission'),
	 (SELECT coalesce(sum(pb.points),0) FROM point_buckets pb JOIN point_accounts pa ON pa.id=pb.account_id AND pa.brand_id=pb.brand_id WHERE pa.brand_id=$1 AND pb.source='commission'),
	 (SELECT count(*) FROM commission_discovery WHERE brand_id=$1 AND state='pending'),
	 (SELECT count(*) FROM commission_discovery WHERE brand_id=$1 AND state='failed'),
	 (SELECT count(*) FROM commission_cycles WHERE brand_id=$1 AND state NOT IN ('ready','failed','cancelled'))`, fixtureBrand).Scan(&entries, &walletPoints, &pendingDiscoveries, &failedDiscoveries, &unresolvedCycles)
	if err != nil {
		return nil, err
	}
	fingerprint, err := economicFingerprint(ctx, db)
	if err != nil {
		return nil, err
	}
	return map[string]any{"admin_id": adminID, "commission_ledger_entries": entries, "commission_wallet_points": walletPoints, "pending_discoveries": pendingDiscoveries, "failed_discoveries": failedDiscoveries, "unresolved_cycles": unresolvedCycles, "economic_fingerprint": fingerprint}, nil
}

// economicFingerprint hashes canonical JSON for the owned fixture members'
// financial accounts, bucket balances, and ledger evidence. The raw records
// stay in the database and are never emitted by this command.
func economicFingerprint(ctx context.Context, db *pgxpool.Pool) (string, error) {
	const query = `WITH owned_accounts AS (
 SELECT pa.id,pa.brand_id,pa.brand_member_id,pa.version
 FROM point_accounts pa
 JOIN brand_members bm ON bm.brand_id=pa.brand_id AND bm.id=pa.brand_member_id
 JOIN global_users u ON u.id=bm.global_user_id
 WHERE pa.brand_id=$1 AND u.username=ANY($2::text[])
)
SELECT jsonb_build_object(
 'accounts',coalesce((SELECT jsonb_agg(jsonb_build_object(
  'id',a.id,'brand_id',a.brand_id,'brand_member_id',a.brand_member_id,'version',a.version) ORDER BY a.id) FROM owned_accounts a),'[]'::jsonb),
 'buckets',coalesce((SELECT jsonb_agg(jsonb_build_object(
  'brand_id',b.brand_id,'account_id',b.account_id,'source',b.source,'state',b.state,'points',b.points)
  ORDER BY b.account_id,b.source,b.state) FROM point_buckets b JOIN owned_accounts a ON a.id=b.account_id AND a.brand_id=b.brand_id),'[]'::jsonb),
 'ledger',coalesce((SELECT jsonb_agg(jsonb_build_object(
  'id',l.id,'brand_id',l.brand_id,'account_id',l.account_id,'entry_type',l.entry_type,
  'reference_type',l.reference_type,'reference_id',l.reference_id,'operation_key',l.operation_key,
  'before_snapshot',l.before_snapshot,'delta_snapshot',l.delta_snapshot,'after_snapshot',l.after_snapshot,
  'source_allocation',l.source_allocation,'reversal_of',l.reversal_of)
  ORDER BY l.account_id,l.created_at,l.id) FROM point_ledger_entries l JOIN owned_accounts a ON a.id=l.account_id AND a.brand_id=l.brand_id),'[]'::jsonb)
)::text`
	var raw []byte
	if err := db.QueryRow(ctx, query, fixtureBrand, []string{fixtureOwner, fixtureUser}).Scan(&raw); err != nil {
		return "", err
	}
	return hashCanonicalJSON(raw)
}

func hashCanonicalJSON(raw []byte) (string, error) {
	canonical, err := canonicalJSON(raw)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func canonicalJSON(raw []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("fingerprint input must contain one JSON value")
	}
	return json.Marshal(value)
}

func validateInitOut(ctx context.Context, db *pgxpool.Pool, out fixtureOutput) error {
	state, err := cycleState(ctx, db, commission.Service{DB: db}, fixtureBrand, out.ReadyCycleID)
	if err != nil || state != "ready" {
		return errors.New("phase A cycle is not ready")
	}
	state, err = cycleState(ctx, db, commission.Service{DB: db}, fixtureBrand, out.FailedCycleID)
	if err != nil || state != "failed" {
		return errors.New("phase B cycle is not failed")
	}
	if err = assertDiscoveryState(ctx, db, out.UnregisteredOrderID, "failed", ""); err != nil {
		return err
	}
	var cycleCount, commissionEntries int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM commission_cycles WHERE brand_id=$1 AND anchor_order_id=$2`, fixtureBrand, out.UnregisteredOrderID).Scan(&cycleCount); err != nil || cycleCount != 0 {
		return errors.New("phase C unexpectedly created a cycle")
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1 AND entry_type='commission'`, fixtureBrand).Scan(&commissionEntries); err != nil || commissionEntries != 0 {
		return errors.New("fixture unexpectedly posted commission ledger entries")
	}
	var phaseATargets, phaseAEarningCount, phaseAEarningPoints int64
	if err = db.QueryRow(ctx, `SELECT c.target_count,
	 (SELECT count(*) FROM commission_earnings e WHERE e.cycle_id=c.id AND e.run_id=c.current_run_id AND e.agent_id=$2),
	 (SELECT coalesce(sum(e.points),0) FROM commission_earnings e WHERE e.cycle_id=c.id AND e.run_id=c.current_run_id AND e.agent_id=$2)
	 FROM commission_cycles c WHERE c.id=$1`, out.ReadyCycleID, out.RootAgentID).Scan(&phaseATargets, &phaseAEarningCount, &phaseAEarningPoints); err != nil || phaseATargets != 2 || phaseAEarningCount != 1 || phaseAEarningPoints != 1 {
		return errors.New("phase A expected two real losing wagers and one half-up commission point")
	}
	var automaticOrders int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM commission_calculations WHERE cycle_id=$1 AND reason='eligible' AND rule_snapshot->'financial_policy'->'config'->>'payout_mode'='automatic'`, out.ReadyCycleID).Scan(&automaticOrders); err != nil || automaticOrders != 2 {
		return errors.New("phase A must capture automatic payout mode in both original bets")
	}
	var windowAFrom, windowATo, windowBFrom, windowBTo, windowCFrom, windowCTo time.Time
	if err = db.QueryRow(ctx, `SELECT window_from,window_to FROM commission_cycles WHERE id=$1`, out.ReadyCycleID).Scan(&windowAFrom, &windowATo); err != nil {
		return err
	}
	if err = db.QueryRow(ctx, `SELECT window_from,window_to FROM commission_cycles WHERE id=$1`, out.FailedCycleID).Scan(&windowBFrom, &windowBTo); err != nil {
		return err
	}
	if err = db.QueryRow(ctx, `SELECT window_from,window_to FROM commission_discovery WHERE brand_id=$1 AND id=$2`, fixtureBrand, out.UnregisteredOrderID).Scan(&windowCFrom, &windowCTo); err != nil {
		return err
	}
	if windowAFrom.Equal(windowBFrom) && windowATo.Equal(windowBTo) ||
		windowAFrom.Equal(windowCFrom) && windowATo.Equal(windowCTo) ||
		windowBFrom.Equal(windowCFrom) && windowBTo.Equal(windowCTo) {
		return errors.New("fixture periods did not capture three distinct weekly commission windows")
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
