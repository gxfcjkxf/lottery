package reportarchive

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"time"
)

const automaticTaskJSON = `jsonb_build_object('id',j.id,'brand_id',j.brand_id,'policy_version',j.policy_version,'window',jsonb_build_object('kind',j.kind,'period_key',j.period_key,'timezone',j.timezone,'from',j.from_at,'to',j.to_at),'state',j.state,'version',j.version,'attempt_count',j.attempt_count,'archive_id',j.archive_id,'last_error_code',j.last_error_code,'creation_audit_log_id',j.creation_audit_log_id,'last_audit_log_id',j.last_audit_log_id,'created_at',j.created_at,'updated_at',j.updated_at)`

func automaticBusy(err error) bool {
	var p *pgconn.PgError
	return errors.Is(err, ErrBusy) || errors.As(err, &p) && (p.Code == "55P03" || p.Code == "40P01" || p.Code == "40001")
}

func (s Service) AutomaticTaskTx(ctx context.Context, tx pgx.Tx, brand, id string) (AutomaticTask, error) {
	var out AutomaticTask
	if tx == nil || !archiveUUID.MatchString(brand) || !archiveUUID.MatchString(id) {
		return out, ErrInvalid
	}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT `+automaticTaskJSON+` FROM report_archive_automatic_tasks j WHERE brand_id=$1 AND id=$2`, brand, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(raw, &out)
	return out, err
}
func (s Service) AutomaticTasksTx(ctx context.Context, tx pgx.Tx, brand string, limit, offset int) (AutomaticTaskPage, error) {
	out := AutomaticTaskPage{BrandID: brand, Items: []AutomaticTask{}, Limit: limit, Offset: offset}
	if tx == nil || !archiveUUID.MatchString(brand) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return out, ErrInvalid
	}
	var raw []byte
	err := tx.QueryRow(ctx, `WITH page AS(SELECT * FROM report_archive_automatic_tasks WHERE brand_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3) SELECT (SELECT count(*)::text FROM report_archive_automatic_tasks WHERE brand_id=$1),coalesce((SELECT jsonb_agg(`+automaticTaskJSON+` ORDER BY j.created_at DESC,j.id DESC) FROM page j),'[]'::jsonb)`, brand, limit, offset).Scan(&out.TotalCount, &raw)
	if err == nil {
		err = json.Unmarshal(raw, &out.Items)
	}
	return out, err
}
func lockAutomaticGate(ctx context.Context, tx pgx.Tx, brand, kind string) (bool, error) {
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE NOWAIT`, brand).Scan(&status); err != nil {
		return false, err
	}
	var enabled bool
	err := tx.QueryRow(ctx, `SELECT CASE WHEN $2='daily' THEN daily_enabled ELSE monthly_enabled END FROM brand_report_archive_policies WHERE brand_id=$1 FOR SHARE NOWAIT`, brand, kind).Scan(&enabled)
	return enabled && status != "disabled", err
}

// DiscoverAutomatic commits bounded independent calendar batches. A cursor
// and its genuine pending tasks advance together; neither modifies points.
func (s Service) DiscoverAutomatic(ctx context.Context, maxTasks int) (int, error) {
	if s.DB == nil || maxTasks < 1 || maxTasks > 100 {
		return 0, ErrInvalid
	}
	created := 0
	for rounds := 0; rounds < maxTasks; rounds++ {
		tx, err := s.DB.Begin(ctx)
		if err != nil {
			return created, err
		}
		var brand, kind, key, zone string
		var pv int64
		var asOf time.Time
		err = tx.QueryRow(ctx, `SELECT c.brand_id::text,c.policy_version,c.kind,c.next_period_key,p.timezone,statement_timestamp() FROM report_archive_automatic_cursors c JOIN brand_report_archive_policies p ON p.brand_id=c.brand_id AND p.version=c.policy_version JOIN brands b ON b.id=c.brand_id WHERE c.next_period_key IS NOT NULL AND c.due_at<=statement_timestamp() AND b.status<>'disabled' AND ((c.kind='daily' AND p.daily_enabled) OR(c.kind='monthly' AND p.monthly_enabled)) ORDER BY c.due_at,c.brand_id,c.kind LIMIT 1 FOR UPDATE OF c SKIP LOCKED`).Scan(&brand, &pv, &kind, &key, &zone, &asOf)
		if errors.Is(err, pgx.ErrNoRows) {
			tx.Rollback(ctx)
			return created, nil
		}
		if err == nil {
			var allowed bool
			allowed, err = lockAutomaticGate(ctx, tx, brand, kind)
			if err == nil && !allowed {
				err = ErrBusy
			}
		}
		var windows []Window
		var next string
		if err == nil {
			windows, next, err = DueArchiveWindows(kind, key, zone, asOf, maxTasks-created)
		}
		batch := 0
		if err == nil {
			for _, window := range windows {
				var exists bool
				err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM report_archive_automatic_tasks WHERE brand_id=$1 AND kind=$2 AND period_key=$3)`, brand, kind, window.PeriodKey).Scan(&exists)
				if err != nil {
					break
				}
				if exists {
					continue
				}
				id := ids.New()
				var aid string
				aid, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "system", Action: "report_archive.task.enqueue", ResourceType: "report_archive_task", ResourceID: id, Reason: "discover elapsed report period", RequestID: ids.New(), After: map[string]any{"id": id, "kind": kind, "period_key": window.PeriodKey, "timezone": zone, "policy_version": pv}})
				if err != nil {
					break
				}
				_, err = tx.Exec(ctx, `INSERT INTO report_archive_automatic_tasks(id,brand_id,policy_version,kind,period_key,timezone,from_at,to_at,creation_audit_log_id,last_audit_log_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$9)`, id, brand, pv, kind, window.PeriodKey, zone, window.From, window.To, aid)
				if err != nil {
					break
				}
				batch++
			}
		}
		if err == nil && next != key {
			var nextValue *string
			if next != "" {
				nextValue = &next
			}
			_, err = tx.Exec(ctx, `UPDATE report_archive_automatic_cursors SET next_period_key=$4 WHERE brand_id=$1 AND policy_version=$2 AND kind=$3`, brand, pv, kind, nextValue)
		}
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			tx.Rollback(ctx)
		}
		if err != nil {
			if automaticBusy(err) {
				continue
			}
			return created, err
		}
		created += batch
		if created >= maxTasks {
			return created, nil
		}
	}
	return created, nil
}
func taskTransition(t AutomaticTask, state string, version, attempts int64, archiveID, errorCode *string) map[string]any {
	return map[string]any{"state": state, "version": version, "attempt_count": attempts, "archive_id": archiveID, "last_error_code": errorCode}
}
func finishAutomatic(ctx context.Context, tx pgx.Tx, t AutomaticTask, state string, archiveID, errorCode *string, actorType, actorID, reason, requestID, ip string) error {
	action := "report_archive.task.finish"
	attempts := t.AttemptCount + 1
	if state == "pending" {
		action = "report_archive.task.retry"
		attempts = t.AttemptCount
	}
	if t.Version >= maxRevision || attempts > maxRevision {
		return ErrVersion
	}
	aid, err := audit.Append(ctx, tx, audit.Record{BrandID: t.BrandID, ActorType: actorType, ActorID: actorID, Action: action, ResourceType: "report_archive_task", ResourceID: t.ID, Reason: reason, RequestID: requestID, IP: ip, Before: taskTransition(t, t.State, t.Version, t.AttemptCount, t.ArchiveID, t.LastErrorCode), After: taskTransition(t, state, t.Version+1, attempts, archiveID, errorCode)})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE report_archive_automatic_tasks SET state=$3,version=version+1,attempt_count=$4,archive_id=$5,last_error_code=$6,last_audit_log_id=$7,updated_at=statement_timestamp() WHERE brand_id=$1 AND id=$2`, t.BrandID, t.ID, state, attempts, archiveID, errorCode, aid)
	return err
}
func (s Service) processAutomaticTx(ctx context.Context, tx pgx.Tx, t AutomaticTask) error {
	allowed, err := lockAutomaticGate(ctx, tx, t.BrandID, t.Window.Kind)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrBusy
	}
	var locked bool
	err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended(current_schema()||':report-archive:'||$1||':'||$2||':'||$3,0))`, t.BrandID, t.Window.Kind, t.Window.PeriodKey).Scan(&locked)
	if err != nil {
		return err
	}
	if !locked {
		return ErrBusy
	}
	var existing string
	var intact bool
	err = tx.QueryRow(ctx, `SELECT id::text,payload_sha256=encode(sha256(convert_to(payload::text,'UTF8')),'hex') FROM report_archives WHERE brand_id=$1 AND kind=$2 AND period_key=$3 ORDER BY revision DESC LIMIT 1`, t.BrandID, t.Window.Kind, t.Window.PeriodKey).Scan(&existing, &intact)
	if err == nil {
		if !intact {
			return ErrIntegrity
		}
		return finishAutomatic(ctx, tx, t, "skipped", &existing, nil, "system", "", "period already has an immutable archive", ids.New(), "")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	id, requestID := ids.New(), ids.New()
	reason := "automatic report archive capture"
	aid, err := audit.Append(ctx, tx, audit.Record{BrandID: t.BrandID, ActorType: "system", Action: "report_archive.create.automatic", ResourceType: "report_archive", ResourceID: id, Reason: reason, RequestID: requestID, After: map[string]any{"id": id, "task_id": t.ID, "policy_version": t.PolicyVersion}})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `WITH capture AS MATERIALIZED(SELECT report_archive_capture($2,$6,$7,$5) AS payload) INSERT INTO report_archives(id,brand_id,kind,period_key,timezone,from_at,to_at,revision,snapshot_at,created_by,reason,request_id,payload,payload_sha256,audit_log_id,created_at,automatic_task_id,automatic_policy_version) SELECT $1,$2,$3,$4,$5,$6,$7,1,(payload->>'snapshot_at')::timestamptz,NULL,$8,$9,payload,encode(sha256(convert_to(payload::text,'UTF8')),'hex'),$10,(payload->>'snapshot_at')::timestamptz,$11,$12 FROM capture`, id, t.BrandID, t.Window.Kind, t.Window.PeriodKey, t.Window.Timezone, t.Window.From, t.Window.To, reason, requestID, aid, t.ID, t.PolicyVersion)
	if err != nil {
		return err
	}
	return finishAutomatic(ctx, tx, t, "completed", &id, nil, "system", "", "immutable automatic archive captured", requestID, "")
}
func (s Service) failAutomatic(ctx context.Context, observed AutomaticTask) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT `+automaticTaskJSON+` FROM report_archive_automatic_tasks j WHERE brand_id=$1 AND id=$2 FOR UPDATE NOWAIT`, observed.BrandID, observed.ID).Scan(&raw)
	if err != nil {
		return err
	}
	var current AutomaticTask
	if err = json.Unmarshal(raw, &current); err != nil {
		return err
	}
	if current.State != "pending" || current.Version != observed.Version {
		return nil
	}
	code := "ARCHIVE_FAILED"
	err = finishAutomatic(ctx, tx, current, "failed", nil, &code, "system", "", "automatic archive attempt failed", ids.New(), "")
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s Service) ProcessAutomatic(ctx context.Context, maxSteps int) (int, error) {
	if s.DB == nil || maxSteps < 1 || maxSteps > 100 {
		return 0, ErrInvalid
	}
	completed := 0
	for n := 0; n < maxSteps; n++ {
		tx, err := s.DB.Begin(ctx)
		if err != nil {
			return completed, err
		}
		var raw []byte
		err = tx.QueryRow(ctx, `SELECT `+automaticTaskJSON+` FROM report_archive_automatic_tasks j JOIN brand_report_archive_policies p ON p.brand_id=j.brand_id JOIN brands b ON b.id=j.brand_id WHERE j.state='pending' AND b.status<>'disabled' AND ((j.kind='daily' AND p.daily_enabled)OR(j.kind='monthly' AND p.monthly_enabled)) ORDER BY j.created_at,j.id LIMIT 1 FOR UPDATE OF j SKIP LOCKED`).Scan(&raw)
		if errors.Is(err, pgx.ErrNoRows) {
			tx.Rollback(ctx)
			return completed, nil
		}
		if err != nil {
			tx.Rollback(ctx)
			return completed, err
		}
		var task AutomaticTask
		err = json.Unmarshal(raw, &task)
		if err == nil {
			err = s.processAutomaticTx(ctx, tx, task)
		}
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			tx.Rollback(ctx)
		}
		if err != nil {
			if automaticBusy(err) {
				continue
			}
			if e := s.failAutomatic(ctx, task); e != nil {
				return completed, e
			}
			continue
		}
		completed++
	}
	return completed, nil
}
func (s Service) RetryAutomaticTaskTx(ctx context.Context, tx pgx.Tx, brand, id string, a access.Account, version int64, reason string, meta points.Metadata) (AutomaticTask, error) {
	var out AutomaticTask
	if tx == nil || !archiveUUID.MatchString(brand) || !archiveUUID.MatchString(id) || !archiveUUID.MatchString(a.ID) || version < 1 || version >= maxRevision || !validReason(reason) || meta.ActorType != "admin" || meta.ActorID != a.ID || meta.RequestID == "" || len(meta.RequestID) > 80 {
		return out, ErrInvalid
	}
	if !AllowedAutomatic(a, brand, "retry") {
		return out, ErrDenied
	}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT `+automaticTaskJSON+` FROM report_archive_automatic_tasks j WHERE brand_id=$1 AND id=$2 FOR UPDATE NOWAIT`, brand, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return out, err
	}
	if out.Version != version {
		return out, ErrVersion
	}
	if out.State != "failed" {
		return out, ErrState
	}
	allowed, err := lockAutomaticGate(ctx, tx, brand, out.Window.Kind)
	if err != nil {
		return out, err
	}
	if !allowed {
		return out, ErrState
	}
	err = finishAutomatic(ctx, tx, out, "pending", nil, nil, "admin", a.ID, reason, meta.RequestID, meta.IP)
	if err != nil {
		return out, err
	}
	return s.AutomaticTaskTx(ctx, tx, brand, id)
}
