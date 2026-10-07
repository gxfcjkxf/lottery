package withdrawal

import (
	"context"
	"errors"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPositiveWithdrawalMultipleDatabaseRejectsNewZero(t *testing.T) {
	s, a, game := fixture(t)
	ctx := context.Background()
	for _, test := range []struct {
		name string
		sql  string
		args []any
	}{
		{"brand current", `UPDATE brand_withdrawal_policies SET version=version+1,config=jsonb_set(config,'{turnover_multiple}','"0"') WHERE brand_id=$1`, []any{policyBrand}},
		{"game current", `UPDATE game_withdrawal_policies SET version=version+1,config='{"turnover_multiple":"0"}' WHERE brand_id=$1 AND game_id=$2`, []any{policyBrand, game}},
		{"brand revision", `INSERT INTO withdrawal_policy_revisions(brand_id,version,config,changed_by,reason) SELECT brand_id,version+1,jsonb_set(config,'{turnover_multiple}','"0"'),$2,'zero rejection' FROM brand_withdrawal_policies WHERE brand_id=$1`, []any{policyBrand, a.ID}},
		{"game revision", `INSERT INTO withdrawal_policy_revisions(brand_id,game_id,version,config,changed_by,reason) SELECT brand_id,game_id,version+1,'{"turnover_multiple":"0"}',$3,'zero rejection' FROM game_withdrawal_policies WHERE brand_id=$1 AND game_id=$2`, []any{policyBrand, game, a.ID}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx, err := s.DB.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(ctx, test.sql, test.args...)
			_ = tx.Rollback(ctx)
			var pgerr *pgconn.PgError
			if !errors.As(err, &pgerr) || pgerr.Code != "23514" {
				t.Fatalf("new zero multiple must fail its database CHECK before commit: %v", err)
			}
		})
	}
}

func TestPositiveWithdrawalMultipleUpgradePreservesLegacyZero(t *testing.T) {
	s, a, game := fixture(t)
	ctx := context.Background()
	// Only this test's disposable schema is changed to represent the old policy
	// schema. No immutable history or previously applied migration is rewritten.
	transact(t, s, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `ALTER TABLE brand_withdrawal_policies DROP CONSTRAINT brand_withdrawal_positive_multiple;
ALTER TABLE game_withdrawal_policies DROP CONSTRAINT game_withdrawal_positive_multiple;
ALTER TABLE withdrawal_policy_revisions DROP CONSTRAINT withdrawal_revision_positive_multiple`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO withdrawal_policy_revisions(brand_id,version,config,changed_by,reason) SELECT brand_id,version+1,jsonb_set(config,'{turnover_multiple}','"0"'),$2,'legacy zero' FROM brand_withdrawal_policies WHERE brand_id=$1`, policyBrand, a.ID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE brand_withdrawal_policies SET version=version+1,config=jsonb_set(config,'{turnover_multiple}','"0"') WHERE brand_id=$1`, policyBrand)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO withdrawal_policy_revisions(brand_id,game_id,version,config,changed_by,reason) SELECT brand_id,game_id,version+1,'{"turnover_multiple":"0"}',$3,'legacy zero' FROM game_withdrawal_policies WHERE brand_id=$1 AND game_id=$2`, policyBrand, game, a.ID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE game_withdrawal_policies SET version=version+1,config='{"turnover_multiple":"0"}' WHERE brand_id=$1 AND game_id=$2`, policyBrand, game)
		return err
	})
	var before string
	if err := s.DB.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(r) ORDER BY id)::text FROM withdrawal_policy_revisions r`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	body, err := migrations.Files.ReadFile("0042_positive_withdrawal_multiple.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(ctx, string(body)); err != nil {
		t.Fatalf("positive N migration must not overwrite or reject existing zero history: %v", err)
	}
	// Replaying 0042's CREATE OR REPLACE in this simulated upgrade also clears
	// function settings. Apply the append-only restore-path repair next, just as
	// an existing 0042 installation upgrades, rather than leaving a test-only
	// unpinned validator behind.
	body, err = migrations.Files.ReadFile("0044_pin_positive_withdrawal_validator.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(ctx, string(body)); err != nil {
		t.Fatal(err)
	}
	var after string
	if err = s.DB.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(r) ORDER BY id)::text FROM withdrawal_policy_revisions r`).Scan(&after); err != nil || before != after {
		t.Fatalf("migration modified historical policies: err=%v", err)
	}
	b, err := s.BrandPolicy(ctx, policyBrand)
	if err != nil || b.Config.TurnoverMultiple != "0" || b.Version != 2 {
		t.Fatalf("legacy current brand must remain readable without normalization: %+v %v", b, err)
	}
	g, err := s.GamePolicy(ctx, policyBrand, game)
	if err != nil || g.Effective.TurnoverMultiple != "0" || g.Effective.Source != "game" || g.Version != 2 {
		t.Fatalf("legacy current game must remain readable without inheritance: %+v %v", g, err)
	}
	c := b.Config
	c.TurnoverMultiple = "2"
	transact(t, s, func(tx pgx.Tx) error {
		_, err := s.UpdateBrand(ctx, tx, policyBrand, a, BrandInput{Version: b.Version, Config: c, Reason: "explicit legacy repair"}, metadata(a.ID))
		return err
	})
	transact(t, s, func(tx pgx.Tx) error {
		_, err := s.UpdateGame(ctx, tx, policyBrand, game, a, GameInput{Version: g.Version, Config: GameConfig{}, Reason: "explicit inherited repair"}, metadata(a.ID))
		return err
	})
	g, err = s.GamePolicy(ctx, policyBrand, game)
	if err != nil || g.Effective.TurnoverMultiple != "2" || g.Effective.Source != "brand" {
		t.Fatalf("authorized repair did not produce the requested inheritance: %+v %v", g, err)
	}
	for _, scope := range []string{"", game} {
		history, err := s.History(ctx, policyBrand, scope, 50, 0)
		if err != nil || len(history) != 3 || history[1].Version != 2 {
			t.Fatalf("legacy revision became unreadable after repair: %v %v", history, err)
		}
	}
	var entries, bets, orders int
	if err = s.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM point_ledger_entries),(SELECT count(*) FROM bet_orders),(SELECT count(*) FROM withdrawal_orders)`).Scan(&entries, &bets, &orders); err != nil || entries != 0 || bets != 0 || orders != 0 {
		t.Fatalf("policy migration/repair created funds: %d/%d/%d %v", entries, bets, orders, err)
	}
}
