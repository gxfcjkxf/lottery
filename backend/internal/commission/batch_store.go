package commission

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// cycleRow is worker state, never a public projection or a cached authority.
type cycleRow struct {
	ID, Brand, Anchor, State string
	From, To                 time.Time
	Calendar                 Calendar
	Version, Count           int64
	Epoch                    int64
	CursorAt                 *time.Time
	CursorID, RunID          *string
	Complete                 bool
}

func cycleLock(ctx context.Context, tx pgx.Tx, brand, id string) (cycleRow, error) {
	var c cycleRow
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT id::text,brand_id::text,anchor_order_id::text,state,window_from,window_to,calendar,version,target_count,manifest_cursor_at,manifest_cursor_id::text,current_run_id::text,scan_complete,evidence_epoch FROM commission_cycles WHERE brand_id=$1 AND id=$2 FOR UPDATE NOWAIT`, brand, id).Scan(&c.ID, &c.Brand, &c.Anchor, &c.State, &c.From, &c.To, &raw, &c.Version, &c.Count, &c.CursorAt, &c.CursorID, &c.RunID, &c.Complete, &c.Epoch)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "55P03" {
		return c, ErrBusy
	}
	if err != nil {
		return c, err
	}
	if json.Unmarshal(raw, &c.Calendar) != nil {
		return c, ErrPolicyEvidence
	}
	w, err := c.Calendar.WindowAt(c.From)
	if err != nil || !w.From.Equal(c.From) || !w.To.Equal(c.To) {
		return c, ErrPolicyEvidence
	}
	return c, nil
}

func cycleActor(a access.Account, brand, action string, meta points.Metadata) error {
	if a.Type != access.AccountAdmin || !canonicalUUID(brand) || !canonicalUUID(a.ID) || meta.ActorType != "admin" || meta.ActorID != a.ID || meta.RequestID == "" || len(meta.RequestID) > 80 {
		return ErrInvalid
	}
	if !AllowedCycle(a, brand, action) {
		return ErrDenied
	}
	return nil
}

func writableCycleBrand(ctx context.Context, tx pgx.Tx, brand string) error {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE`, brand).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status == "disabled" {
		return ErrCycleState
	}
	return nil
}

// CreateCycleTx closes admission to a historical UTC window. It uses only an
// enabled immutable bet-time policy, not the current mutable policy. Registry
// enumeration and final evidence processing are durable worker pages.
func (s Service) CreateCycleTx(ctx context.Context, tx pgx.Tx, brand string, a access.Account, in CreateCycleInput, meta points.Metadata) (Cycle, error) {
	if tx == nil || in.Validate() != nil {
		return Cycle{}, ErrInvalid
	}
	if err := cycleActor(a, brand, "run", meta); err != nil {
		return Cycle{}, err
	}
	if err := writableCycleBrand(ctx, tx, brand); err != nil {
		return Cycle{}, err
	}
	snap, err := (Source{}).PolicySnapshotTx(ctx, tx, brand, in.AnchorOrderID)
	if err != nil {
		return Cycle{}, err
	}
	if !snap.Financial.Config.Enabled || snap.Financial.Config.Calendar == nil {
		return Cycle{}, ErrCycleState
	}
	w, err := snap.Financial.Config.Calendar.WindowAt(snap.CapturedAt)
	if err != nil {
		return Cycle{}, err
	}
	// This is a short admission barrier, not a whole-cycle wallet/period lock.
	var locked string
	if err = tx.QueryRow(ctx, `SELECT brand_id::text FROM brand_commission_policies WHERE brand_id=$1 FOR UPDATE`, brand).Scan(&locked); err != nil {
		return Cycle{}, err
	}
	var closed, exists bool
	err = tx.QueryRow(ctx, `SELECT clock_timestamp()>=$2::timestamptz,EXISTS(SELECT 1 FROM commission_cycles WHERE brand_id=$1 AND window_from=$3 AND window_to=$2)`, brand, w.To, w.From).Scan(&closed, &exists)
	if err != nil {
		return Cycle{}, err
	}
	if !closed || exists {
		return Cycle{}, ErrCycleState
	}
	id := ids.New()
	log, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: meta.ActorType, ActorID: meta.ActorID, Action: "commission.cycle.create", ResourceType: "commission_cycle", ResourceID: id, Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, After: map[string]any{"version": 1, "state": "enumerating", "anchor_order_id": in.AnchorOrderID, "window_from": w.From, "window_to": w.To, "calendar": snap.Financial.Config.Calendar}})
	if err != nil {
		return Cycle{}, err
	}
	raw, _ := json.Marshal(snap.Financial.Config.Calendar)
	_, err = tx.Exec(ctx, `INSERT INTO commission_cycles(id,brand_id,window_from,window_to,anchor_order_id,calendar,created_by,reason,creation_audit_log_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, brand, w.From, w.To, in.AnchorOrderID, raw, a.ID, in.Reason, log)
	if err != nil {
		return Cycle{}, err
	}
	return s.CycleTx(ctx, tx, brand, id)
}

func appendCycleStep(ctx context.Context, tx pgx.Tx, c cycleRow, state, operation, reason string, run *string, meta points.Metadata) (string, error) {
	if c.Version >= maxCycleVersion {
		return "", ErrCycleVersion
	}
	log, err := audit.Append(ctx, tx, audit.Record{BrandID: c.Brand, ActorType: meta.ActorType, ActorID: meta.ActorID, Action: "commission.cycle." + operation, ResourceType: "commission_cycle", ResourceID: c.ID, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, Before: map[string]any{"version": c.Version, "state": c.State}, After: map[string]any{"version": c.Version + 1, "state": state, "run_id": run}})
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `INSERT INTO commission_cycle_steps(id,brand_id,cycle_id,version,from_state,to_state,operation,run_id,reason,actor_type,actor_id,audit_log_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,'')::uuid,$12)`, ids.New(), c.Brand, c.ID, c.Version+1, c.State, state, operation, run, reason, meta.ActorType, meta.ActorID, log)
	return log, err
}

func saveCycle(ctx context.Context, tx pgx.Tx, c cycleRow, state string, run *string, errCode *string) error {
	_, err := tx.Exec(ctx, `UPDATE commission_cycles SET state=$3,version=version+1,target_count=$4,manifest_cursor_at=$5,manifest_cursor_id=$6,scan_complete=$7,current_run_id=$8,last_error_code=$9,next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, c.Brand, c.ID, state, c.Count, c.CursorAt, c.CursorID, c.Complete, run, errCode)
	return err
}

func (s Service) RetryCycleTx(ctx context.Context, tx pgx.Tx, brand, id string, a access.Account, in RetryCycleInput, meta points.Metadata) (Cycle, error) {
	if tx == nil || !canonicalUUID(id) || in.Validate() != nil {
		return Cycle{}, ErrInvalid
	}
	if err := cycleActor(a, brand, "retry", meta); err != nil {
		return Cycle{}, err
	}
	if err := writableCycleBrand(ctx, tx, brand); err != nil {
		return Cycle{}, err
	}
	c, err := cycleLock(ctx, tx, brand, id)
	if err != nil {
		return Cycle{}, err
	}
	if c.Version != in.Version {
		return Cycle{}, ErrCycleVersion
	}
	if c.State != "failed" {
		return Cycle{}, ErrCycleState
	}
	state := "enumerating"
	if c.Complete {
		state = "waiting"
	}
	if c.RunID != nil {
		if _, err = tx.Exec(ctx, `UPDATE commission_runs SET state='abandoned' WHERE id=$1 AND state<>'abandoned'`, c.RunID); err != nil {
			return Cycle{}, err
		}
	}
	if _, err = appendCycleStep(ctx, tx, c, state, "retry", in.Reason, c.RunID, meta); err != nil {
		return Cycle{}, err
	}
	if err = saveCycle(ctx, tx, c, state, nil, nil); err != nil {
		return Cycle{}, err
	}
	return s.CycleTx(ctx, tx, brand, id)
}

func pendingCycle(ctx context.Context, tx pgx.Tx, c cycleRow) (bool, error) {
	var pending bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM commission_cycle_targets t JOIN bet_orders o ON o.brand_id=t.brand_id AND o.id=t.order_id JOIN periods p ON p.brand_id=o.brand_id AND p.id=o.period_id
 WHERE t.cycle_id=$1 AND (o.status='placed' OR (o.status IN('won','lost') AND (p.status<>'settled' OR EXISTS(SELECT 1 FROM draw_corrections dc WHERE dc.brand_id=o.brand_id AND dc.period_id=p.id AND dc.state<>'completed')))))`, c.ID).Scan(&pending)
	return pending, err
}

func abandonCycleRun(ctx context.Context, tx pgx.Tx, c cycleRow, meta points.Metadata) error {
	if c.RunID != nil {
		if _, err := tx.Exec(ctx, `UPDATE commission_runs SET state='abandoned' WHERE id=$1 AND state<>'abandoned'`, c.RunID); err != nil {
			return err
		}
	}
	if _, err := appendCycleStep(ctx, tx, c, "waiting", "invalidate", "final settlement evidence changed; preserve previous run", c.RunID, meta); err != nil {
		return err
	}
	return saveCycle(ctx, tx, c, "waiting", nil, nil)
}
