package compliance

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func (s Service) ReadPlatform(ctx context.Context) (Policy, error) {
	return scanPolicy(s.DB.QueryRow(ctx, `SELECT '',version,config,updated_at,'',false FROM platform_compliance_settings WHERE id=true`))
}

// Copy defaults into inherited brand policies using the same audited revisions as
// an explicit brand edit, so business gates retain their existing policy witnesses.
func (s Service) UpdatePlatform(ctx context.Context, tx pgx.Tx, a access.Account, in Input, meta points.Metadata) (Policy, error) {
	var out Policy
	if in.Version < 1 || in.InheritPlatform || !ValidConfig(in.Config) || !ValidReason(in.Reason) || !validMeta(a, meta) {
		return out, ErrInvalid
	}
	if !a.SuperAdmin || !access.Authorize(a, "compliance_policy", "write", access.ScopePlatform, "") {
		return out, ErrDenied
	}
	before, err := scanPolicy(tx.QueryRow(ctx, `SELECT '',version,config,updated_at,'',false FROM platform_compliance_settings WHERE id=true FOR UPDATE`))
	if err != nil {
		return out, err
	}
	if before.Version != in.Version {
		return out, ErrVersion
	}
	raw, err := json.Marshal(in.Config)
	if err != nil {
		return out, err
	}
	out, err = scanPolicy(tx.QueryRow(ctx, `UPDATE platform_compliance_settings SET version=version+1,config=$1,updated_at=clock_timestamp() WHERE id=true RETURNING '',version,config,updated_at,'',false`, raw))
	if err != nil {
		return out, err
	}
	out.AuditLogID, err = audit.Append(ctx, tx, audit.Record{ActorType: "admin", ActorID: a.ID, Action: "compliance.platform.update", ResourceType: "platform_compliance_settings", Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, Before: before, After: out})
	if err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT brand_id::text,version FROM brand_compliance_policies WHERE inherit_platform ORDER BY brand_id`)
	if err != nil {
		return out, err
	}
	type target struct {
		brand   string
		version int64
	}
	targets := []target{}
	for rows.Next() {
		var v target
		if err = rows.Scan(&v.brand, &v.version); err != nil {
			rows.Close()
			return out, err
		}
		targets = append(targets, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, v := range targets {
		if _, err = s.update(ctx, tx, v.brand, a, Input{Version: v.version, InheritPlatform: true, Config: in.Config, Reason: in.Reason}, meta, true); err != nil {
			return out, err
		}
	}
	return out, nil
}
