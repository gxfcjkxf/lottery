package commission

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

const discoveryReadJSON = `jsonb_build_object(
 'id',d.id::text,'brand_id',d.brand_id::text,'state',d.state,'version',d.version,
 'cycle_id',d.cycle_id::text,'window_from',d.window_from,'window_to',d.window_to,
 'next_check_at',d.next_check_at,'last_error_code',d.last_error_code,
 'last_audit_log_id',d.last_audit_log_id::text,'created_at',d.created_at,'updated_at',d.updated_at)`

func (s Service) DiscoveryTx(ctx context.Context, tx pgx.Tx, brand, id string) (Discovery, error) {
	var out Discovery
	if tx == nil || !canonicalUUID(brand) || !canonicalUUID(id) {
		return out, ErrInvalid
	}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT `+discoveryReadJSON+` FROM commission_discovery d WHERE d.brand_id=$1 AND d.id=$2`, brand, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return Discovery{}, err
	}
	if out.Version < 1 || out.Version > maxCycleVersion {
		return Discovery{}, ErrInvalid
	}
	if out.WindowFrom != nil {
		*out.WindowFrom = out.WindowFrom.UTC()
	}
	if out.WindowTo != nil {
		*out.WindowTo = out.WindowTo.UTC()
	}
	out.NextCheckAt = out.NextCheckAt.UTC()
	out.CreatedAt = out.CreatedAt.UTC()
	out.UpdatedAt = out.UpdatedAt.UTC()
	return out, nil
}

func (s Service) DiscoveriesTx(ctx context.Context, tx pgx.Tx, brand string, limit, offset int) (DiscoveryPage, error) {
	out := DiscoveryPage{BrandID: brand, Items: []Discovery{}, Limit: limit, Offset: offset}
	if tx == nil || !validCyclePage(brand, limit, offset) {
		return out, ErrInvalid
	}
	var raw []byte
	err := tx.QueryRow(ctx, `WITH page AS (
 SELECT d.* FROM commission_discovery d WHERE d.brand_id=$1 ORDER BY d.created_at DESC,d.id DESC LIMIT $2 OFFSET $3
), contents AS (
 SELECT `+discoveryReadJSON+` data,d.created_at,d.id FROM page d
)
SELECT (SELECT count(*)::text FROM commission_discovery WHERE brand_id=$1),
 coalesce((SELECT jsonb_agg(data ORDER BY created_at DESC,id DESC) FROM contents),'[]'::jsonb)`, brand, limit, offset).Scan(&out.TotalCount, &raw)
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out.Items); err != nil {
		return out, err
	}
	for i := range out.Items {
		if out.Items[i].Version < 1 || out.Items[i].Version > maxCycleVersion {
			return DiscoveryPage{BrandID: brand, Items: []Discovery{}, Limit: limit, Offset: offset}, ErrInvalid
		}
		if out.Items[i].WindowFrom != nil {
			*out.Items[i].WindowFrom = out.Items[i].WindowFrom.UTC()
		}
		if out.Items[i].WindowTo != nil {
			*out.Items[i].WindowTo = out.Items[i].WindowTo.UTC()
		}
		out.Items[i].NextCheckAt = out.Items[i].NextCheckAt.UTC()
		out.Items[i].CreatedAt = out.Items[i].CreatedAt.UTC()
		out.Items[i].UpdatedAt = out.Items[i].UpdatedAt.UTC()
	}
	return out, nil
}
