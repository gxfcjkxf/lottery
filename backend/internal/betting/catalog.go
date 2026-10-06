package betting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

// Catalog views expose published playing rules, never workflow contributors,
// reviewer identities, unpublished drafts, credentials or source endpoints.
type CatalogGame struct {
	ID       string      `json:"id"`
	Code     string      `json:"code"`
	Name     string      `json:"name"`
	Model    rules.Model `json:"model"`
	Timezone string      `json:"timezone"`
	Status   string      `json:"status"`
}
type CatalogPlay struct {
	ID             string           `json:"id"`
	GameID         string           `json:"game_id"`
	Code           string           `json:"code"`
	Name           string           `json:"name"`
	RuleVersionID  string           `json:"rule_version_id"`
	DefinitionHash string           `json:"definition_hash"`
	Definition     rules.Definition `json:"definition"`
}
type GameCatalog struct {
	Game           CatalogGame     `json:"game"`
	Plays          []CatalogPlay   `json:"plays"`
	Period         *Period         `json:"period"`
	ServerTime     time.Time       `json:"server_time"`
	BrandStatus    string          `json:"brand_status"`
	Policy         EffectivePolicy `json:"policy"`
	PolicyVersions PolicyVersions  `json:"policy_versions"`
}

func scanCatalogGame(row pgx.Row) (g CatalogGame, err error) {
	var model []byte
	err = row.Scan(&g.ID, &g.Code, &g.Name, &model, &g.Timezone, &g.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return g, ErrNotFound
	}
	if err != nil {
		return g, err
	}
	if json.Unmarshal(model, &g.Model) != nil || rules.ValidateModel(g.Model) != nil {
		return g, ErrInvalid
	}
	return g, nil
}

// catalogTx reads the published catalog consistently from the primary. It takes
// no row locks and reserves neither money nor a quote. Place revalidates everything.
func (s Service) catalogTx(ctx context.Context, brand string) (pgx.Tx, string, time.Time, error) {
	if s.DB == nil || !validIDs(brand) {
		return nil, "", time.Time{}, ErrInvalid
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, "", time.Time{}, err
	}
	var status string
	var now time.Time
	err = tx.QueryRow(ctx, `SELECT status,clock_timestamp() FROM brands WHERE id=$1 AND status IN ('active','paused')`, brand).Scan(&status, &now)
	if err != nil {
		_ = tx.Rollback(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			err = ErrNotFound
		}
		return nil, "", time.Time{}, err
	}
	return tx, status, now.UTC(), nil
}

const catalogGameFields = `id::text,code,name,model,timezone,status`

func (s Service) Games(ctx context.Context, brand string, limit, offset int) ([]CatalogGame, error) {
	items := []CatalogGame{}
	if limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return items, ErrInvalid
	}
	tx, _, _, err := s.catalogTx(ctx, brand)
	if err != nil {
		return items, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT `+catalogGameFields+` FROM games WHERE brand_id=$1 AND status IN ('active','paused') ORDER BY code,id LIMIT $2 OFFSET $3`, brand, limit, offset)
	if err != nil {
		return items, err
	}
	for rows.Next() {
		g, e := scanCatalogGame(rows)
		if e != nil {
			rows.Close()
			return items, e
		}
		items = append(items, g)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return items, err
	}
	return items, tx.Commit(ctx)
}

func (s Service) Catalog(ctx context.Context, brand, game string) (GameCatalog, error) {
	out := GameCatalog{Plays: []CatalogPlay{}}
	if !validIDs(game) {
		return out, ErrInvalid
	}
	tx, status, now, err := s.catalogTx(ctx, brand)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	out.BrandStatus, out.ServerTime = status, now
	out.Game, err = scanCatalogGame(tx.QueryRow(ctx, `SELECT `+catalogGameFields+` FROM games WHERE brand_id=$1 AND id=$2 AND status IN ('active','paused')`, brand, game))
	if err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT p.id::text,p.game_id::text,p.code,p.name,r.id::text,r.definition_sha256,r.definition
 FROM play_definitions p JOIN rule_versions r ON r.brand_id=p.brand_id AND r.game_id=p.game_id AND r.play_id=p.id AND r.id=p.active_version_id
 WHERE p.brand_id=$1 AND p.game_id=$2 AND p.status='active' AND r.status='active' ORDER BY p.code,p.id`, brand, game)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var p CatalogPlay
		var raw []byte
		if err = rows.Scan(&p.ID, &p.GameID, &p.Code, &p.Name, &p.RuleVersionID, &p.DefinitionHash, &raw); err != nil {
			rows.Close()
			return out, err
		}
		if json.Unmarshal(raw, &p.Definition) != nil || rules.ValidateDefinition(p.Definition) != nil {
			rows.Close()
			return out, ErrInvalid
		}
		canonical, e := json.Marshal(p.Definition)
		if e != nil {
			rows.Close()
			return out, e
		}
		hash := sha256.Sum256(canonical)
		if hex.EncodeToString(hash[:]) != p.DefinitionHash {
			rows.Close()
			return out, ErrInvalid
		}
		out.Plays = append(out.Plays, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	// Prefer an actually accepting period, then the nearest reserved future
	// period, then the latest in-progress/completed period. The timestamp is an
	// estimate for display only; stake admission still uses the database clock.
	var p Period
	err = tx.QueryRow(ctx, `SELECT id::text,game_id::text,period_no,status,bet_start_at,bet_end_at,draw_at,coalesce(draw_result_id::text,'') FROM periods
 WHERE brand_id=$1 AND game_id=$2 AND status NOT IN ('bet_cancelled','judged_cancelled') AND (status<>'pending' OR bet_end_at>$3)
 ORDER BY CASE WHEN status='betting' AND bet_start_at<=$3 AND bet_end_at>$3 THEN 0 WHEN status='pending' THEN 1 WHEN status='settled' THEN 3 ELSE 2 END,
 CASE WHEN status='pending' THEN bet_start_at END ASC NULLS LAST,bet_start_at DESC,id DESC LIMIT 1`, brand, game, now).Scan(&p.ID, &p.GameID, &p.PeriodNo, &p.Status, &p.BetStartAt, &p.BetEndAt, &p.DrawAt, &p.DrawResultID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if err == nil {
		p.BetStartAt = p.BetStartAt.UTC()
		p.BetEndAt = p.BetEndAt.UTC()
		p.DrawAt = p.DrawAt.UTC()
		out.Period = &p
	}
	brandPolicy, err := scanBrandPolicy(tx.QueryRow(ctx, `SELECT `+brandPolicyFields+` FROM brand_bet_policies WHERE brand_id=$1`, brand))
	if err != nil {
		return out, err
	}
	gamePolicy, err := scanGamePolicy(tx.QueryRow(ctx, `SELECT `+gamePolicyFields+` FROM game_bet_policies WHERE brand_id=$1 AND game_id=$2`, brand, game))
	if err != nil {
		return out, err
	}
	out.Policy, err = ResolvePolicy(brandPolicy.Config, gamePolicy.Config)
	if err != nil {
		return out, err
	}
	out.PolicyVersions = PolicyVersions{Brand: brandPolicy.Version, Game: gamePolicy.Version}
	return out, tx.Commit(ctx)
}
