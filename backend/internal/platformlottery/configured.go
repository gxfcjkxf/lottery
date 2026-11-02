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
type ConfiguredRulesInput struct {
	ID       string          `json:"id"`
	Revision int64           `json:"revision"`
	Snapshot json.RawMessage `json:"snapshot"`
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

// ConfiguredRules reads only the effective rules, never an unfinished version.
func (s Service) ConfiguredRules(ctx context.Context, brand, game string) (*ConfiguredVersion, error) {
	if _, err := s.Detail(ctx, brand, game, 1, 0); err != nil {
		return nil, err
	}
	current, err := ReadConfiguredPublication(ctx, s.DB, game)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &current, nil
}

// SaveConfiguredRules atomically replaces the current rules while preserving frozen orders.
func (s Service) SaveConfiguredRules(ctx context.Context, tx pgx.Tx, a access.Account, game string, in ConfiguredRulesInput, meta points.Metadata) (ConfiguredVersion, error) {
	if !Allowed(a, "", "write") {
		return ConfiguredVersion{}, ErrDenied
	}
	g, err := lockedGame(ctx, tx, game)
	if err != nil {
		return ConfiguredVersion{}, err
	}
	current, err := ReadConfiguredPublication(ctx, tx, game)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return ConfiguredVersion{}, err
	}
	if in.ID != current.ID || in.Revision != current.Revision {
		return ConfiguredVersion{}, ErrVersionConflict
	}
	engine, err := rules.RestoreConfiguredEngine(in.Snapshot)
	if err != nil {
		return ConfiguredVersion{}, ErrInvalid
	}
	var snap rules.ConfiguredSnapshot
	if json.Unmarshal(engine.Snapshot(), &snap) != nil || !reflect.DeepEqual(g.Model, snap.Catalog.Model) {
		return ConfiguredVersion{}, ErrInvalid
	}
	var before any
	if current.ID != "" {
		before = current
	}
	id := ids.New()
	if _, err = tx.Exec(ctx, `INSERT INTO configured_rule_versions(id,game_id,snapshot,snapshot_hash,cases,status,created_by) VALUES($1,$2,$3,$4,'[]','published',$5)`, id, game, engine.Snapshot(), engine.SnapshotHash(), a.ID); err != nil {
		return ConfiguredVersion{}, err
	}
	if current.ID != "" {
		if _, err = tx.Exec(ctx, `UPDATE configured_rule_versions SET status='retired',revision=revision+1 WHERE id=$1`, current.ID); err != nil {
			return ConfiguredVersion{}, err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO configured_rule_heads(game_id,version_id) VALUES($1,$2) ON CONFLICT(game_id) DO UPDATE SET version_id=excluded.version_id`, game, id); err != nil {
		return ConfiguredVersion{}, err
	}
	saved, err := scanConfiguredVersion(tx.QueryRow(ctx, `SELECT `+configuredVersionFields+` FROM configured_rule_versions WHERE id=$1`, id))
	if err == nil {
		err = record(ctx, tx, a, "", "configured_rule.save", id, "", meta, before, saved)
	}
	return saved, err
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
	var before any
	if current.Version > 0 {
		before = current
	}
	raw, _ := json.Marshal(in.Prices)
	in.Version++
	_, e = tx.Exec(ctx, `INSERT INTO configured_sales(brand_id,game_id,version,rule_id,enabled,prices,min_points,max_points,max_period_points,max_user_period_points) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(brand_id,game_id) DO UPDATE SET version=excluded.version,rule_id=excluded.rule_id,enabled=excluded.enabled,prices=excluded.prices,min_points=excluded.min_points,max_points=excluded.max_points,max_period_points=excluded.max_period_points,max_user_period_points=excluded.max_user_period_points`, brand, game, in.Version, in.RuleID, in.Enabled, raw, in.MinPoints, in.MaxPoints, in.MaxPeriodPoints, in.MaxUserPeriodPoints)
	if e == nil {
		e = record(ctx, tx, a, brand, "configured_sales.write", game, "", meta, before, in)
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
