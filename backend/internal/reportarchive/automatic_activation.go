package reportarchive

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

// AutomaticActivationInput specifies execution rules; the database derives the next target using platform time.
type AutomaticActivationInput struct {
	AutomaticSchedule
	Version        int64  `json:"version"`
	DailyEnabled   bool   `json:"daily_enabled"`
	MonthlyEnabled bool   `json:"monthly_enabled"`
	Reason         string `json:"reason"`
}

// UpdateAutomaticActivationTx starts the new schedule at its next future occurrence.
func (s Service) UpdateAutomaticActivationTx(ctx context.Context, tx pgx.Tx, brand string, a access.Account, in AutomaticActivationInput, meta points.Metadata) (AutomaticPolicy, error) {
	var out AutomaticPolicy
	if tx == nil || !archiveUUID.MatchString(brand) || !archiveUUID.MatchString(a.ID) || in.Version < 1 || in.Version >= maxRevision || !validReason(in.Reason) || meta.ActorType != "admin" || meta.ActorID != a.ID || meta.RequestID == "" || len(meta.RequestID) > 80 {
		return out, ErrInvalid
	}
	if !AllowedAutomatic(a, brand, "policy") {
		return out, ErrDenied
	}
	var zone, status string
	err := tx.QueryRow(ctx, `SELECT p.timezone,b.status FROM brands b CROSS JOIN platform_settings p WHERE b.id=$1 AND p.id=true FOR SHARE OF b,p NOWAIT`, brand).Scan(&zone, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if status == "disabled" {
		return out, ErrState
	}
	var raw []byte
	var at time.Time
	err = tx.QueryRow(ctx, `SELECT to_jsonb(p),statement_timestamp() FROM brand_report_archive_policies p WHERE brand_id=$1 FOR UPDATE NOWAIT`, brand).Scan(&raw, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	var before AutomaticPolicy
	if err = json.Unmarshal(raw, &before); err != nil {
		return out, err
	}
	if before.Version != in.Version {
		return out, ErrVersion
	}
	if !in.AutomaticSchedule.Valid() {
		return out, ErrInvalid
	}
	var daily, monthly *string
	err = tx.QueryRow(ctx, `SELECT
 CASE WHEN $2 THEN report_archive_next_target('daily',$1,$4,$5,1,$9) ELSE $10 END,
 CASE WHEN $3 THEN report_archive_next_target('monthly',$1,$7,$8,$6,$9) ELSE $11 END`,
		zone, in.DailyEnabled, in.MonthlyEnabled, in.DailyExecutionTime, in.DailyPeriodOffset,
		in.MonthlyExecutionDay, in.MonthlyExecutionTime, in.MonthlyPeriodOffset, at, before.DailyStartPeriod, before.MonthlyStartPeriod).Scan(&daily, &monthly)
	if err != nil {
		return out, err
	}
	return s.UpdateAutomaticPolicyTx(ctx, tx, brand, a, AutomaticPolicyInput{
		AutomaticSchedule: in.AutomaticSchedule, Version: in.Version, DailyEnabled: in.DailyEnabled, MonthlyEnabled: in.MonthlyEnabled,
		DailyStartPeriod: daily, MonthlyStartPeriod: monthly, Reason: in.Reason,
	}, meta)
}
