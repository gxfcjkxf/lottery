package dataretention

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (s Service) PolicyTx(ctx context.Context, tx pgx.Tx) (Policy, error) {
	var p Policy
	var execute, cutoff *time.Time
	err := tx.QueryRow(ctx, `SELECT r.version,r.enabled,r.execution_day,r.execution_time,p.timezone,r.next_execute_at,r.next_cutoff_at,r.updated_at FROM data_retention_policy r CROSS JOIN platform_settings p WHERE r.id AND p.id`).Scan(&p.Version, &p.Enabled, &p.ExecutionDay, &p.ExecutionTime, &p.Timezone, &execute, &cutoff, &p.UpdatedAt)
	if execute != nil && cutoff != nil {
		p.NextRun = &Run{ExecuteAt: *execute, CutoffAt: *cutoff}
	}
	return p, err
}

func (s Service) UpdatePolicyTx(ctx context.Context, tx pgx.Tx, in PolicyInput, meta points.Metadata) (Policy, error) {
	if in.Validate() != nil || in.Version < 1 || in.Version >= 9007199254740991 || meta.ActorType != "admin" || !uuid.MatchString(meta.ActorID) {
		return Policy{}, ErrInvalid
	}
	var version int64
	if err := tx.QueryRow(ctx, `SELECT version FROM data_retention_policy WHERE id FOR UPDATE`).Scan(&version); err != nil {
		return Policy{}, err
	}
	if version != in.Version {
		return Policy{}, ErrVersion
	}
	old, err := s.PolicyTx(ctx, tx)
	if err != nil {
		return Policy{}, err
	}
	var execute, cutoff *time.Time
	if in.Enabled {
		if old.Enabled && old.Schedule == in.Schedule {
			execute = &old.NextRun.ExecuteAt
			cutoff = &old.NextRun.CutoffAt
		} else {
			var now time.Time
			if err = tx.QueryRow(ctx, `SELECT statement_timestamp()`).Scan(&now); err != nil {
				return Policy{}, err
			}
			run, e := in.NextRun(now, old.Timezone)
			if e != nil {
				return Policy{}, e
			}
			execute = &run.ExecuteAt
			cutoff = &run.CutoffAt
		}
	}
	_, err = tx.Exec(ctx, `UPDATE data_retention_policy SET version=version+1,enabled=$1,execution_day=$2,execution_time=$3,next_execute_at=$4,next_cutoff_at=$5,next_timezone=$6,updated_at=statement_timestamp() WHERE id`, in.Enabled, in.ExecutionDay, in.ExecutionTime, execute, cutoff, old.Timezone)
	if err != nil {
		return Policy{}, err
	}
	_, err = audit.Append(ctx, tx, audit.Record{ActorType: meta.ActorType, ActorID: meta.ActorID, Action: "data_retention.policy.update", ResourceType: "data_retention_policy", RequestID: meta.RequestID, IP: meta.IP, Before: old, After: in})
	if err != nil {
		return Policy{}, err
	}
	return s.PolicyTx(ctx, tx)
}

// Platform timezone edits are schedule edits. Already-created tasks keep their
// saved timezone and cutoff; undiscovered runs start at the next future clock.
func (s Service) SyncTimezoneTx(ctx context.Context, tx pgx.Tx, oldZone, newZone string, meta points.Metadata) error {
	if oldZone == newZone {
		return nil
	}
	if _, err := tx.Exec(ctx, `SELECT id FROM data_retention_policy WHERE id FOR UPDATE`); err != nil {
		return err
	}
	p, err := s.PolicyTx(ctx, tx)
	if err != nil {
		return err
	}
	p.Timezone = oldZone
	var execute, cutoff *time.Time
	if p.Enabled {
		var now time.Time
		if err = tx.QueryRow(ctx, `SELECT statement_timestamp()`).Scan(&now); err != nil {
			return err
		}
		run, e := p.Schedule.NextRun(now, newZone)
		if e != nil {
			return e
		}
		execute = &run.ExecuteAt
		cutoff = &run.CutoffAt
	}
	if _, err = tx.Exec(ctx, `UPDATE data_retention_policy SET version=version+1,next_execute_at=$1,next_cutoff_at=$2,next_timezone=$3,updated_at=statement_timestamp() WHERE id`, execute, cutoff, newZone); err != nil {
		return err
	}
	after, err := s.PolicyTx(ctx, tx)
	if err != nil {
		return err
	}
	_, err = audit.Append(ctx, tx, audit.Record{ActorType: meta.ActorType, ActorID: meta.ActorID, Action: "data_retention.policy.sync", ResourceType: "data_retention_policy", RequestID: meta.RequestID, IP: meta.IP, Before: p, After: after})
	return err
}

func (s Service) Discover(ctx context.Context) (int, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var p Policy
	var execute, cutoff *time.Time
	var zone string
	var now time.Time
	err = tx.QueryRow(ctx, `SELECT version,enabled,execution_day,execution_time,next_execute_at,next_cutoff_at,coalesce(next_timezone,''),statement_timestamp() FROM data_retention_policy WHERE id FOR UPDATE SKIP LOCKED`).Scan(&p.Version, &p.Enabled, &p.ExecutionDay, &p.ExecutionTime, &execute, &cutoff, &zone, &now)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !p.Enabled || execute.After(now) {
		return 0, nil
	}
	// One persisted monthly cursor is advanced per transaction. A stopped worker
	// catches up without replacing a previous task's cutoff with today's month.
	brands := []string{""}
	rows, err := tx.Query(ctx, `SELECT id::text FROM brands ORDER BY id`)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var b string
		if err = rows.Scan(&b); err != nil {
			rows.Close()
			return 0, err
		}
		brands = append(brands, b)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, b := range brands {
		result, e := tx.Exec(ctx, `INSERT INTO data_retention_tasks(id,brand_id,policy_version,timezone,execute_at,cutoff_at) VALUES($1,nullif($2,'')::uuid,$3,$4,$5,$6) ON CONFLICT(brand_id,execute_at) DO NOTHING`, ids.New(), b, p.Version, zone, execute, cutoff)
		if e != nil {
			return 0, e
		}
		n += int(result.RowsAffected())
	}
	var currentZone string
	if err = tx.QueryRow(ctx, `SELECT timezone FROM platform_settings WHERE id`).Scan(&currentZone); err != nil {
		return 0, err
	}
	next, err := p.Schedule.NextRun(*execute, currentZone)
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(ctx, `UPDATE data_retention_policy SET next_execute_at=$1,next_cutoff_at=$2,next_timezone=$3 WHERE id`, next.ExecuteAt, next.CutoffAt, currentZone)
	if err != nil {
		return 0, err
	}
	return n, tx.Commit(ctx)
}

func (s Service) TasksTx(ctx context.Context, tx pgx.Tx, brand string, limit, offset int) (Page, error) {
	out := Page{Items: []Task{}, Limit: limit, Offset: offset}
	if brand != "" && !uuid.MatchString(brand) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return out, ErrInvalid
	}
	var raw []byte
	err := tx.QueryRow(ctx, `WITH selected AS(SELECT t.*,b.name AS brand_name FROM data_retention_tasks t LEFT JOIN brands b ON b.id=t.brand_id WHERE ($1='' OR t.brand_id=nullif($1,'')::uuid) ORDER BY execute_at DESC,t.id DESC LIMIT $2 OFFSET $3) SELECT (SELECT count(*)::text FROM data_retention_tasks WHERE ($1='' OR brand_id=nullif($1,'')::uuid)),coalesce((SELECT jsonb_agg(to_jsonb(s)||jsonb_build_object('brand_id',coalesce(s.brand_id::text,''),'brand_name',coalesce(s.brand_name,'')) ORDER BY execute_at DESC,id DESC) FROM selected s),'[]')`, brand, limit, offset).Scan(&out.TotalCount, &raw)
	if err == nil {
		err = json.Unmarshal(raw, &out.Items)
	}
	return out, err
}

func (s Service) RetryTx(ctx context.Context, tx pgx.Tx, id string, meta points.Metadata) error {
	if !uuid.MatchString(id) {
		return ErrInvalid
	}
	result, err := tx.Exec(ctx, `UPDATE data_retention_tasks SET state='pending',error_code='',finished_at=NULL WHERE id=$1 AND state='failed'`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrVersion
	}
	_, err = audit.Append(ctx, tx, audit.Record{ActorType: meta.ActorType, ActorID: meta.ActorID, Action: "data_retention.task.retry", ResourceType: "data_retention_task", ResourceID: id, RequestID: meta.RequestID, IP: meta.IP})
	return err
}

type family struct{ name, clock, eligible, dimensions, totals string }

var families = []family{
	{"configured_orders", "placed_at", `x.status='settled' AND x.settled_at<$2 AND (x.commission_rule_snapshot->'financial_policy'->'config'->>'enabled'='false' OR (x.commission_rule_snapshot->'financial_policy'->'config'->>'enabled'='true' AND NOT EXISTS(SELECT 1 FROM commission_discovery d WHERE d.id=x.id AND d.state<>'registered'))) AND NOT EXISTS(SELECT 1 FROM configured_orders pending WHERE pending.period_id=x.period_id AND pending.status='placed') AND NOT EXISTS(SELECT 1 FROM commission_cycle_targets ct JOIN commission_cycles c ON c.id=ct.cycle_id WHERE ct.order_id=x.id AND (c.state<>'ready' OR EXISTS(SELECT 1 FROM commission_payments p WHERE p.cycle_id=c.id AND p.state NOT IN('paid','stale')) OR EXISTS(SELECT 1 FROM commission_correction_executions e WHERE e.cycle_id=c.id AND e.state<>'completed')))`, `jsonb_build_object('member_id',x.member_id,'game_id',x.game_id,'status',CASE WHEN (x.result->>'prize_points')::numeric>0 THEN 'won' ELSE 'lost' END,'attribution_snapshot',jsonb_set(x.attribution_snapshot,'{captured_at}',to_jsonb((x.placed_at AT TIME ZONE $3)::date::text)))`, `jsonb_build_object('order_count','1','stake_points',x.total_points::text,'prize_points',coalesce(x.result->>'prize_points','0'),'refund_points',CASE WHEN x.snapshot->'catalog'->>'funding'='reserve' THEN '0' ELSE coalesce(x.result->>'refund_points','0') END,'effective_points',coalesce(x.result->>'effective_points','0'))`},
	{"cash_orders", "created_at", `x.state IN('confirmed','cancelled','paid','rejected','failed','completed') AND NOT EXISTS(SELECT 1 FROM cash_entries e WHERE e.order_id=x.id AND e.created_at>=$2) AND NOT EXISTS(SELECT 1 FROM cash_orders active WHERE active.brand_id=x.brand_id AND active.member_id=x.member_id AND active.state IN('submitted','reviewing','processing'))`, `jsonb_build_object('member_id',x.member_id,'currency',x.currency,'kind',x.kind,'state',x.state)`, `jsonb_build_object('order_count','1','amount',x.amount::text,'points',x.points::text,'fee',x.fee::text)`},
	{"recharge_orders", "created_at", `x.state IN('confirmed','cancelled') AND (x.confirmed_at IS NULL OR x.confirmed_at<$2)`, `jsonb_build_object('member_id',x.member_id,'state',x.state)`, `jsonb_build_object('order_count','1','points',x.points::text)`},
	{"withdrawal_orders", "created_at", `x.state IN('paid','rejected','failed','cancelled') AND x.completed_at<$2`, `jsonb_build_object('member_id',x.member_id,'state',x.state)`, `jsonb_build_object('order_count','1','points',x.points::text)`},
	{"cash_entries", "created_at", `EXISTS(SELECT 1 FROM data_retention_receipts r WHERE r.family='cash_orders' AND r.id=x.order_id)`, `jsonb_build_object('member_id',x.member_id,'currency',x.currency,'operation',x.operation)`, `jsonb_build_object('entry_count','1','deposited_delta',x.delta_snapshot->>0,'withdrawable_delta',x.delta_snapshot->>1,'reserved_delta',x.delta_snapshot->>2)`},
	{"point_ledger_entries", "created_at", `NOT EXISTS(SELECT 1 FROM configured_orders o WHERE o.id=x.reference_id) AND NOT EXISTS(SELECT 1 FROM cash_orders o WHERE o.id=x.reference_id) AND NOT EXISTS(SELECT 1 FROM recharge_orders o WHERE o.id=x.reference_id) AND NOT EXISTS(SELECT 1 FROM withdrawal_orders o WHERE o.id=x.reference_id)`, `jsonb_build_object('member_id',x.member_id,'entry_type',x.entry_type)`, `jsonb_build_object('entry_count','1','net_points',(SELECT coalesce(sum(v.value::numeric),0)::text FROM jsonb_each(x.delta_snapshot) s CROSS JOIN LATERAL jsonb_each_text(s.value) v),'positive_points',(SELECT greatest(coalesce(sum(v.value::numeric),0),0)::text FROM jsonb_each(x.delta_snapshot) s CROSS JOIN LATERAL jsonb_each_text(s.value) v),'negative_points',(SELECT greatest(-coalesce(sum(v.value::numeric),0),0)::text FROM jsonb_each(x.delta_snapshot) s CROSS JOIN LATERAL jsonb_each_text(s.value) v))`},
	{"notifications", "created_at", `EXISTS(SELECT 1 FROM consumed_events c WHERE c.event_id=x.event_id AND c.consumer='in-app-notification-v1') AND NOT EXISTS(SELECT 1 FROM notification_deliveries d WHERE d.event_id=x.event_id AND d.status<>'sent')`, `jsonb_build_object('event_type',x.event_type)`, `jsonb_build_object('record_count','1')`},
	{"audit_logs", "created_at", `NOT EXISTS(SELECT 1 FROM configured_orders o WHERE o.id=x.resource_id) AND (x.resource_type<>'point_account' OR NOT EXISTS(SELECT 1 FROM point_ledger_entries l WHERE l.id::text=x.after_json->>'ledger_entry_id'))`, `jsonb_build_object('action',x.action,'resource_type',x.resource_type)`, `jsonb_build_object('record_count','1')`},
}

func add(a, b string) string {
	left, ok := new(big.Int).SetString(a, 10)
	if !ok {
		panic("invalid stored retention count")
	}
	right, ok := new(big.Int).SetString(b, 10)
	if !ok {
		panic("invalid retention count")
	}
	return left.Add(left, right).String()
}

// Process commits at most one small batch. Receipt, summary, deletion and task
// progress are atomic. All family names and predicates are closed constants.
func (s Service) Process(ctx context.Context) (bool, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var t Task
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT to_jsonb(t)||jsonb_build_object('brand_id',coalesce(t.brand_id::text,'')) FROM data_retention_tasks t WHERE state='pending' ORDER BY execute_at,id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = json.Unmarshal(raw, &t); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('retention:'||$1,0))`, t.BrandID); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `SAVEPOINT retention_batch`); err != nil {
		return false, err
	}
	worked, err := s.processBatch(ctx, tx, &t)
	if err != nil {
		if _, e := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT retention_batch`); e != nil {
			return false, err
		}
		if _, e := tx.Exec(ctx, `UPDATE data_retention_tasks SET state='failed',error_code='DATA_RETENTION_BATCH_FAILED',finished_at=statement_timestamp() WHERE id=$1`, t.ID); e != nil {
			return false, e
		}
		if e := tx.Commit(ctx); e != nil {
			return false, e
		}
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return worked, nil
}

func (s Service) processBatch(ctx context.Context, tx pgx.Tx, t *Task) (bool, error) {
	for _, f := range families {
		query := fmt.Sprintf(`SELECT x.id::text,to_jsonb(x),%s,%s,(x.%s AT TIME ZONE $3)::date::text FROM %s x WHERE x.brand_id IS NOT DISTINCT FROM nullif($1,'')::uuid AND x.%s<$2 AND (%s) ORDER BY x.%s,x.id LIMIT 100 FOR UPDATE OF x SKIP LOCKED`, f.dimensions, f.totals, f.clock, f.name, f.clock, f.eligible, f.clock)
		rows, err := tx.Query(ctx, query, t.BrandID, t.CutoffAt, t.Timezone)
		if err != nil {
			return false, err
		}
		type item struct {
			id, day                  string
			body, dimensions, totals []byte
		}
		items := []item{}
		for rows.Next() {
			var i item
			if err = rows.Scan(&i.id, &i.body, &i.dimensions, &i.totals, &i.day); err != nil {
				rows.Close()
				return false, err
			}
			items = append(items, i)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return false, err
		}
		if len(items) == 0 {
			continue
		}
		c, exists := t.Counts[f.name]
		if !exists {
			c = Counts{"0", "0", "0"}
		}
		for _, i := range items {
			_, err = tx.Exec(ctx, `INSERT INTO data_retention_receipts(family,id,brand_id,task_id,payload,original_sha256,discard) VALUES($1,$2,nullif($3,'')::uuid,$4,retention_payload($1,$5::jsonb),encode(sha256(convert_to(($5::jsonb)::text,'UTF8')),'hex'),($1='audit_logs' AND $5::jsonb->>'resource_type'='query') OR $1='notifications')`, f.name, i.id, t.BrandID, t.ID, i.body)
			if err != nil {
				return false, err
			}
			if f.name == "point_ledger_entries" {
				_, err = tx.Exec(ctx, `INSERT INTO data_retention_account_days(brand_id,account_id,timezone,day,first_version,last_version,entry_count,opening_snapshot,delta_snapshot,closing_snapshot)
 SELECT $1,($4::jsonb->>'account_id')::uuid,$2,$3,($4::jsonb->>'version')::bigint,($4::jsonb->>'version')::bigint,1,$4::jsonb->'before_snapshot',$4::jsonb->'delta_snapshot',$4::jsonb->'after_snapshot'
 ON CONFLICT(brand_id,account_id,timezone,day) DO UPDATE SET
 opening_snapshot=CASE WHEN EXCLUDED.first_version<data_retention_account_days.first_version THEN EXCLUDED.opening_snapshot ELSE data_retention_account_days.opening_snapshot END,
 closing_snapshot=CASE WHEN EXCLUDED.last_version>data_retention_account_days.last_version THEN EXCLUDED.closing_snapshot ELSE data_retention_account_days.closing_snapshot END,
 first_version=least(EXCLUDED.first_version,data_retention_account_days.first_version),last_version=greatest(EXCLUDED.last_version,data_retention_account_days.last_version),entry_count=data_retention_account_days.entry_count+1,
 delta_snapshot=retention_add_snapshots(data_retention_account_days.delta_snapshot,EXCLUDED.delta_snapshot)`, t.BrandID, t.Timezone, i.day, i.body)
				if err != nil {
					return false, err
				}
			}
			_, err = tx.Exec(ctx, `INSERT INTO data_retention_daily(brand_id,timezone,day,family,dimensions,totals) VALUES(nullif($1,'')::uuid,$2,$3,$4,$5,$6) ON CONFLICT ON CONSTRAINT data_retention_daily_pkey DO UPDATE SET totals=retention_add_totals(data_retention_daily.totals,EXCLUDED.totals)`, t.BrandID, t.Timezone, i.day, f.name, i.dimensions, i.totals)
			if err != nil {
				return false, err
			}
			result, e := tx.Exec(ctx, `DELETE FROM `+f.name+` WHERE brand_id IS NOT DISTINCT FROM nullif($1,'')::uuid AND id=$2`, t.BrandID, i.id)
			if e != nil {
				return false, e
			}
			if result.RowsAffected() != 1 {
				return false, ErrVersion
			}
			var discard bool
			if err = tx.QueryRow(ctx, `SELECT discard FROM data_retention_receipts WHERE family=$1 AND id=$2`, f.name, i.id).Scan(&discard); err != nil {
				return false, err
			}
			if discard {
				if f.name == "audit_logs" {
					if _, err = tx.Exec(ctx, `DELETE FROM retention_keys_audit_logs WHERE id=$1`, i.id); err != nil {
						return false, err
					}
				}
				if _, err = tx.Exec(ctx, `DELETE FROM data_retention_receipts WHERE family=$1 AND id=$2`, f.name, i.id); err != nil {
					return false, err
				}
			} else {
				c.Receipts = add(c.Receipts, "1")
			}
			c.Deleted = add(c.Deleted, "1")
		}
		t.Counts[f.name] = c
		counts, _ := json.Marshal(t.Counts)
		_, err = tx.Exec(ctx, `UPDATE data_retention_tasks SET counts=$2 WHERE id=$1`, t.ID, counts)
		return true, err
	}
	if worked, err := processTemporary(ctx, tx, t); worked || err != nil {
		return worked, err
	}
	state := "completed"
	for _, f := range families {
		var pending string
		err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT count(*)::text FROM %s WHERE brand_id IS NOT DISTINCT FROM nullif($1,'')::uuid AND %s<$2`, f.name, f.clock), t.BrandID, t.CutoffAt).Scan(&pending)
		if err != nil {
			return false, err
		}
		c, ok := t.Counts[f.name]
		if !ok {
			c = Counts{"0", "0", "0"}
		}
		c.Deferred = pending
		t.Counts[f.name] = c
		if pending != "0" {
			state = "deferred"
		}
	}
	counts, _ := json.Marshal(t.Counts)
	_, err := tx.Exec(ctx, `UPDATE data_retention_tasks SET state=$2,counts=$3,finished_at=statement_timestamp() WHERE id=$1`, t.ID, state, counts)
	if err != nil {
		return false, err
	}
	_, err = audit.Append(ctx, tx, audit.Record{BrandID: t.BrandID, ActorType: "system", Action: "data_retention.task.complete", ResourceType: "data_retention_task", ResourceID: t.ID, RequestID: "retention:" + t.ID, After: map[string]any{"state": state, "cutoff_at": t.CutoffAt, "counts": t.Counts}})
	return true, err
}

func processTemporary(ctx context.Context, tx pgx.Tx, t *Task) (bool, error) {
	for _, table := range []string{"sessions", "agent_sessions", "auth_challenges", "auth_rate_limits", "idempotency_requests", "platform_idempotency_requests", "notification_deliveries", "platform_automation_attempts", "platform_hk_source_checks"} {
		var query, key string
		if table == "auth_rate_limits" || table == "platform_idempotency_requests" || table == "platform_automation_attempts" || table == "platform_hk_source_checks" {
			if t.BrandID != "" {
				continue
			}
		}
		scope := `brand_id IS NOT DISTINCT FROM nullif($1,'')::uuid`
		if table == "auth_rate_limits" || table == "platform_idempotency_requests" {
			scope = `$1=''`
		}
		if table == "notification_deliveries" {
			key = "notification_records"
			query = `WITH batch AS(SELECT d.ctid FROM notification_deliveries d JOIN outbox_events e ON e.id=d.event_id WHERE d.brand_id IS NOT DISTINCT FROM nullif($1,'')::uuid AND d.status='sent' AND d.sent_at<$2 AND e.created_at<$2 AND EXISTS(SELECT 1 FROM consumed_events c WHERE c.event_id=d.event_id AND c.consumer='in-app-notification-v1') ORDER BY d.sent_at,d.event_id LIMIT 100 FOR UPDATE OF d SKIP LOCKED) DELETE FROM notification_deliveries d USING batch b WHERE d.ctid=b.ctid`
		} else if table == "platform_automation_attempts" {
			key = "operational_records"
			query = `WITH batch AS(SELECT a.ctid FROM platform_automation_attempts a JOIN platform_game_periods p ON p.id=a.period_id WHERE $1='' AND a.last_attempt_at<$2 AND p.result IS NOT NULL AND a.period_id<>(SELECT recent.period_id FROM platform_automation_attempts recent JOIN platform_game_periods rp ON rp.id=recent.period_id WHERE rp.game_id=p.game_id ORDER BY recent.last_attempt_at DESC,recent.period_id DESC LIMIT 1) ORDER BY a.last_attempt_at,a.period_id LIMIT 100 FOR UPDATE OF a SKIP LOCKED) DELETE FROM platform_automation_attempts a USING batch b WHERE a.ctid=b.ctid`
		} else if table == "platform_hk_source_checks" {
			key = "operational_records"
			query = `WITH batch AS(SELECT c.ctid FROM platform_hk_source_checks c WHERE $1='' AND c.checked_at<$2 AND NOT EXISTS(SELECT 1 FROM platform_automation_settings h WHERE h.game_id=c.game_id AND h.version=c.version) AND NOT EXISTS(SELECT 1 FROM platform_game_periods p WHERE p.game_id=c.game_id AND p.automation_version=c.version AND p.result IS NULL) ORDER BY c.checked_at,c.game_id LIMIT 100 FOR UPDATE OF c SKIP LOCKED) DELETE FROM platform_hk_source_checks c USING batch b WHERE c.ctid=b.ctid`
		} else if table == "idempotency_requests" || table == "platform_idempotency_requests" {
			key = "request_receipts"
			query = fmt.Sprintf(`WITH batch AS(SELECT ctid FROM %s WHERE %s AND created_at<$2 AND archived_at IS NULL AND response IS NOT NULL ORDER BY created_at LIMIT 100 FOR UPDATE SKIP LOCKED) UPDATE %s x SET archived_at=statement_timestamp(),response='{}'::jsonb,status_code=409 FROM batch b WHERE x.ctid=b.ctid`, table, scope, table)
		} else {
			key = "temporary_records"
			query = fmt.Sprintf(`WITH batch AS(SELECT ctid FROM %s WHERE %s AND expires_at<$2 ORDER BY expires_at LIMIT 100 FOR UPDATE SKIP LOCKED) DELETE FROM %s x USING batch b WHERE x.ctid=b.ctid`, table, scope, table)
		}
		result, err := tx.Exec(ctx, query, t.BrandID, t.CutoffAt)
		if err != nil {
			return false, err
		}
		if result.RowsAffected() == 0 {
			continue
		}
		c, ok := t.Counts[key]
		if !ok {
			c = Counts{"0", "0", "0"}
		}
		n := fmt.Sprint(result.RowsAffected())
		c.Deleted = add(c.Deleted, n)
		if key == "request_receipts" {
			c.Receipts = add(c.Receipts, n)
		}
		t.Counts[key] = c
		raw, _ := json.Marshal(t.Counts)
		_, err = tx.Exec(ctx, `UPDATE data_retention_tasks SET counts=$2 WHERE id=$1`, t.ID, raw)
		return true, err
	}
	return false, nil
}
