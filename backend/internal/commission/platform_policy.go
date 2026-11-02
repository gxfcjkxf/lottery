package commission

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type PlatformPolicy struct {
	PayoutsEnabled bool         `json:"payouts_enabled"`
	Version        int64        `json:"version"`
	Config         PolicyConfig `json:"config"`
}

type PlatformPolicyInput struct {
	Version        int64        `json:"version"`
	Config         PolicyConfig `json:"config"`
	PayoutsEnabled bool         `json:"payouts_enabled"`
	Reason         string       `json:"reason"`
}

func (in *PlatformPolicyInput) UnmarshalJSON(data []byte) error {
	if closedObject(data, "version", "config", "payouts_enabled", "reason") != nil {
		return ErrInvalid
	}
	type plain PlatformPolicyInput
	return json.Unmarshal(data, (*plain)(in))
}

func (s Service) PlatformPolicy(ctx context.Context) (PlatformPolicy, error) {
	var out PlatformPolicy
	var raw []byte
	err := s.DB.QueryRow(ctx, `SELECT version,config,payouts_enabled FROM platform_commission_policy WHERE id=true`).Scan(&out.Version, &raw, &out.PayoutsEnabled)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(raw, &out.Config)
	return out, err
}

func (s Service) UpdatePlatformPolicy(ctx context.Context, tx pgx.Tx, a access.Account, in PlatformPolicyInput, meta points.Metadata) (PlatformPolicy, error) {
	if !access.Authorize(a, "platform_settings", "write", access.ScopePlatform, "") {
		return PlatformPolicy{}, ErrDenied
	}
	if (PolicyInput{Version: in.Version, Config: in.Config, Reason: in.Reason}).Validate() != nil || meta.ActorType != "admin" || meta.ActorID != a.ID || meta.RequestID == "" {
		return PlatformPolicy{}, ErrInvalid
	}
	var timezone string
	if err := tx.QueryRow(ctx, `SELECT timezone FROM platform_settings WHERE id=true FOR SHARE`).Scan(&timezone); err != nil {
		return PlatformPolicy{}, err
	}
	if in.Config.Calendar != nil && (in.Config.Calendar.Timezone != timezone || in.Config.Calendar.BoundaryTime != "00:00:00") {
		return PlatformPolicy{}, ErrInvalid
	}
	var version int64
	var beforePayouts bool
	var beforeConfig []byte
	if err := tx.QueryRow(ctx, `SELECT version,config,payouts_enabled FROM platform_commission_policy WHERE id=true FOR UPDATE`).Scan(&version, &beforeConfig, &beforePayouts); err != nil {
		return PlatformPolicy{}, err
	}
	if version != in.Version {
		return PlatformPolicy{}, ErrVersion
	}
	var previousConfig PolicyConfig
	if err := json.Unmarshal(beforeConfig, &previousConfig); err != nil {
		return PlatformPolicy{}, err
	}
	raw, err := json.Marshal(in.Config)
	if err != nil {
		return PlatformPolicy{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE platform_commission_policy SET version=version+1,config=$1,payouts_enabled=$2 WHERE id=true`, raw, in.PayoutsEnabled)
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Message == "agent mode change must be applied before changing commission policy" {
			return PlatformPolicy{}, ErrPolicyEvidence
		}
		return PlatformPolicy{}, err
	}
	rows, err := tx.Query(ctx, `SELECT brand_id::text,version,enabled FROM brand_commission_payment_policies ORDER BY brand_id FOR UPDATE`)
	if err != nil {
		return PlatformPolicy{}, err
	}
	type gate struct {
		brand   string
		version int64
		enabled bool
	}
	var gates []gate
	for rows.Next() {
		var g gate
		if err = rows.Scan(&g.brand, &g.version, &g.enabled); err != nil {
			rows.Close()
			return PlatformPolicy{}, err
		}
		gates = append(gates, g)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return PlatformPolicy{}, err
	}
	for _, g := range gates {
		if g.enabled == in.PayoutsEnabled {
			continue
		}
		log, err := audit.Append(ctx, tx, audit.Record{BrandID: g.brand, ActorType: meta.ActorType, ActorID: a.ID, Action: "commission.payment_policy.update", ResourceType: "commission_payment_policy", ResourceID: g.brand, Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, Before: map[string]any{"version": g.version, "enabled": g.enabled}, After: map[string]any{"version": g.version + 1, "enabled": in.PayoutsEnabled}})
		if err != nil {
			return PlatformPolicy{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE brand_commission_payment_policies SET version=version+1,enabled=$2,audit_log_id=$3 WHERE brand_id=$1`, g.brand, in.PayoutsEnabled, log); err != nil {
			return PlatformPolicy{}, err
		}
	}
	if _, err = audit.Append(ctx, tx, audit.Record{ActorType: meta.ActorType, ActorID: a.ID, Action: "commission.platform_policy.update", ResourceType: "platform_commission_policy", Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, Before: PlatformPolicy{Version: version, PayoutsEnabled: beforePayouts, Config: previousConfig}, After: PlatformPolicy{Version: version + 1, PayoutsEnabled: in.PayoutsEnabled, Config: in.Config}}); err != nil {
		return PlatformPolicy{}, err
	}

	return PlatformPolicy{Version: version + 1, Config: in.Config, PayoutsEnabled: in.PayoutsEnabled}, nil
}
