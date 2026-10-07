package agency

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"strings"
)

type Service struct{ DB *pgxpool.Pool }
type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func idsValid(values ...string) bool {
	for _, s := range values {
		if !uuid.MatchString(s) {
			return false
		}
	}
	return true
}
func adminAllowed(a access.Account, brand, resource string) bool {
	return !a.SuperAdmin && access.Authorize(a, resource, "write", access.ScopeBrand, brand)
}
func scanPolicy(row pgx.Row) (Policy, error) {
	var p Policy
	var raw []byte
	e := row.Scan(&p.BrandID, &p.Version, &raw, &p.UpdatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(raw, &p.Config)
	}
	return p, e
}
func (s Service) Policy(ctx context.Context, brand string) (Policy, error) {
	if !idsValid(brand) {
		return Policy{}, ErrInvalid
	}
	return scanPolicy(s.DB.QueryRow(ctx, `SELECT brand_id::text,version,config,updated_at FROM brand_agent_policies WHERE brand_id=$1`, brand))
}

// Every mutation takes this mutex before locking a node. Config changes are rare
// administrative operations, independent of the high-throughput betting locks.
func (s Service) LockPolicy(ctx context.Context, tx pgx.Tx, brand string) (Policy, error) {
	if tx == nil || !idsValid(brand) {
		return Policy{}, ErrInvalid
	}
	// Match financial-policy edits and betting: financial -> agency -> wallet.
	var financialBrand string
	if err := tx.QueryRow(ctx, `SELECT brand_id::text FROM brand_commission_policies WHERE brand_id=$1 FOR SHARE`, brand).Scan(&financialBrand); err != nil {
		return Policy{}, err
	}
	return scanPolicy(tx.QueryRow(ctx, `SELECT brand_id::text,version,config,updated_at FROM brand_agent_policies WHERE brand_id=$1 FOR UPDATE`, brand))
}

const nodeFields = `n.id::text,n.brand_id::text,n.member_id::text,n.parent_id::text,n.depth,n.path::text[],n.version,n.config,n.created_by::text,n.created_at,n.updated_at,p.version,
 (SELECT version FROM agent_nodes WHERE brand_id=n.brand_id AND id=n.parent_id),
 coalesce((SELECT config->>'mode' FROM agent_nodes WHERE brand_id=n.brand_id AND id=ANY(n.path) AND config->'mode'<>'null'::jsonb ORDER BY depth DESC LIMIT 1),p.config->>'mode'),
 (SELECT id::text FROM agent_nodes WHERE brand_id=n.brand_id AND id=ANY(n.path) AND config->'mode'<>'null'::jsonb ORDER BY depth DESC LIMIT 1)`

func scanNode(row pgx.Row) (Node, error) {
	var n Node
	var raw []byte
	e := row.Scan(&n.ID, &n.BrandID, &n.MemberID, &n.ParentID, &n.Depth, &n.Path, &n.Version, &raw, &n.CreatedBy, &n.CreatedAt, &n.UpdatedAt, &n.PolicyVersion, &n.ParentVersion, &n.EffectiveMode, &n.ModeSourceAgentID)
	if errors.Is(e, pgx.ErrNoRows) {
		return n, ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(raw, &n.Config)
	}
	return n, e
}
func node(ctx context.Context, q rowQuerier, brand, id string) (Node, error) {
	return scanNode(q.QueryRow(ctx, `SELECT `+nodeFields+` FROM agent_nodes n JOIN brand_agent_policies p ON p.brand_id=n.brand_id WHERE n.brand_id=$1 AND n.id=$2`, brand, id))
}
func (s Service) Node(ctx context.Context, brand, id string) (Node, error) {
	if !idsValid(brand, id) {
		return Node{}, ErrInvalid
	}
	return node(ctx, s.DB, brand, id)
}
func (s Service) Me(ctx context.Context, brand, member string) (Node, error) {
	if !idsValid(brand, member) {
		return Node{}, ErrInvalid
	}
	return scanNode(s.DB.QueryRow(ctx, `SELECT `+nodeFields+` FROM agent_nodes n JOIN brand_agent_policies p ON p.brand_id=n.brand_id WHERE n.brand_id=$1 AND n.member_id=$2`, brand, member))
}
func paging(limit, offset int) bool {
	return limit >= 1 && limit <= 100 && offset >= 0 && offset <= 1000000
}
func (s Service) Tree(ctx context.Context, brand string, parent *string, limit, offset int) (Tree, error) {
	out := Tree{BrandID: brand, ParentID: parent, Items: []Node{}, Limit: limit, Offset: offset}
	if !idsValid(brand) || !paging(limit, offset) || parent != nil && !idsValid(*parent) {
		return out, ErrInvalid
	}
	var valid bool
	var raw []byte
	// Use one JSON query with explicit DTO aliases rather than unstable duplicate
	// column names from nodeFields. This same statement keeps page/count coherent.
	sql := `WITH selected AS (SELECT n.id::text AS id,n.brand_id::text AS brand_id,n.member_id::text AS member_id,n.parent_id::text AS parent_id,n.depth,n.path::text[] AS path,n.version,n.config,n.created_by::text AS created_by,n.created_at,n.updated_at,p.version AS policy_version,
 (SELECT version FROM agent_nodes WHERE brand_id=n.brand_id AND id=n.parent_id) AS parent_version,
 coalesce((SELECT config->>'mode' FROM agent_nodes WHERE brand_id=n.brand_id AND id=ANY(n.path) AND config->'mode'<>'null'::jsonb ORDER BY depth DESC LIMIT 1),p.config->>'mode') AS effective_mode,
 (SELECT id::text FROM agent_nodes WHERE brand_id=n.brand_id AND id=ANY(n.path) AND config->'mode'<>'null'::jsonb ORDER BY depth DESC LIMIT 1) AS mode_source_agent_id
 FROM agent_nodes n JOIN brand_agent_policies p ON p.brand_id=n.brand_id WHERE n.brand_id=$1 AND n.parent_id IS NOT DISTINCT FROM $2::uuid),
 page AS (SELECT * FROM selected ORDER BY created_at,id LIMIT $3 OFFSET $4)
 SELECT EXISTS(SELECT 1 FROM brand_agent_policies WHERE brand_id=$1) AND ($2::uuid IS NULL OR EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=$1 AND id=$2)),
 (SELECT count(*)::text FROM selected),COALESCE((SELECT jsonb_agg(to_jsonb(page) ORDER BY created_at,id) FROM page),'[]'::jsonb)`
	e := s.DB.QueryRow(ctx, sql, brand, parent, limit, offset).Scan(&valid, &out.TotalCount, &raw)
	if e == nil && !valid {
		return out, ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(raw, &out.Items)
	}
	return out, e
}
func (s Service) History(ctx context.Context, brand string, agent *string, limit, offset int) (History, error) {
	out := History{BrandID: brand, AgentID: agent, Items: []Revision{}, Limit: limit, Offset: offset}
	if !idsValid(brand) || !paging(limit, offset) || agent != nil && !idsValid(*agent) {
		return out, ErrInvalid
	}
	var valid bool
	var raw []byte
	e := s.DB.QueryRow(ctx, `WITH selected AS (SELECT id::text,brand_id::text,agent_id::text,version,config,actor_type,actor_id::text,reason,created_at,audit_log_id::text FROM agent_config_revisions WHERE brand_id=$1 AND agent_id IS NOT DISTINCT FROM $2::uuid),page AS(SELECT * FROM selected ORDER BY version DESC LIMIT $3 OFFSET $4)
 SELECT CASE WHEN $2::uuid IS NULL THEN EXISTS(SELECT 1 FROM brand_agent_policies WHERE brand_id=$1) ELSE EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=$1 AND id=$2) END,(SELECT count(*)::text FROM selected),COALESCE((SELECT jsonb_agg(to_jsonb(page) ORDER BY version DESC) FROM page),'[]'::jsonb)`, brand, agent, limit, offset).Scan(&valid, &out.TotalCount, &raw)
	if e == nil && !valid {
		return out, ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(raw, &out.Items)
	}
	return out, e
}
func revision(ctx context.Context, tx pgx.Tx, brand string, agent *string, version int64, cfg any, reason string, m points.Metadata, before any) (string, error) {
	resource, id, action := "agent_policy", brand, "agent.policy.update"
	if agent != nil {
		resource, id, action = "agent_node", *agent, "agent.node.update"
		if version == 1 {
			action = "agent.node.create"
		}
	}
	log, e := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: m.ActorType, ActorID: m.ActorID, Action: action, ResourceType: resource, ResourceID: id, Reason: strings.TrimSpace(reason), RequestID: m.RequestID, IP: m.IP, Before: before, After: map[string]any{"version": version, "config": cfg}})
	if e != nil {
		return "", e
	}
	raw, e := json.Marshal(cfg)
	if e != nil {
		return "", e
	}
	_, e = tx.Exec(ctx, `INSERT INTO agent_config_revisions(id,brand_id,agent_id,version,config,actor_type,actor_id,reason,audit_log_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, ids.New(), brand, agent, version, raw, m.ActorType, m.ActorID, strings.TrimSpace(reason), log)
	return log, e
}
func (s Service) SavePolicy(ctx context.Context, tx pgx.Tx, brand string, a access.Account, in PolicyInput, m points.Metadata) (Policy, error) {
	if tx == nil || !idsValid(brand, a.ID) || in.Version < 1 || !in.Config.Valid() || !validReason(in.Reason) {
		return Policy{}, ErrInvalid
	}
	if !adminAllowed(a, brand, "agent_policy") {
		return Policy{}, ErrDenied
	}
	p, e := s.LockPolicy(ctx, tx, brand)
	if e != nil {
		return p, e
	}
	if p.Version != in.Version || p.Version == math.MaxInt64 {
		return p, ErrVersion
	}
	var financialConflict bool
	if e = tx.QueryRow(ctx, `SELECT config->'enabled'='true'::jsonb AND (NOT $2::boolean OR config->'calendar'->>'cycle' IS DISTINCT FROM $3::text) FROM brand_commission_policies WHERE brand_id=$1`, brand, in.Config.Enabled, in.Config.Cycle).Scan(&financialConflict); e != nil {
		return p, e
	}
	if financialConflict {
		return p, ErrState
	}
	cap, _ := RatioMicros(in.Config.RatioCap)
	var exceeds bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=$1 AND ((config->>'ratio')::numeric*1000000>$2 OR depth>$3))`, brand, cap, in.Config.MaxDepth).Scan(&exceeds)
	if e != nil {
		return p, e
	}
	if exceeds {
		return p, ErrLimit
	}
	if in.Config.Mode != p.Config.Mode {
		var mismatched bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(
 SELECT 1 FROM agent_nodes child JOIN agent_nodes parent
  ON parent.brand_id=child.brand_id AND parent.id=child.parent_id
 WHERE child.brand_id=$1
  AND agent_effective_mode_for_path(child.brand_id,child.path,NULL,NULL,$2)
      IS DISTINCT FROM agent_effective_mode_for_path(parent.brand_id,parent.path,NULL,NULL,$2)
 )`, brand, in.Config.Mode).Scan(&mismatched)
		if e != nil {
			return p, e
		}
		if mismatched {
			return p, ErrLimit
		}
	}
	old, _ := json.Marshal(p.Config)
	next, _ := json.Marshal(in.Config)
	if bytesEqual(old, next) {
		return p, ErrState
	}
	m.ActorType, m.ActorID = "admin", a.ID
	log, e := revision(ctx, tx, brand, nil, p.Version+1, in.Config, in.Reason, m, p)
	if e != nil {
		return p, e
	}
	_, e = tx.Exec(ctx, `UPDATE brand_agent_policies SET version=version+1,config=$2 WHERE brand_id=$1`, brand, next)
	if e != nil {
		return p, e
	}
	p, e = scanPolicy(tx.QueryRow(ctx, `SELECT brand_id::text,version,config,updated_at FROM brand_agent_policies WHERE brand_id=$1`, brand))
	p.AuditLogID = log
	return p, e
}
func bytesEqual(a, b []byte) bool { return string(a) == string(b) }
func (s Service) Create(ctx context.Context, tx pgx.Tx, brand string, a access.Account, in CreateInput, m points.Metadata) (Node, error) {
	if tx == nil || !idsValid(brand, a.ID, in.MemberID) || in.PolicyVersion < 1 || !in.Config.Valid() || !validReason(in.Reason) || (in.ParentID == nil) != (in.ParentVersion == nil) || in.ParentID != nil && (!idsValid(*in.ParentID) || *in.ParentVersion < 1) {
		return Node{}, ErrInvalid
	}
	if !adminAllowed(a, brand, "agent") {
		return Node{}, ErrDenied
	}
	p, e := s.LockPolicy(ctx, tx, brand)
	if e != nil {
		return Node{}, e
	}
	if p.Version != in.PolicyVersion {
		return Node{}, ErrVersion
	}
	if !p.Config.Enabled {
		return Node{}, ErrState
	}
	var found, normal, exists bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brand_members WHERE brand_id=$1 AND id=$2),EXISTS(SELECT 1 FROM brand_members WHERE brand_id=$1 AND id=$2 AND status='normal'),EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=$1 AND member_id=$2)`, brand, in.MemberID).Scan(&found, &normal, &exists)
	if e != nil {
		return Node{}, e
	}
	if !found {
		return Node{}, ErrNotFound
	}
	if !normal || exists {
		return Node{}, ErrState
	}
	id := ids.New()
	depth, path := 1, []string{id}
	ceiling, _ := RatioMicros(p.Config.RatioCap)
	if in.ParentID != nil {
		par, e := node(ctx, tx, brand, *in.ParentID)
		if e != nil {
			return Node{}, e
		}
		if par.Version != *in.ParentVersion {
			return Node{}, ErrVersion
		}
		ok, e := ancestorsActive(ctx, tx, par)
		if e != nil {
			return Node{}, e
		}
		if !ok || !par.Config.CanCreateChildren {
			return Node{}, ErrState
		}
		if in.Config.Mode != nil && *in.Config.Mode != par.EffectiveMode {
			return Node{}, ErrInvalid
		}
		var mismatched bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(
 SELECT 1 FROM agent_nodes child JOIN agent_nodes parent
  ON parent.brand_id=child.brand_id AND parent.id=child.parent_id
 WHERE child.brand_id=$1 AND child.id=ANY($2::uuid[])
  AND agent_effective_mode_for_path(child.brand_id,child.path,NULL,NULL,$3)
      IS DISTINCT FROM agent_effective_mode_for_path(parent.brand_id,parent.path,NULL,NULL,$3)
 )`, brand, par.Path, p.Config.Mode).Scan(&mismatched)
		if e != nil {
			return Node{}, e
		}
		if mismatched {
			return Node{}, ErrLimit
		}
		depth = par.Depth + 1
		path = append(append([]string{}, par.Path...), id)
		ceiling, _ = RatioMicros(par.Config.Ratio)
	}
	r, _ := RatioMicros(in.Config.Ratio)
	if r > ceiling || depth > p.Config.MaxDepth {
		return Node{}, ErrLimit
	}
	cfg, _ := json.Marshal(in.Config)
	_, e = tx.Exec(ctx, `INSERT INTO agent_nodes(id,brand_id,member_id,parent_id,depth,path,config,created_by) VALUES($1,$2,$3,$4,$5,$6::uuid[],$7,$8)`, id, brand, in.MemberID, in.ParentID, depth, path, cfg, a.ID)
	if e != nil {
		return Node{}, e
	}
	m.ActorType, m.ActorID = "admin", a.ID
	log, e := revision(ctx, tx, brand, &id, 1, in.Config, in.Reason, m, nil)
	if e != nil {
		return Node{}, e
	}
	out, e := node(ctx, tx, brand, id)
	out.AuditLogID = log
	return out, e
}
func ancestorsActive(ctx context.Context, tx pgx.Tx, n Node) (bool, error) {
	var ok bool
	e := tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=$1 AND id=ANY($2::uuid[]) AND config->>'status'<>'active')`, n.BrandID, n.Path).Scan(&ok)
	return ok, e
}
func (s Service) update(ctx context.Context, tx pgx.Tx, brand, id string, in UpdateInput, m points.Metadata) (Node, error) {
	p, e := s.LockPolicy(ctx, tx, brand)
	if e != nil {
		return Node{}, e
	}
	n, e := node(ctx, tx, brand, id)
	if e != nil {
		return n, e
	}
	if n.Version != in.Version || n.Version == math.MaxInt64 || p.Version != in.PolicyVersion {
		return n, ErrVersion
	}
	ceiling, _ := RatioMicros(p.Config.RatioCap)
	if n.ParentID == nil {
		if in.ParentVersion != nil {
			return n, ErrInvalid
		}
	} else {
		if in.ParentVersion == nil || n.ParentVersion == nil {
			return n, ErrInvalid
		}
		if *in.ParentVersion != *n.ParentVersion {
			return n, ErrVersion
		}
		par, e := node(ctx, tx, brand, *n.ParentID)
		if e != nil {
			return n, e
		}
		ceiling, _ = RatioMicros(par.Config.Ratio)
		if in.Config.Mode != nil && *in.Config.Mode != par.EffectiveMode {
			return n, ErrInvalid
		}
	}
	var mismatchedModes bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(
 SELECT 1 FROM agent_nodes child
 LEFT JOIN agent_nodes parent ON parent.brand_id=child.brand_id AND parent.id=child.parent_id
 WHERE child.brand_id=$1
  AND (child.path @> ARRAY[$2::uuid] OR child.id=ANY($5::uuid[]))
  AND child.parent_id IS NOT NULL
  AND agent_effective_mode_for_path(child.brand_id,child.path,$2,$3::jsonb,$4)
      IS DISTINCT FROM agent_effective_mode_for_path(parent.brand_id,parent.path,$2,$3::jsonb,$4)
	 )`, brand, id, mustJSON(in.Config), p.Config.Mode, n.Path).Scan(&mismatchedModes)
	if e != nil {
		return n, e
	}
	if mismatchedModes {
		return n, ErrLimit
	}
	r, _ := RatioMicros(in.Config.Ratio)
	var exceeds bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=$1 AND parent_id=$2 AND (config->>'ratio')::numeric*1000000>$3)`, brand, id, r).Scan(&exceeds)
	if e != nil {
		return n, e
	}
	if r > ceiling || exceeds {
		return n, ErrLimit
	}
	old, _ := json.Marshal(n.Config)
	cfg, _ := json.Marshal(in.Config)
	if bytesEqual(old, cfg) {
		return n, ErrState
	}
	log, e := revision(ctx, tx, brand, &id, n.Version+1, in.Config, in.Reason, m, n)
	if e != nil {
		return n, e
	}
	_, e = tx.Exec(ctx, `UPDATE agent_nodes SET version=version+1,config=$3 WHERE brand_id=$1 AND id=$2`, brand, id, cfg)
	if e != nil {
		return n, e
	}
	out, e := node(ctx, tx, brand, id)
	out.AuditLogID = log
	return out, e
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
func (s Service) Update(ctx context.Context, tx pgx.Tx, brand string, a access.Account, id string, in UpdateInput, m points.Metadata) (Node, error) {
	if tx == nil || !idsValid(brand, a.ID, id) || in.Version < 1 || in.PolicyVersion < 1 || !in.Config.Valid() || !validReason(in.Reason) {
		return Node{}, ErrInvalid
	}
	if !adminAllowed(a, brand, "agent") {
		return Node{}, ErrDenied
	}
	m.ActorType, m.ActorID = "admin", a.ID
	return s.update(ctx, tx, brand, id, in, m)
}

// CheckUserWrite is also called by the HTTP idempotency validator before cached
// receipts are returned. It locks the policy exclusively from the outset, never
// upgrading a shared config lock after waiting for an idempotency key.
func (s Service) CheckUserWrite(ctx context.Context, tx pgx.Tx, brand string, v identity.Session, id string) (Node, error) {
	if !idsValid(brand, v.Member.ID, v.User.ID, id) || v.Member.BrandID != brand || v.Member.Status != "normal" || v.User.Status != "active" {
		return Node{}, ErrDenied
	}
	p, e := s.LockPolicy(ctx, tx, brand)
	if e != nil {
		return Node{}, e
	}
	if !p.Config.Enabled {
		return Node{}, ErrDenied
	}
	own, e := scanNode(tx.QueryRow(ctx, `SELECT `+nodeFields+` FROM agent_nodes n JOIN brand_agent_policies p ON p.brand_id=n.brand_id WHERE n.brand_id=$1 AND n.member_id=$2`, brand, v.Member.ID))
	if e != nil {
		return Node{}, ErrDenied
	}
	active, e := ancestorsActive(ctx, tx, own)
	if e != nil {
		return own, e
	}
	if !active {
		return own, ErrDenied
	}
	child, e := node(ctx, tx, brand, id)
	if e != nil {
		return own, ErrDenied
	}
	if child.ParentID == nil || !strings.EqualFold(*child.ParentID, own.ID) || child.Config.Status != "active" {
		return own, ErrDenied
	}
	return child, nil
}
func (s Service) UpdateChild(ctx context.Context, tx pgx.Tx, brand string, v identity.Session, id string, in ChildInput, m points.Metadata) (Node, error) {
	if tx == nil || in.Version < 1 || in.PolicyVersion < 1 || in.ParentVersion < 1 || !validReason(in.Reason) {
		return Node{}, ErrInvalid
	}
	if _, e := RatioMicros(in.Ratio); e != nil || in.Mode != nil && !validMode(*in.Mode) {
		return Node{}, ErrInvalid
	}
	child, e := s.CheckUserWrite(ctx, tx, brand, v, id)
	if e != nil {
		return child, e
	}
	cfg := child.Config
	cfg.Ratio, cfg.Mode = in.Ratio, in.Mode
	m.ActorType, m.ActorID = "user", v.User.ID
	pv := in.ParentVersion
	return s.update(ctx, tx, brand, id, UpdateInput{Version: in.Version, PolicyVersion: in.PolicyVersion, ParentVersion: &pv, Config: cfg, Reason: in.Reason}, m)
}
