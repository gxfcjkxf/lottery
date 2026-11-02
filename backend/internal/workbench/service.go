package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/jackc/pgx/v5"
	"regexp"
	"strings"
)

var uuid = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Service struct{}

// ReadTx issues exactly one data SELECT. Only authorized SQL fragments are
// present, so forbidden sections do not read their underlying business tables.
// All selected facts share one PostgreSQL statement snapshot and timestamp.
func (Service) ReadTx(ctx context.Context, tx pgx.Tx, a access.Account, brand string) (Snapshot, error) {
	var out Snapshot
	if tx == nil || !uuid.MatchString(brand) {
		return out, ErrInvalid
	}
	if !Allowed(a, brand) {
		return out, ErrDenied
	}
	fields := []string{"'brand_id',s.id::text", "'snapshot_at',s.snapshot_at", "'timezone',s.timezone", "'day_from',s.day_from"}
	add := func(name, resource, query string) {
		value := `jsonb_build_object('status','forbidden','data',NULL)`
		if CanView(a, brand, resource) {
			value = `jsonb_build_object('status','ready','data',(` + query + `))`
		}
		fields = append(fields, "'"+name+"',"+value)
	}
	add("brand", "brand", `SELECT jsonb_build_object('name',s.name,'code',s.code,'state',s.status)`)

	add("orders", "bet", `SELECT jsonb_build_object('placed',count(*) FILTER(WHERE status='placed')::text,'abnormal',count(*) FILTER(WHERE status='abnormal')::text) FROM betting_financial_facts WHERE brand_id=s.id`)
	add("today_bets", "report_betting", `SELECT jsonb_build_object('order_count',count(*)::text,'stake_points',coalesce(sum(total_points::numeric),0)::text,
 'cancelled_count',count(*) FILTER(WHERE status IN('bet_cancelled','judged_cancelled'))::text,'abnormal_count',count(*) FILTER(WHERE status='abnormal')::text)
 FROM betting_financial_facts WHERE brand_id=s.id AND placed_at>=s.day_from AND placed_at<s.snapshot_at`)

	add("recharges", "recharge", `SELECT jsonb_build_object('pending_count',count(*)::text,'pending_points',coalesce(sum(points::numeric),0)::text)
 FROM recharge_orders WHERE brand_id=s.id AND state='pending'`)
	add("ledger", "report_ledger", `SELECT jsonb_build_object('entry_count',count(*)::text,'net_points',coalesce(sum(net_points),0)::text,
 'recharge_points',coalesce(sum(greatest(net_points,0)) FILTER(WHERE entry_type='recharge'),0)::text,
 'prize_credit_points',coalesce(sum(greatest(net_points,0)) FILTER(WHERE entry_type='prize'),0)::text,
 'prize_reversal_points',coalesce(sum(greatest(-net_points,0)) FILTER(WHERE entry_type='prize_reversal'),0)::text,
 'refund_points',coalesce(sum(greatest(net_points,0)) FILTER(WHERE entry_type='refund'),0)::text)
 FROM (SELECT l.entry_type,(SELECT sum(v.value::numeric) FROM jsonb_each(l.delta_snapshot) x CROSS JOIN LATERAL jsonb_each_text(x.value) v) net_points
 FROM point_ledger_entries l WHERE l.brand_id=s.id AND l.created_at>=s.day_from AND l.created_at<s.snapshot_at) entries`)
	add("balances", "report_ledger", `SELECT jsonb_build_object('account_count',(SELECT count(*)::text FROM point_accounts WHERE brand_id=s.id),
 'available_points',coalesce(sum(points::numeric) FILTER(WHERE state='available'),0)::text,
 'frozen_points',coalesce(sum(points::numeric) FILTER(WHERE state IN('manual_frozen','system_frozen')),0)::text,
 'withdrawal_points',coalesce(sum(points::numeric) FILTER(WHERE state='withdrawal'),0)::text,
 'total_points',coalesce(sum(points::numeric),0)::text) FROM point_buckets WHERE brand_id=s.id`)

	add("withdrawals", "withdrawal", `SELECT jsonb_build_object(
 'reviewing_count',count(*) FILTER(WHERE state='reviewing')::text,
 'reviewing_points',coalesce(sum(points::numeric) FILTER(WHERE state='reviewing'),0)::text,
 'processing_count',count(*) FILTER(WHERE state='processing')::text,
 'processing_points',coalesce(sum(points::numeric) FILTER(WHERE state='processing'),0)::text)
 FROM withdrawal_orders WHERE brand_id=s.id AND state IN('reviewing','processing')`)
	add("rewards", "reward", `SELECT jsonb_build_object(
 'granted_count',count(*) FILTER(WHERE state='granted')::text,
 'revoked_count',count(*) FILTER(WHERE state='revoked')::text)
 FROM reward_orders WHERE brand_id=s.id`)
	add("commissions", "commission", commissionTaskQuery)
	query := `WITH s AS MATERIALIZED(SELECT id,name,code,status,timezone,statement_timestamp() snapshot_at,
 date_trunc('day',statement_timestamp() AT TIME ZONE timezone) AT TIME ZONE timezone day_from FROM brands WHERE id=$1)
 SELECT jsonb_build_object(` + strings.Join(fields, ",") + `) FROM s`
	var raw []byte
	err := tx.QueryRow(ctx, query, brand).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	if err == nil {
		out.SnapshotAt = out.SnapshotAt.UTC()
		out.DayFrom = out.DayFrom.UTC()
	}
	return out, err
}
