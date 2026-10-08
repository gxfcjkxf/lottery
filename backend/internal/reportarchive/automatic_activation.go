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

// AutomaticActivationInput deliberately has no caller-selected start or zone.
// Historical starts remain an internal fixture/core facility, not a public API.
type AutomaticActivationInput struct {
	Version        int64  `json:"version"`
	DailyEnabled   bool   `json:"daily_enabled"`
	MonthlyEnabled bool   `json:"monthly_enabled"`
	Reason         string `json:"reason"`
}

func activationStartPeriods(before AutomaticPolicy, in AutomaticActivationInput, zone string, at time.Time) (*string, *string, error) {
	currentDaily, currentMonthly, err := ActivationPeriods(at, zone)
	if err != nil {
		return nil, nil, ErrInvalid
	}
	daily, monthly := before.DailyStartPeriod, before.MonthlyStartPeriod
	if in.DailyEnabled && daily == nil {
		daily = &currentDaily
	}
	if in.MonthlyEnabled && monthly == nil {
		monthly = &currentMonthly
	}
	return daily, monthly, nil
}

// UpdateAutomaticActivationTx implements the confirmed first-activation rule:
// each kind starts in its current brand-local period, never before it. Existing
// saved starts and tasks survive disabling and re-enabling. The database clock
// and locked brand timezone are authoritative; client clocks are not inputs.
func (s Service) UpdateAutomaticActivationTx(ctx context.Context, tx pgx.Tx, brand string, a access.Account, in AutomaticActivationInput, meta points.Metadata) (AutomaticPolicy, error) {
	var out AutomaticPolicy
	if tx == nil || !archiveUUID.MatchString(brand) || !archiveUUID.MatchString(a.ID) || in.Version < 1 || in.Version >= maxRevision || !validReason(in.Reason) || meta.ActorType != "admin" || meta.ActorID != a.ID || meta.RequestID == "" || len(meta.RequestID) > 80 {
		return out, ErrInvalid
	}
	if !AllowedAutomatic(a, brand, "policy") {
		return out, ErrDenied
	}
	var zone, status string
	err := tx.QueryRow(ctx, `SELECT timezone,status FROM brands WHERE id=$1 FOR SHARE NOWAIT`, brand).Scan(&zone, &status)
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
	daily, monthly, err := activationStartPeriods(before, in, zone, at)
	if err != nil {
		return out, err
	}
	return s.UpdateAutomaticPolicyTx(ctx, tx, brand, a, AutomaticPolicyInput{
		Version: in.Version, DailyEnabled: in.DailyEnabled, MonthlyEnabled: in.MonthlyEnabled,
		DailyStartPeriod: daily, MonthlyStartPeriod: monthly, Reason: in.Reason,
	}, meta)
}
