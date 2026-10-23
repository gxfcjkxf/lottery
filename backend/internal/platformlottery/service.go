package platformlottery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalid         = errors.New("invalid platform lottery input")
	ErrDenied          = errors.New("platform lottery permission denied")
	ErrNotFound        = errors.New("platform lottery not found")
	ErrConflict        = errors.New("platform lottery conflict")
	ErrModelLocked     = errors.New("platform lottery model is already in use")
	ErrVersionConflict = errors.New("platform lottery version conflict")
	codePattern        = regexp.MustCompile(`^[a-z][a-z0-9_]{0,47}$`)
)

type Service struct{ DB *pgxpool.Pool }
type Game struct {
	ID            string      `json:"id"`
	Code          string      `json:"code"`
	Name          string      `json:"name"`
	Model         rules.Model `json:"model"`
	Timezone      string      `json:"timezone"`
	Version       int64       `json:"version"`
	Connected     bool        `json:"connected"`
	Status        string      `json:"status"`
	Deletable     bool        `json:"deletable"`
	BrandEnabled  bool        `json:"brand_enabled"`
	BrandVersion  int64       `json:"brand_version"`
	RulePublished bool        `json:"rule_published"`
	SalesStatus   string      `json:"sales_status"`
}
type GameInput struct {
	Code     string      `json:"code"`
	Name     string      `json:"name"`
	Model    rules.Model `json:"model"`
	Timezone string      `json:"timezone"`
	Reason   string      `json:"reason"`
}
type PlayInput struct {
	Code       string           `json:"code"`
	Name       string           `json:"name"`
	Definition rules.Definition `json:"definition"`
	Reason     string           `json:"reason"`
}
type NameInput struct {
	Name            string `json:"name"`
	ExpectedVersion int64  `json:"expected_version"`
}

func (s Service) UpdateName(ctx context.Context, tx pgx.Tx, a access.Account, id string, in NameInput, meta points.Metadata) (Game, error) {
	if !Allowed(a, "", "write") {
		return Game{}, ErrDenied
	}
	if in.ExpectedVersion < 1 || !textOK(in.Name, 120) || in.Name != strings.TrimSpace(in.Name) {
		return Game{}, ErrInvalid
	}
	g, e := lockedGame(ctx, tx, id)
	if e != nil {
		return g, e
	}
	if g.Version != in.ExpectedVersion {
		return g, ErrVersionConflict
	}
	if _, e = tx.Exec(ctx, `UPDATE platform_games SET name=$2,version=version+1 WHERE id=$1`, id, in.Name); e != nil {
		return g, e
	}
	g.Name = in.Name
	g.Version++
	return g, record(ctx, tx, a, "", "platform_game.name.update", id, "", meta, g)
}

type ModelInput struct {
	Model           rules.Model `json:"model"`
	ExpectedVersion int64       `json:"expected_version"`
	Reason          string      `json:"reason"`
}

func lockedGame(ctx context.Context, tx pgx.Tx, id string) (Game, error) {
	return scanGame(tx.QueryRow(ctx, `SELECT `+gameFields+`,false,false,0,`+gameRulePublished+`,`+strings.ReplaceAll(gameSalesStatus, "$1", "''")+` FROM platform_games g WHERE g.id=$1 AND g.status<>'deleted' FOR UPDATE`, id))
}

func (s Service) UpdateModel(ctx context.Context, tx pgx.Tx, a access.Account, id string, in ModelInput, meta points.Metadata) (Game, error) {
	if !Allowed(a, "", "write") {
		return Game{}, ErrDenied
	}
	if in.ExpectedVersion < 1 || len(in.Reason) > 500 || rules.ValidateModel(in.Model) != nil {
		return Game{}, ErrInvalid
	}
	g, e := lockedGame(ctx, tx, id)
	if e != nil {
		return g, e
	}
	if g.Version != in.ExpectedVersion {
		return g, ErrVersionConflict
	}
	var used bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform_game_plays WHERE game_id=$1) OR EXISTS(SELECT 1 FROM platform_game_periods WHERE game_id=$1) OR EXISTS(SELECT 1 FROM platform_canada_settings WHERE game_id=$1) OR EXISTS(SELECT 1 FROM platform_mark_six_settings WHERE game_id=$1) OR EXISTS(SELECT 1 FROM platform_mark_six_attributes WHERE game_id=$1) OR EXISTS(SELECT 1 FROM configured_rule_versions WHERE game_id=$1)`, id).Scan(&used); e != nil {
		return g, e
	}
	if used {
		return g, ErrModelLocked
	}
	raw, _ := json.Marshal(in.Model)
	if _, e = tx.Exec(ctx, `UPDATE platform_games SET model=$2,version=version+1 WHERE id=$1`, id, raw); e != nil {
		return g, e
	}
	g.Model = in.Model
	g.Version++
	return g, record(ctx, tx, a, "", "platform_game.model.update", id, in.Reason, meta, g)
}

type Play struct {
	ID             string           `json:"id"`
	Code           string           `json:"code"`
	Name           string           `json:"name"`
	Definition     rules.Definition `json:"definition"`
	DefinitionHash string           `json:"definition_hash"`
}
type PeriodInput struct {
	AutomationVersion *int64    `json:"-"`
	PeriodNo          string    `json:"period_no"`
	BetStartAt        time.Time `json:"bet_start_at"`
	BetEndAt          time.Time `json:"bet_end_at"`
	DrawAt            time.Time `json:"draw_at"`
	Reason            string    `json:"reason"`
}
type Period struct {
	Attributes  *MarkSixAttributes `json:"attributes"`
	ID          string             `json:"id"`
	GameID      string             `json:"game_id"`
	PeriodNo    string             `json:"period_no"`
	BetStartAt  time.Time          `json:"bet_start_at"`
	BetEndAt    time.Time          `json:"bet_end_at"`
	DrawAt      time.Time          `json:"draw_at"`
	Result      *rules.Draw        `json:"result"`
	PublishedAt *time.Time         `json:"published_at"`
}
type Detail struct {
	RuleProtocol      string   `json:"rule_protocol"`
	Game              Game     `json:"game"`
	Plays             []Play   `json:"plays"`
	Periods           []Period `json:"periods"`
	MarkSixConfigured bool     `json:"mark_six_configured"`
}
type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func Allowed(a access.Account, brand, action string) bool {
	if brand == "" {
		return a.SuperAdmin && access.Authorize(a, "platform_game", action, access.ScopePlatform, "")
	}
	return !a.SuperAdmin && access.Authorize(a, "platform_game", action, access.ScopeBrand, brand)
}
func textOK(s string, max int) bool { return strings.TrimSpace(s) != "" && len(s) <= max }
func record(ctx context.Context, tx pgx.Tx, a access.Account, brand, action, id, reason string, meta points.Metadata, out any) error {
	_, e := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: action, ResourceType: "platform_game", ResourceID: id, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, After: out})
	return e
}
func scanGame(row pgx.Row) (g Game, e error) {
	var raw []byte
	e = row.Scan(&g.ID, &g.Code, &g.Name, &raw, &g.Timezone, &g.Version, &g.Status, &g.Deletable, &g.Connected, &g.BrandEnabled, &g.BrandVersion, &g.RulePublished, &g.SalesStatus)
	if errors.Is(e, pgx.ErrNoRows) {
		return g, ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(raw, &g.Model)
	}
	return
}

const gameFields = `g.id::text,g.code,g.name,g.model,g.timezone,g.version,g.status,NOT (` + gameDeletionBlockers + `)`

const gameRulePublished = `EXISTS(SELECT 1 FROM configured_rule_heads h WHERE h.game_id=g.id)`

// Empty brand scope reports whether any connected brand has current sales;
// a brand-scoped read only inspects that brand's own sales.
const gameSalesStatus = `CASE
 WHEN EXISTS(SELECT 1 FROM configured_sales s JOIN brand_platform_games b ON b.game_id=s.game_id AND b.brand_id=s.brand_id JOIN configured_rule_heads h ON h.game_id=s.game_id AND h.version_id=s.rule_id WHERE s.game_id=g.id AND ($1='' OR s.brand_id=NULLIF($1,'')::uuid) AND s.enabled AND b.enabled) THEN 'ready'
 WHEN EXISTS(SELECT 1 FROM configured_sales s JOIN configured_rule_heads h ON h.game_id=s.game_id AND h.version_id=s.rule_id WHERE s.game_id=g.id AND ($1='' OR s.brand_id=NULLIF($1,'')::uuid)) THEN 'disabled'
 WHEN EXISTS(SELECT 1 FROM configured_sales s WHERE s.game_id=g.id AND ($1='' OR s.brand_id=NULLIF($1,'')::uuid)) THEN 'outdated'
 ELSE 'unconfigured' END`

func (s Service) ConnectedGames(ctx context.Context, brand string, limit, offset int) ([]Game, error) {
	rows, e := s.DB.Query(ctx, `SELECT `+gameFields+`,true,b.enabled,b.version,`+gameRulePublished+`,`+gameSalesStatus+` FROM platform_games g JOIN brand_platform_games b ON b.game_id=g.id WHERE b.brand_id=$1 AND b.enabled AND g.status='active' ORDER BY g.code,g.id LIMIT $2 OFFSET $3`, brand, limit, offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Game{}
	for rows.Next() {
		g, e := scanGame(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s Service) Games(ctx context.Context, brand string, limit, offset int) ([]Game, error) {
	rows, e := s.DB.Query(ctx, `SELECT `+gameFields+`,EXISTS(SELECT 1 FROM brand_platform_games b WHERE b.game_id=g.id AND b.brand_id=NULLIF($1,'')::uuid),coalesce((SELECT enabled FROM brand_platform_games b WHERE b.game_id=g.id AND b.brand_id=NULLIF($1,'')::uuid),false),coalesce((SELECT version FROM brand_platform_games b WHERE b.game_id=g.id AND b.brand_id=NULLIF($1,'')::uuid),0),`+gameRulePublished+`,`+gameSalesStatus+` FROM platform_games g WHERE g.status<>'deleted' ORDER BY g.code,g.id LIMIT $2 OFFSET $3`, brand, limit, offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Game{}
	for rows.Next() {
		g, e := scanGame(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
func readGame(ctx context.Context, q queryer, id string) (Game, error) {
	return scanGame(q.QueryRow(ctx, `SELECT `+gameFields+`,false,false,0,`+gameRulePublished+`,`+strings.ReplaceAll(gameSalesStatus, "$1", "''")+` FROM platform_games g WHERE g.id=$1 AND g.status<>'deleted'`, id))
}
func (s Service) Detail(ctx context.Context, brand, id string, limit, offset int) (Detail, error) {
	out := Detail{Plays: []Play{}, Periods: []Period{}}
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	// History remains readable after catalogue deletion; writes still use lockedGame.
	out.Game, e = scanGame(tx.QueryRow(ctx, `SELECT `+gameFields+`,false,false,0,`+gameRulePublished+`,`+strings.ReplaceAll(gameSalesStatus, "$1", "$2")+` FROM platform_games g WHERE g.id=$1`, id, brand))
	if e != nil {
		return out, e
	}
	if brand != "" {
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brand_platform_games WHERE brand_id=$1 AND game_id=$2),coalesce((SELECT enabled FROM brand_platform_games WHERE brand_id=$1 AND game_id=$2),false),coalesce((SELECT version FROM brand_platform_games WHERE brand_id=$1 AND game_id=$2),0)`, brand, id).Scan(&out.Game.Connected, &out.Game.BrandEnabled, &out.Game.BrandVersion)
		if e != nil {
			return out, e
		}
		if !out.Game.Connected {
			return out, ErrNotFound
		}
	}
	rows, e := tx.Query(ctx, `SELECT id::text,code,name,definition,definition_hash FROM platform_game_plays WHERE game_id=$1 ORDER BY code,id`, id)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var p Play
		var raw []byte
		if e = rows.Scan(&p.ID, &p.Code, &p.Name, &raw, &p.DefinitionHash); e != nil {
			rows.Close()
			return out, e
		}
		if e = json.Unmarshal(raw, &p.Definition); e != nil {
			rows.Close()
			return out, e
		}
		out.Plays = append(out.Plays, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	rows, e = tx.Query(ctx, `SELECT id::text,game_id::text,period_no,bet_start_at,bet_end_at,draw_at,result,published_at FROM platform_game_periods WHERE game_id=$1 ORDER BY draw_at DESC,id DESC LIMIT $2 OFFSET $3`, id, limit, offset)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var p Period
		var raw []byte
		if e = rows.Scan(&p.ID, &p.GameID, &p.PeriodNo, &p.BetStartAt, &p.BetEndAt, &p.DrawAt, &raw, &p.PublishedAt); e != nil {
			rows.Close()
			return out, e
		}
		if len(raw) > 0 {
			p.Result = &rules.Draw{}
			if e = json.Unmarshal(raw, p.Result); e != nil {
				rows.Close()
				return out, e
			}
		}
		out.Periods = append(out.Periods, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	var configured bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM configured_rule_heads WHERE game_id=$1)`, id).Scan(&configured); e != nil {
		return out, e
	}
	if !configured && rules.ValidateMarkSixModel(out.Game.Model) == nil {
		for i := range out.Periods {
			attributes, err := ReadPeriodMarkSixAttributes(ctx, tx, out.Periods[i].ID)
			if err != nil && !errors.Is(err, ErrAttributesMissing) {
				return out, err
			}
			out.Periods[i].Attributes = attributes
		}
	}
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform_mark_six_settings WHERE game_id=$1) OR EXISTS(SELECT 1 FROM platform_mark_six_attributes WHERE game_id=$1)`, id).Scan(&out.MarkSixConfigured); e != nil {
		return out, e
	}
	out.RuleProtocol = "generic"
	if configured {
		out.RuleProtocol = "configured"
	}
	return out, tx.Commit(ctx)
}
func (s Service) Create(ctx context.Context, tx pgx.Tx, a access.Account, in GameInput, meta points.Metadata) (Game, error) {
	if !Allowed(a, "", "write") {
		return Game{}, ErrDenied
	}
	if !codePattern.MatchString(in.Code) || !textOK(in.Name, 120) || len(in.Reason) > 500 || rules.ValidateModel(in.Model) != nil {
		return Game{}, ErrInvalid
	}
	if _, e := time.LoadLocation(in.Timezone); e != nil {
		return Game{}, ErrInvalid
	}
	raw, _ := json.Marshal(in.Model)
	id := ids.New()
	cmd, e := tx.Exec(ctx, `INSERT INTO platform_games(id,code,name,model,timezone,created_by) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(code) DO NOTHING`, id, in.Code, in.Name, raw, in.Timezone, a.ID)
	if e != nil {
		return Game{}, e
	}
	if cmd.RowsAffected() == 0 {
		return Game{}, ErrConflict
	}
	out, e := readGame(ctx, tx, id)
	if e != nil {
		return out, e
	}
	return out, record(ctx, tx, a, "", "platform_game.create", id, in.Reason, meta, out)
}
func (s Service) Connect(ctx context.Context, tx pgx.Tx, a access.Account, brand, id, reason string, meta points.Metadata) (Game, error) {
	if !Allowed(a, brand, "connect") {
		return Game{}, ErrDenied
	}
	if len(reason) > 500 {
		return Game{}, ErrInvalid
	}
	g, e := lockedGame(ctx, tx, id)
	if e != nil {
		return g, e
	}
	if g.Status != "active" {
		return g, ErrConflict
	}
	var active bool
	if e = tx.QueryRow(ctx, `SELECT status='active' FROM brands WHERE id=$1 FOR SHARE`, brand).Scan(&active); e != nil {
		return g, e
	}
	if !active {
		return g, ErrConflict
	}
	cmd, e := tx.Exec(ctx, `INSERT INTO brand_platform_games(brand_id,game_id,connected_by) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, brand, id, a.ID)
	if e != nil {
		return g, e
	}
	if cmd.RowsAffected() == 0 {
		return g, ErrConflict
	}
	g.Connected = true
	g.BrandEnabled = true
	g.BrandVersion = 1
	return g, record(ctx, tx, a, brand, "platform_game.connect", id, reason, meta, g)
}
func (s Service) CreatePlay(ctx context.Context, tx pgx.Tx, a access.Account, id string, in PlayInput, meta points.Metadata) (Play, error) {
	if !Allowed(a, "", "write") {
		return Play{}, ErrDenied
	}
	if !codePattern.MatchString(in.Code) || !textOK(in.Name, 120) || len(in.Reason) > 500 || rules.ValidateDefinition(in.Definition) != nil {
		return Play{}, ErrInvalid
	}
	g, e := lockedGame(ctx, tx, id)
	if e != nil {
		return Play{}, e
	}
	model, _ := json.Marshal(g.Model)
	defined, _ := json.Marshal(in.Definition.Model)
	if string(model) != string(defined) {
		return Play{}, ErrInvalid
	}
	raw, _ := json.Marshal(in.Definition)
	hash := sha256.Sum256(raw)
	p := Play{ID: ids.New(), Code: in.Code, Name: in.Name, Definition: in.Definition, DefinitionHash: hex.EncodeToString(hash[:])}
	cmd, e := tx.Exec(ctx, `INSERT INTO platform_game_plays(id,game_id,code,name,definition,definition_hash,created_by) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(game_id,code) DO NOTHING`, p.ID, id, p.Code, p.Name, raw, p.DefinitionHash, a.ID)
	if e != nil {
		return p, e
	}
	if cmd.RowsAffected() == 0 {
		return p, ErrConflict
	}
	return p, record(ctx, tx, a, "", "platform_game.play.create", p.ID, in.Reason, meta, p)
}
func (s Service) CreatePeriod(ctx context.Context, tx pgx.Tx, a access.Account, id string, in PeriodInput, meta points.Metadata) (Period, error) {
	if !Allowed(a, "", "write") {
		return Period{}, ErrDenied
	}
	if !textOK(in.PeriodNo, 100) || len(in.Reason) > 500 || !in.BetStartAt.Before(in.BetEndAt) || in.DrawAt.Before(in.BetEndAt) {
		return Period{}, ErrInvalid
	}
	g, e := lockedGame(ctx, tx, id)
	if e != nil {
		return Period{}, e
	}
	if g.Status != "active" {
		return Period{}, ErrConflict
	}
	var attributes *MarkSixAttributes
	var configured bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM configured_rule_heads WHERE game_id=$1)`, id).Scan(&configured); e != nil {
		return Period{}, e
	}
	var sourceRequiresAttributes bool
	if in.AutomationVersion != nil {
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform_automation_versions WHERE game_id=$1 AND version=$2 AND source_type='hk_marksix_6hch')`, id, *in.AutomationVersion).Scan(&sourceRequiresAttributes); e != nil {
			return Period{}, e
		}
	}
	if (!configured || sourceRequiresAttributes) && rules.ValidateMarkSixModel(g.Model) == nil {
		attributes, e = ReadEffectiveMarkSixAttributes(ctx, tx, id, in.DrawAt)
		if e != nil {
			return Period{}, e
		}
	}
	p := Period{Attributes: attributes, ID: ids.New(), GameID: id, PeriodNo: in.PeriodNo, BetStartAt: in.BetStartAt, BetEndAt: in.BetEndAt, DrawAt: in.DrawAt}
	cmd, e := tx.Exec(ctx, `INSERT INTO platform_game_periods(id,game_id,period_no,bet_start_at,bet_end_at,draw_at,automation_version) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(game_id,period_no) DO NOTHING`, p.ID, id, p.PeriodNo, p.BetStartAt, p.BetEndAt, p.DrawAt, in.AutomationVersion)
	if e != nil {
		return p, e
	}
	if cmd.RowsAffected() == 0 {
		return p, ErrConflict
	}
	if attributes != nil {
		if _, e = tx.Exec(ctx, `INSERT INTO platform_mark_six_period_attributes(period_id,game_id,attribute_id)VALUES($1,$2,$3)`, p.ID, id, attributes.ID); e != nil {
			return p, e
		}
	}
	return p, record(ctx, tx, a, "", "platform_game.period.create", p.ID, in.Reason, meta, p)
}
func (s Service) Publish(ctx context.Context, tx pgx.Tx, a access.Account, id string, result rules.Draw, reason string, meta points.Metadata) (Period, error) {
	result.Regular = append([]int{}, result.Regular...)
	result.Special = append([]int{}, result.Special...)
	result.Digits = append([]int{}, result.Digits...)
	if !Allowed(a, "", "write") {
		return Period{}, ErrDenied
	}
	if reason != "" && !textOK(reason, 500) {
		return Period{}, ErrInvalid
	}
	var p Period
	var model, previous []byte
	var now time.Time
	e := tx.QueryRow(ctx, `SELECT p.id::text,p.game_id::text,p.period_no,p.bet_start_at,p.bet_end_at,p.draw_at,p.result,g.model,clock_timestamp() FROM platform_game_periods p JOIN platform_games g ON g.id=p.game_id WHERE p.id=$1 FOR UPDATE OF p`, id).Scan(&p.ID, &p.GameID, &p.PeriodNo, &p.BetStartAt, &p.BetEndAt, &p.DrawAt, &previous, &model, &now)
	if errors.Is(e, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	if e != nil {
		return p, e
	}
	if len(previous) > 0 || now.Before(p.DrawAt) {
		return p, ErrConflict
	}
	var m rules.Model
	if json.Unmarshal(model, &m) != nil || rules.ValidateDraw(m, result) != nil {
		return p, ErrInvalid
	}
	if err := validateConfiguredPeriodDraw(ctx, tx, id, result); err != nil {
		return p, err
	}

	raw, _ := json.Marshal(result)
	p.Result = &result
	p.PublishedAt = &now
	_, e = tx.Exec(ctx, `UPDATE platform_game_periods SET result=$2,published_at=$3,published_by=$4 WHERE id=$1`, id, raw, now, a.ID)
	if e != nil {
		return p, e
	}
	return p, record(ctx, tx, a, "", "platform_game.result.publish", id, reason, meta, p)
}
