package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
	"github.com/jackc/pgx/v5"
)

func resolveAdminEntry(w http.ResponseWriter, r *http.Request, d Dependencies) (tenant.Brand, bool) {
	b, err := d.Brands.Resolve(r.Context(), r.Host, "")
	if err == nil {
		return b, true
	}
	if errors.Is(err, tenant.ErrNotFound) {
		if p, ok := d.Brands.(interface {
			PlatformEntry(context.Context, string) (bool, error)
		}); ok {
			allowed, e := p.PlatformEntry(r.Context(), r.Host)
			if e != nil {
				failure(w, r, 503, "SERVICE_UNAVAILABLE", "无法检查管理入口")
				return b, false
			}
			if allowed {
				return tenant.Brand{}, true
			}
		}
		failure(w, r, 404, "BRAND_NOT_FOUND", "当前入口没有可用品牌或平台管理入口")
		return b, false
	}
	failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法检查管理入口")
	return b, false
}

// Authentication on a brandless platform host is public, but still admits only
// a current trusted entry. Credentials/rate limits remain in AdminLogin. This
// check holds the entry row across idempotency replay and session creation.
func checkPlatformAdminEntry(ctx context.Context, tx pgx.Tx, r *http.Request) error {
	var host string
	err := tx.QueryRow(ctx, `SELECT domain FROM platform_domains WHERE domain=$1 AND enabled FOR SHARE`, tenant.NormalizeHost(r.Host)).Scan(&host)
	if errors.Is(err, pgx.ErrNoRows) {
		return tenant.ErrNotFound
	}
	return err
}
