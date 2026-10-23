package agency

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

type PendingModeChange struct {
	ID          string    `json:"id"`
	RootID      string    `json:"root_id"`
	Mode        string    `json:"mode"`
	Ratio       string    `json:"ratio"`
	EffectiveAt time.Time `json:"effective_at"`
}
type ModeRatioInput struct {
	AgentID string `json:"agent_id"`
	Version int64  `json:"version"`
	Ratio   string `json:"ratio"`
}
type ModeChangeInput struct {
	PolicyVersion int64            `json:"policy_version"`
	Mode          string           `json:"mode"`
	Items         []ModeRatioInput `json:"items"`
}

func (c *ModeRatioInput) UnmarshalJSON(b []byte) error {
	if closed(b, "agent_id", "version", "ratio") != nil {
		return ErrInvalid
	}
	type plain ModeRatioInput
	return json.Unmarshal(b, (*plain)(c))
}
func (c *ModeChangeInput) UnmarshalJSON(b []byte) error {
	if closed(b, "policy_version", "mode", "items") != nil {
		return ErrInvalid
	}
	type plain ModeChangeInput
	return json.Unmarshal(b, (*plain)(c))
}

type ModeChangeScope struct {
	BrandID       string `json:"brand_id"`
	RootID        string `json:"root_id"`
	PolicyVersion int64  `json:"policy_version"`
	Items         []Node `json:"items"`
}
type ModeChangeReceipt struct {
	ID          string           `json:"id"`
	BrandID     string           `json:"brand_id"`
	RootID      string           `json:"root_id"`
	Mode        string           `json:"mode"`
	EffectiveAt time.Time        `json:"effective_at"`
	Items       []ModeRatioInput `json:"items"`
}

func readModeScope(ctx context.Context, q rowQuerier, brand, id string) (ModeChangeScope, error) {
	out := ModeChangeScope{BrandID: brand, RootID: id, Items: []Node{}}
	if !idsValid(brand, id) {
		return out, ErrInvalid
	}
	var raw []byte
	var exists bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=$1 AND id=$2 AND parent_id IS NULL),p.version,
 COALESCE((SELECT jsonb_agg(to_jsonb(items) ORDER BY depth,id) FROM (SELECT `+listedNodeFields+` FROM agent_nodes_at($1,statement_timestamp()) n JOIN brand_agent_policies p ON p.brand_id=n.brand_id WHERE n.brand_id=$1 AND n.path[1]=$2) items),'[]'::jsonb)
 FROM brand_agent_policies p WHERE p.brand_id=$1`, brand, id).Scan(&exists, &out.PolicyVersion, &raw)
	if err != nil {
		return out, err
	}
	if !exists {
		return out, ErrNotFound
	}
	err = json.Unmarshal(raw, &out.Items)
	return out, err
}
func (s Service) ModeScope(ctx context.Context, brand, id string) (ModeChangeScope, error) {
	return readModeScope(ctx, s.DB, brand, id)
}
func pendingLine(ctx context.Context, tx pgx.Tx, brand, root string) (bool, error) {
	var pending bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_mode_changes WHERE brand_id=$1 AND root_id=$2 AND applied_at IS NULL)`, brand, root).Scan(&pending)
	return pending, err
}
func (s Service) ScheduleModeChange(ctx context.Context, tx pgx.Tx, brand string, a access.Account, id string, in ModeChangeInput, m points.Metadata) (ModeChangeReceipt, error) {
	out := ModeChangeReceipt{BrandID: brand, RootID: id, Mode: in.Mode, Items: in.Items}
	if tx == nil || !idsValid(brand, id, a.ID) || in.PolicyVersion < 1 || !validMode(in.Mode) || len(in.Items) == 0 {
		return out, ErrInvalid
	}
	if !adminAllowed(a, brand, "agent") {
		return out, ErrDenied
	}
	p, err := s.LockPolicy(ctx, tx, brand)
	if err != nil {
		return out, err
	}
	if p.Version != in.PolicyVersion {
		return out, ErrVersion
	}
	scope, err := readModeScope(ctx, tx, brand, id)
	if err != nil {
		return out, err
	}
	pending, err := pendingLine(ctx, tx, brand, id)
	if err != nil {
		return out, err
	}
	if pending {
		return out, ErrModePending
	}
	if scope.Items[0].EffectiveMode == in.Mode {
		return out, ErrState
	}
	if len(scope.Items) != len(in.Items) {
		return out, ErrVersion
	}
	inputs := map[string]ModeRatioInput{}
	for _, item := range in.Items {
		if !idsValid(item.AgentID) || item.Version < 1 {
			return out, ErrInvalid
		}
		if _, err = RatioMicros(item.Ratio); err != nil {
			return out, ErrInvalid
		}
		if _, ok := inputs[item.AgentID]; ok {
			return out, ErrInvalid
		}
		inputs[item.AgentID] = item
	}
	cap, err := p.Config.CapMicros(in.Mode)
	if err != nil {
		return out, err
	}
	for _, n := range scope.Items {
		item, ok := inputs[n.ID]
		if !ok || item.Version != n.Version {
			return out, ErrVersion
		}
		r, _ := RatioMicros(item.Ratio)
		if r > cap {
			return out, ErrLimit
		}
		if n.ParentID != nil {
			par, ok := inputs[*n.ParentID]
			if !ok {
				return out, ErrInvalid
			}
			ceiling, _ := RatioMicros(par.Ratio)
			if r > ceiling {
				return out, ErrLimit
			}
		}
	}
	// Calendar and activation boundary are calculated by the same SQL calendar
	// used for saved commission snapshots. No default calendar is substituted.
	var created time.Time
	err = tx.QueryRow(ctx, `SELECT statement_timestamp(),w.window_to FROM brand_commission_policies f CROSS JOIN LATERAL commission_calendar_window(f.config->'calendar',statement_timestamp()) w WHERE f.brand_id=$1 AND f.config->'calendar'<>'null'::jsonb`, brand).Scan(&created, &out.EffectiveAt)
	if err == pgx.ErrNoRows {
		return out, ErrCalendarRequired
	}
	if err != nil {
		return out, err
	}
	out.ID = ids.New()
	_, err = tx.Exec(ctx, `INSERT INTO agent_mode_changes(id,brand_id,root_id,mode,effective_at,created_at) VALUES($1,$2,$3,$4,$5,$6)`, out.ID, brand, id, in.Mode, out.EffectiveAt, created)
	if err != nil {
		return out, err
	}
	m.ActorType, m.ActorID = "admin", a.ID
	for _, n := range scope.Items {
		cfg := n.Config
		cfg.Ratio = inputs[n.ID].Ratio
		cfg.Mode = nil
		if n.ParentID == nil {
			cfg.Mode = &in.Mode
		}
		_, err = revision(ctx, tx, brand, n.ID, n.Version+1, cfg, "", m, n)
		if err != nil {
			return out, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO agent_mode_change_targets(brand_id,change_id,agent_id,version,config) VALUES($1,$2,$3,$4,$5)`, brand, out.ID, n.ID, n.Version+1, mustJSON(cfg))
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
