package withdrawal

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ DB *pgxpool.Pool }
type BrandPolicy struct {
	BrandID    string      `json:"brand_id"`
	Version    int64       `json:"version"`
	Config     BrandConfig `json:"config"`
	UpdatedAt  time.Time   `json:"updated_at"`
	AuditLogID string      `json:"audit_log_id,omitempty"`
}
type EffectiveMultiple struct {
	TurnoverMultiple string `json:"turnover_multiple"`
	Source           string `json:"source"`
	BrandVersion     int64  `json:"brand_version"`
	GameVersion      int64  `json:"game_version"`
}
type GamePolicy struct {
	BrandID    string            `json:"brand_id"`
	GameID     string            `json:"game_id"`
	Version    int64             `json:"version"`
	Config     GameConfig        `json:"config"`
	Effective  EffectiveMultiple `json:"effective"`
	UpdatedAt  time.Time         `json:"updated_at"`
	AuditLogID string            `json:"audit_log_id,omitempty"`
}
type Revision struct {
	ID        string          `json:"id"`
	BrandID   string          `json:"brand_id"`
	GameID    string          `json:"game_id"`
	Version   int64           `json:"version"`
	Config    json.RawMessage `json:"config"`
	ChangedBy string          `json:"changed_by"`
	Reason    string          `json:"reason"`
	CreatedAt time.Time       `json:"created_at"`
}
type row interface{ Scan(...any) error }

func scanBrand(r row) (v BrandPolicy, e error) {
	var raw []byte
	e = r.Scan(&v.BrandID, &v.Version, &raw, &v.UpdatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	if json.Unmarshal(raw, &v.Config) != nil {
		return v, ErrInvalid
	}
	v.UpdatedAt = v.UpdatedAt.UTC()
	return v, nil
}
func scanGame(r row) (v GamePolicy, e error) {
	var raw []byte
	e = r.Scan(&v.BrandID, &v.GameID, &v.Version, &raw, &v.UpdatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	if json.Unmarshal(raw, &v.Config) != nil {
		return v, ErrInvalid
	}
	v.UpdatedAt = v.UpdatedAt.UTC()
	return v, nil
}

const brandFields = `brand_id::text,version,config,updated_at`
const gameFields = `brand_id::text,game_id::text,version,config,updated_at`

func resolveMultiple(b BrandPolicy, g *GamePolicy) {
	g.Effective = EffectiveMultiple{TurnoverMultiple: b.Config.TurnoverMultiple, Source: "brand", BrandVersion: b.Version, GameVersion: g.Version}
	if g.Config.TurnoverMultiple != nil {
		g.Effective.TurnoverMultiple = *g.Config.TurnoverMultiple
		g.Effective.Source = "game"
	}
}
func (s Service) BrandPolicy(ctx context.Context, brand string) (BrandPolicy, error) {
	if s.DB == nil || !validIDs(brand) {
		return BrandPolicy{}, ErrInvalid
	}
	return scanBrand(s.DB.QueryRow(ctx, `SELECT `+brandFields+` FROM brand_withdrawal_policies WHERE brand_id=$1`, brand))
}
func (s Service) GamePolicy(ctx context.Context, brand, game string) (GamePolicy, error) {
	var out GamePolicy
	if s.DB == nil || !validIDs(brand, game) {
		return out, ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	b, e := scanBrand(tx.QueryRow(ctx, `SELECT `+brandFields+` FROM brand_withdrawal_policies WHERE brand_id=$1`, brand))
	if e != nil {
		return out, e
	}
	out, e = scanGame(tx.QueryRow(ctx, `SELECT `+gameFields+` FROM game_withdrawal_policies WHERE brand_id=$1 AND game_id=$2`, brand, game))
	if e != nil {
		return out, e
	}
	resolveMultiple(b, &out)
	return out, tx.Commit(ctx)
}
func checkWrite(tx pgx.Tx, brand string, a access.Account, version int64, reason string) error {
	if tx == nil || !validIDs(brand, a.ID) || version < 1 || strings.TrimSpace(reason) == "" || len(reason) > 500 || !utf8.ValidString(reason) {
		return ErrInvalid
	}
	if a.SuperAdmin || !access.Authorize(a, "withdrawal_policy", "write", access.ScopeBrand, brand) {
		return ErrDenied
	}
	return nil
}
func (s Service) appendRevision(ctx context.Context, tx pgx.Tx, brand, game string, version int64, config any, reason string, a access.Account) (string, error) {
	raw, e := json.Marshal(config)
	if e != nil {
		return "", e
	}
	id := ids.New()
	_, e = tx.Exec(ctx, `INSERT INTO withdrawal_policy_revisions(id,brand_id,game_id,version,config,changed_by,reason) VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5,$6,$7)`, id, brand, game, version, raw, a.ID, strings.TrimSpace(reason))
	return id, e
}
func (s Service) UpdateBrand(ctx context.Context, tx pgx.Tx, brand string, a access.Account, in BrandInput, meta points.Metadata) (BrandPolicy, error) {
	var out BrandPolicy
	if e := checkWrite(tx, brand, a, in.Version, in.Reason); e != nil {
		return out, e
	}
	if ValidateBrandConfig(in.Config) != nil {
		return out, ErrInvalid
	}
	before, e := scanBrand(tx.QueryRow(ctx, `SELECT `+brandFields+` FROM brand_withdrawal_policies WHERE brand_id=$1 FOR UPDATE`, brand))
	if e != nil {
		return out, e
	}
	if before.Version != in.Version || before.Version == math.MaxInt64 {
		return out, ErrVersion
	}
	revision, e := s.appendRevision(ctx, tx, brand, "", before.Version+1, in.Config, in.Reason, a)
	if e != nil {
		return out, e
	}
	raw, _ := json.Marshal(in.Config)
	out, e = scanBrand(tx.QueryRow(ctx, `UPDATE brand_withdrawal_policies SET version=version+1,config=$2 WHERE brand_id=$1 RETURNING `+brandFields, brand, raw))
	if e != nil {
		return out, e
	}
	out.AuditLogID, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "withdrawal_policy.brand.update", ResourceType: "brand_withdrawal_policy", ResourceID: brand, Reason: strings.TrimSpace(in.Reason), RequestID: meta.RequestID, IP: meta.IP, Before: before, After: map[string]any{"policy": out, "revision_id": revision}})
	return out, e
}
func (s Service) UpdateGame(ctx context.Context, tx pgx.Tx, brand, game string, a access.Account, in GameInput, meta points.Metadata) (GamePolicy, error) {
	var out GamePolicy
	if e := checkWrite(tx, brand, a, in.Version, in.Reason); e != nil {
		return out, e
	}
	if !validIDs(game) || ValidateGameConfig(in.Config) != nil {
		return out, ErrInvalid
	}
	b, e := scanBrand(tx.QueryRow(ctx, `SELECT `+brandFields+` FROM brand_withdrawal_policies WHERE brand_id=$1 FOR SHARE`, brand))
	if e != nil {
		return out, e
	}
	before, e := scanGame(tx.QueryRow(ctx, `SELECT `+gameFields+` FROM game_withdrawal_policies WHERE brand_id=$1 AND game_id=$2 FOR UPDATE`, brand, game))
	if e != nil {
		return out, e
	}
	resolveMultiple(b, &before)
	if before.Version != in.Version || before.Version == math.MaxInt64 {
		return out, ErrVersion
	}
	revision, e := s.appendRevision(ctx, tx, brand, game, before.Version+1, in.Config, in.Reason, a)
	if e != nil {
		return out, e
	}
	raw, _ := json.Marshal(in.Config)
	out, e = scanGame(tx.QueryRow(ctx, `UPDATE game_withdrawal_policies SET version=version+1,config=$3 WHERE brand_id=$1 AND game_id=$2 RETURNING `+gameFields, brand, game, raw))
	if e != nil {
		return out, e
	}
	resolveMultiple(b, &out)
	out.AuditLogID, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "withdrawal_policy.game.update", ResourceType: "game_withdrawal_policy", ResourceID: game, Reason: strings.TrimSpace(in.Reason), RequestID: meta.RequestID, IP: meta.IP, Before: before, After: map[string]any{"policy": out, "revision_id": revision}})
	return out, e
}
func (s Service) History(ctx context.Context, brand, game string, limit, offset int) ([]Revision, error) {
	out := []Revision{}
	if s.DB == nil || !validIDs(brand) || game != "" && !validIDs(game) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return out, ErrInvalid
	}
	if game == "" {
		if _, e := s.BrandPolicy(ctx, brand); e != nil {
			return out, e
		}
	} else {
		if _, e := s.GamePolicy(ctx, brand, game); e != nil {
			return out, e
		}
	}
	rows, e := s.DB.Query(ctx, `SELECT id::text,brand_id::text,coalesce(game_id::text,''),version,config,coalesce(changed_by::text,''),reason,created_at FROM withdrawal_policy_revisions WHERE brand_id=$1 AND game_id IS NOT DISTINCT FROM NULLIF($2,'')::uuid ORDER BY version DESC LIMIT $3 OFFSET $4`, brand, game, limit, offset)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v Revision
		if e = rows.Scan(&v.ID, &v.BrandID, &v.GameID, &v.Version, &v.Config, &v.ChangedBy, &v.Reason, &v.CreatedAt); e != nil {
			return out, e
		}
		v.CreatedAt = v.CreatedAt.UTC()
		out = append(out, v)
	}
	return out, rows.Err()
}
