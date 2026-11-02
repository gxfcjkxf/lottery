package betting

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/jackc/pgx/v5"
	"regexp"
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validIDs(ids ...string) bool {
	for _, id := range ids {
		if !uuidPattern.MatchString(id) {
			return false
		}
	}
	return true
}
func eligibility(ctx context.Context, tx pgx.Tx, brand string, v identity.Session, normal bool) error {
	if tx == nil {
		return ErrInvalid
	}
	if !validIDs(brand, v.User.ID, v.Member.ID, v.ID) || v.Member.BrandID != brand {
		return ErrDenied
	}
	var ok, sessionOK bool
	var brandStatus string
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brands b JOIN brand_members m ON m.brand_id=b.id JOIN global_users u ON u.id=m.global_user_id WHERE b.id=$1 AND m.id=$2 AND u.id=$3 AND u.status='active' AND m.terms_accepted=true AND m.status IN ('normal','frozen') AND b.status<>'disabled' AND (NOT $4 OR (m.status='normal' AND b.status='active'))), EXISTS(SELECT 1 FROM sessions s WHERE s.id=$5 AND s.brand_id=$1 AND s.member_id=$2 AND s.user_id=$3 AND s.admin_id IS NULL AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp()), COALESCE((SELECT status FROM brands WHERE id=$1),'')`, brand, v.Member.ID, v.User.ID, normal, v.ID).Scan(&ok, &sessionOK, &brandStatus)
	if e != nil {
		return e
	}
	if !sessionOK {
		return identity.ErrSession
	}
	if !ok {
		if normal && brandStatus == "paused" {
			return ErrBrandPaused
		}
		return ErrDenied
	}
	return nil
}
