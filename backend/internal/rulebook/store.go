// Package rulebook persists brand-owned rule versions and their review evidence.
// Administrative writes participate in the caller's authorization/idempotency transaction.
package rulebook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/periodgate"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalid    = errors.New("invalid rule workflow request")
	ErrDenied     = errors.New("rule workflow denied")
	ErrNotFound   = errors.New("rule workflow resource not found")
	ErrVersion    = errors.New("rule record version conflict")
	ErrState      = errors.New("rule state conflict")
	ErrValidation = errors.New("rule validation required")
)
var codePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,47}$`)
var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Store struct{ DB *pgxpool.Pool }
type Game struct {
	ID              string      `json:"id"`
	BrandID         string      `json:"brand_id"`
	Code            string      `json:"code"`
	Name            string      `json:"name"`
	Model           rules.Model `json:"model"`
	Timezone        string      `json:"timezone"`
	Status          string      `json:"status"`
	Version         int64       `json:"version"`
	StartedSequence int64       `json:"started_sequence"`
}
type Play struct {
	ID              string `json:"id"`
	BrandID         string `json:"brand_id"`
	GameID          string `json:"game_id"`
	Code            string `json:"code"`
	Name            string `json:"name"`
	Status          string `json:"status"`
	ActiveVersionID string `json:"active_version_id"`
	Version         int64  `json:"version"`
}
type Version struct {
	ID                string                  `json:"id"`
	BrandID           string                  `json:"brand_id"`
	GameID            string                  `json:"game_id"`
	PlayID            string                  `json:"play_id"`
	VersionNo         int64                   `json:"version_no"`
	Version           int64                   `json:"version"`
	Definition        rules.Definition        `json:"definition"`
	DefinitionHash    string                  `json:"definition_hash"`
	Status            string                  `json:"status"`
	EffectMode        string                  `json:"effect_mode"`
	CreatedBy         string                  `json:"created_by"`
	ReviewedBy        string                  `json:"reviewed_by"`
	ReviewComment     string                  `json:"review_comment"`
	CreatedAt         time.Time               `json:"created_at"`
	UpdatedAt         time.Time               `json:"updated_at"`
	EffectiveAt       *time.Time              `json:"effective_at,omitempty"`
	EffectivePeriodID string                  `json:"effective_period_id,omitempty"`
	EffectiveSequence *int64                  `json:"effective_sequence,omitempty"`
	SourceVersionID   string                  `json:"source_version_id,omitempty"`
	Validation        *rules.ValidationReport `json:"validation,omitempty"`
	AuditLogID        string                  `json:"audit_log_id,omitempty"`
}
type Period struct {
	ID          string    `json:"id"`
	BrandID     string    `json:"brand_id"`
	GameID      string    `json:"game_id"`
	PeriodNo    string    `json:"period_no"`
	Sequence    int64     `json:"sequence"`
	BetStartAt  time.Time `json:"bet_start_at"`
	BetEndAt    time.Time `json:"bet_end_at"`
	DrawAt      time.Time `json:"draw_at"`
	Status      string    `json:"status"`
	Version     int64     `json:"version"`
	ScheduleID  string    `json:"schedule_id,omitempty"`
	StateReason string    `json:"state_reason"`
}

func Allowed(a access.Account, brand, resource, action string) bool {
	if action == "view" {
		return access.Authorize(a, resource, action, access.ScopeBrand, brand) || access.Authorize(a, resource, action, access.ScopePlatform, "")
	}
	return !a.SuperAdmin && access.Authorize(a, resource, action, access.ScopeBrand, brand)
}
func validText(s string, max int) bool {
	return strings.TrimSpace(s) != "" && utf8.ValidString(s) && len(s) <= max
}
func modeOK(s string) bool { return s == "immediate" || s == "next_period" }
func check(tx pgx.Tx, brand string, a access.Account, resource, action, reason string) error {
	if tx == nil || !uuidPattern.MatchString(brand) || !validText(reason, 500) {
		return ErrInvalid
	}
	if !Allowed(a, brand, resource, action) {
		return ErrDenied
	}
	return nil
}
func hashDefinition(d rules.Definition) ([]byte, string, error) {
	raw, e := json.Marshal(d)
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:]), e
}
func canonicalModel(m rules.Model) string {
	m.RegularPool.Values = append([]int{}, m.RegularPool.Values...)
	m.SpecialPool.Values = append([]int{}, m.SpecialPool.Values...)
	sort.Ints(m.RegularPool.Values)
	sort.Ints(m.SpecialPool.Values)
	raw, _ := json.Marshal(m)
	return string(raw)
}
func logChange(ctx context.Context, tx pgx.Tx, brand string, a access.Account, action, id, reason string, meta points.Metadata, before, after any) (string, error) {
	return audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: action, ResourceType: "rule_workflow", ResourceID: id, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, Before: before, After: after})
}
func snapshot(v Version) map[string]any {
	return map[string]any{"id": v.ID, "play_id": v.PlayID, "version_no": v.VersionNo, "version": v.Version, "status": v.Status, "definition_hash": v.DefinitionHash, "effect_mode": v.EffectMode, "reviewed_by": v.ReviewedBy, "source_version_id": v.SourceVersionID, "effective_sequence": v.EffectiveSequence, "effective_period_id": v.EffectivePeriodID}
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

const gameFields = `id::text,brand_id::text,code,name,model,timezone,status,version,started_sequence`

func scanGame(row pgx.Row) (g Game, e error) {
	var raw []byte
	e = row.Scan(&g.ID, &g.BrandID, &g.Code, &g.Name, &raw, &g.Timezone, &g.Status, &g.Version, &g.StartedSequence)
	if errors.Is(e, pgx.ErrNoRows) {
		return g, ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(raw, &g.Model)
	}
	return
}

const playFields = `id::text,brand_id::text,game_id::text,code,name,status,coalesce(active_version_id::text,''),version`

func scanPlay(row pgx.Row) (p Play, e error) {
	e = row.Scan(&p.ID, &p.BrandID, &p.GameID, &p.Code, &p.Name, &p.Status, &p.ActiveVersionID, &p.Version)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	return
}

const versionFields = `id::text,brand_id::text,game_id::text,play_id::text,version_no,version,definition,definition_sha256,status,effect_mode,created_by::text,coalesce(reviewed_by::text,''),review_comment,created_at,updated_at,effective_at,coalesce(effective_period_id::text,''),effective_sequence,coalesce(source_version_id::text,''),validation`

func scanVersion(row pgx.Row) (v Version, e error) {
	var definition, validation []byte
	e = row.Scan(&v.ID, &v.BrandID, &v.GameID, &v.PlayID, &v.VersionNo, &v.Version, &definition, &v.DefinitionHash, &v.Status, &v.EffectMode, &v.CreatedBy, &v.ReviewedBy, &v.ReviewComment, &v.CreatedAt, &v.UpdatedAt, &v.EffectiveAt, &v.EffectivePeriodID, &v.EffectiveSequence, &v.SourceVersionID, &validation)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if e != nil {
		return
	}
	if e = json.Unmarshal(definition, &v.Definition); e != nil {
		return
	}
	if len(validation) > 0 {
		v.Validation = &rules.ValidationReport{}
		e = json.Unmarshal(validation, v.Validation)
	}
	return
}
func (s Store) Games(ctx context.Context, brand string, limit, offset int) ([]Game, error) {
	out := []Game{}
	rows, e := s.DB.Query(ctx, `SELECT `+gameFields+` FROM games WHERE brand_id=$1 ORDER BY code LIMIT $2 OFFSET $3`, brand, limit, offset)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		g, e := scanGame(rows)
		if e != nil {
			return out, e
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
func (s Store) Plays(ctx context.Context, brand, game string, limit, offset int) ([]Play, error) {
	if _, e := scanGame(s.DB.QueryRow(ctx, `SELECT `+gameFields+` FROM games WHERE brand_id=$1 AND id=$2`, brand, game)); e != nil {
		return nil, e
	}
	out := []Play{}
	rows, e := s.DB.Query(ctx, `SELECT `+playFields+` FROM play_definitions WHERE brand_id=$1 AND game_id=$2 ORDER BY code LIMIT $3 OFFSET $4`, brand, game, limit, offset)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		p, e := scanPlay(rows)
		if e != nil {
			return out, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s Store) GetVersion(ctx context.Context, brand, id string) (Version, error) {
	return scanVersion(s.DB.QueryRow(ctx, `SELECT `+versionFields+` FROM rule_versions WHERE brand_id=$1 AND id=$2`, brand, id))
}
func (s Store) Versions(ctx context.Context, brand, play string, limit, offset int) ([]Version, error) {
	var one int
	if e := s.DB.QueryRow(ctx, `SELECT 1 FROM play_definitions WHERE brand_id=$1 AND id=$2`, brand, play).Scan(&one); errors.Is(e, pgx.ErrNoRows) {
		return nil, ErrNotFound
	} else if e != nil {
		return nil, e
	}
	out := []Version{}
	rows, e := s.DB.Query(ctx, `SELECT `+versionFields+` FROM rule_versions WHERE brand_id=$1 AND play_id=$2 ORDER BY version_no DESC LIMIT $3 OFFSET $4`, brand, play, limit, offset)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		v, e := scanVersion(rows)
		if e != nil {
			return out, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s Store) CreateGame(ctx context.Context, tx pgx.Tx, brand string, a access.Account, code, name string, model rules.Model, zone, reason string, meta points.Metadata) (Game, error) {
	if e := check(tx, brand, a, "game", "write", reason); e != nil {
		return Game{}, e
	}
	if !codePattern.MatchString(code) || !validText(name, 120) || rules.ValidateModel(model) != nil || len(zone) > 100 {
		return Game{}, ErrInvalid
	}
	if _, e := time.LoadLocation(zone); e != nil {
		return Game{}, ErrInvalid
	}
	var one int
	if e := tx.QueryRow(ctx, `SELECT 1 FROM brands WHERE id=$1 FOR UPDATE`, brand).Scan(&one); e != nil {
		return Game{}, e
	}
	var exists bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM games WHERE brand_id=$1 AND code=$2)`, brand, code).Scan(&exists); e != nil {
		return Game{}, e
	}
	if exists {
		return Game{}, ErrVersion
	}
	raw, _ := json.Marshal(model)
	id := ids.New()
	g, e := scanGame(tx.QueryRow(ctx, `INSERT INTO games(id,brand_id,code,name,model,timezone,created_by) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING `+gameFields, id, brand, code, name, raw, zone, a.ID))
	if e != nil {
		return g, e
	}
	_, e = logChange(ctx, tx, brand, a, "game.create", id, reason, meta, nil, g)
	return g, e
}
func lockGame(ctx context.Context, tx pgx.Tx, brand, id string) (Game, error) {
	return scanGame(tx.QueryRow(ctx, `SELECT `+gameFields+` FROM games WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, id))
}
func (s Store) CreatePlay(ctx context.Context, tx pgx.Tx, brand string, a access.Account, game, code, name, reason string, meta points.Metadata) (Play, error) {
	if e := check(tx, brand, a, "game", "write", reason); e != nil {
		return Play{}, e
	}
	if !uuidPattern.MatchString(game) || !codePattern.MatchString(code) || !validText(name, 120) {
		return Play{}, ErrInvalid
	}
	if _, e := lockGame(ctx, tx, brand, game); e != nil {
		return Play{}, e
	}
	var exists bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM play_definitions WHERE brand_id=$1 AND game_id=$2 AND code=$3)`, brand, game, code).Scan(&exists); e != nil {
		return Play{}, e
	}
	if exists {
		return Play{}, ErrVersion
	}
	id := ids.New()
	p, e := scanPlay(tx.QueryRow(ctx, `INSERT INTO play_definitions(id,brand_id,game_id,code,name,created_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING `+playFields, id, brand, game, code, name, a.ID))
	if e != nil {
		return p, e
	}
	_, e = logChange(ctx, tx, brand, a, "play.create", id, reason, meta, nil, p)
	return p, e
}
func lockPlay(ctx context.Context, tx pgx.Tx, brand, play string) (Game, Play, error) {
	var game string
	e := tx.QueryRow(ctx, `SELECT game_id::text FROM play_definitions WHERE brand_id=$1 AND id=$2`, brand, play).Scan(&game)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	if e != nil {
		return Game{}, Play{}, e
	}
	g, e := lockGame(ctx, tx, brand, game)
	if e != nil {
		return g, Play{}, e
	}
	p, e := scanPlay(tx.QueryRow(ctx, `SELECT `+playFields+` FROM play_definitions WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, play))
	return g, p, e
}
func lockedVersion(ctx context.Context, tx pgx.Tx, brand, id string, version int64) (Game, Play, Version, error) {
	if !uuidPattern.MatchString(id) || version < 1 || version == math.MaxInt64 {
		return Game{}, Play{}, Version{}, ErrInvalid
	}
	var play string
	e := tx.QueryRow(ctx, `SELECT play_id::text FROM rule_versions WHERE brand_id=$1 AND id=$2`, brand, id).Scan(&play)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	if e != nil {
		return Game{}, Play{}, Version{}, e
	}
	g, p, e := lockPlay(ctx, tx, brand, play)
	if e != nil {
		return g, p, Version{}, e
	}
	v, e := scanVersion(tx.QueryRow(ctx, `SELECT `+versionFields+` FROM rule_versions WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, id))
	if e != nil {
		return g, p, v, e
	}
	if version < 1 || version == math.MaxInt64 {
		return g, p, v, ErrInvalid
	}
	if v.Version != version {
		return g, p, v, ErrVersion
	}
	return g, p, v, nil
}
func createDraft(ctx context.Context, tx pgx.Tx, brand string, a access.Account, g Game, p Play, d rules.Definition, mode, source string) (Version, error) {
	if !modeOK(mode) || rules.ValidateDefinition(d) != nil || canonicalModel(g.Model) != canonicalModel(d.Model) {
		return Version{}, ErrInvalid
	}
	var no int64
	if e := tx.QueryRow(ctx, `SELECT next_version_no FROM play_definitions WHERE id=$1`, p.ID).Scan(&no); e != nil {
		return Version{}, e
	}
	if no == math.MaxInt64 {
		return Version{}, ErrVersion
	}
	raw, hash, e := hashDefinition(d)
	if e != nil {
		return Version{}, e
	}
	id := ids.New()
	v, e := scanVersion(tx.QueryRow(ctx, `INSERT INTO rule_versions(id,brand_id,game_id,play_id,version_no,definition,definition_sha256,effect_mode,created_by,source_version_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,'')::uuid) RETURNING `+versionFields, id, brand, g.ID, p.ID, no, raw, hash, mode, a.ID, source))
	if e != nil {
		return v, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO rule_version_contributors(rule_version_id,admin_id) VALUES($1,$2)`, id, a.ID); e != nil {
		return v, e
	}
	_, e = tx.Exec(ctx, `UPDATE play_definitions SET next_version_no=next_version_no+1 WHERE id=$1`, p.ID)
	return v, e
}
func (s Store) CreateVersion(ctx context.Context, tx pgx.Tx, brand string, a access.Account, play string, d rules.Definition, mode, reason string, meta points.Metadata) (Version, error) {
	if e := check(tx, brand, a, "rule", "write", reason); e != nil {
		return Version{}, e
	}
	if !uuidPattern.MatchString(play) {
		return Version{}, ErrInvalid
	}
	g, p, e := lockPlay(ctx, tx, brand, play)
	if e != nil {
		return Version{}, e
	}
	v, e := createDraft(ctx, tx, brand, a, g, p, d, mode, "")
	if e != nil {
		return v, e
	}
	v.AuditLogID, e = logChange(ctx, tx, brand, a, "rule.draft.create", v.ID, reason, meta, nil, snapshot(v))
	return v, e
}
func (s Store) UpdateVersion(ctx context.Context, tx pgx.Tx, brand string, a access.Account, id string, version int64, d rules.Definition, mode, reason string, meta points.Metadata) (Version, error) {
	if e := check(tx, brand, a, "rule", "write", reason); e != nil {
		return Version{}, e
	}
	g, _, v, e := lockedVersion(ctx, tx, brand, id, version)
	if e != nil {
		return v, e
	}
	if v.Status != "draft" {
		return v, ErrState
	}
	if !modeOK(mode) || rules.ValidateDefinition(d) != nil || canonicalModel(g.Model) != canonicalModel(d.Model) {
		return v, ErrInvalid
	}
	before := snapshot(v)
	raw, hash, _ := hashDefinition(d)
	if v.SourceVersionID != "" && hash != v.DefinitionHash {
		return v, ErrState
	}
	v, e = scanVersion(tx.QueryRow(ctx, `UPDATE rule_versions SET definition=$2,definition_sha256=$3,effect_mode=$4,validation=NULL,version=version+1,updated_at=now() WHERE id=$1 RETURNING `+versionFields, id, raw, hash, mode))
	if e != nil {
		return v, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO rule_version_contributors(rule_version_id,admin_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, a.ID); e != nil {
		return v, e
	}
	v.AuditLogID, e = logChange(ctx, tx, brand, a, "rule.draft.update", id, reason, meta, before, snapshot(v))
	return v, e
}
func (s Store) Validate(ctx context.Context, tx pgx.Tx, brand string, a access.Account, id string, version int64, cases []rules.ValidationCase, reason string, meta points.Metadata) (Version, error) {
	if e := check(tx, brand, a, "rule", "validate", reason); e != nil {
		return Version{}, e
	}
	_, _, v, e := lockedVersion(ctx, tx, brand, id, version)
	if e != nil {
		return v, e
	}
	if v.Status != "draft" {
		return v, ErrState
	}
	report, e := rules.ValidateCases(ctx, v.Definition, cases)
	if e != nil {
		return v, e
	}
	before := snapshot(v)
	raw, _ := json.Marshal(report)
	v, e = scanVersion(tx.QueryRow(ctx, `UPDATE rule_versions SET validation=$2,version=version+1,updated_at=now() WHERE id=$1 RETURNING `+versionFields, id, raw))
	if e != nil {
		return v, e
	}
	v.AuditLogID, e = logChange(ctx, tx, brand, a, "rule.draft.validate", id, reason, meta, before, map[string]any{"version": v.Version, "definition_hash": report.DefinitionHash, "passed": report.Passed, "case_count": len(report.Cases), "warnings": report.Warnings, "findings": report.Findings})
	return v, e
}
func (s Store) Submit(ctx context.Context, tx pgx.Tx, brand string, a access.Account, id string, version int64, reason string, meta points.Metadata) (Version, error) {
	if e := check(tx, brand, a, "rule", "submit", reason); e != nil {
		return Version{}, e
	}
	_, _, v, e := lockedVersion(ctx, tx, brand, id, version)
	if e != nil {
		return v, e
	}
	if v.Status != "draft" {
		return v, ErrState
	}
	if v.Validation == nil || !v.Validation.Passed || v.Validation.DefinitionHash != v.DefinitionHash {
		return v, ErrValidation
	}
	before := snapshot(v)
	v, e = scanVersion(tx.QueryRow(ctx, `UPDATE rule_versions SET status='pending_review',version=version+1,updated_at=now() WHERE id=$1 RETURNING `+versionFields, id))
	if e != nil {
		return v, e
	}
	v.AuditLogID, e = logChange(ctx, tx, brand, a, "rule.review.submit", id, reason, meta, before, snapshot(v))
	return v, e
}
func expireActive(ctx context.Context, tx pgx.Tx, p Play, v Version) error {
	if p.Version == math.MaxInt64 {
		return ErrVersion
	}
	if p.ActiveVersionID != "" {
		oldStatus := "expired"
		if v.SourceVersionID != "" {
			oldStatus = "rolled_back"
		}
		if _, e := tx.Exec(ctx, `UPDATE rule_versions SET status=$2,version=version+1,updated_at=now() WHERE id=$1`, p.ActiveVersionID, oldStatus); e != nil {
			return e
		}
	}
	return nil
}
func activate(ctx context.Context, tx pgx.Tx, brand string, p Play, v Version, period string) (Version, error) {
	if e := expireActive(ctx, tx, p, v); e != nil {
		return v, e
	}
	var e error
	v, e = scanVersion(tx.QueryRow(ctx, `UPDATE rule_versions SET status='active',effective_at=now(),effective_period_id=NULLIF($2,'')::uuid,version=version+1,updated_at=now() WHERE id=$1 RETURNING `+versionFields, v.ID, period))
	if e != nil {
		return v, e
	}
	_, e = tx.Exec(ctx, `UPDATE play_definitions SET active_version_id=$2,version=version+1 WHERE id=$1`, p.ID, v.ID)
	return v, e
}
func (s Store) Review(ctx context.Context, tx pgx.Tx, brand string, a access.Account, id string, version int64, approve, ack bool, reason string, meta points.Metadata) (Version, error) {
	if e := check(tx, brand, a, "rule", "review", reason); e != nil {
		return Version{}, e
	}
	g, p, v, e := lockedVersion(ctx, tx, brand, id, version)
	if e != nil {
		return v, e
	}
	if v.Status != "pending_review" {
		return v, ErrState
	}
	var contributed bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM rule_version_contributors WHERE rule_version_id=$1 AND admin_id=$2)`, id, a.ID).Scan(&contributed); e != nil {
		return v, e
	}
	if contributed || v.CreatedBy == a.ID {
		return v, ErrDenied
	}
	if v.Validation == nil || !v.Validation.Passed || v.Validation.DefinitionHash != v.DefinitionHash {
		return v, ErrValidation
	}
	if approve && len(v.Validation.Warnings) > 0 && !ack {
		return v, ErrValidation
	}
	before := snapshot(v)
	status := "rejected"
	if approve {
		var queued bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM rule_versions WHERE brand_id=$1 AND play_id=$2 AND status='approved')`, brand, p.ID).Scan(&queued); e != nil {
			return v, e
		}
		if queued {
			return v, ErrState
		}
		if g.StartedSequence == math.MaxInt64 {
			return v, ErrVersion
		}
		status = "approved"
	}
	if approve && v.EffectMode == "immediate" {
		status = "active"
		if e = expireActive(ctx, tx, p, v); e != nil {
			return v, e
		}
	}
	v, e = scanVersion(tx.QueryRow(ctx, `UPDATE rule_versions SET status=$2,reviewed_by=$3,reviewed_at=now(),review_comment=$4,effective_sequence=CASE WHEN $2='approved' THEN $5::bigint ELSE NULL END,effective_at=CASE WHEN $2='active' THEN now() ELSE NULL END,version=version+1,updated_at=now() WHERE id=$1 RETURNING `+versionFields, id, status, a.ID, reason, g.StartedSequence+1))
	if e != nil {
		return v, e
	}
	if status == "active" {
		if _, e = tx.Exec(ctx, `UPDATE play_definitions SET active_version_id=$2,version=version+1 WHERE id=$1`, p.ID, v.ID); e != nil {
			return v, e
		}
	}
	action := "rule.review.reject"
	if approve {
		action = "rule.review.approve"
	}
	after := snapshot(v)
	after["warnings_acknowledged"] = approve && ack
	after["previous_active_version_id"] = p.ActiveVersionID
	v.AuditLogID, e = logChange(ctx, tx, brand, a, action, id, reason, meta, before, after)
	return v, e
}
func (s Store) Clone(ctx context.Context, tx pgx.Tx, brand string, a access.Account, id, mode, reason string, meta points.Metadata) (Version, error) {
	if e := check(tx, brand, a, "rule", "write", reason); e != nil {
		return Version{}, e
	}
	source, e := scanVersion(tx.QueryRow(ctx, `SELECT `+versionFields+` FROM rule_versions WHERE brand_id=$1 AND id=$2`, brand, id))
	if e != nil {
		return source, e
	}
	if source.Status != "active" && source.Status != "expired" && source.Status != "rolled_back" {
		return source, ErrState
	}
	g, p, e := lockPlay(ctx, tx, brand, source.PlayID)
	if e != nil {
		return source, e
	}
	v, e := createDraft(ctx, tx, brand, a, g, p, source.Definition, mode, source.ID)
	if e != nil {
		return v, e
	}
	v.AuditLogID, e = logChange(ctx, tx, brand, a, "rule.rollback.draft", v.ID, reason, meta, snapshot(source), snapshot(v))
	return v, e
}

// OpenPeriod is an internal scheduler integration, not an admin "activate now"
// shortcut. Approval and opening serialize on the game row. The time window must
// actually be open according to the database clock; S4-c adds schedule generation.
func (s Store) OpenPeriod(ctx context.Context, tx pgx.Tx, brand, game, no string, start, end, draw time.Time) (Period, error) {
	out := Period{}
	if tx == nil || !uuidPattern.MatchString(brand) || !uuidPattern.MatchString(game) || !validText(no, 80) || !start.Before(end) || end.After(draw) {
		return out, ErrInvalid
	}
	g, e := lockGame(ctx, tx, brand, game)
	if e != nil {
		return out, e
	}
	var now time.Time
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return out, e
	}
	if start.After(now) || !end.After(now) || g.Status != "active" || g.StartedSequence == math.MaxInt64 || g.Version == math.MaxInt64 {
		return out, ErrInvalid
	}
	var exists bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM periods WHERE brand_id=$1 AND game_id=$2 AND period_no=$3)`, brand, game, no).Scan(&exists); e != nil {
		return out, e
	}
	if exists {
		return out, ErrState
	}
	var seq int64
	if e = tx.QueryRow(ctx, `SELECT coalesce(max(sequence),0) FROM periods WHERE brand_id=$1 AND game_id=$2`, brand, game).Scan(&seq); e != nil {
		return out, e
	}
	if seq == math.MaxInt64 {
		return out, ErrVersion
	}
	if e = periodgate.Check(ctx, tx, brand, game, seq+1, draw); e != nil {
		if errors.Is(e, periodgate.ErrBlocked) {
			return out, ErrState
		}
		return out, e
	}
	out = Period{ID: ids.New(), BrandID: brand, GameID: game, PeriodNo: no, Sequence: seq + 1, BetStartAt: start, BetEndAt: end, DrawAt: draw, Status: "betting", Version: 1}
	if _, e = tx.Exec(ctx, `INSERT INTO periods(id,brand_id,game_id,period_no,sequence,bet_start_at,bet_end_at,draw_at,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'betting')`, out.ID, brand, game, no, out.Sequence, start, end, draw); e != nil {
		return out, e
	}
	return out, bindOpening(ctx, tx, g, out)
}

// The activation ordinal counts actual openings, not reservations in a generated
// calendar. Skipped pending periods must not activate next-period rules.
func bindOpening(ctx context.Context, tx pgx.Tx, g Game, out Period) error {
	brand, game := g.BrandID, g.ID
	ordinal := g.StartedSequence + 1
	rows, e := tx.Query(ctx, `SELECT id::text FROM play_definitions WHERE brand_id=$1 AND game_id=$2 ORDER BY id FOR UPDATE`, brand, game)
	if e != nil {
		return e
	}
	plays := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		plays = append(plays, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, play := range plays {
		p, e := scanPlay(tx.QueryRow(ctx, `SELECT `+playFields+` FROM play_definitions WHERE id=$1`, play))
		if e != nil {
			return e
		}
		v, e := scanVersion(tx.QueryRow(ctx, `SELECT `+versionFields+` FROM rule_versions WHERE brand_id=$1 AND play_id=$2 AND status='approved' AND effective_sequence<=$3 FOR UPDATE`, brand, play, ordinal))
		if errors.Is(e, ErrNotFound) {
			continue
		}
		if e != nil {
			return e
		}
		before := snapshot(v)
		v, e = activate(ctx, tx, brand, p, v, out.ID)
		if e != nil {
			return e
		}
		after := snapshot(v)
		after["previous_active_version_id"] = p.ActiveVersionID
		if _, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "system", Action: "rule.period.activate", ResourceType: "rule_version", ResourceID: v.ID, Reason: "new betting period opened", RequestID: out.ID, Before: before, After: after}); e != nil {
			return e
		}
	}
	if _, e = tx.Exec(ctx, `INSERT INTO period_rule_versions(brand_id,game_id,period_id,play_id,rule_version_id) SELECT brand_id,game_id,$3,id,active_version_id FROM play_definitions WHERE brand_id=$1 AND game_id=$2 AND active_version_id IS NOT NULL`, brand, game, out.ID); e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE games SET started_sequence=$2,version=version+1 WHERE id=$1`, game, ordinal)
	return e
}
