package betting

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// lockWithdrawalSnapshotPolicies keeps both current withdrawal policies stable
// through the bet transaction and validates the effective bet-time multiple.
func lockWithdrawalSnapshotPolicies(ctx context.Context, tx pgx.Tx, brand, game string) error {
	var brandConfig string
	if err := tx.QueryRow(ctx, `SELECT config FROM brand_withdrawal_policies WHERE brand_id=$1 FOR SHARE`, brand).Scan(&brandConfig); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSnapshot
		}
		return err
	}
	var gameConfig string
	if err := tx.QueryRow(ctx, `SELECT config FROM game_withdrawal_policies WHERE brand_id=$1 AND game_id=$2 FOR SHARE`, brand, game).Scan(&gameConfig); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSnapshot
		}
		return err
	}
	var valid bool
	if err := tx.QueryRow(ctx, `SELECT valid_positive_withdrawal_multiple(
		CASE WHEN $2::jsonb->'turnover_multiple'<>'null'::jsonb
		     THEN $2::jsonb->'turnover_multiple'
		     ELSE $1::jsonb->'turnover_multiple' END)`, brandConfig, gameConfig).Scan(&valid); err != nil || !valid {
		if err != nil {
			return err
		}
		return ErrSnapshot
	}
	return nil
}
