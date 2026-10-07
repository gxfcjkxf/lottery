package commission

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/jackc/pgx/v5"
)

// BetSnapshot is internal financial evidence, never a user-order DTO. Version
// strings preserve bigint precision. Historical NULL must not be backfilled
// from today's policy, attribution or agent ratios.
type BetSnapshot struct {
	SchemaVersion int           `json:"schema_version"`
	BrandID       string        `json:"brand_id"`
	MemberID      string        `json:"member_id"`
	CapturedAt    time.Time     `json:"captured_at"`
	Financial     FinancialRule `json:"financial_policy"`
	Agency        AgencyRule    `json:"agency_policy"`
	Path          []AgentRule   `json:"agent_path"`
}

type FinancialRule struct {
	RevisionID string       `json:"revision_id"`
	Version    string       `json:"version"`
	Config     PolicyConfig `json:"config"`
}
type AgencyRule struct {
	RevisionID string              `json:"revision_id"`
	Version    string              `json:"version"`
	Config     agency.PolicyConfig `json:"config"`
}
type AgentRule struct {
	ID            string            `json:"id"`
	MemberID      string            `json:"member_id"`
	ParentID      *string           `json:"parent_id"`
	Depth         int               `json:"depth"`
	RevisionID    string            `json:"revision_id"`
	Version       string            `json:"version"`
	Config        agency.NodeConfig `json:"config"`
	EffectiveMode string            `json:"effective_mode"`
}

func canonicalUUID(s string) bool { return uuid.MatchString(s) && s == strings.ToLower(s) }
func snapshotVersion(s string) bool {
	v, err := strconv.ParseInt(s, 10, 64)
	return err == nil && v > 0 && strconv.FormatInt(v, 10) == s
}

// ParseBetSnapshot validates closed shapes, chronology and tree arithmetic;
// it alone is not a financial authorization. PolicySnapshotTx additionally
// binds every referenced revision and immutable attribution in PostgreSQL.
func ParseBetSnapshot(raw []byte, brand, member string, placed time.Time) (BetSnapshot, error) {
	var out BetSnapshot
	if closedObject(raw, "schema_version", "brand_id", "member_id", "captured_at", "financial_policy", "agency_policy", "agent_path") != nil ||
		json.Unmarshal(raw, &out) != nil || out.SchemaVersion != 1 || !canonicalUUID(out.BrandID) || !canonicalUUID(out.MemberID) ||
		out.BrandID != brand || out.MemberID != member || placed.IsZero() || !out.CapturedAt.Equal(placed) ||
		!canonicalUUID(out.Financial.RevisionID) || !snapshotVersion(out.Financial.Version) ||
		!canonicalUUID(out.Agency.RevisionID) || !snapshotVersion(out.Agency.Version) || !out.Agency.Config.Valid() || out.Path == nil {
		return BetSnapshot{}, ErrPolicyEvidence
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	if closedObject(fields["financial_policy"], "revision_id", "version", "config") != nil ||
		closedObject(fields["agency_policy"], "revision_id", "version", "config") != nil {
		return BetSnapshot{}, ErrPolicyEvidence
	}
	if out.Financial.Config.Enabled && (!out.Agency.Config.Enabled || out.Financial.Config.Calendar == nil || out.Financial.Config.Calendar.Cycle != out.Agency.Config.Cycle) {
		return BetSnapshot{}, ErrPolicyEvidence
	}
	if len(out.Path) > out.Agency.Config.MaxDepth {
		return BetSnapshot{}, ErrPolicyEvidence
	}
	var rawPath []json.RawMessage
	_ = json.Unmarshal(fields["agent_path"], &rawPath)
	cap, _ := agency.RatioMicros(out.Agency.Config.RatioCap)
	mode := out.Agency.Config.Mode
	seenID, seenMember := map[string]bool{}, map[string]bool{}
	for i, n := range out.Path {
		if closedObject(rawPath[i], "id", "member_id", "parent_id", "depth", "revision_id", "version", "config", "effective_mode") != nil ||
			!canonicalUUID(n.ID) || !canonicalUUID(n.MemberID) || !canonicalUUID(n.RevisionID) || !snapshotVersion(n.Version) ||
			n.Depth != i+1 || !n.Config.Valid() || seenID[n.ID] || seenMember[n.MemberID] ||
			(i == 0 && n.ParentID != nil) || (i > 0 && (n.ParentID == nil || *n.ParentID != out.Path[i-1].ID)) {
			return BetSnapshot{}, ErrPolicyEvidence
		}
		seenID[n.ID], seenMember[n.MemberID] = true, true
		rate, _ := agency.RatioMicros(n.Config.Ratio)
		if rate > cap {
			return BetSnapshot{}, ErrPolicyEvidence
		}
		cap = rate
		if n.Config.Mode != nil {
			if i > 0 && *n.Config.Mode != mode {
				return BetSnapshot{}, ErrPolicyEvidence
			}
			mode = *n.Config.Mode
		}
		if n.EffectiveMode != mode {
			return BetSnapshot{}, ErrPolicyEvidence
		}
		// Disabled only restricts agent operations; the saved chain still earns.
	}
	return out, nil
}

func (Source) PolicySnapshotTx(ctx context.Context, tx pgx.Tx, brand, order string) (BetSnapshot, error) {
	if tx == nil || !canonicalUUID(brand) || !canonicalUUID(order) {
		return BetSnapshot{}, ErrInvalid
	}
	var member string
	var placed time.Time
	var raw, attribution []byte
	err := tx.QueryRow(ctx, `SELECT brand_member_id::text,placed_at,commission_rule_snapshot,attribution_snapshot FROM bet_orders WHERE brand_id=$1 AND id=$2`, brand, order).Scan(&member, &placed, &raw, &attribution)
	if err == pgx.ErrNoRows {
		return BetSnapshot{}, ErrNotFound
	}
	if err != nil {
		return BetSnapshot{}, err
	}
	snapshot, err := ParseBetSnapshot(raw, brand, member, placed)
	if err != nil {
		return BetSnapshot{}, err
	}
	var attr struct {
		Member struct {
			AgentID *string `json:"agent_id"`
		} `json:"member_attribution"`
		Policy struct {
			Version int64               `json:"version"`
			Config  agency.PolicyConfig `json:"config"`
		} `json:"agent_policy"`
		Nodes []struct {
			ID       string            `json:"id"`
			ParentID *string           `json:"parent_id"`
			Depth    int               `json:"depth"`
			Version  int64             `json:"version"`
			Config   agency.NodeConfig `json:"config"`
		} `json:"agent_configs_at_bet"`
	}
	if json.Unmarshal(attribution, &attr) != nil || strconv.FormatInt(attr.Policy.Version, 10) != snapshot.Agency.Version || attr.Policy.Config != snapshot.Agency.Config || len(attr.Nodes) != len(snapshot.Path) ||
		(len(snapshot.Path) == 0 && attr.Member.AgentID != nil) ||
		(len(snapshot.Path) > 0 && (attr.Member.AgentID == nil || *attr.Member.AgentID != snapshot.Path[len(snapshot.Path)-1].ID)) {
		return BetSnapshot{}, ErrPolicyEvidence
	}
	for i, n := range snapshot.Path {
		a := attr.Nodes[i]
		nc, _ := json.Marshal(n.Config)
		ac, _ := json.Marshal(a.Config)
		if a.ID != n.ID || a.Depth != n.Depth || strconv.FormatInt(a.Version, 10) != n.Version || string(ac) != string(nc) ||
			(a.ParentID == nil) != (n.ParentID == nil) || (a.ParentID != nil && *a.ParentID != *n.ParentID) {
			return BetSnapshot{}, ErrPolicyEvidence
		}
	}
	financial, _ := json.Marshal(snapshot.Financial.Config)
	agencyConfig, _ := json.Marshal(snapshot.Agency.Config)
	var matched bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM commission_policy_revisions WHERE brand_id=$1 AND id=$2 AND version::text=$3 AND config=$4::jsonb AND created_at<=$5) AND
 EXISTS(SELECT 1 FROM agent_config_revisions WHERE brand_id=$1 AND agent_id IS NULL AND id=$6 AND version::text=$7 AND config=$8::jsonb AND created_at<=$5)`, brand, snapshot.Financial.RevisionID, snapshot.Financial.Version, financial, placed, snapshot.Agency.RevisionID, snapshot.Agency.Version, agencyConfig).Scan(&matched)
	if err != nil {
		return BetSnapshot{}, err
	}
	if !matched {
		return BetSnapshot{}, ErrPolicyEvidence
	}
	for _, n := range snapshot.Path {
		config, _ := json.Marshal(n.Config)
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_config_revisions r JOIN agent_nodes a ON a.brand_id=r.brand_id AND a.id=r.agent_id
 WHERE r.brand_id=$1 AND r.agent_id=$2 AND r.id=$3 AND r.version::text=$4 AND r.config=$5::jsonb AND r.created_at<=$6
 AND a.member_id=$7 AND a.depth=$8 AND a.parent_id IS NOT DISTINCT FROM $9::uuid)`, brand, n.ID, n.RevisionID, n.Version, config, placed, n.MemberID, n.Depth, n.ParentID).Scan(&matched)
		if err != nil {
			return BetSnapshot{}, err
		}
		if !matched {
			return BetSnapshot{}, ErrPolicyEvidence
		}
	}
	return snapshot, nil
}
