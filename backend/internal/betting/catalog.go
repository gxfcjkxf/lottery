package betting

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s Service) catalogTx(ctx context.Context, brand string) (pgx.Tx, string, time.Time, error) {
	if s.DB == nil || !validIDs(brand) {
		return nil, "", time.Time{}, ErrInvalid
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, "", time.Time{}, err
	}
	var status string
	var now time.Time
	err = tx.QueryRow(ctx, `SELECT status,clock_timestamp() FROM brands WHERE id=$1 AND status IN ('active','paused')`, brand).Scan(&status, &now)
	if err != nil {
		_ = tx.Rollback(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			err = ErrNotFound
		}
		return nil, "", time.Time{}, err
	}
	return tx, status, now.UTC(), nil
}

const catalogGameFields = `id::text,code,name,model,timezone,status`
