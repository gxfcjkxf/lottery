package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/jackc/pgx/v5"
)

// All betting protocols are aggregated on the server, never by paging the
// member's history in the browser. Refunds are neutral; Niu uses actual loss,
// not its temporary maximum-loss reserve, when calculating settled profit.
const memberOrderMetricsSQL = ` SELECT total_points::numeric AS stake, status IN ('placed','abnormal') AS pending,
 prize_points::numeric-total_points::numeric AS net, settled_at
 FROM bet_orders WHERE brand_id=$1 AND brand_member_id=$2
 UNION ALL
 SELECT total_points::numeric,status='placed',prize_points::numeric+refund_points::numeric-total_points::numeric,settled_at
 FROM platform_mark_six_orders WHERE brand_id=$1 AND member_id=$2
 UNION ALL
 SELECT total_points::numeric,status='placed',CASE WHEN profile='canada_niuniu' THEN prize_points::numeric-lost_points::numeric ELSE prize_points::numeric+refund_points::numeric-total_points::numeric END,settled_at
 FROM platform_canada_orders WHERE brand_id=$1 AND member_id=$2
 UNION ALL
 SELECT total_points::numeric,status='placed',CASE WHEN snapshot->'catalog'->>'funding'='reserve' THEN (result->>'prize_points')::numeric-coalesce((result->>'consumed_points')::numeric,0) ELSE (result->>'prize_points')::numeric+(result->>'refund_points')::numeric-total_points::numeric END,settled_at
 FROM configured_orders WHERE brand_id=$1 AND member_id=$2`

const memberSummarySQL = `WITH bounds AS (
 SELECT (date_trunc('day',now() AT TIME ZONE timezone) AT TIME ZONE timezone) AS start_at,
 ((date_trunc('day',now() AT TIME ZONE timezone)+interval '1 day') AT TIME ZONE timezone) AS end_at
 FROM platform_settings WHERE id=true
), orders AS (
` + memberOrderMetricsSQL + `
)
SELECT coalesce(sum(stake) FILTER(WHERE pending),0)::text,
 coalesce(sum(net) FILTER(WHERE settled_at>=start_at AND settled_at<end_at),0)::text
FROM orders CROSS JOIN bounds`

func registerMemberSummary(mux routeRegistrar, d Dependencies) {
	for _, prefix := range []string{"/api/v1", "/api/v1/b/{brandCode}"} {
		mux.HandleFunc("GET "+prefix+"/betting-summary", func(w http.ResponseWriter, r *http.Request) {
			if d.Identity == nil || d.Admins.DB == nil {
				failure(w, r, 503, "POINTS_UNAVAILABLE", "Summary unavailable")
				return
			}
			if len(r.URL.Query()) != 0 {
				failure(w, r, 400, "REQUEST_INVALID", "Summary accepts no filters")
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			r = r.WithContext(ctx)
			brand, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			tx, err := d.Admins.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
			if err != nil {
				pointOutput(w, r, nil, err)
				return
			}
			defer tx.Rollback(ctx)
			session, err := d.Identity.AuthenticateTx(ctx, tx, brand.ID, requestToken(r, userCookieName(brand.ID)))
			if errors.Is(err, identity.ErrSession) {
				failure(w, r, 401, "AUTH_SESSION_REVOKED", "Session unavailable")
				return
			}
			if err != nil {
				pointOutput(w, r, nil, err)
				return
			}
			var pending, net string
			if err = tx.QueryRow(ctx, memberSummarySQL, brand.ID, session.Member.ID).Scan(&pending, &net); err != nil {
				pointOutput(w, r, nil, err)
				return
			}
			if err = tx.Commit(ctx); err != nil {
				pointOutput(w, r, nil, err)
				return
			}
			respond(w, r, 200, map[string]string{"brand_id": brand.ID, "member_id": session.Member.ID, "unsettled_points": pending, "today_net_points": net})
		})
	}
}
