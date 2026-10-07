package commission

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

const cyclePageSize = 100

type runRow struct {
	ID                       string
	Epoch                    int64
	CursorAt                 *time.Time
	CursorOrder, AgentCursor *string
	State                    string
}

func readRun(ctx context.Context, tx pgx.Tx, id string) (runRow, error) {
	var r runRow
	err := tx.QueryRow(ctx, `SELECT id::text,evidence_epoch,cursor_at,cursor_order_id::text,earnings_cursor_agent::text,state FROM commission_runs WHERE id=$1`, id).Scan(&r.ID, &r.Epoch, &r.CursorAt, &r.CursorOrder, &r.AgentCursor, &r.State)
	return r, err
}

// ProcessCycles advances bounded, independently committed pages. Contention is
// retryable; evidence errors stop a cycle for audited manual retry. No wallets
// or point-ledger entries are modified by this calculation worker.
func (s Service) ProcessCycles(ctx context.Context, maxSteps int) (int, error) {
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
		err = tx.QueryRow(ctx, `SELECT c.brand_id::text,c.id::text FROM commission_cycles c LEFT JOIN commission_runs r ON r.id=c.current_run_id
 WHERE (c.state IN('enumerating','waiting','calculating','summarizing') AND c.next_work_at<=clock_timestamp()) OR (c.state='ready' AND r.evidence_epoch<>c.evidence_epoch)
 ORDER BY c.next_work_at,c.id FOR UPDATE OF c SKIP LOCKED LIMIT 1`).Scan(&brand, &id)
		if errors.Is(err, pgx.ErrNoRows) {
			_ = tx.Rollback(ctx)
			break
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return committed, err
		}
		c, err := cycleLock(ctx, tx, brand, id)
		if err != nil {
			_ = tx.Rollback(ctx)
			return committed, err
		}
		meta := points.Metadata{ActorType: "system", RequestID: ids.New()}
		err = s.cycleStep(ctx, tx, c, meta)
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		if err != nil {
			if errors.Is(err, ErrBusy) {
				if _, e := s.DB.Exec(ctx, `UPDATE commission_cycles SET next_work_at=clock_timestamp()+interval '1 second' WHERE id=$1 AND version=$2`, id, c.Version); e != nil {
					return committed, e
				}
				continue
			}
			code := "COMMISSION_CALCULATION_FAILED"
			if errors.Is(err, ErrPolicyEvidence) || errors.Is(err, ErrEvidence) || errors.Is(err, ErrInvalid) {
				code = "COMMISSION_EVIDENCE_INVALID"
			}
			if errors.Is(err, points.ErrOverflow) {
				code = "COMMISSION_AMOUNT_OVERFLOW"
			}
			if e := s.failCycle(ctx, c, code); e != nil {
				return committed, e
			}
			committed++
			continue
		}
		committed++
	}
	return committed, nil
}

func (s Service) failCycle(ctx context.Context, observed cycleRow, code string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	c, err := cycleLock(ctx, tx, observed.Brand, observed.ID)
	if err != nil {
		return err
	}
	if c.Version != observed.Version {
		return nil
	}
	if _, err = appendCycleStep(ctx, tx, c, "failed", "failure", code, c.RunID, points.Metadata{ActorType: "system", RequestID: ids.New()}); err != nil {
		return err
	}
	if err = saveCycle(ctx, tx, c, "failed", c.RunID, &code); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s Service) cycleStep(ctx context.Context, tx pgx.Tx, c cycleRow, meta points.Metadata) error {
	if c.State == "enumerating" {
		return enumerateCycle(ctx, tx, c, meta)
	}
	epoch := c.Epoch
	var err error
	if c.State == "waiting" {
		pending, err := pendingCycle(ctx, tx, c)
		if err != nil {
			return err
		}
		if pending {
			_, err = tx.Exec(ctx, `UPDATE commission_cycles SET next_work_at=clock_timestamp()+interval '5 seconds' WHERE id=$1`, c.ID)
			return err
		}
		id := ids.New()
		_, err = tx.Exec(ctx, `INSERT INTO commission_runs(id,brand_id,cycle_id,generation,evidence_epoch,state) SELECT $1,$2,$3,coalesce(max(generation),0)+1,$4,'calculating' FROM commission_runs WHERE cycle_id=$3`, id, c.Brand, c.ID, epoch)
		if err != nil {
			return err
		}
		if _, err = appendCycleStep(ctx, tx, c, "calculating", "start_run", "all cycle orders reached final states", &id, meta); err != nil {
			return err
		}
		return saveCycle(ctx, tx, c, "calculating", &id, nil)
	}
	if c.RunID == nil {
		return ErrEvidence
	}
	r, err := readRun(ctx, tx, *c.RunID)
	if err != nil {
		return err
	}
	if r.Epoch != epoch {
		return abandonCycleRun(ctx, tx, c, meta)
	}
	switch c.State {
	case "calculating":
		return calculateCyclePage(ctx, tx, c, r, meta)
	case "summarizing":
		return summarizeCyclePage(ctx, tx, c, r, meta)
	default:
		return ErrCycleState
	}
}

func enumerateCycle(ctx context.Context, tx pgx.Tx, c cycleRow, meta points.Metadata) error {
	rows, err := tx.Query(ctx, `SELECT id::text,placed_at FROM bet_orders WHERE brand_id=$1 AND placed_at>=$2 AND placed_at<$3 AND ($4::timestamptz IS NULL OR (placed_at,id)>($4,$5::uuid)) ORDER BY placed_at,id LIMIT $6`, c.Brand, c.From, c.To, c.CursorAt, c.CursorID, cyclePageSize+1)
	if err != nil {
		return err
	}
	var keys []CycleCursor
	for rows.Next() {
		var key CycleCursor
		if err = rows.Scan(&key.OrderID, &key.PlacedAt); err != nil {
			rows.Close()
			return err
		}
		keys = append(keys, key)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	more := len(keys) > cyclePageSize
	if more {
		keys = keys[:cyclePageSize]
	}
	for _, key := range keys {
		if _, err = tx.Exec(ctx, `INSERT INTO commission_cycle_targets(brand_id,cycle_id,order_id,placed_at) VALUES($1,$2,$3,$4)`, c.Brand, c.ID, key.OrderID, key.PlacedAt); err != nil {
			return err
		}
	}
	if len(keys) > 0 {
		last := keys[len(keys)-1]
		c.CursorAt = &last.PlacedAt
		c.CursorID = &last.OrderID
	}
	c.Count += int64(len(keys))
	c.Complete = !more
	state := "enumerating"
	if c.Complete {
		state = "waiting"
	}
	if _, err = appendCycleStep(ctx, tx, c, state, "manifest_page", "capture immutable closed-window order manifest", nil, meta); err != nil {
		return err
	}
	return saveCycle(ctx, tx, c, state, nil, nil)
}

type calculationRow struct {
	ID, Order, Member, Account, Status, Reason string
	Stake, Prize, Base                         points.Amount
	Job, Calculation                           *string
	Generation                                 *int64
	Snapshot                                   []byte
	Allocations                                []OrderAllocation
	Placed                                     time.Time
}

func calculateCyclePage(ctx context.Context, tx pgx.Tx, c cycleRow, r runRow, meta points.Metadata) error {
	rows, err := tx.Query(ctx, `SELECT t.order_id::text,t.placed_at,o.brand_member_id::text,o.account_id::text,o.status,o.total_points,o.prize_points,o.commission_rule_snapshot FROM commission_cycle_targets t JOIN bet_orders o ON o.brand_id=t.brand_id AND o.id=t.order_id WHERE t.cycle_id=$1 AND ($2::timestamptz IS NULL OR (t.placed_at,t.order_id)>($2,$3::uuid)) ORDER BY t.placed_at,t.order_id LIMIT $4`, c.ID, r.CursorAt, r.CursorOrder, cyclePageSize+1)
	if err != nil {
		return err
	}
	var items []calculationRow
	for rows.Next() {
		var item calculationRow
		if err = rows.Scan(&item.Order, &item.Placed, &item.Member, &item.Account, &item.Status, &item.Stake, &item.Prize, &item.Snapshot); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	more := len(items) > cyclePageSize
	if more {
		items = items[:cyclePageSize]
	}
	for i := range items {
		item := &items[i]
		res, e := (Source{}).FinalOrderTx(ctx, tx, c.Brand, item.Order)
		if e != nil {
			return e
		}
		if res.Reason == "not_final" || res.Reason == "correction_open" {
			return abandonCycleRun(ctx, tx, c, meta)
		}
		snap, e := (Source{}).PolicySnapshotTx(ctx, tx, c.Brand, item.Order)
		if e != nil {
			return e
		}
		// Preserve the physical JSONB evidence, including its timestamp spelling.
		// A decoded time.Time round trip would rewrite +00:00 to Z and must not
		// replace the immutable bet snapshot used by the SQL witness guard.
		item.ID = ids.New()
		item.Reason = res.Reason
		if res.Fact != nil {
			fact := res.Fact
			item.Job = &fact.JobID
			item.Calculation = &fact.CalculationID
			item.Generation = &fact.Generation
			if fact.MemberID != item.Member || fact.AccountID != item.Account || fact.Status != item.Status || fact.PrizePoints != item.Prize || fact.Basis.TurnoverPoints != item.Stake {
				return ErrEvidence
			}
			item.Reason = "policy_disabled"
			if snap.Financial.Config.Enabled {
				w, e := snap.Financial.Config.Calendar.WindowAt(item.Placed)
				if e != nil {
					return e
				}
				if !w.From.Equal(c.From) || !w.To.Equal(c.To) {
					item.Reason = "other_cycle"
				} else if len(snap.Path) == 0 {
					item.Reason = "unattributed"
				} else {
					item.Reason = "eligible"
					item.Base = fact.Basis.LossPoints
					if snap.Path[0].EffectiveMode == "turnover" {
						item.Base = fact.Basis.TurnoverPoints
					}
					item.Allocations, e = EvaluateOrder(snap, *fact)
					if e != nil {
						return e
					}
				}
			}
		}
	}
	state := "calculating"
	if !more {
		state = "summarizing"
	}
	log, err := appendCycleStep(ctx, tx, c, state, "calculation_page", "record exact final-order differential evidence", c.RunID, meta)
	if err != nil {
		return err
	}
	for _, item := range items {
		_, err = tx.Exec(ctx, `INSERT INTO commission_calculations(id,brand_id,cycle_id,run_id,order_id,reason,status,member_id,account_id,stake_points,prize_points,base_points,job_id,calculation_id,generation,rule_snapshot,audit_log_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, item.ID, c.Brand, c.ID, r.ID, item.Order, item.Reason, item.Status, item.Member, item.Account, item.Stake, item.Prize, item.Base, item.Job, item.Calculation, item.Generation, item.Snapshot, log)
		if err != nil {
			return err
		}
		for _, allocation := range item.Allocations {
			_, err = tx.Exec(ctx, `INSERT INTO commission_allocations(brand_id,cycle_id,run_id,calculation_id,order_id,agent_id,member_id,numerator,denominator) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, c.Brand, c.ID, r.ID, item.ID, item.Order, allocation.AgentID, allocation.MemberID, allocation.Exact.Numerator, allocation.Exact.Denominator)
			if err != nil {
				return err
			}
		}
	}
	if len(items) > 0 {
		last := items[len(items)-1]
		r.CursorAt = &last.Placed
		r.CursorOrder = &last.Order
	}
	_, err = tx.Exec(ctx, `UPDATE commission_runs SET state=$2,cursor_at=$3,cursor_order_id=$4 WHERE id=$1`, r.ID, state, r.CursorAt, r.CursorOrder)
	if err != nil {
		return err
	}
	return saveCycle(ctx, tx, c, state, c.RunID, nil)
}

func summarizeCyclePage(ctx context.Context, tx pgx.Tx, c cycleRow, r runRow, meta points.Metadata) error {
	rows, err := tx.Query(ctx, `SELECT agent_id::text,member_id::text,trunc(sum(numerator::numeric*(1000000/denominator::numeric)))::text FROM commission_allocations WHERE run_id=$1 AND ($2::uuid IS NULL OR agent_id>$2) GROUP BY agent_id,member_id ORDER BY agent_id LIMIT $3`, r.ID, r.AgentCursor, cyclePageSize+1)
	if err != nil {
		return err
	}
	type total struct{ Agent, Member, Micro string }
	var sums []total
	for rows.Next() {
		var sum total
		if err = rows.Scan(&sum.Agent, &sum.Member, &sum.Micro); err != nil {
			rows.Close()
			return err
		}
		sums = append(sums, sum)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	more := len(sums) > cyclePageSize
	if more {
		sums = sums[:cyclePageSize]
	}
	state := "summarizing"
	if !more {
		state = "ready"
	}
	if _, err = appendCycleStep(ctx, tx, c, state, "earnings_page", "sum exact per-agent cycle amounts and round once", c.RunID, meta); err != nil {
		return err
	}
	for _, sum := range sums {
		micro, ok := new(big.Int).SetString(sum.Micro, 10)
		if !ok || micro.Sign() < 0 {
			return ErrEvidence
		}
		rat := new(big.Rat).SetFrac(micro, big.NewInt(1000000))
		exact, whole, e := Aggregate([]ExactAmount{{rat.Num().String(), rat.Denom().String()}})
		if e != nil {
			return e
		}
		_, err = tx.Exec(ctx, `INSERT INTO commission_earnings(id,brand_id,cycle_id,run_id,agent_id,member_id,numerator,denominator,points) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, ids.New(), c.Brand, c.ID, r.ID, sum.Agent, sum.Member, exact.Numerator, exact.Denominator, whole)
		if err != nil {
			return err
		}
	}
	if len(sums) > 0 {
		r.AgentCursor = &sums[len(sums)-1].Agent
	}
	_, err = tx.Exec(ctx, `UPDATE commission_runs SET state=$2,earnings_cursor_agent=$3 WHERE id=$1`, r.ID, state, r.AgentCursor)
	if err != nil {
		return err
	}
	return saveCycle(ctx, tx, c, state, c.RunID, nil)
}
