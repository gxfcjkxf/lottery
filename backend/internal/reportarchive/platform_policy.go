package reportarchive

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"time"
)

type PlatformPolicy struct {
	AutomaticSchedule
	Version        int64              `json:"version"`
	DailyEnabled   bool               `json:"daily_enabled"`
	MonthlyEnabled bool               `json:"monthly_enabled"`
	Timezone       string             `json:"timezone"`
	NextRuns       *AutomaticNextRuns `json:"next_runs"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

func (s Service) PlatformPolicyTx(ctx context.Context, tx pgx.Tx) (PlatformPolicy, error) {
	var out PlatformPolicy
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT (to_jsonb(c)-ARRAY['id','audit_log_id'])||jsonb_build_object('timezone',p.timezone) FROM platform_report_archive_policy c CROSS JOIN platform_settings p WHERE c.id=true AND p.id=true`).Scan(&raw)
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return out, err
	}
	out.NextRuns = &AutomaticNextRuns{}
	for _, kind := range []string{Daily, Monthly} {
		enabled, clock, offset, day := out.DailyEnabled, out.DailyExecutionTime, out.DailyPeriodOffset, 1
		if kind == Monthly {
			enabled, clock, offset, day = out.MonthlyEnabled, out.MonthlyExecutionTime, out.MonthlyPeriodOffset, out.MonthlyExecutionDay
		}
		if !enabled {
			continue
		}
		var run ScheduledRun
		err = tx.QueryRow(ctx, `SELECT key,report_archive_scheduled_due($1,key,$2,$3,$4,$5) FROM (SELECT report_archive_next_target($1,$2,$3,$4,$5,statement_timestamp()) key) n`, kind, out.Timezone, clock, offset, day).Scan(&run.PeriodKey, &run.ExecuteAt)
		if err != nil {
			return out, err
		}
		if kind == Daily {
			out.NextRuns.Daily = &run
		} else {
			out.NextRuns.Monthly = &run
		}
	}
	return out, nil
}
func (s Service) UpdatePlatformPolicyTx(ctx context.Context, tx pgx.Tx, a access.Account, in AutomaticActivationInput, meta points.Metadata) (PlatformPolicy, error) {
	var out PlatformPolicy
	if !access.Authorize(a, "platform_settings", "write", access.ScopePlatform, "") {
		return out, ErrDenied
	}
	if !in.AutomaticSchedule.Valid() || in.Version < 1 || in.Version >= maxRevision || !validReason(in.Reason) || meta.ActorType != "admin" || meta.ActorID != a.ID || meta.RequestID == "" {
		return out, ErrInvalid
	}
	var raw []byte
	var version int64
	if err := tx.QueryRow(ctx, `SELECT to_jsonb(c),version FROM platform_report_archive_policy c WHERE id=true FOR UPDATE`).Scan(&raw, &version); err != nil {
		return out, err
	}
	if version != in.Version {
		return out, ErrVersion
	}
	after := map[string]any{"id": true, "version": version + 1, "daily_enabled": in.DailyEnabled, "monthly_enabled": in.MonthlyEnabled, "daily_execution_time": in.DailyExecutionTime, "daily_period_offset": in.DailyPeriodOffset, "monthly_execution_day": in.MonthlyExecutionDay, "monthly_execution_time": in.MonthlyExecutionTime, "monthly_period_offset": in.MonthlyPeriodOffset}
	aid, err := audit.Append(ctx, tx, audit.Record{ActorType: "admin", ActorID: a.ID, Action: "report_archive.platform_policy.update", ResourceType: "platform_report_archive_policy", Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, Before: json.RawMessage(raw), After: after})
	if err != nil {
		return out, err
	}
	_, err = tx.Exec(ctx, `UPDATE platform_report_archive_policy SET version=version+1,daily_enabled=$1,monthly_enabled=$2,daily_execution_time=$3,daily_period_offset=$4,monthly_execution_day=$5,monthly_execution_time=$6,monthly_period_offset=$7,audit_log_id=$8,updated_at=statement_timestamp() WHERE id=true`, in.DailyEnabled, in.MonthlyEnabled, in.DailyExecutionTime, in.DailyPeriodOffset, in.MonthlyExecutionDay, in.MonthlyExecutionTime, in.MonthlyPeriodOffset, aid)
	if err != nil {
		return out, err
	}
	return s.PlatformPolicyTx(ctx, tx)
}
