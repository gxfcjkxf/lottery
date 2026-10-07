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

type discoveryRow struct {
	ID, Brand, State string
	Version          int64
	From, To         *time.Time
}

func discoveryLock(ctx context.Context, tx pgx.Tx, brand, id string) (discoveryRow, error) {
	var d discoveryRow
	err := tx.QueryRow(ctx, `SELECT id::text,brand_id::text,state,version,window_from,window_to FROM commission_discovery WHERE brand_id=$1 AND id=$2 FOR UPDATE NOWAIT`, brand, id).Scan(&d.ID, &d.Brand, &d.State, &d.Version, &d.From, &d.To)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, discoveryBusy(err)
}

func discoveryBusy(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "55P03" {
		return ErrBusy
	}
	return err
}

func discoveryTransition(ctx context.Context, tx pgx.Tx, d discoveryRow, state, operation, reason string, cycle, code *string, meta points.Metadata) error {
	if d.Version >= maxCycleVersion {
		return ErrCycleVersion
	}
	log, err := audit.Append(ctx, tx, audit.Record{BrandID: d.Brand, ActorType: meta.ActorType, ActorID: meta.ActorID, Action: "commission.discovery." + operation, ResourceType: "commission_discovery", ResourceID: d.ID, Reason: reason, RequestID: meta.RequestID, IP: meta.IP,
		Before: map[string]any{"version": d.Version, "state": d.State}, After: map[string]any{"version": d.Version + 1, "state": state, "cycle_id": cycle, "last_error_code": code}})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE commission_discovery SET state=$3,version=version+1,cycle_id=$4,last_error_code=$5,last_audit_log_id=$6,next_check_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, d.Brand, d.ID, state, cycle, code, log)
	return err
}

// ProcessDiscovery examines bounded independent queue records. A pending bet
// is scheduled at its own saved boundary, never today's mutable calendar.
// Registration does not approve or pay commission, or retry failed cycles.
func (s Service) ProcessDiscovery(ctx context.Context, maxSteps int) (int, error) {
	if s.DB == nil || maxSteps < 1 || maxSteps > 100 {
		return 0, ErrInvalid
	}
	committed := 0
	for i := 0; i < maxSteps; i++ {
		tx, err := s.DB.Begin(ctx)
		if err != nil {
			return committed, err
		}
		var brand, id string
		err = tx.QueryRow(ctx, `SELECT brand_id::text,id::text FROM commission_discovery WHERE state='pending' AND next_check_at<=clock_timestamp() ORDER BY next_check_at,id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&brand, &id)
		if errors.Is(err, pgx.ErrNoRows) {
			_ = tx.Rollback(ctx)
			break
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return committed, err
		}
		d, err := discoveryLock(ctx, tx, brand, id)
		if err != nil {
			_ = tx.Rollback(ctx)
			return committed, err
		}
		err = s.discover(ctx, tx, d)
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		if err != nil {
			if errors.Is(discoveryBusy(err), ErrBusy) {
				_, e := s.DB.Exec(ctx, `UPDATE commission_discovery SET next_check_at=clock_timestamp()+interval '1 second' WHERE id=$1 AND state='pending' AND version=$2`, id, d.Version)
				if e != nil {
					return committed, e
				}
				continue
			}
			if ctx.Err() != nil {
				return committed, ctx.Err()
			}
			code := "COMMISSION_DISCOVERY_FAILED"
			if errors.Is(err, ErrPolicyEvidence) || errors.Is(err, ErrInvalid) || errors.Is(err, ErrCalendarBoundary) {
				code = "COMMISSION_DISCOVERY_EVIDENCE_INVALID"
			}
			if e := s.failDiscovery(ctx, d, code); e != nil {
				return committed, e
			}
		}
		committed++
	}
	return committed, nil
}

func (s Service) discover(ctx context.Context, tx pgx.Tx, d discoveryRow) error {
	snap, err := (Source{}).PolicySnapshotTx(ctx, tx, d.Brand, d.ID)
	if err != nil {
		return err
	}
	if !snap.Financial.Config.Enabled || snap.Financial.Config.Calendar == nil {
		return ErrPolicyEvidence
	}
	w, err := snap.Financial.Config.Calendar.WindowAt(snap.CapturedAt)
	if err != nil {
		return err
	}
	if d.From != nil && (!d.From.Equal(w.From) || !d.To.Equal(w.To)) {
		return ErrPolicyEvidence
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	if d.From == nil {
		if _, err = tx.Exec(ctx, `UPDATE commission_discovery SET window_from=$3,window_to=$4 WHERE brand_id=$1 AND id=$2`, d.Brand, d.ID, w.From, w.To); err != nil {
			return err
		}
	}
	if now.Before(w.To) {
		_, err = tx.Exec(ctx, `UPDATE commission_discovery SET next_check_at=$3 WHERE brand_id=$1 AND id=$2`, d.Brand, d.ID, w.To)
		return err
	}
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE NOWAIT`, d.Brand).Scan(&status); err != nil {
		return discoveryBusy(err)
	}
	if status == "disabled" {
		_, err = tx.Exec(ctx, `UPDATE commission_discovery SET next_check_at=clock_timestamp()+interval '1 minute' WHERE brand_id=$1 AND id=$2`, d.Brand, d.ID)
		return err
	}
	var cycle string
	err = tx.QueryRow(ctx, `SELECT id::text FROM commission_cycles WHERE brand_id=$1 AND window_from=$2 AND window_to=$3`, d.Brand, w.From, w.To).Scan(&cycle)
	if errors.Is(err, pgx.ErrNoRows) {
		// Existing immutable cycles need no admission mutex. Only new windows
		// contend with bets/policy writers; recheck after taking the barrier.
		var locked string
		if err = tx.QueryRow(ctx, `SELECT brand_id::text FROM brand_commission_policies WHERE brand_id=$1 FOR UPDATE NOWAIT`, d.Brand).Scan(&locked); err != nil {
			return discoveryBusy(err)
		}
		err = tx.QueryRow(ctx, `SELECT id::text FROM commission_cycles WHERE brand_id=$1 AND window_from=$2 AND window_to=$3`, d.Brand, w.From, w.To).Scan(&cycle)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		cycle = ids.New()
		meta := points.Metadata{ActorType: "system", RequestID: ids.New()}
		reason := "automatically register closed saved commission window"
		log, e := audit.Append(ctx, tx, audit.Record{BrandID: d.Brand, ActorType: meta.ActorType, Action: "commission.cycle.create", ResourceType: "commission_cycle", ResourceID: cycle, Reason: reason, RequestID: meta.RequestID,
			After: map[string]any{"version": 1, "state": "enumerating", "anchor_order_id": d.ID, "window_from": w.From, "window_to": w.To, "calendar": snap.Financial.Config.Calendar}})
		if e != nil {
			return e
		}
		raw, _ := json.Marshal(snap.Financial.Config.Calendar)
		_, err = tx.Exec(ctx, `INSERT INTO commission_cycles(id,brand_id,window_from,window_to,anchor_order_id,calendar,creation_actor_type,created_by,reason,creation_audit_log_id) VALUES($1,$2,$3,$4,$5,$6,'system',NULL,$7,$8)`, cycle, d.Brand, w.From, w.To, d.ID, raw, reason, log)
	}
	if err != nil {
		return err
	}
	return discoveryTransition(ctx, tx, d, "registered", "register", "saved window registered; no commission payment", &cycle, nil, points.Metadata{ActorType: "system", RequestID: ids.New()})
}

func (s Service) failDiscovery(ctx context.Context, observed discoveryRow, code string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	d, err := discoveryLock(ctx, tx, observed.Brand, observed.ID)
	if err != nil {
		return err
	}
	if d.Version != observed.Version || d.State != "pending" {
		return nil
	}
	if err = discoveryTransition(ctx, tx, d, "failed", "failure", code, nil, &code, points.Metadata{ActorType: "system", RequestID: ids.New()}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s Service) RetryDiscoveryTx(ctx context.Context, tx pgx.Tx, brand, id string, a access.Account, in RetryCycleInput, meta points.Metadata) (Discovery, error) {
	if tx == nil || !canonicalUUID(id) || in.Validate() != nil {
		return Discovery{}, ErrInvalid
	}
	if err := cycleActor(a, brand, "retry", meta); err != nil {
		return Discovery{}, err
	}
	if err := writableCycleBrand(ctx, tx, brand); err != nil {
		return Discovery{}, err
	}
	d, err := discoveryLock(ctx, tx, brand, id)
	if err != nil {
		return Discovery{}, err
	}
	if d.Version != in.Version {
		return Discovery{}, ErrCycleVersion
	}
	if d.State != "failed" {
		return Discovery{}, ErrCycleState
	}
	if err = discoveryTransition(ctx, tx, d, "pending", "retry", in.Reason, nil, nil, meta); err != nil {
		return Discovery{}, err
	}
	return s.DiscoveryTx(ctx, tx, brand, id)
}
