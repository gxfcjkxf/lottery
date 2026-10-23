package agency

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"regexp"
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
	if _, err := tx.Exec(ctx, `SELECT apply_due_agent_mode_changes($1)`, brand); err != nil {
		return Policy{}, err
	}
	return scanPolicy(tx.QueryRow(ctx, `SELECT brand_id::text,version,config,updated_at FROM brand_agent_policies WHERE brand_id=$1 FOR UPDATE`, brand))
}

const nodeFields = `n.id::text,n.brand_id::text,n.member_id::text,n.parent_id::text,n.depth,n.path::text[],n.version,n.config,n.created_by::text,n.created_at,n.updated_at,p.version,
 (SELECT version FROM agent_nodes_at(n.brand_id,statement_timestamp()) WHERE brand_id=n.brand_id AND id=n.parent_id),
 coalesce((SELECT config->>'mode' FROM agent_nodes_at(n.brand_id,statement_timestamp()) WHERE brand_id=n.brand_id AND id=ANY(n.path) AND config->'mode'<>'null'::jsonb ORDER BY depth DESC LIMIT 1),p.config->>'mode'),
 (SELECT id::text FROM agent_nodes_at(n.brand_id,statement_timestamp()) WHERE brand_id=n.brand_id AND id=ANY(n.path) AND config->'mode'<>'null'::jsonb ORDER BY depth DESC LIMIT 1),agent_pending_mode_change(n.brand_id,n.id)`

func scanNode(row pgx.Row) (Node, error) {
	var n Node
	var raw, pendingRaw []byte
	e := row.Scan(&n.ID, &n.BrandID, &n.MemberID, &n.ParentID, &n.Depth, &n.Path, &n.Version, &raw, &n.CreatedBy, &n.CreatedAt, &n.UpdatedAt, &n.PolicyVersion, &n.ParentVersion, &n.EffectiveMode, &n.ModeSourceAgentID, &pendingRaw)
	if errors.Is(e, pgx.ErrNoRows) {
		return n, ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(raw, &n.Config)
		if e == nil && pendingRaw != nil {
			e = json.Unmarshal(pendingRaw, &n.PendingChange)
		}
	}
	return n, e
}
func node(ctx context.Context, q rowQuerier, brand, id string) (Node, error) {
	return scanNode(q.QueryRow(ctx, `SELECT `+nodeFields+` FROM agent_nodes_at($1,statement_timestamp()) n JOIN brand_agent_policies p ON p.brand_id=n.brand_id WHERE n.brand_id=$1 AND n.id=$2`, brand, id))
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
	return scanNode(s.DB.QueryRow(ctx, `SELECT `+nodeFields+` FROM agent_nodes_at($1,statement_timestamp()) n JOIN brand_agent_policies p ON p.brand_id=n.brand_id WHERE n.brand_id=$1 AND n.member_id=$2`, brand, member))
}
func paging(limit, offset int) bool {
	return limit >= 1 && limit <= 100 && offset >= 0 && offset <= 1000000
}

const listedNodeFields = `n.id::text AS id,n.brand_id::text AS brand_id,n.member_id::text AS agent_account_id,n.parent_id::text AS parent_id,n.depth,n.path::text[] AS path,n.version,n.config,n.created_by::text AS created_by,n.created_at,n.updated_at,p.version AS policy_version,
 (SELECT version FROM agent_nodes_at(n.brand_id,statement_timestamp()) WHERE brand_id=n.brand_id AND id=n.parent_id) AS parent_version,
 coalesce((SELECT config->>'mode' FROM agent_nodes_at(n.brand_id,statement_timestamp()) WHERE brand_id=n.brand_id AND id=ANY(n.path) AND config->'mode'<>'null'::jsonb ORDER BY depth DESC LIMIT 1),p.config->>'mode') AS effective_mode,
 (SELECT id::text FROM agent_nodes_at(n.brand_id,statement_timestamp()) WHERE brand_id=n.brand_id AND id=ANY(n.path) AND config->'mode'<>'null'::jsonb ORDER BY depth DESC LIMIT 1) AS mode_source_agent_id,agent_pending_mode_change(n.brand_id,n.id) AS pending_change
`

func (s Service) List(ctx context.Context, brand string, filter DirectoryFilter, limit, offset int) (AgentList, error) {
	out := AgentList{BrandID: brand, Items: []DirectoryNode{}, Limit: limit, Offset: offset}
	if !idsValid(brand) || !paging(limit, offset) {
		return out, ErrInvalid
	}
	var valid bool
	var raw []byte
	err := s.DB.QueryRow(ctx, `WITH candidates AS (SELECT `+listedNodeFields+`,
 aa.username AS username,
 EXISTS(SELECT 1 FROM agent_sessions se JOIN brands br ON br.id=se.brand_id
 WHERE se.agent_account_id=aa.id AND se.brand_id=aa.brand_id
 AND se.last_seen_at>=now()-interval '5 minutes' AND se.revoked_at IS NULL AND se.expires_at>now()
 AND n.config->>'status'='active' AND br.status<>'disabled') AS online,
 CASE WHEN $6::boolean THEN (SELECT COALESCE(sum(pb.points),0)::text FROM point_accounts pa JOIN point_buckets pb ON pb.account_id=pa.id AND pb.brand_id=pa.brand_id
 WHERE pa.brand_id=aa.brand_id AND pa.brand_member_id=aa.id AND pb.state IN ('available','manual_frozen','system_frozen')) ELSE NULL END AS display_points
 FROM agent_nodes_at($1,statement_timestamp()) n JOIN brand_agent_policies p ON p.brand_id=n.brand_id JOIN agent_accounts aa ON aa.id=n.member_id AND aa.brand_id=n.brand_id
 WHERE n.brand_id=$1 AND ($4='' OR strpos(lower(concat_ws(' ',aa.username,aa.display_name)),lower($4))>0)),
 selected AS (SELECT * FROM candidates WHERE $5::boolean IS NULL OR online=$5),
 page AS (SELECT * FROM selected ORDER BY created_at,id LIMIT $2 OFFSET $3)
 SELECT EXISTS(SELECT 1 FROM brand_agent_policies WHERE brand_id=$1),
 (SELECT count(*)::text FROM selected),COALESCE((SELECT jsonb_agg(to_jsonb(page) ORDER BY created_at,id) FROM page),'[]'::jsonb)`, brand, limit, offset, filter.Search, filter.Online, filter.ViewPoints).Scan(&valid, &out.TotalCount, &raw)
	if err == nil && !valid {
		return out, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(raw, &out.Items)
	}
	return out, err
}
func (s Service) Tree(ctx context.Context, brand string, parent *string, limit, offset int) (Tree, error) {
	return s.readNodes(ctx, brand, parent, false, limit, offset)
}
func (s Service) readNodes(ctx context.Context, brand string, parent *string, all bool, limit, offset int) (Tree, error) {
	out := Tree{BrandID: brand, ParentID: parent, Items: []Node{}, Limit: limit, Offset: offset}
	if !idsValid(brand) || !paging(limit, offset) || parent != nil && !idsValid(*parent) {
		return out, ErrInvalid
	}
	var valid bool
	var raw []byte
	// Use one JSON query with explicit DTO aliases rather than unstable duplicate
	// column names from nodeFields. This same statement keeps page/count coherent.
	sql := `WITH selected AS (SELECT ` + listedNodeFields + ` FROM agent_nodes_at($1,statement_timestamp()) n JOIN brand_agent_policies p ON p.brand_id=n.brand_id WHERE n.brand_id=$1 AND ($5::boolean OR n.parent_id IS NOT DISTINCT FROM $2::uuid)),
 page AS (SELECT * FROM selected ORDER BY created_at,id LIMIT $3 OFFSET $4)
 SELECT EXISTS(SELECT 1 FROM brand_agent_policies WHERE brand_id=$1) AND ($2::uuid IS NULL OR EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=$1 AND id=$2)),
 (SELECT count(*)::text FROM selected),COALESCE((SELECT jsonb_agg(to_jsonb(page) ORDER BY created_at,id) FROM page),'[]'::jsonb)`
	e := s.DB.QueryRow(ctx, sql, brand, parent, limit, offset, all).Scan(&valid, &out.TotalCount, &raw)
	if e == nil && !valid {
		return out, ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(raw, &out.Items)
	}
	return out, e
}
func (s Service) History(ctx context.Context, brand string, agent *string, limit, offset int) (History, error) {
	if agent == nil {
		return History{}, ErrInvalid
	}
	out := History{BrandID: brand, AgentID: agent, Items: []Revision{}, Limit: limit, Offset: offset}
	if !idsValid(brand, *agent) || !paging(limit, offset) {
		return out, ErrInvalid
	}
	var valid bool
	var raw []byte
	e := s.DB.QueryRow(ctx, `WITH selected AS (SELECT id::text,brand_id::text,agent_id::text,version,config,actor_type,actor_id::text,reason,created_at,audit_log_id::text FROM agent_config_revisions WHERE brand_id=$1 AND agent_id IS NOT DISTINCT FROM $2::uuid),page AS(SELECT * FROM selected ORDER BY version DESC LIMIT $3 OFFSET $4)
 SELECT EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=$1 AND id=$2),(SELECT count(*)::text FROM selected),COALESCE((SELECT jsonb_agg(to_jsonb(page) ORDER BY version DESC) FROM page),'[]'::jsonb)`, brand, agent, limit, offset).Scan(&valid, &out.TotalCount, &raw)
	if e == nil && !valid {
		return out, ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(raw, &out.Items)
	}
	return out, e
}
func revision(ctx context.Context, tx pgx.Tx, brand string, agent string, version int64, cfg any, reason string, m points.Metadata, before any) (string, error) {
	action := "agent.node.update"
	if version == 1 {
		action = "agent.node.create"
	}
	log, e := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: m.ActorType, ActorID: m.ActorID, Action: action, ResourceType: "agent_node", ResourceID: agent, Reason: strings.TrimSpace(reason), RequestID: m.RequestID, IP: m.IP, Before: before, After: map[string]any{"version": version, "config": cfg}})
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
	if tx == nil || !idsValid(brand, a.ID) || in.Version < 1 || !in.Config.Enabled || !in.Config.Valid() || !validReason(in.Reason) {
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
	if e = tx.QueryRow(ctx, `SELECT config->'calendar'<>'null'::jsonb AND config->'calendar'->>'cycle' IS DISTINCT FROM $2::text FROM brand_commission_policies WHERE brand_id=$1`, brand, in.Config.Cycle).Scan(&financialConflict); e != nil {
		return p, e
	}
	if financialConflict {
		return p, ErrState
	}
	var pending bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_mode_changes WHERE brand_id=$1 AND applied_at IS NULL)`, brand).Scan(&pending); e != nil {
		return p, e
	}
	if pending {
		return p, ErrModePending
	}
	lossCap, _ := RatioMicros(in.Config.LossRatioCap)
	turnoverCap, _ := RatioMicros(in.Config.TurnoverRatioCap)
	var exceeds bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=$1 AND ((config->>'ratio')::numeric*1000000>CASE WHEN agent_effective_mode_for_path(brand_id,path,NULL,NULL,$5)='loss' THEN $2::numeric ELSE $4::numeric END OR depth>$3))`, brand, lossCap, in.Config.MaxDepth, turnoverCap, in.Config.Mode).Scan(&exceeds)
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
	_, e = tx.Exec(ctx, `UPDATE brand_agent_policies SET version=version+1,config=$2 WHERE brand_id=$1`, brand, next)
	if e != nil {
		return p, e
	}
	p, e = scanPolicy(tx.QueryRow(ctx, `SELECT brand_id::text,version,config,updated_at FROM brand_agent_policies WHERE brand_id=$1`, brand))
	return p, e
}
func bytesEqual(a, b []byte) bool { return string(a) == string(b) }
func (s Service) Create(ctx context.Context, tx pgx.Tx, brand string, a access.Account, in CreateInput, m points.Metadata) (Node, error) {
	if tx == nil || !idsValid(brand, a.ID) || in.PolicyVersion < 1 || !in.Config.Valid() || !validReason(in.Reason) || (in.ParentID == nil) != (in.ParentVersion == nil) || in.ParentID != nil && (!idsValid(*in.ParentID) || *in.ParentVersion < 1) {
		return Node{}, ErrInvalid
	}
	if !adminAllowed(a, brand, "agent") {
		return Node{}, ErrDenied
	}
	m.ActorType, m.ActorID = "admin", a.ID
	return s.createAccountNode(ctx, tx, brand, a.ID, in, m)
}
func (s Service) createAccountNode(ctx context.Context, tx pgx.Tx, brand, creator string, in CreateInput, m points.Metadata) (Node, error) {
	p, e := s.LockPolicy(ctx, tx, brand)
	if e != nil {
		return Node{}, e
	}
	if p.Version != in.PolicyVersion {
		return Node{}, ErrVersion
	}
	username := strings.ToLower(strings.TrimSpace(in.Username))
	if !regexp.MustCompile(`^[a-z][a-z0-9_]{2,31}$`).MatchString(username) {
		return Node{}, ErrInvalid
	}
	hash, err := authcrypto.HashPassword(in.Password)
	if err != nil {
		return Node{}, ErrInvalid
	}
	accountID := ids.New()
	if _, err = tx.Exec(ctx, `INSERT INTO agent_accounts(id,brand_id,username,password_hash) VALUES($1,$2,$3,$4)`, accountID, brand, username, hash); err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			return Node{}, ErrExists
		}
		return Node{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, ids.New(), brand, accountID); err != nil {
		return Node{}, err
	}
	if in.ParentID == nil && in.Config.Mode == nil {
		return Node{}, ErrInvalid
	}
	if in.ParentID != nil {
		par, err := node(ctx, tx, brand, *in.ParentID)
		if err != nil {
			return Node{}, err
		}
		pending, err := pendingLine(ctx, tx, brand, par.Path[0])
		if err != nil {
			return Node{}, err
		}
		if pending {
			return Node{}, ErrModePending
		}
	}
	id := ids.New()
	depth, path := 1, []string{id}
	var ceiling int64
	if in.ParentID == nil {
		ceiling, e = p.Config.CapMicros(*in.Config.Mode)
		if e != nil {
			return Node{}, e
		}
	}
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

	if _, e = tx.Exec(ctx, `INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT pa.brand_id,pa.id,source,state FROM point_accounts pa CROSS JOIN unnest(ARRAY['recharge','winning','gift']) source CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) state WHERE pa.brand_id=$1 AND pa.brand_member_id=$2`, brand, accountID); e != nil {
		return Node{}, e
	}
	cfg, _ := json.Marshal(in.Config)
	_, e = tx.Exec(ctx, `INSERT INTO agent_nodes(id,brand_id,member_id,parent_id,depth,path,config,created_by) VALUES($1,$2,$3,$4,$5,$6::uuid[],$7,$8)`, id, brand, accountID, in.ParentID, depth, path, cfg, creator)
	if e != nil {
		return Node{}, e
	}
	log, e := revision(ctx, tx, brand, id, 1, in.Config, in.Reason, m, nil)
	if e != nil {
		return Node{}, e
	}
	out, e := node(ctx, tx, brand, id)
	out.AuditLogID = log
	if _, e = tx.Exec(ctx, `SELECT ensure_agent_code($1,$2)`, brand, id); e != nil {
		return Node{}, e
	}
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
	pending, err := pendingLine(ctx, tx, brand, n.Path[0])
	if err != nil {
		return n, err
	}
	if pending {
		return n, ErrModePending
	}
	if n.ParentID == nil && (in.Config.Mode == nil || *in.Config.Mode != n.EffectiveMode) {
		return n, ErrInvalid
	}
	ceiling, e := p.Config.CapMicros(n.EffectiveMode)
	if e != nil {
		return n, e
	}
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
	log, e := revision(ctx, tx, brand, id, n.Version+1, in.Config, in.Reason, m, n)
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
func (s Service) CheckUserWrite(ctx context.Context, tx pgx.Tx, brand string, v identity.AgentSession, id string) (Node, error) {
	if !idsValid(brand, v.AccountID, id) || v.BrandID != brand {
		return Node{}, ErrDenied
	}
	_, e := s.LockPolicy(ctx, tx, brand)
	if e != nil {
		return Node{}, e
	}
	own, e := scanNode(tx.QueryRow(ctx, `SELECT `+nodeFields+` FROM agent_nodes_at($1,statement_timestamp()) n JOIN brand_agent_policies p ON p.brand_id=n.brand_id WHERE n.brand_id=$1 AND n.member_id=$2`, brand, v.AccountID))
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
func (s Service) UpdateChild(ctx context.Context, tx pgx.Tx, brand string, v identity.AgentSession, id string, in ChildInput, m points.Metadata) (Node, error) {
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
	m.ActorType, m.ActorID = "agent", v.AccountID
	pv := in.ParentVersion
	return s.update(ctx, tx, brand, id, UpdateInput{Version: in.Version, PolicyVersion: in.PolicyVersion, ParentVersion: &pv, Config: cfg, Reason: in.Reason}, m)
}

// Agent-side creation is scoped to the authenticated account's direct child.
func (s Service) CreateChild(ctx context.Context, tx pgx.Tx, brand string, v identity.AgentSession, in CreateInput, m points.Metadata) (Node, error) {
	if tx == nil || v.BrandID != brand || !idsValid(brand, v.AccountID) || !in.Config.Valid() || in.PolicyVersion < 1 || !validReason(in.Reason) {
		return Node{}, ErrInvalid
	}
	_, err := s.LockPolicy(ctx, tx, brand)
	if err != nil {
		return Node{}, err
	}
	own, err := s.Me(ctx, brand, v.AccountID)
	if err != nil {
		return Node{}, err
	}
	active, err := ancestorsActive(ctx, tx, own)
	if err != nil {
		return Node{}, err
	}
	if !active || !own.Config.CanCreateChildren {
		return Node{}, ErrDenied
	}
	if in.ParentID != nil && *in.ParentID != own.ID {
		return Node{}, ErrDenied
	}
	if in.ParentVersion != nil && *in.ParentVersion != own.Version {
		return Node{}, ErrVersion
	}
	in.ParentID = &own.ID
	in.ParentVersion = &own.Version
	m.ActorType, m.ActorID = "agent", v.AccountID
	return s.createAccountNode(ctx, tx, brand, v.AccountID, in, m)
}
