package reportarchive

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func AllowedAutomatic(a access.Account, brand, action string) bool {
	if action == "view" {
		return Allowed(a, brand, "view")
	}
	resource, verb := "report_archive_policy", "write"
	if action == "retry" {
		resource, verb = "report_archive_task", "retry"
	} else if action != "policy" {
		return false
	}
	return !a.SuperAdmin && access.Authorize(a, "report_archive", "view", access.ScopeBrand, brand) && access.Authorize(a, resource, verb, access.ScopeBrand, brand)
}
func (s Service) AutomaticPolicyTx(ctx context.Context, tx pgx.Tx, brand string) (AutomaticPolicy, error) {
	var out AutomaticPolicy
	if tx == nil || !archiveUUID.MatchString(brand) {
		return out, ErrInvalid
	}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT to_jsonb(p) FROM brand_report_archive_policies p WHERE brand_id=$1`, brand).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(raw, &out)
	return out, err
}

// Saved starts are explicit in the core. The management layer chooses the
// agreed activation rule; it must not silently infer a historical backfill.
func (s Service) UpdateAutomaticPolicyTx(ctx context.Context, tx pgx.Tx, brand string, a access.Account, in AutomaticPolicyInput, meta points.Metadata) (AutomaticPolicy, error) {
	var out AutomaticPolicy
	if tx == nil || !archiveUUID.MatchString(brand) || !archiveUUID.MatchString(a.ID) || in.Version < 1 || in.Version >= maxRevision || !validReason(in.Reason) || meta.ActorType != "admin" || meta.ActorID != a.ID || meta.RequestID == "" || len(meta.RequestID) > 80 {
		return out, ErrInvalid
	}
	if !AllowedAutomatic(a, brand, "policy") {
		return out, ErrDenied
	}
	var zone, status string
	if err := tx.QueryRow(ctx, `SELECT timezone,status FROM brands WHERE id=$1 FOR SHARE NOWAIT`, brand).Scan(&zone, &status); errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	} else if err != nil {
		return out, err
	}
	if status == "disabled" {
		return out, ErrState
	}
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT to_jsonb(p) FROM brand_report_archive_policies p WHERE brand_id=$1 FOR UPDATE NOWAIT`, brand).Scan(&raw); err != nil {
		return out, err
	}
	var before AutomaticPolicy
	if err := json.Unmarshal(raw, &before); err != nil {
		return out, err
	}
	if before.Version != in.Version {
		return out, ErrVersion
	}
	for _, v := range []struct {
		kind    string
		enabled bool
		key     *string
	}{{Daily, in.DailyEnabled, in.DailyStartPeriod}, {Monthly, in.MonthlyEnabled, in.MonthlyStartPeriod}} {
		if v.enabled && v.key == nil {
			return out, ErrInvalid
		}
		if v.key != nil {
			if _, err := ResolveWindow(v.kind, *v.key, zone); err != nil {
				return out, ErrInvalid
			}
		}
	}
	after := map[string]any{"brand_id": brand, "version": in.Version + 1, "daily_enabled": in.DailyEnabled, "monthly_enabled": in.MonthlyEnabled, "daily_start_period": in.DailyStartPeriod, "monthly_start_period": in.MonthlyStartPeriod, "timezone": zone}
	auditID, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "report_archive.policy.update", ResourceType: "report_archive_policy", ResourceID: brand, Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, Before: before, After: after})
	if err != nil {
		return out, err
	}
	_, err = tx.Exec(ctx, `UPDATE brand_report_archive_policies SET version=version+1,daily_enabled=$2,monthly_enabled=$3,daily_start_period=$4,monthly_start_period=$5,timezone=$6,audit_log_id=$7,updated_at=statement_timestamp() WHERE brand_id=$1`, brand, in.DailyEnabled, in.MonthlyEnabled, in.DailyStartPeriod, in.MonthlyStartPeriod, zone, auditID)
	if err != nil {
		return out, err
	}
	for _, v := range []struct {
		kind    string
		enabled bool
		key     *string
	}{{Daily, in.DailyEnabled, in.DailyStartPeriod}, {Monthly, in.MonthlyEnabled, in.MonthlyStartPeriod}} {
		if v.enabled {
			if _, err = tx.Exec(ctx, `INSERT INTO report_archive_automatic_cursors(brand_id,policy_version,kind,next_period_key) VALUES($1,$2,$3,$4)`, brand, in.Version+1, v.kind, v.key); err != nil {
				return out, err
			}
		}
	}
	return s.AutomaticPolicyTx(ctx, tx, brand)
}
