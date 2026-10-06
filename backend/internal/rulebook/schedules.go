package rulebook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/schedule"
	"github.com/jackc/pgx/v5"
)

type ScheduleRecord struct {
	ID          string        `json:"id"`
	BrandID     string        `json:"brand_id"`
	GameID      string        `json:"game_id"`
	Revision    int64         `json:"revision"`
	Spec        schedule.Spec `json:"spec"`
	GameVersion int64         `json:"game_version"`
	CreatedAt   time.Time     `json:"created_at"`
}

func scanSchedule(row pgx.Row) (v ScheduleRecord, err error) {
	var raw []byte
	err = row.Scan(&v.ID, &v.BrandID, &v.GameID, &v.Revision, &raw, &v.GameVersion, &v.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(raw, &v.Spec)
	}
	return
}

const scheduleFields = `s.id::text,s.brand_id::text,s.game_id::text,s.revision,s.spec,g.version,s.created_at`

func (s Store) Schedule(ctx context.Context, brand, game string) (ScheduleRecord, error) {
	return scanSchedule(s.DB.QueryRow(ctx, `SELECT `+scheduleFields+` FROM games g JOIN game_schedules s ON s.id=g.schedule_id WHERE g.brand_id=$1 AND g.id=$2`, brand, game))
}
func (s Store) WriteSchedule(ctx context.Context, tx pgx.Tx, brand string, a access.Account, game string, version int64, spec schedule.Spec, reason string, meta points.Metadata) (ScheduleRecord, error) {
	out := ScheduleRecord{}
	if e := check(tx, brand, a, "schedule", "write", reason); e != nil {
		return out, e
	}
	if schedule.Validate(spec) != nil {
		return out, ErrInvalid
	}
	g, e := lockGame(ctx, tx, brand, game)
	if e != nil {
		return out, e
	}
	if g.Version != version || version == math.MaxInt64 {
		return out, ErrVersion
	}
	if spec.Timezone != g.Timezone {
		return out, ErrInvalid
	}
	var revision int64
	if e = tx.QueryRow(ctx, `SELECT coalesce(max(revision),0) FROM game_schedules WHERE brand_id=$1 AND game_id=$2`, brand, game).Scan(&revision); e != nil {
		return out, e
	}
	if revision == math.MaxInt64 {
		return out, ErrVersion
	}
	var previous string
	if e = tx.QueryRow(ctx, `SELECT coalesce(schedule_id::text,'') FROM games WHERE id=$1`, game).Scan(&previous); e != nil {
		return out, e
	}
	raw, e := json.Marshal(spec)
	if e != nil {
		return out, e
	}
	id := ids.New()
	if _, e = tx.Exec(ctx, `INSERT INTO game_schedules(id,brand_id,game_id,revision,spec,created_by) VALUES($1,$2,$3,$4,$5,$6)`, id, brand, game, revision+1, raw, a.ID); e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, `UPDATE games SET schedule_id=$2,version=version+1 WHERE id=$1`, game, id); e != nil {
		return out, e
	}
	out, e = scanSchedule(tx.QueryRow(ctx, `SELECT `+scheduleFields+` FROM games g JOIN game_schedules s ON s.id=g.schedule_id WHERE g.id=$1`, game))
	if e != nil {
		return out, e
	}
	_, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "schedule.write", ResourceType: "game_schedule", ResourceID: id, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, Before: map[string]any{"schedule_id": previous, "game_version": version}, After: out})
	return out, e
}

const periodFields = `id::text,brand_id::text,game_id::text,period_no,sequence,bet_start_at,bet_end_at,draw_at,status,version,coalesce(schedule_id::text,''),state_reason`

func scanPeriod(row pgx.Row) (p Period, e error) {
	e = row.Scan(&p.ID, &p.BrandID, &p.GameID, &p.PeriodNo, &p.Sequence, &p.BetStartAt, &p.BetEndAt, &p.DrawAt, &p.Status, &p.Version, &p.ScheduleID, &p.StateReason)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	if e == nil {
		p.BetStartAt = p.BetStartAt.UTC()
		p.BetEndAt = p.BetEndAt.UTC()
		p.DrawAt = p.DrawAt.UTC()
	}
	return
}
func (s Store) Period(ctx context.Context, brand, id string) (Period, error) {
	if !uuidPattern.MatchString(brand) || !uuidPattern.MatchString(id) {
		return Period{}, ErrInvalid
	}
	return scanPeriod(s.DB.QueryRow(ctx, `SELECT `+periodFields+` FROM periods WHERE brand_id=$1 AND id=$2`, brand, id))
}
func (s Store) Periods(ctx context.Context, brand, game string, limit, offset int) ([]Period, error) {
	out := []Period{}
	if !uuidPattern.MatchString(brand) || !uuidPattern.MatchString(game) || limit < 1 || limit > 100 || offset < 0 {
		return out, ErrInvalid
	}
	if _, e := scanGame(s.DB.QueryRow(ctx, `SELECT `+gameFields+` FROM games WHERE brand_id=$1 AND id=$2`, brand, game)); e != nil {
		return out, e
	}
	rows, e := s.DB.Query(ctx, `SELECT `+periodFields+` FROM periods WHERE brand_id=$1 AND game_id=$2 ORDER BY bet_start_at DESC,id DESC LIMIT $3 OFFSET $4`, brand, game, limit, offset)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		p, e := scanPeriod(rows)
		if e != nil {
			return out, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type Generation struct {
	Created  int      `json:"created"`
	Existing int      `json:"existing"`
	Periods  []Period `json:"periods"`
}

func (s Store) GeneratePeriods(ctx context.Context, tx pgx.Tx, brand string, a access.Account, game string, from, to time.Time, reason string, meta points.Metadata) (Generation, error) {
	if e := check(tx, brand, a, "period", "generate", reason); e != nil {
		return Generation{}, e
	}
	g, e := lockGame(ctx, tx, brand, game)
	if e != nil {
		return Generation{}, e
	}
	return generatePeriods(ctx, tx, g, from, to, a.ID, reason, meta)
}
func generatePeriods(ctx context.Context, tx pgx.Tx, g Game, from, to time.Time, actor, reason string, meta points.Metadata) (Generation, error) {
	out := Generation{Periods: []Period{}}
	record, e := scanSchedule(tx.QueryRow(ctx, `SELECT `+scheduleFields+` FROM games g JOIN game_schedules s ON s.id=g.schedule_id WHERE g.id=$1`, g.ID))
	if e != nil {
		return out, e
	}
	slots, e := schedule.Expand(record.Spec, from, to)
	if e != nil {
		return out, e
	}
	// Generation is a reservation, never a late opening or a backdated activation.
	var now time.Time
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return out, e
	}
	if to.Before(now) || from.After(now.Add(7*24*time.Hour)) || to.After(now.Add(7*24*time.Hour)) {
		return out, ErrInvalid
	}
	var seq int64
	if e = tx.QueryRow(ctx, `SELECT coalesce(max(sequence),0) FROM periods WHERE brand_id=$1 AND game_id=$2`, g.BrandID, g.ID).Scan(&seq); e != nil {
		return out, e
	}
	for _, slot := range slots {
		p, e := scanPeriod(tx.QueryRow(ctx, `SELECT `+periodFields+` FROM periods WHERE brand_id=$1 AND game_id=$2 AND period_no=$3`, g.BrandID, g.ID, slot.PeriodNo))
		if e == nil {
			if !p.BetStartAt.Equal(slot.BetStartAt) || !p.BetEndAt.Equal(slot.BetEndAt) || !p.DrawAt.Equal(slot.DrawAt) {
				return out, ErrState
			}
			out.Existing++
			if len(out.Periods) < 100 {
				out.Periods = append(out.Periods, p)
			}
			continue
		}
		if !errors.Is(e, ErrNotFound) {
			return out, e
		}
		if !slot.BetEndAt.After(now) {
			continue
		}
		if seq == math.MaxInt64 {
			return out, ErrVersion
		}
		seq++
		p, e = scanPeriod(tx.QueryRow(ctx, `INSERT INTO periods(id,brand_id,game_id,period_no,sequence,bet_start_at,bet_end_at,draw_at,status,schedule_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'pending',$9) RETURNING `+periodFields, ids.New(), g.BrandID, g.ID, slot.PeriodNo, seq, slot.BetStartAt, slot.BetEndAt, slot.DrawAt, record.ID))
		if e != nil {
			return out, e
		}
		out.Created++
		if len(out.Periods) < 100 {
			out.Periods = append(out.Periods, p)
		}
	}
	if actor != "" || out.Created > 0 {
		actorType := "system"
		if actor != "" {
			actorType = "admin"
		}
		_, e = audit.Append(ctx, tx, audit.Record{BrandID: g.BrandID, ActorType: actorType, ActorID: actor, Action: "period.generate", ResourceType: "game", ResourceID: g.ID, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, After: map[string]any{"schedule_id": record.ID, "from": from, "to": to, "created": out.Created, "existing": out.Existing}})
	}
	return out, e
}

// Tick uses the primary database clock, game -> period -> play lock order, and
// one transaction per game. Multiple workers may race without double opening.
func (s Store) Tick(ctx context.Context) (int, error) {
	rows, e := s.DB.Query(ctx, `SELECT DISTINCT p.brand_id::text,p.game_id::text FROM periods p JOIN games g ON g.id=p.game_id WHERE (p.status='pending' AND ((p.bet_start_at<=clock_timestamp() AND g.status='active') OR p.bet_end_at<=clock_timestamp())) OR (p.status='betting' AND p.bet_end_at<=clock_timestamp()) OR (p.status='closed' AND p.draw_at<=clock_timestamp()) ORDER BY 1,2 LIMIT 25`)
	if e != nil {
		return 0, e
	}
	games := [][2]string{}
	for rows.Next() {
		var pair [2]string
		if e = rows.Scan(&pair[0], &pair[1]); e != nil {
			rows.Close()
			return 0, e
		}
		games = append(games, pair)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	count := 0
	var failures error
	for _, pair := range games {
		tx, e := s.DB.Begin(ctx)
		if e != nil {
			failures = errors.Join(failures, fmt.Errorf("game %s: %w", pair[1], e))
			continue
		}
		n, e := tickGame(ctx, tx, pair[0], pair[1])
		if e == nil {
			e = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		if e != nil {
			failures = errors.Join(failures, fmt.Errorf("game %s: %w", pair[1], e))
			continue
		}
		count += n
	}
	return count, failures
}
func tickGame(ctx context.Context, tx pgx.Tx, brand, game string) (int, error) {
	g, e := lockGame(ctx, tx, brand, game)
	if e != nil {
		return 0, e
	}
	var now time.Time
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return 0, e
	}
	rows, e := tx.Query(ctx, `SELECT `+periodFields+` FROM periods WHERE brand_id=$1 AND game_id=$2 AND ((status='pending' AND ((bet_start_at<=$3 AND $4) OR bet_end_at<=$3)) OR (status='betting' AND bet_end_at<=$3) OR (status='closed' AND draw_at<=$3)) ORDER BY bet_start_at,id LIMIT 100 FOR UPDATE`, brand, game, now, g.Status == "active")
	if e != nil {
		return 0, e
	}
	due := []Period{}
	for rows.Next() {
		p, e := scanPeriod(rows)
		if e != nil {
			rows.Close()
			return 0, e
		}
		due = append(due, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	n := 0
	for _, p := range due {
		// Re-read time after possible contention; never open an already expired window.
		if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
			return n, e
		}
		if p.Status == "pending" {
			if !p.BetEndAt.After(now) {
				p, e = transitionPeriod(ctx, tx, p, "judged_cancelled", "opening window elapsed before scheduler opened", now)
				if e != nil {
					return n, e
				}
				n++
				continue
			}
			if g.Status != "active" {
				continue
			}
			if g.StartedSequence == math.MaxInt64 || g.Version == math.MaxInt64 {
				return n, ErrVersion
			}
			p, e = transitionPeriod(ctx, tx, p, "betting", "scheduled betting window opened", now)
			if e != nil {
				return n, e
			}
			if e = bindOpening(ctx, tx, g, p); e != nil {
				return n, e
			}
			g.StartedSequence++
			g.Version++
			n++
		}
		if p.Status == "betting" && !p.BetEndAt.After(now) {
			p, e = transitionPeriod(ctx, tx, p, "closed", "betting deadline reached", now)
			if e != nil {
				return n, e
			}
			n++
		}
		if p.Status == "closed" && !p.DrawAt.After(now) {
			_, e = transitionPeriod(ctx, tx, p, "waiting_draw", "draw time reached", now)
			if e != nil {
				return n, e
			}
			n++
		}
	}
	return n, nil
}
func transitionPeriod(ctx context.Context, tx pgx.Tx, p Period, status, reason string, now time.Time) (Period, error) {
	if p.Version == math.MaxInt64 {
		return p, ErrVersion
	}
	before := p
	p, e := scanPeriod(tx.QueryRow(ctx, `UPDATE periods SET status=$2,state_reason=$3,version=version+1 WHERE id=$1 RETURNING `+periodFields, p.ID, status, reason))
	if e != nil {
		return p, e
	}
	_, e = audit.Append(ctx, tx, audit.Record{BrandID: p.BrandID, ActorType: "system", Action: "period." + status, ResourceType: "period", ResourceID: p.ID, Reason: reason, RequestID: p.ID, Before: before, After: map[string]any{"period": p, "observed_at": now}})
	return p, e
}

// FillCalendar is called less often than Tick and reserves at most the next day.
// The worker never edits already generated periods when a schedule is replaced.
func (s Store) FillCalendar(ctx context.Context) (int, error) {
	rows, e := s.DB.Query(ctx, `SELECT brand_id::text,id::text FROM games WHERE schedule_id IS NOT NULL AND status='active' ORDER BY id`)
	if e != nil {
		return 0, e
	}
	list := [][2]string{}
	for rows.Next() {
		var p [2]string
		if e = rows.Scan(&p[0], &p[1]); e != nil {
			rows.Close()
			return 0, e
		}
		list = append(list, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	n := 0
	var failures error
	for _, p := range list {
		tx, e := s.DB.Begin(ctx)
		if e != nil {
			failures = errors.Join(failures, fmt.Errorf("game %s: %w", p[1], e))
			continue
		}
		g, e := lockGame(ctx, tx, p[0], p[1])
		var now time.Time
		if e == nil {
			e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now)
		}
		var result Generation
		if e == nil && g.Status == "active" {
			result, e = generatePeriods(ctx, tx, g, now, now.Add(24*time.Hour), "", "automatic calendar reservation", points.Metadata{})
		}
		if e == nil {
			e = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		if e != nil {
			failures = errors.Join(failures, fmt.Errorf("game %s: %w", p[1], e))
			continue
		}
		n += result.Created
	}
	return n, failures
}
