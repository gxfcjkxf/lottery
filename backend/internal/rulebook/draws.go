package rulebook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/drawfeed"
	"github.com/gxfcjkxf/lottery/backend/internal/drawnotice"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

var ErrDrawAbnormal = errors.New("draw repeats previous result")

type SourceSet struct {
	ID          string                  `json:"id"`
	BrandID     string                  `json:"brand_id"`
	GameID      string                  `json:"game_id"`
	Revision    int64                   `json:"revision"`
	GameVersion int64                   `json:"game_version"`
	CreatedAt   time.Time               `json:"created_at"`
	Sources     []drawfeed.SourceConfig `json:"sources"`
}

const sourceSetFields = `s.id::text,s.brand_id::text,s.game_id::text,s.revision,g.version,s.created_at,s.sources`

func scanSourceSet(row pgx.Row) (v SourceSet, e error) {
	var raw []byte
	e = row.Scan(&v.ID, &v.BrandID, &v.GameID, &v.Revision, &v.GameVersion, &v.CreatedAt, &raw)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(raw, &v.Sources)
		v.CreatedAt = v.CreatedAt.UTC()
	}
	return
}
func (s Store) Sources(ctx context.Context, brand, game string) (SourceSet, error) {
	if !uuidPattern.MatchString(brand) || !uuidPattern.MatchString(game) {
		return SourceSet{}, ErrInvalid
	}
	return scanSourceSet(s.DB.QueryRow(ctx, `SELECT `+sourceSetFields+` FROM games g JOIN draw_source_sets s ON s.id=g.draw_source_set_id WHERE g.brand_id=$1 AND g.id=$2`, brand, game))
}
func (s Store) WriteSources(ctx context.Context, tx pgx.Tx, brand string, a access.Account, game string, version int64, sources []drawfeed.SourceConfig, reason string, meta points.Metadata) (SourceSet, error) {
	out := SourceSet{}
	if e := check(tx, brand, a, "draw_source", "write", reason); e != nil {
		return out, e
	}
	if !uuidPattern.MatchString(game) || drawfeed.ValidateSources(sources) != nil {
		return out, ErrInvalid
	}
	g, e := lockGame(ctx, tx, brand, game)
	if e != nil {
		return out, e
	}
	if version != g.Version || version == math.MaxInt64 {
		return out, ErrVersion
	}
	configs := append([]drawfeed.SourceConfig{}, sources...)
	for i := range configs {
		configs[i].ID = strings.ToLower(configs[i].ID)
	}
	// Stable identity insertion order avoids cross-game UUID collision deadlocks.
	identities := append([]drawfeed.SourceConfig{}, configs...)
	sort.Slice(identities, func(i, j int) bool { return identities[i].ID < identities[j].ID })
	for _, c := range identities {
		if _, e = tx.Exec(ctx, `INSERT INTO draw_sources(id,brand_id,game_id,type) VALUES($1,$2,$3,$4) ON CONFLICT(id) DO NOTHING`, c.ID, brand, game, c.Type); e != nil {
			return out, e
		}
		var matches bool
		if e = tx.QueryRow(ctx, `SELECT brand_id=$2::uuid AND game_id=$3::uuid AND type=$4 FROM draw_sources WHERE id=$1`, c.ID, brand, game, c.Type).Scan(&matches); e != nil {
			return out, e
		}
		if !matches {
			return out, ErrInvalid
		}
	}
	var revision int64
	var previous string
	if e = tx.QueryRow(ctx, `SELECT coalesce(max(revision),0) FROM draw_source_sets WHERE brand_id=$1 AND game_id=$2`, brand, game).Scan(&revision); e != nil {
		return out, e
	}
	if revision == math.MaxInt64 {
		return out, ErrVersion
	}
	if e = tx.QueryRow(ctx, `SELECT coalesce(draw_source_set_id::text,'') FROM games WHERE id=$1`, game).Scan(&previous); e != nil {
		return out, e
	}
	raw, e := json.Marshal(configs)
	if e != nil {
		return out, e
	}
	id := ids.New()
	if _, e = tx.Exec(ctx, `INSERT INTO draw_source_sets(id,brand_id,game_id,revision,sources,created_by) VALUES($1,$2,$3,$4,$5,$6)`, id, brand, game, revision+1, raw, a.ID); e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, `UPDATE games SET draw_source_set_id=$2,version=version+1 WHERE id=$1`, game, id); e != nil {
		return out, e
	}
	out, e = scanSourceSet(tx.QueryRow(ctx, `SELECT `+sourceSetFields+` FROM games g JOIN draw_source_sets s ON s.id=g.draw_source_set_id WHERE g.id=$1`, game))
	if e != nil {
		return out, e
	}
	_, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "draw_source.write", ResourceType: "draw_source_set", ResourceID: id, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, Before: map[string]any{"source_set_id": previous, "game_version": version}, After: out})
	return out, e
}

type DrawResult struct {
	ID              string     `json:"id"`
	BrandID         string     `json:"brand_id"`
	GameID          string     `json:"game_id"`
	PeriodID        string     `json:"period_id"`
	SourceID        string     `json:"source_id"`
	Kind            string     `json:"kind"`
	Result          rules.Draw `json:"result"`
	ResultHash      string     `json:"result_hash"`
	DrawnAt         time.Time  `json:"drawn_at"`
	CreatedAt       time.Time  `json:"created_at"`
	CreatedBy       string     `json:"created_by,omitempty"`
	CorrectedFromID string     `json:"corrected_from_id,omitempty"`
}
type AttemptBatch struct {
	ID                    string             `json:"id"`
	BrandID               string             `json:"brand_id"`
	GameID                string             `json:"game_id"`
	PeriodID              string             `json:"period_id"`
	SourceSetID           string             `json:"source_set_id"`
	ObservedPeriodVersion int64              `json:"observed_period_version"`
	Status                string             `json:"status"`
	Attempts              []drawfeed.Attempt `json:"attempts"`
	CreatedAt             time.Time          `json:"created_at"`
}
type DrawHistory struct {
	Current  *DrawResult    `json:"current"`
	History  []DrawResult   `json:"history"`
	Attempts []AttemptBatch `json:"attempts"`
	Limit    int            `json:"limit"`
	Offset   int            `json:"offset"`
}

const drawResultFields = `id::text,brand_id::text,game_id::text,period_id::text,source_id::text,kind,result,result_hash,drawn_at,created_at,coalesce(created_by::text,''),coalesce(corrected_from_id::text,'')`

func scanDrawResult(row pgx.Row) (v DrawResult, e error) {
	var raw []byte
	e = row.Scan(&v.ID, &v.BrandID, &v.GameID, &v.PeriodID, &v.SourceID, &v.Kind, &raw, &v.ResultHash, &v.DrawnAt, &v.CreatedAt, &v.CreatedBy, &v.CorrectedFromID)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(raw, &v.Result)
		v.DrawnAt = v.DrawnAt.UTC()
		v.CreatedAt = v.CreatedAt.UTC()
	}
	return
}
func (s Store) Draw(ctx context.Context, brand, period string, limit, offset int) (DrawHistory, error) {
	out := DrawHistory{History: []DrawResult{}, Attempts: []AttemptBatch{}, Limit: limit, Offset: offset}
	if !uuidPattern.MatchString(brand) || !uuidPattern.MatchString(period) || limit < 1 || limit > 100 || offset < 0 {
		return out, ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	var current string
	e = tx.QueryRow(ctx, `SELECT coalesce(draw_result_id::text,'') FROM periods WHERE brand_id=$1 AND id=$2`, brand, period).Scan(&current)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if e != nil {
		return out, e
	}
	if current != "" {
		v, e := scanDrawResult(tx.QueryRow(ctx, `SELECT `+drawResultFields+` FROM draw_results WHERE id=$1`, current))
		if e != nil {
			return out, e
		}
		out.Current = &v
	}
	rows, e := tx.Query(ctx, `SELECT `+drawResultFields+` FROM draw_results WHERE brand_id=$1 AND period_id=$2 ORDER BY created_at DESC,id DESC LIMIT $3 OFFSET $4`, brand, period, limit, offset)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		v, e := scanDrawResult(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.History = append(out.History, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	rows, e = tx.Query(ctx, `SELECT id::text,brand_id::text,game_id::text,period_id::text,source_set_id::text,observed_period_version,status,attempts,created_at FROM draw_attempt_batches WHERE brand_id=$1 AND period_id=$2 ORDER BY created_at DESC,id DESC LIMIT $3 OFFSET $4`, brand, period, limit, offset)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var v AttemptBatch
		var raw []byte
		if e = rows.Scan(&v.ID, &v.BrandID, &v.GameID, &v.PeriodID, &v.SourceSetID, &v.ObservedPeriodVersion, &v.Status, &raw, &v.CreatedAt); e == nil {
			e = json.Unmarshal(raw, &v.Attempts)
		}
		if e != nil {
			rows.Close()
			return out, e
		}
		v.CreatedAt = v.CreatedAt.UTC()
		out.Attempts = append(out.Attempts, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	return out, tx.Commit(ctx)
}

// All result writers acquire the game before the period; no network work is
// performed while these locks are held.
func lockDrawPeriod(ctx context.Context, tx pgx.Tx, brand, period string) (Game, Period, error) {
	if !uuidPattern.MatchString(brand) || !uuidPattern.MatchString(period) {
		return Game{}, Period{}, ErrInvalid
	}
	var game string
	e := tx.QueryRow(ctx, `SELECT game_id::text FROM periods WHERE brand_id=$1 AND id=$2`, brand, period).Scan(&game)
	if errors.Is(e, pgx.ErrNoRows) {
		return Game{}, Period{}, ErrNotFound
	}
	if e != nil {
		return Game{}, Period{}, e
	}
	g, e := lockGame(ctx, tx, brand, game)
	if e != nil {
		return g, Period{}, e
	}
	p, e := scanPeriod(tx.QueryRow(ctx, `SELECT `+periodFields+` FROM periods WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, period))
	return g, p, e
}
func previousDraw(ctx context.Context, q queryer, p Period) (*rules.Draw, error) {
	var raw []byte
	e := q.QueryRow(ctx, `SELECT r.result FROM periods p JOIN draw_results r ON r.id=p.draw_result_id WHERE p.brand_id=$1 AND p.game_id=$2 AND p.draw_at<$3 ORDER BY p.draw_at DESC,p.id DESC LIMIT 1`, p.BrandID, p.GameID, p.DrawAt).Scan(&raw)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var d rules.Draw
	e = json.Unmarshal(raw, &d)
	return &d, e
}
func (s Store) ManualDraw(ctx context.Context, tx pgx.Tx, brand string, a access.Account, period string, version int64, periodNo string, result rules.Draw, drawnAt time.Time, reason string, meta points.Metadata) (DrawResult, error) {
	if e := check(tx, brand, a, "draw", "manual_create", reason); e != nil {
		return DrawResult{}, e
	}
	g, p, e := lockDrawPeriod(ctx, tx, brand, period)
	if e != nil {
		return DrawResult{}, e
	}
	if p.Version != version || version == math.MaxInt64 {
		return DrawResult{}, ErrVersion
	}
	var source string
	e = tx.QueryRow(ctx, `SELECT id::text FROM draw_sources WHERE brand_id=$1 AND game_id=$2 AND type='manual'`, brand, g.ID).Scan(&source)
	if errors.Is(e, pgx.ErrNoRows) {
		source = ids.New()
		_, e = tx.Exec(ctx, `INSERT INTO draw_sources(id,brand_id,game_id,type) VALUES($1,$2,$3,'manual')`, source, brand, g.ID)
	}
	if e != nil {
		return DrawResult{}, e
	}
	return appendDrawResult(ctx, tx, g, p, source, "manual", drawfeed.Candidate{PeriodNo: periodNo, Draw: result, DrawnAt: drawnAt}, a.ID, reason, meta)
}

// Caller owns the game/period locks and the complete mutation transaction.
// Revalidation here prevents an in-flight candidate from bypassing a newer
// previous result, manual selection, or primary-database clock.
func appendDrawResult(ctx context.Context, tx pgx.Tx, g Game, p Period, source, kind string, c drawfeed.Candidate, actor, reason string, meta points.Metadata) (DrawResult, error) {
	out := DrawResult{}
	if p.Version == math.MaxInt64 {
		return out, ErrVersion
	}
	if kind != "manual" && kind != "api" && kind != "dom" {
		return out, ErrInvalid
	}
	if (kind == "manual") != (actor != "") || !uuidPattern.MatchString(source) {
		return out, ErrInvalid
	}
	var current string
	if e := tx.QueryRow(ctx, `SELECT coalesce(draw_result_id::text,'') FROM periods WHERE id=$1`, p.ID).Scan(&current); e != nil {
		return out, e
	}
	if p.Status == "waiting_draw" {
		if current != "" {
			return out, ErrState
		}
	} else if p.Status == "drawn" && kind == "manual" && current != "" {
		var oldKind string
		if e := tx.QueryRow(ctx, `SELECT kind FROM draw_results WHERE id=$1`, current).Scan(&oldKind); e != nil {
			return out, e
		}
		if oldKind == "manual" {
			return out, ErrState
		}
	} else {
		return out, ErrState
	}
	previous, e := previousDraw(ctx, tx, p)
	if e != nil {
		return out, e
	}
	e = drawfeed.ValidateCandidate(drawfeed.Request{PeriodNo: p.PeriodNo, Model: g.Model, Previous: previous}, c)
	if errors.Is(e, drawfeed.ErrAbnormal) {
		return out, ErrDrawAbnormal
	}
	if e != nil {
		return out, ErrInvalid
	}
	var now time.Time
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return out, e
	}
	if now.Before(p.DrawAt) || c.DrawnAt.Before(p.DrawAt) || c.DrawnAt.After(now) {
		return out, ErrInvalid
	}
	d := c.Draw
	d.Regular = append([]int{}, d.Regular...)
	d.Special = append([]int{}, d.Special...)
	d.Digits = append([]int{}, d.Digits...)
	if !g.Model.Ordered {
		sort.Ints(d.Regular)
		sort.Ints(d.Special)
	}
	raw, e := json.Marshal(d)
	if e != nil {
		return out, e
	}
	sum := sha256.Sum256(raw)
	id := ids.New()
	out, e = scanDrawResult(tx.QueryRow(ctx, `INSERT INTO draw_results(id,brand_id,game_id,period_id,source_id,kind,result,result_hash,drawn_at,created_by,corrected_from_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,'')::uuid,NULLIF($11,'')::uuid) RETURNING `+drawResultFields, id, g.BrandID, g.ID, p.ID, source, kind, raw, hex.EncodeToString(sum[:]), c.DrawnAt.UTC(), actor, current))
	if e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, `UPDATE periods SET draw_result_id=$2,status='drawn',version=version+1,state_reason=$3,draw_claim_token=NULL,draw_claim_until=NULL,draw_next_poll_at=NULL WHERE id=$1`, p.ID, id, "result locked: "+kind); e != nil {
		return out, e
	}
	actorType := "system"
	if actor != "" {
		actorType = "admin"
	}
	var auditID string
	auditID, e = audit.Append(ctx, tx, audit.Record{BrandID: g.BrandID, ActorType: actorType, ActorID: actor, Action: "draw.lock", ResourceType: "draw_result", ResourceID: id, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, Before: map[string]any{"period_id": p.ID, "period_version": p.Version, "status": p.Status, "draw_result_id": current}, After: out})
	if e == nil {
		e = drawnotice.Append(ctx, tx, g.BrandID, p.ID, id, auditID)
	}
	return out, e
}
