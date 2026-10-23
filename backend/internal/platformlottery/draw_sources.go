package platformlottery

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DrawSource struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Revision int64          `json:"revision"`
	Plan     AutomationPlan `json:"plan"`
}
type DrawSourceInput struct {
	DrawSource
}
type DrawSourceActivation struct {
	Revision int64 `json:"revision"`
	Version  int64 `json:"version"`
}
type DrawSourceList struct {
	Items                []DrawSource `json:"items"`
	ActiveSourceID       string       `json:"active_source_id"`
	ActiveSourceRevision int64        `json:"active_source_revision"`
	Version              int64        `json:"version"`
}

func scanDrawSource(row pgx.Row) (d DrawSource, err error) {
	var raw []byte
	err = row.Scan(&d.ID, &d.Name, &d.Revision, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(raw, &d.Plan)
	}
	return
}
func (s Service) DrawSources(ctx context.Context, game string) (out DrawSourceList, err error) {
	out.Items = []DrawSource{}
	if _, err = s.Detail(ctx, "", game, 1, 0); err != nil {
		return
	}
	rows, err := s.DB.Query(ctx, `SELECT id::text,name,revision,plan FROM platform_draw_sources WHERE game_id=$1 ORDER BY name,id`, game)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		d, e := scanDrawSource(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return
	}
	err = s.DB.QueryRow(ctx, `SELECT s.version,coalesce(a.source_id::text,''),coalesce(a.source_revision,0) FROM platform_automation_settings s LEFT JOIN platform_draw_source_activations a ON a.game_id=s.game_id AND a.automation_version=s.version WHERE s.game_id=$1`, game).Scan(&out.Version, &out.ActiveSourceID, &out.ActiveSourceRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	return
}
func (s Service) SaveDrawSource(ctx context.Context, tx pgx.Tx, a access.Account, game string, in DrawSourceInput, meta points.Metadata) (DrawSource, error) {
	if !Allowed(a, "", "write") {
		return DrawSource{}, ErrDenied
	}
	if !textOK(in.Name, 120) || in.Revision < 0 || ValidateAutomation(in.Plan) != nil {
		return DrawSource{}, ErrInvalid
	}
	g, err := lockedGame(ctx, tx, game)
	if err != nil {
		return DrawSource{}, err
	}
	if validateMappingModel(in.Plan, g.Model) != nil {
		return DrawSource{}, ErrInvalid
	}
	in.Plan.Version = 0
	raw, err := json.Marshal(in.Plan)
	if err != nil {
		return DrawSource{}, err
	}
	var out DrawSource
	if in.ID == "" {
		if in.Revision != 0 {
			return out, ErrVersionConflict
		}
		out, err = scanDrawSource(tx.QueryRow(ctx, `INSERT INTO platform_draw_sources(id,game_id,name,revision,plan,created_by) VALUES($1,$2,$3,1,$4,$5) ON CONFLICT(game_id,name) DO NOTHING RETURNING id::text,name,revision,plan`, ids.New(), game, in.Name, raw, a.ID))
		if errors.Is(err, ErrNotFound) {
			return out, ErrConflict
		}
	} else {
		out, err = scanDrawSource(tx.QueryRow(ctx, `UPDATE platform_draw_sources SET name=$3,plan=$4,revision=revision+1,updated_at=clock_timestamp() WHERE game_id=$1 AND id=$2 AND revision=$5 RETURNING id::text,name,revision,plan`, game, in.ID, in.Name, raw, in.Revision))
		if errors.Is(err, ErrNotFound) {
			return out, ErrVersionConflict
		}
	}
	if err != nil {
		var conflict *pgconn.PgError
		if errors.As(err, &conflict) && conflict.Code == "23505" {
			return out, ErrConflict
		}
		return out, err
	}
	return out, record(ctx, tx, a, "", "platform_game.draw_source.save", out.ID, "", meta, out)
}
func (s Service) ActivateDrawSource(ctx context.Context, tx pgx.Tx, a access.Account, game, id string, in DrawSourceActivation, meta points.Metadata) (AutomationPlan, error) {
	if !Allowed(a, "", "write") {
		return AutomationPlan{}, ErrDenied
	}
	if in.Revision < 1 || in.Version < 0 {
		return AutomationPlan{}, ErrInvalid
	}
	if _, err := lockedGame(ctx, tx, game); err != nil {
		return AutomationPlan{}, err
	}
	d, err := scanDrawSource(tx.QueryRow(ctx, `SELECT id::text,name,revision,plan FROM platform_draw_sources WHERE game_id=$1 AND id=$2 FOR UPDATE`, game, id))
	if err != nil {
		return AutomationPlan{}, err
	}
	if d.Revision != in.Revision {
		return AutomationPlan{}, ErrVersionConflict
	}
	d.Plan.Version = in.Version
	p, err := s.publishAutomation(ctx, tx, a, game, AutomationInput{AutomationPlan: d.Plan}, d, meta)
	if err != nil {
		return p, err
	}
	return p, nil
}
