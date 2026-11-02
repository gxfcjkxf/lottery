package hkjccalendar

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Fetcher interface {
	Fetch(context.Context) ([]Month, error)
}
type Service struct {
	DB     *pgxpool.Pool
	Source Fetcher
}

var ErrBusy = errors.New("HKJC calendar sync already running")
var ErrFetch = errors.New("HKJC calendar collection failed")

type SyncResult struct {
	Status string `json:"status"`
	Months int    `json:"months"`
}

// SyncDue shares the same transaction lock and collector as manual requests.
func (s Service) SyncDue(ctx context.Context) (int, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	out, err := s.sync(ctx, tx, false)
	if errors.Is(err, ErrBusy) {
		return 0, nil
	}
	if err != nil && !errors.Is(err, ErrFetch) {
		return 0, err
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		return 0, commitErr
	}
	return out.Months, err
}

// SyncNow participates in the caller's checked, audited idempotent transaction.
// A source failure updates retry metadata and must be committed with its result.
func (s Service) SyncNow(ctx context.Context, tx pgx.Tx) (SyncResult, error) {
	return s.sync(ctx, tx, true)
}

func (s Service) sync(ctx context.Context, tx pgx.Tx, force bool) (SyncResult, error) {
	out := SyncResult{}
	var locked bool
	if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(79001034)`).Scan(&locked); err != nil {
		return out, err
	}
	if !locked {
		return out, ErrBusy
	}
	if _, err := tx.Exec(ctx, `INSERT INTO hkjc_calendar_sync(id) VALUES(1) ON CONFLICT DO NOTHING`); err != nil {
		return out, err
	}
	var due bool
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT next_attempt_at<=clock_timestamp(),clock_timestamp() FROM hkjc_calendar_sync WHERE id=1 FOR UPDATE`).Scan(&due, &now); err != nil {
		return out, err
	}
	if !force && !due {
		return out, nil
	}
	months, fetchErr := s.Source.Fetch(ctx)
	if fetchErr != nil {
		if _, err := tx.Exec(ctx, `UPDATE hkjc_calendar_sync SET last_attempt_at=$1,last_error_code='HKJC_CALENDAR_FETCH_FAILED',next_attempt_at=$1::timestamptz+interval '1 hour' WHERE id=1`, now); err != nil {
			return out, err
		}
		return SyncResult{Status: "failed"}, fmt.Errorf("%w: %w", ErrFetch, fetchErr)
	}
	if err := save(ctx, tx, months, now); err != nil {
		return out, err
	}
	if _, err := tx.Exec(ctx, `UPDATE hkjc_calendar_sync SET last_attempt_at=$1,last_success_at=clock_timestamp(),last_error_code='',next_attempt_at=((date_trunc('day',($1::timestamptz AT TIME ZONE 'Asia/Hong_Kong')-interval '4 hours')+interval '1 day 4 hours') AT TIME ZONE 'Asia/Hong_Kong') WHERE id=1`, now); err != nil {
		return out, err
	}
	return SyncResult{Status: "synced", Months: len(months)}, nil
}

func save(ctx context.Context, tx pgx.Tx, months []Month, now time.Time) error {
	// Reconcile only future/current months; retain historical calendars. Omitted
	// future months are unpublished, never interpreted as new betting schedules.
	if _, err := tx.Exec(ctx, `DELETE FROM hkjc_calendar_months WHERE month>=date_trunc('month',$1::timestamptz AT TIME ZONE 'Asia/Hong_Kong')::date`, now); err != nil {
		return err
	}
	for _, m := range months {
		_, err := tx.Exec(ctx, `INSERT INTO hkjc_calendar_months(month,draw_dates,snowball_dates,presale_dates,synced_at) VALUES($1,$2::text[]::date[],$3::text[]::date[],$4::text[]::date[],$5) ON CONFLICT(month) DO UPDATE SET draw_dates=EXCLUDED.draw_dates,snowball_dates=EXCLUDED.snowball_dates,presale_dates=EXCLUDED.presale_dates,synced_at=EXCLUDED.synced_at`, m.Month, m.DrawDates, m.SnowballDates, m.PresaleDates, now)
		if err != nil {
			return err
		}
	}
	return nil
}
