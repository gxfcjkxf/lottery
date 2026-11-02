package reportarchive

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"time"
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
	if err != nil {
		return out, err
	}
	out.NextRuns = &AutomaticNextRuns{}
	for _, k := range []string{Daily, Monthly} {
		var key *string
		var due *time.Time
		err = tx.QueryRow(ctx, `SELECT c.next_period_key,c.due_at FROM report_archive_automatic_cursors c WHERE c.brand_id=$1 AND c.policy_version=$2 AND c.kind=$3`, brand, out.Version, k).Scan(&key, &due)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return out, err
		}
		if key != nil && due != nil {
			run := &ScheduledRun{PeriodKey: *key, ExecuteAt: *due}
			if k == Daily && out.DailyEnabled {
				out.NextRuns.Daily = run
			}
			if k == Monthly && out.MonthlyEnabled {
				out.NextRuns.Monthly = run
			}
		}
	}
	return out, nil
}

// Saved starts are explicit in the core. The management layer chooses the
// agreed activation rule; it must not silently infer a historical backfill.
func (s Service) UpdateAutomaticPolicyTx(ctx context.Context, tx pgx.Tx, brand string, a access.Account, in AutomaticPolicyInput, meta points.Metadata) (AutomaticPolicy, error) {
	var out AutomaticPolicy
	if !in.AutomaticSchedule.Valid() || tx == nil || !archiveUUID.MatchString(brand) || !archiveUUID.MatchString(a.ID) || in.Version < 1 || in.Version >= maxRevision || !validReason(in.Reason) || meta.ActorType != "admin" || meta.ActorID != a.ID || meta.RequestID == "" || len(meta.RequestID) > 80 {
		return out, ErrInvalid
	}
	if !AllowedAutomatic(a, brand, "policy") {
		return out, ErrDenied
	}
	var zone, status string
	if err := tx.QueryRow(ctx, `SELECT p.timezone,b.status FROM brands b CROSS JOIN platform_settings p WHERE b.id=$1 AND p.id=true FOR SHARE OF b,p NOWAIT`, brand).Scan(&zone, &status); errors.Is(err, pgx.ErrNoRows) {
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
	after := map[string]any{"brand_id": brand, "version": in.Version + 1, "daily_enabled": in.DailyEnabled, "monthly_enabled": in.MonthlyEnabled, "daily_start_period": in.DailyStartPeriod, "monthly_start_period": in.MonthlyStartPeriod, "timezone": zone, "daily_execution_time": in.DailyExecutionTime, "daily_period_offset": in.DailyPeriodOffset, "monthly_execution_day": in.MonthlyExecutionDay, "monthly_execution_time": in.MonthlyExecutionTime, "monthly_period_offset": in.MonthlyPeriodOffset}
	auditID, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "report_archive.policy.update", ResourceType: "report_archive_policy", ResourceID: brand, Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, Before: before, After: after})
	if err != nil {
		return out, err
	}
	_, err = tx.Exec(ctx, `UPDATE brand_report_archive_policies SET version=version+1,daily_enabled=$2,monthly_enabled=$3,daily_start_period=$4,monthly_start_period=$5,timezone=$6,audit_log_id=$7,daily_execution_time=$8,daily_period_offset=$9,monthly_execution_day=$10,monthly_execution_time=$11,monthly_period_offset=$12,updated_at=statement_timestamp() WHERE brand_id=$1`, brand, in.DailyEnabled, in.MonthlyEnabled, in.DailyStartPeriod, in.MonthlyStartPeriod, zone, auditID, in.DailyExecutionTime, in.DailyPeriodOffset, in.MonthlyExecutionDay, in.MonthlyExecutionTime, in.MonthlyPeriodOffset)
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
