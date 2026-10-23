package platformlottery

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

type ConfiguredCase struct {
	Context  *rules.ConfiguredSettlementContext `json:"context,omitempty"`
	Requests []rules.ConfiguredRequest          `json:"requests"`
	Draw     rules.Draw                         `json:"draw"`
	Bet      points.Amount                      `json:"bet_points"`
	Prize    points.Amount                      `json:"prize_points"`
	Refund   points.Amount                      `json:"refund_points"`
}
type ConfiguredVersion struct {
	ID         string           `json:"id"`
	GameID     string           `json:"game_id"`
	Revision   int64            `json:"revision"`
	Snapshot   json.RawMessage  `json:"snapshot"`
	Hash       string           `json:"snapshot_hash"`
	Cases      []ConfiguredCase `json:"cases"`
	Status     string           `json:"status"`
	Validation json.RawMessage  `json:"validation"`
}
type ConfiguredDraftInput struct {
	ID       string           `json:"id"`
	Revision int64            `json:"revision"`
	Snapshot json.RawMessage  `json:"snapshot"`
	Cases    []ConfiguredCase `json:"cases"`
}
type ConfiguredAction struct {
	Revision int64  `json:"revision"`
	Action   string `json:"action"`
}
type ConfiguredSales struct {
	Version             int64                  `json:"version"`
	RuleID              string                 `json:"rule_id"`
	Enabled             bool                   `json:"enabled"`
	Prices              rules.ConfiguredPrices `json:"prices"`
	MinPoints           points.Amount          `json:"min_points"`
	MaxPoints           points.Amount          `json:"max_points"`
	MaxPeriodPoints     points.Amount          `json:"max_period_points"`
	MaxUserPeriodPoints points.Amount          `json:"max_user_period_points"`
}

const configuredVersionFields = `id::text,game_id::text,revision,snapshot,snapshot_hash,cases,status,validation`

func scanConfiguredVersion(row pgx.Row) (v ConfiguredVersion, err error) {
	var cases []byte
	err = row.Scan(&v.ID, &v.GameID, &v.Revision, &v.Snapshot, &v.Hash, &cases, &v.Status, &v.Validation)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(cases, &v.Cases)
	}
	return
}
func (s Service) ConfiguredVersions(ctx context.Context, brand, game string) ([]ConfiguredVersion, error) {
	if _, err := s.Detail(ctx, brand, game, 1, 0); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `SELECT `+configuredVersionFields+` FROM configured_rule_versions WHERE game_id=$1 AND ($2='' OR status IN('published','retired')) ORDER BY created_at DESC,id DESC LIMIT 50`, game, brand)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConfiguredVersion{}
	for rows.Next() {
		v, e := scanConfiguredVersion(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s Service) SaveConfiguredDraft(ctx context.Context, tx pgx.Tx, a access.Account, game string, in ConfiguredDraftInput, meta points.Metadata) (ConfiguredVersion, error) {
	if !Allowed(a, "", "write") {
		return ConfiguredVersion{}, ErrDenied
	}
	g, err := lockedGame(ctx, tx, game)
	if err != nil {
		return ConfiguredVersion{}, err
	}
	engine, err := rules.RestoreConfiguredEngine(in.Snapshot)
	if err != nil {
		return ConfiguredVersion{}, ErrInvalid
	}
	var snap rules.ConfiguredSnapshot
	if json.Unmarshal(engine.Snapshot(), &snap) != nil || !reflect.DeepEqual(g.Model, snap.Catalog.Model) || len(in.Cases) > 32 {
		return ConfiguredVersion{}, ErrInvalid
	}
	if in.Cases == nil {
		return ConfiguredVersion{}, ErrInvalid
	}
	normalized := append([]ConfiguredCase{}, in.Cases...)
	for i := range normalized {
		if normalized[i].Requests == nil {
			return ConfiguredVersion{}, ErrInvalid
		}
		for _, request := range normalized[i].Requests {
			if request.Options == nil {
				return ConfiguredVersion{}, ErrInvalid
			}
		}
		normalized[i].Draw.Regular = append([]int{}, normalized[i].Draw.Regular...)
		normalized[i].Draw.Special = append([]int{}, normalized[i].Draw.Special...)
		normalized[i].Draw.Digits = append([]int{}, normalized[i].Draw.Digits...)
	}
	cases, err := json.Marshal(normalized)
	if err != nil {
		return ConfiguredVersion{}, ErrInvalid
	}
	id := in.ID
	if id == "" {
		if in.Revision != 0 {
			return ConfiguredVersion{}, ErrVersionConflict
		}
		id = ids.New()
		_, err = tx.Exec(ctx, `INSERT INTO configured_rule_versions(id,game_id,snapshot,snapshot_hash,cases,created_by) VALUES($1,$2,$3,$4,$5,$6)`, id, game, engine.Snapshot(), engine.SnapshotHash(), cases, a.ID)
	} else {
		cmd, e := tx.Exec(ctx, `UPDATE configured_rule_versions SET snapshot=$4,snapshot_hash=$5,cases=$6,validation=NULL,status='draft',revision=revision+1 WHERE game_id=$1 AND id=$2 AND revision=$3 AND status IN('draft','validated','rejected')`, game, id, in.Revision, engine.Snapshot(), engine.SnapshotHash(), cases)
		err = e
		if e == nil && cmd.RowsAffected() != 1 {
			return ConfiguredVersion{}, ErrVersionConflict
		}
	}
	if err != nil {
		return ConfiguredVersion{}, err
	}
	v, err := scanConfiguredVersion(tx.QueryRow(ctx, `SELECT `+configuredVersionFields+` FROM configured_rule_versions WHERE id=$1`, id))
	if err == nil {
		err = record(ctx, tx, a, "", "configured_rule.save", id, "", meta, v)
	}
	return v, err
}
func (s Service) ActConfiguredRule(ctx context.Context, tx pgx.Tx, a access.Account, game, id string, in ConfiguredAction, meta points.Metadata) (ConfiguredVersion, error) {
	if !Allowed(a, "", "write") {
		return ConfiguredVersion{}, ErrDenied
	}
	if _, e := lockedGame(ctx, tx, game); e != nil {
		return ConfiguredVersion{}, e
	}
	v, e := scanConfiguredVersion(tx.QueryRow(ctx, `SELECT `+configuredVersionFields+` FROM configured_rule_versions WHERE game_id=$1 AND id=$2 FOR UPDATE`, game, id))
	if e != nil {
		return v, e
	}
	if in.Revision != v.Revision {
		return v, ErrVersionConflict
	}
	status := ""
	report := v.Validation
	switch in.Action {
	case "validate":
		if v.Status != "draft" && v.Status != "validated" && v.Status != "rejected" || len(v.Cases) < 1 {
			return v, ErrInvalid
		}
		engine, err := rules.RestoreConfiguredEngine(v.Snapshot)
		if err != nil {
			return v, ErrInvalid
		}
		results := []rules.ConfiguredResult{}
		for _, c := range v.Cases {
			q, err := engine.Prepare(ctx, c.Requests)
			if err != nil {
				return v, ErrInvalid
			}
			settlement := rules.ConfiguredSettlementContext{}
			if c.Context != nil {
				settlement = *c.Context
			} else {
				settlement = engine.DefaultSettlementContext()
			}
			r, err := engine.SettleWithContext(ctx, q, c.Draw, settlement)
			if err != nil || r.BetPoints != c.Bet || r.PrizePoints != c.Prize || r.RefundPoints != c.Refund {
				return v, ErrInvalid
			}
			results = append(results, r)
		}
		report, _ = json.Marshal(map[string]any{"snapshot_hash": v.Hash, "passed": true, "results": results})
		status = "validated"
	case "submit":
		if v.Status != "validated" || len(report) == 0 {
			return v, ErrConflict
		}
		status = "submitted"
	case "approve", "reject":
		if v.Status != "submitted" {
			return v, ErrConflict
		}
		status = "approved"
		if in.Action == "reject" {
			status = "rejected"
		}
	case "publish":
		if v.Status != "approved" {
			return v, ErrConflict
		}
		// A row lock on the canonical game serializes publications, sales and admissions.
		if _, e = tx.Exec(ctx, `UPDATE configured_rule_versions SET status='retired',revision=revision+1 WHERE id=(SELECT version_id FROM configured_rule_heads WHERE game_id=$1)`, game); e != nil {
			return v, e
		}
		status = "published"
	default:
		return v, ErrInvalid
	}
	_, e = tx.Exec(ctx, `UPDATE configured_rule_versions SET status=$2,validation=$3,revision=revision+1,reviewed_by=CASE WHEN $4 IN('approve','reject') THEN $5::uuid ELSE reviewed_by END WHERE id=$1`, id, status, report, in.Action, a.ID)
	if e != nil {
		return v, e
	}
	if status == "published" {
		_, e = tx.Exec(ctx, `INSERT INTO configured_rule_heads(game_id,version_id) VALUES($1,$2) ON CONFLICT(game_id) DO UPDATE SET version_id=excluded.version_id`, game, id)
		if e != nil {
			return v, e
		}
	}
	v, e = scanConfiguredVersion(tx.QueryRow(ctx, `SELECT `+configuredVersionFields+` FROM configured_rule_versions WHERE id=$1`, id))
	if e == nil {
		e = record(ctx, tx, a, "", "configured_rule."+in.Action, id, "", meta, v)
	}
	return v, e
}

func ReadConfiguredPublication(ctx context.Context, q queryer, game string) (ConfiguredVersion, error) {
	return scanConfiguredVersion(q.QueryRow(ctx, `SELECT `+configuredVersionFields+` FROM configured_rule_versions WHERE id=(SELECT version_id FROM configured_rule_heads WHERE game_id=$1) AND status='published'`, game))
}
func ReadConfiguredSales(ctx context.Context, q queryer, brand, game string, lock bool) (v ConfiguredSales, err error) {
	clause := ""
	if lock {
		clause = " FOR UPDATE"
	}
	var raw []byte
	err = q.QueryRow(ctx, `SELECT version,rule_id::text,enabled,prices,min_points,max_points,max_period_points,max_user_period_points FROM configured_sales WHERE brand_id=$1 AND game_id=$2`+clause, brand, game).Scan(&v.Version, &v.RuleID, &v.Enabled, &raw, &v.MinPoints, &v.MaxPoints, &v.MaxPeriodPoints, &v.MaxUserPeriodPoints)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(raw, &v.Prices)
	}
	return
}
func ConfiguredSalesEngine(v ConfiguredVersion, sales ConfiguredSales) (*rules.ConfiguredEngine, error) {
	if sales.RuleID != v.ID {
		return nil, ErrVersionConflict
	}
	var snapshot rules.ConfiguredSnapshot
	if json.Unmarshal(v.Snapshot, &snapshot) != nil {
		return nil, ErrInvalid
	}
	ceiling := snapshot.Prices
	engine, err := rules.NewConfiguredEngine(snapshot.Catalog, snapshot.Bindings, sales.Prices)
	if err != nil {
		return nil, ErrInvalid
	}
	if engine.CheckPriceCeilings(ceiling) != nil {
		return nil, ErrInvalid
	}
	return engine, nil
}
func (s Service) WriteConfiguredSales(ctx context.Context, tx pgx.Tx, a access.Account, brand, game string, in ConfiguredSales, meta points.Metadata) (ConfiguredSales, error) {
	if !Allowed(a, brand, "sell") {
		return in, ErrDenied
	}
	if _, e := lockedGame(ctx, tx, game); e != nil {
		return in, e
	}
	var connected bool
	if e := tx.QueryRow(ctx, `SELECT enabled FROM brand_platform_games WHERE brand_id=$1 AND game_id=$2 FOR SHARE`, brand, game).Scan(&connected); errors.Is(e, pgx.ErrNoRows) {
		return in, ErrNotFound
	} else if e != nil {
		return in, e
	}
	v, e := ReadConfiguredPublication(ctx, tx, game)
	if e != nil {
		return in, e
	}
	if _, e = ConfiguredSalesEngine(v, in); e != nil {
		return in, e
	}
	if in.MinPoints < 1 || in.MaxPoints < in.MinPoints || in.MaxUserPeriodPoints < in.MaxPoints || in.MaxPeriodPoints < in.MaxPoints {
		return in, ErrInvalid
	}
	current, e := ReadConfiguredSales(ctx, tx, brand, game, true)
	if e != nil && !errors.Is(e, ErrNotFound) {
		return in, e
	}
	if current.Version != in.Version {
		return in, ErrVersionConflict
	}
	raw, _ := json.Marshal(in.Prices)
	in.Version++
	_, e = tx.Exec(ctx, `INSERT INTO configured_sales(brand_id,game_id,version,rule_id,enabled,prices,min_points,max_points,max_period_points,max_user_period_points) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(brand_id,game_id) DO UPDATE SET version=excluded.version,rule_id=excluded.rule_id,enabled=excluded.enabled,prices=excluded.prices,min_points=excluded.min_points,max_points=excluded.max_points,max_period_points=excluded.max_period_points,max_user_period_points=excluded.max_user_period_points`, brand, game, in.Version, in.RuleID, in.Enabled, raw, in.MinPoints, in.MaxPoints, in.MaxPeriodPoints, in.MaxUserPeriodPoints)
	if e == nil {
		e = record(ctx, tx, a, brand, "configured_sales.write", game, "", meta, in)
	}
	return in, e
}

func validateConfiguredPeriodDraw(ctx context.Context, tx pgx.Tx, id string, result rules.Draw) error {
	rows, err := tx.Query(ctx, `SELECT DISTINCT snapshot FROM configured_orders WHERE period_id=$1`, id)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var snapshot []byte
		if err = rows.Scan(&snapshot); err != nil {
			return err
		}
		engine, err := rules.RestoreConfiguredEngine(snapshot)
		if err != nil || engine.ValidateDraw(result) != nil {
			return ErrInvalid
		}
	}
	return rows.Err()
}
