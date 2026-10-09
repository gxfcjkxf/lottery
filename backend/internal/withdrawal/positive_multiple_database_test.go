package withdrawal

import (
	"context"
	"errors"
	"testing"

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
