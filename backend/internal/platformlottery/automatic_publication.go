package platformlottery

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/jackc/pgx/v5"
)

// Callers match the source to its frozen period and validate source-specific
// evidence before publishing. All automatic readers share the same immutable
// result, configured-rule validation and audit transaction.
func publishAutomaticResult(ctx context.Context, tx pgx.Tx, id, actor string, candidate *SourceObservedDraw, action, reason string) error {
	if err := validateConfiguredPeriodDraw(ctx, tx, id, candidate.Result); err != nil {
		return err
	}
	candidate.Result.Regular = append([]int{}, candidate.Result.Regular...)
	candidate.Result.Special = append([]int{}, candidate.Result.Special...)
	candidate.Result.Digits = append([]int{}, candidate.Result.Digits...)
	raw, err := json.Marshal(candidate.Result)
	if err != nil {
		return err
	}
	changed, err := tx.Exec(ctx, `UPDATE platform_game_periods SET result=$2,published_at=clock_timestamp(),published_by=$3 WHERE id=$1 AND period_no=$4 AND result IS NULL`, id, raw, actor, candidate.PeriodNo)
	if err != nil {
		return err
	}
	if changed.RowsAffected() != 1 {
		return ErrConflict
	}
	_, err = audit.Append(ctx, tx, audit.Record{ActorType: "system", Action: action, ResourceType: "platform_game_period", ResourceID: id, Reason: reason, After: candidate})
	return err
}
