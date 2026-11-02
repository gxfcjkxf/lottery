package hkjccalendar

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type CalendarMonth struct {
	Month         string   `json:"month"`
	DrawDates     []string `json:"draw_dates"`
	SnowballDates []string `json:"snowball_dates"`
	PresaleDates  []string `json:"presale_dates"`
}
type Overview struct {
	Months        []CalendarMonth `json:"months"`
	CurrentMonth  string          `json:"current_month"`
	Status        string          `json:"status"`
	LastSuccessAt *time.Time      `json:"last_success_at"`
	LastAttemptAt *time.Time      `json:"last_attempt_at"`
	NextAttemptAt *time.Time      `json:"next_attempt_at"`
}

// Overview returns Hong Kong calendar dates, never derived period numbers/times.
func (s Service) Overview(ctx context.Context) (Overview, error) {
	out := Overview{Months: []CalendarMonth{}, Status: "pending"}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = tx.QueryRow(ctx, `SELECT to_char(clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong','YYYY-MM')`).Scan(&out.CurrentMonth); err != nil {
		return out, err
	}
	err = tx.QueryRow(ctx, `SELECT last_success_at,last_attempt_at,CASE WHEN next_attempt_at='-infinity'::timestamptz THEN NULL ELSE next_attempt_at END,
 CASE WHEN last_error_code<>'' THEN 'failed' WHEN last_success_at IS NULL THEN 'pending' WHEN next_attempt_at<clock_timestamp()-interval '5 minutes' THEN 'overdue' ELSE 'synced' END
 FROM hkjc_calendar_sync WHERE id=1`).Scan(&out.LastSuccessAt, &out.LastAttemptAt, &out.NextAttemptAt, &out.Status)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT to_char(month,'YYYY-MM'),ARRAY(SELECT d::text FROM unnest(draw_dates) d ORDER BY d),ARRAY(SELECT d::text FROM unnest(snowball_dates) d ORDER BY d),ARRAY(SELECT d::text FROM unnest(presale_dates) d ORDER BY d) FROM hkjc_calendar_months ORDER BY month`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var m CalendarMonth
		if err = rows.Scan(&m.Month, &m.DrawDates, &m.SnowballDates, &m.PresaleDates); err != nil {
			rows.Close()
			return out, err
		}
		out.Months = append(out.Months, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
