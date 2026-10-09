package attribution

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"strings"
	"time"
)

type Service struct{ DB *pgxpool.Pool }
type querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

const codeFields = `c.id::text,c.brand_id::text,c.kind,c.code,c.owner_member_id::text,c.agent_id::text,c.status,c.starts_at,c.expires_at,c.version,join_code_usable(c),c.created_at,c.updated_at`

func scanCode(row pgx.Row) (Code, error) {
	var c Code
	e := row.Scan(&c.ID, &c.BrandID, &c.Kind, &c.Code, &c.OwnerMemberID, &c.AgentID, &c.Status, &c.StartsAt, &c.ExpiresAt, &c.Version, &c.Usable, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	return c, DatabaseError(e)
}
func idsValid(values ...string) bool {
	for _, v := range values {
		if !uuid.MatchString(v) {
			return false
		}
	}
	return true
}
func (s Service) Lock(ctx context.Context, tx pgx.Tx, brand string) error {
	if tx == nil || !idsValid(brand) {
		return ErrInvalid
	}
	var v int64
	e := tx.QueryRow(ctx, `SELECT version FROM brand_agent_policies WHERE brand_id=$1 FOR SHARE`, brand).Scan(&v)
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return e
}
func get(ctx context.Context, q querier, brand, id string) (Code, error) {
	return scanCode(q.QueryRow(ctx, `SELECT `+codeFields+` FROM join_codes c WHERE c.brand_id=$1 AND c.id=$2`, brand, id))
}
func (s Service) Get(ctx context.Context, brand, id string) (Code, error) {
	if !idsValid(brand, id) {
		return Code{}, ErrInvalid
	}
	return get(ctx, s.DB, brand, id)
}
func allowed(a access.Account, brand string) bool {
	return !a.SuperAdmin && access.Authorize(a, "join_code", "write", access.ScopeBrand, brand)
}
func auditRevision(ctx context.Context, tx pgx.Tx, code Code, a access.Account, reason string, m points.Metadata, before any) (string, error) {
	action := "join_code.update"
	if code.Version == 1 {
		action = "join_code.create"
	}
	reason = strings.TrimSpace(reason)
	log, e := audit.Append(ctx, tx, audit.Record{BrandID: code.BrandID, ActorType: "admin", ActorID: a.ID, Action: action, ResourceType: "join_code", ResourceID: code.ID, Reason: reason, RequestID: m.RequestID, IP: m.IP, Before: before, After: code})
	if e != nil {
		return "", e
	}
	_, e = tx.Exec(ctx, `INSERT INTO join_code_revisions(id,brand_id,code_id,version,status,starts_at,expires_at,actor_id,reason,audit_log_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, ids.New(), code.BrandID, code.ID, code.Version, code.Status, code.StartsAt, code.ExpiresAt, a.ID, reason, log)
	return log, e
}
func (s Service) Create(ctx context.Context, tx pgx.Tx, brand string, a access.Account, in CreateInput, m points.Metadata) (Code, error) {
	if tx == nil || !idsValid(brand, a.ID, in.OwnerMemberID) || !validReason(in.Reason) || !validWindow(in.StartsAt, in.ExpiresAt) || (in.Kind != "agent" && in.Kind != "referral") || (in.Kind == "agent") != (in.AgentID != nil) || (in.AgentID != nil && !idsValid(*in.AgentID)) {
		return Code{}, ErrInvalid
	}
	if !allowed(a, brand) {
		return Code{}, ErrDenied
	}
	if e := s.Lock(ctx, tx, brand); e != nil {
		return Code{}, e
	}
	var owner bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brand_members WHERE brand_id=$1 AND id=$2)`, brand, in.OwnerMemberID).Scan(&owner); e != nil {
		return Code{}, e
	}
	if !owner {
		return Code{}, ErrNotFound
	}
	if in.AgentID != nil {
		var match bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=$1 AND id=$2 AND member_id=$3)`, brand, *in.AgentID, in.OwnerMemberID).Scan(&match); e != nil {
			return Code{}, e
		}
		if !match {
			return Code{}, ErrNotFound
		}
	}
	b := make([]byte, 12)
	if _, e := rand.Read(b); e != nil {
		return Code{}, e
	}
	id, code := ids.New(), strings.ToUpper(hex.EncodeToString(b))
	_, e := tx.Exec(ctx, `INSERT INTO join_codes(id,brand_id,kind,code,owner_member_id,agent_id,starts_at,expires_at,created_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, brand, in.Kind, code, in.OwnerMemberID, in.AgentID, in.StartsAt, in.ExpiresAt, a.ID)
	if e != nil {
		e = DatabaseError(e)
		if errors.Is(e, ErrUnavailable) {
			e = ErrState
		}
		return Code{}, e
	}
	out, e := get(ctx, tx, brand, id)
	if e == nil {
		out.AuditLogID, e = auditRevision(ctx, tx, out, a, in.Reason, m, nil)
	}
	return out, e
}
func equalTime(a, b *time.Time) bool {
	return a == nil && b == nil || a != nil && b != nil && a.Equal(*b)
}
func (s Service) Update(ctx context.Context, tx pgx.Tx, brand string, a access.Account, id string, in UpdateInput, m points.Metadata) (Code, error) {
	if tx == nil || !idsValid(brand, a.ID, id) || in.Version < 1 || !validReason(in.Reason) || !validWindow(in.StartsAt, in.ExpiresAt) || (in.Status != "active" && in.Status != "disabled") {
		return Code{}, ErrInvalid
	}
	if !allowed(a, brand) {
		return Code{}, ErrDenied
	}
	if e := s.Lock(ctx, tx, brand); e != nil {
		return Code{}, e
	}
	old, e := scanCode(tx.QueryRow(ctx, `SELECT `+codeFields+` FROM join_codes c WHERE c.brand_id=$1 AND c.id=$2 FOR UPDATE`, brand, id))
	if e != nil {
		return Code{}, e
	}
	if old.Version != in.Version || old.Version == math.MaxInt64 {
		return Code{}, ErrVersion
	}
	if old.Status == in.Status && equalTime(old.StartsAt, in.StartsAt) && equalTime(old.ExpiresAt, in.ExpiresAt) {
		return Code{}, ErrState
	}
	_, e = tx.Exec(ctx, `UPDATE join_codes SET status=$3,starts_at=$4,expires_at=$5,version=version+1 WHERE brand_id=$1 AND id=$2`, brand, id, in.Status, in.StartsAt, in.ExpiresAt)
	if e != nil {
		e = DatabaseError(e)
		if errors.Is(e, ErrUnavailable) {
			e = ErrState
		}
		return Code{}, e
	}
	out, e := get(ctx, tx, brand, id)
	if e == nil {
		out.AuditLogID, e = auditRevision(ctx, tx, out, a, in.Reason, m, old)
	}
	return out, e
}
func paging(limit, offset int) bool {
	return limit >= 1 && limit <= 100 && offset >= 0 && offset <= 1000000
}
func (s Service) List(ctx context.Context, brand string, kind, owner *string, limit, offset int) (Page, error) {
	out := Page{BrandID: brand, Kind: kind, OwnerMemberID: owner, Items: []Code{}, Limit: limit, Offset: offset}
	if !idsValid(brand) || !paging(limit, offset) || (owner != nil && !idsValid(*owner)) || (kind != nil && *kind != "agent" && *kind != "referral") {
		return out, ErrInvalid
	}
	var valid bool
	var raw []byte
	e := s.DB.QueryRow(ctx, `WITH selected AS(SELECT c.*,join_code_usable(c) AS usable FROM join_codes c WHERE c.brand_id=$1 AND ($2::text IS NULL OR c.kind=$2) AND ($3::uuid IS NULL OR c.owner_member_id=$3)),page AS(SELECT * FROM selected ORDER BY created_at,id LIMIT $4 OFFSET $5)
 SELECT EXISTS(SELECT 1 FROM brands WHERE id=$1) AND ($3::uuid IS NULL OR EXISTS(SELECT 1 FROM brand_members WHERE brand_id=$1 AND id=$3)),(SELECT count(*)::text FROM selected),COALESCE((SELECT jsonb_agg(to_jsonb(page)-'created_by' ORDER BY created_at,id)FROM page),'[]'::jsonb)`, brand, kind, owner, limit, offset).Scan(&valid, &out.TotalCount, &raw)
	if e == nil && !valid {
		e = ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(raw, &out.Items)
	}
	return out, e
}
func (s Service) SelfCodes(ctx context.Context, brand, member string, limit, offset int) (SelfPage, error) {
	p, e := s.List(ctx, brand, nil, &member, limit, offset)
	return SelfPage{BrandID: brand, MemberID: member, Items: p.Items, Limit: limit, Offset: offset, TotalCount: p.TotalCount}, e
}
func (s Service) History(ctx context.Context, brand, id string, limit, offset int) (History, error) {
	out := History{BrandID: brand, CodeID: id, Items: []Revision{}, Limit: limit, Offset: offset}
	if !idsValid(brand, id) || !paging(limit, offset) {
		return out, ErrInvalid
	}
	var valid bool
	var raw []byte
	e := s.DB.QueryRow(ctx, `WITH selected AS(SELECT * FROM join_code_revisions WHERE brand_id=$1 AND code_id=$2),page AS(SELECT * FROM selected ORDER BY version DESC LIMIT $3 OFFSET $4) SELECT EXISTS(SELECT 1 FROM join_codes WHERE brand_id=$1 AND id=$2),(SELECT count(*)::text FROM selected),COALESCE((SELECT jsonb_agg(to_jsonb(page) ORDER BY version DESC)FROM page),'[]'::jsonb)`, brand, id, limit, offset).Scan(&valid, &out.TotalCount, &raw)
	if e == nil && !valid {
		e = ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(raw, &out.Items)
	}
	return out, e
}
func (s Service) Attribution(ctx context.Context, brand, member string) (PublicAttribution, error) {
	out := PublicAttribution{BrandID: brand, MemberID: member}
	if !idsValid(brand, member) {
		return out, ErrInvalid
	}
	var snapshot []byte
	e := s.DB.QueryRow(ctx, `SELECT attribution_snapshot FROM brand_members WHERE brand_id=$1 AND id=$2`, brand, member).Scan(&snapshot)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	} else if e == nil {
		e = parsePublicAttribution(snapshot, &out)
	}
	return out, e
}

func parsePublicAttribution(raw []byte, out *PublicAttribution) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 14 {
		return ErrInvalid
	}
	for _, key := range []string{"schema_version", "join_method", "join_domain", "joined_at", "code_id", "code_version", "code", "code_kind", "owner_member_id", "agent_id", "referrer_member_id", "agent_path", "agent_configs_at_join", "agent_policy_at_join"} {
		if _, ok := fields[key]; !ok {
			return ErrInvalid
		}
	}
	var snapshot struct {
		SchemaVersion int       `json:"schema_version"`
		JoinMethod    string    `json:"join_method"`
		JoinedAt      time.Time `json:"joined_at"`
		CodeID        *string   `json:"code_id"`
		Code          *string   `json:"code"`
	}
	if json.Unmarshal(raw, &snapshot) != nil || snapshot.SchemaVersion != 1 ||
		(snapshot.JoinMethod != "domain" && snapshot.JoinMethod != "operator" && snapshot.JoinMethod != "agent_code" && snapshot.JoinMethod != "referral_code") || snapshot.JoinedAt.IsZero() ||
		(snapshot.CodeID == nil) != (snapshot.Code == nil) ||
		(snapshot.CodeID != nil && (!uuid.MatchString(*snapshot.CodeID) || !codePattern.MatchString(*snapshot.Code))) {
		return ErrInvalid
	}
	out.JoinMethod, out.JoinedAt, out.CodeID, out.SourceCode = snapshot.JoinMethod, snapshot.JoinedAt, snapshot.CodeID, snapshot.Code
	return nil
}
