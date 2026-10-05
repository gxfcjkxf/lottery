package betting

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

const policyTestBrand = "0199a000-0000-7000-8000-000000000001"

func amountPtr(value points.Amount) *points.Amount { return &value }

func policyActor(id, brand string) access.Account {
	return access.Account{ID: id, Type: access.AccountAdmin, BrandIDs: []string{brand}, Roles: []access.Role{{
		BrandID:     brand,
		Permissions: []access.Permission{{Resource: "bet_policy", Action: "write", Scope: access.ScopeBrand}},
	}}}
}

func policyMeta(actor string) points.Metadata {
	return points.Metadata{ActorType: "admin", ActorID: actor, RequestID: "policy-test-request"}
}

func policyTx(t *testing.T, db interface {
	Begin(context.Context) (pgx.Tx, error)
}, fn func(pgx.Tx) error) {
	t.Helper()
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err = fn(tx); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultAndResolvedPolicyModes(t *testing.T) {
	brand := DefaultBrandPolicy()
	game := DefaultGamePolicy()
	if brand.MinBetPoints != 1 || brand.MaxBetPoints != nil || brand.MaxPeriodPoints != nil || brand.MaxUserPeriodPoints != nil || brand.UserCancelAllowed {
		t.Fatalf("unexpected brand defaults: %+v", brand)
	}
	if err := ValidateBrandPolicy(brand); err != nil {
		t.Fatal(err)
	}
	if err := ValidateGamePolicy(game); err != nil {
		t.Fatal(err)
	}
	base := points.Amount(100)
	brand.MaxBetPoints = &base
	resolved, err := ResolvePolicy(brand, game)
	if err != nil || resolved.MinBetPoints != 1 || resolved.MaxBetPoints == nil || *resolved.MaxBetPoints != 100 {
		t.Fatalf("inherit resolution=%+v err=%v", resolved, err)
	}
	value := points.Amount(50)
	game.MaxBetPoints = LimitOverride{Mode: "value", Points: &value}
	resolved, err = ResolvePolicy(brand, game)
	if err != nil || resolved.MaxBetPoints == nil || *resolved.MaxBetPoints != 50 {
		t.Fatalf("value resolution=%+v err=%v", resolved, err)
	}
	game.MaxBetPoints = LimitOverride{Mode: "unlimited"}
	resolved, err = ResolvePolicy(brand, game)
	if err != nil || resolved.MaxBetPoints != nil {
		t.Fatalf("unlimited game override did not override brand default: %+v err=%v", resolved, err)
	}
	brand.MaxBetPoints = nil
	resolved, err = ResolvePolicy(brand, game)
	if err != nil || resolved.MaxBetPoints != nil {
		t.Fatalf("unlimited result=%+v err=%v", resolved, err)
	}
	cancel := true
	game.UserCancelAllowed = &cancel
	resolved, err = ResolvePolicy(brand, game)
	if err != nil || !resolved.UserCancelAllowed {
		t.Fatalf("cancel override=%+v err=%v", resolved, err)
	}
}

func TestPolicyValidationRejectsNoncanonicalOrInconsistentLimits(t *testing.T) {
	for _, config := range []BrandPolicyConfig{
		{MinBetPoints: 0}, {MinBetPoints: -1},
		{MinBetPoints: 5, MaxBetPoints: amountPtr(4)},
		{MinBetPoints: 5, MaxPeriodPoints: amountPtr(4)},
		{MinBetPoints: 5, MaxUserPeriodPoints: amountPtr(4)},
		{MinBetPoints: 1, MaxBetPoints: amountPtr(0)},
	} {
		if err := ValidateBrandPolicy(config); !errors.Is(err, ErrInvalid) {
			t.Errorf("ValidateBrandPolicy(%+v)=%v", config, err)
		}
	}
	for _, override := range []LimitOverride{
		{Mode: "inherit", Points: amountPtr(3)},
		{Mode: "unlimited", Points: amountPtr(3)},
		{Mode: "value"}, {Mode: "value", Points: amountPtr(0)},
		{Mode: "other"},
	} {
		config := DefaultGamePolicy()
		config.MaxBetPoints = override
		if err := ValidateGamePolicy(config); !errors.Is(err, ErrInvalid) {
			t.Errorf("accepted override %+v", override)
		}
	}
	config := DefaultGamePolicy()
	config.MinBetPoints = LimitOverride{Mode: "unlimited"}
	if !errors.Is(ValidateGamePolicy(config), ErrInvalid) {
		t.Fatal("minimum unlimited override accepted")
	}
	brand := BrandPolicyConfig{MinBetPoints: 10, MaxBetPoints: amountPtr(20)}
	config = DefaultGamePolicy()
	config.MinBetPoints = LimitOverride{Mode: "value", Points: amountPtr(21)}
	if _, err := ResolvePolicy(brand, config); !errors.Is(err, ErrInvalid) {
		t.Fatalf("inconsistent resolved min accepted: %v", err)
	}
	config = DefaultGamePolicy()
	config.MaxBetPoints = LimitOverride{Mode: "value", Points: amountPtr(21)}
	if got, err := ResolvePolicy(brand, config); err != nil || got.MaxBetPoints == nil || *got.MaxBetPoints != 21 {
		t.Fatalf("game override should replace brand default: %+v err=%v", got, err)
	}
}

func TestPolicyReadWriteAuditScopeAndVersions(t *testing.T) {
	db := testdb.New(t)
	svc := Service{DB: db}
	ctx := context.Background()
	brandRecord, err := svc.BrandPolicy(ctx, policyTestBrand)
	if err != nil {
		t.Fatal(err)
	}
	if brandRecord.Version != 1 || brandRecord.Config != DefaultBrandPolicy() {
		t.Fatalf("brand default=%+v", brandRecord)
	}
	adminID, gameID := ids.New(), ids.New()
	if _, err = db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, adminID, "betpolicy_"+adminID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO games(id,brand_id,code,name,model,timezone,created_by) VALUES($1,$2,$3,'Policy Test','{"model":"DIGITS_0_9"}','UTC',$4)`, gameID, policyTestBrand, "policy_"+gameID[:8], adminID); err != nil {
		t.Fatal(err)
	}
	gameRecord, err := svc.GamePolicy(ctx, policyTestBrand, gameID)
	if err != nil {
		t.Fatal(err)
	}
	if gameRecord.Version != 1 || gameRecord.Config != DefaultGamePolicy() {
		t.Fatalf("game default=%+v", gameRecord)
	}
	actor := policyActor(adminID, policyTestBrand)
	brandCfg := DefaultBrandPolicy()
	brandCfg.MaxBetPoints = amountPtr(100)
	brandCfg.MaxPeriodPoints = amountPtr(1000)
	brandCfg.MaxUserPeriodPoints = amountPtr(500)
	brandCfg.UserCancelAllowed = true
	policyTx(t, db, func(tx pgx.Tx) error {
		updated, e := WriteBrandPolicy(ctx, tx, policyTestBrand, actor, 1, brandCfg, "set policy", policyMeta(adminID))
		if e == nil && updated.Version != 2 {
			t.Fatalf("brand version=%d", updated.Version)
		}
		return e
	})
	gameCfg := DefaultGamePolicy()
	gameCfg.MaxBetPoints = LimitOverride{Mode: "unlimited"}
	gameCfg.MaxPeriodPoints = LimitOverride{Mode: "value", Points: amountPtr(800)}
	cancel := false
	gameCfg.UserCancelAllowed = &cancel
	policyTx(t, db, func(tx pgx.Tx) error {
		updated, e := WriteGamePolicy(ctx, tx, policyTestBrand, gameID, actor, 1, gameCfg, "set game policy", policyMeta(adminID))
		if e == nil && updated.Version != 2 {
			t.Fatalf("game version=%d", updated.Version)
		}
		return e
	})
	lockedTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	effective, versions, err := svc.LockedPolicy(ctx, lockedTx, policyTestBrand, gameID)
	if err != nil {
		_ = lockedTx.Rollback(ctx)
		t.Fatal(err)
	}
	if versions != (PolicyVersions{Brand: 2, Game: 2}) || effective.MaxBetPoints != nil || effective.MaxPeriodPoints == nil || *effective.MaxPeriodPoints != 800 || effective.MaxUserPeriodPoints == nil || *effective.MaxUserPeriodPoints != 500 || effective.UserCancelAllowed {
		_ = lockedTx.Rollback(ctx)
		t.Fatalf("effective=%+v versions=%+v", effective, versions)
	}
	if err = lockedTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	staleGameTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = WriteGamePolicy(ctx, staleGameTx, policyTestBrand, gameID, actor, 1, DefaultGamePolicy(), "stale game", policyMeta(adminID))
	_ = staleGameTx.Rollback(ctx)
	if !errors.Is(err, ErrVersion) {
		t.Fatalf("stale game update=%v, want ErrVersion", err)
	}
	var auditCount int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action IN ('bet_policy.write.brand','bet_policy.write.game')`, policyTestBrand).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 2 {
		t.Fatalf("policy audit rows=%d", auditCount)
	}
	_, err = svc.GamePolicy(ctx, "0199a000-0000-7000-8000-000000000002", gameID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-brand game read=%v, want ErrNotFound", err)
	}
	otherBrandActor := policyActor(adminID, "0199a000-0000-7000-8000-000000000002")
	crossBrandTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = WriteGamePolicy(ctx, crossBrandTx, "0199a000-0000-7000-8000-000000000002", gameID, otherBrandActor, 1, DefaultGamePolicy(), "cross brand", policyMeta(adminID))
	_ = crossBrandTx.Rollback(ctx)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-brand game write=%v, want ErrNotFound", err)
	}
	if _, err = svc.BrandPolicy(ctx, ids.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing brand read=%v", err)
	}
	_ = math.MaxInt64 // keep overflow edge explicit in the test package contract.
}

func TestPolicyWritesRejectAuthorizationInvalidConfigAndStaleVersion(t *testing.T) {
	db := testdb.New(t)
	svc := Service{DB: db}
	ctx := context.Background()
	adminID, gameID := ids.New(), ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, adminID, "betpolicy_invalid_"+adminID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO games(id,brand_id,code,name,model,timezone,created_by) VALUES($1,$2,$3,'Policy Test','{"model":"DIGITS_0_9"}','UTC',$4)`, gameID, policyTestBrand, "policy_"+gameID[:8], adminID); err != nil {
		t.Fatal(err)
	}
	actor := policyActor(adminID, policyTestBrand)
	denied := actor
	denied.Roles = nil
	for _, test := range []struct {
		name, reason string
		account      access.Account
		config       BrandPolicyConfig
		want         error
	}{
		{name: "permission", reason: "valid", account: denied, config: DefaultBrandPolicy(), want: ErrDenied},
		{name: "super admin", reason: "valid", account: access.Account{ID: adminID, Type: access.AccountAdmin, SuperAdmin: true, BrandIDs: []string{policyTestBrand}, Roles: actor.Roles}, config: DefaultBrandPolicy(), want: ErrDenied},
		{name: "nul reason", reason: "bad\x00reason", account: actor, config: DefaultBrandPolicy(), want: ErrInvalid},
		{name: "invalid config", reason: "valid", account: actor, config: BrandPolicyConfig{MinBetPoints: 5, MaxBetPoints: amountPtr(4)}, want: ErrInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			_, err = WriteBrandPolicy(ctx, tx, policyTestBrand, test.account, 1, test.config, test.reason, policyMeta(adminID))
			if !errors.Is(err, test.want) {
				t.Fatalf("write error=%v want=%v", err, test.want)
			}
		})
	}
	staleTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = WriteBrandPolicy(ctx, staleTx, policyTestBrand, actor, 2, DefaultBrandPolicy(), "stale", policyMeta(adminID))
	_ = staleTx.Rollback(ctx)
	if !errors.Is(err, ErrVersion) {
		t.Fatalf("stale update=%v, want ErrVersion", err)
	}
	brandRecord, err := svc.BrandPolicy(ctx, policyTestBrand)
	if err != nil || brandRecord.Version != 1 {
		t.Fatalf("invalid/stale write persisted: %+v err=%v", brandRecord, err)
	}
}

func TestBrandPolicyUpdateChecksEveryExistingGameOverride(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	svc := Service{DB: db}
	adminID, gameID := ids.New(), ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, adminID, "betpolicy_cross_"+adminID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO games(id,brand_id,code,name,model,timezone,created_by) VALUES($1,$2,$3,'Policy Test','{"model":"DIGITS_0_9"}','UTC',$4)`, gameID, policyTestBrand, "policy_"+gameID[:8], adminID); err != nil {
		t.Fatal(err)
	}
	actor := policyActor(adminID, policyTestBrand)
	cfg := DefaultGamePolicy()
	cfg.MaxBetPoints = LimitOverride{Mode: "value", Points: amountPtr(50)}
	policyTx(t, db, func(tx pgx.Tx) error {
		_, e := WriteGamePolicy(ctx, tx, policyTestBrand, gameID, actor, 1, cfg, "game cap", policyMeta(adminID))
		return e
	})
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	newBrand := DefaultBrandPolicy()
	newBrand.MinBetPoints = 51
	_, err = WriteBrandPolicy(ctx, tx, policyTestBrand, actor, 1, newBrand, "raise inherited minimum above game cap", policyMeta(adminID))
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("brand update invalidating game override=%v", err)
	}
	current, err := svc.BrandPolicy(ctx, policyTestBrand)
	if err != nil || current.Version != 1 {
		t.Fatalf("failed brand write changed version: %+v err=%v", current, err)
	}
}

func TestPolicyVersionOverflowGuard(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	adminID := ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, adminID, "betpolicy_overflow_"+adminID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE brand_bet_policies SET version=$2 WHERE brand_id=$1`, policyTestBrand, int64(math.MaxInt64)); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = WriteBrandPolicy(ctx, tx, policyTestBrand, policyActor(adminID, policyTestBrand), math.MaxInt64, DefaultBrandPolicy(), "overflow guard", policyMeta(adminID))
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrVersion) {
		t.Fatalf("max version write=%v, want ErrVersion", err)
	}
}
