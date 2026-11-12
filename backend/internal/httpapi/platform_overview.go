package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// One PostgreSQL statement gives every card the same snapshot. Amounts and
// counts travel as decimal strings, including totals beyond JavaScript's range.
// Risk reserves released by Niu are neither winnings nor cancelled stakes.
const platformOverviewSQL = `WITH settings AS (
 SELECT timezone,now() AS snapshot_at,now() AT TIME ZONE timezone AS local_now
 FROM platform_settings WHERE id=true
), bounds AS (
 SELECT *,date_trunc('day',local_now) AS local_day FROM settings
), member_balances AS (
 SELECT pb.state,pb.points
 FROM point_buckets pb
 JOIN point_accounts pa ON pa.id=pb.account_id AND pa.brand_id=pb.brand_id
 JOIN brand_members m ON m.id=pa.brand_member_id AND m.brand_id=pa.brand_id
 JOIN global_users u ON u.id=m.global_user_id
 WHERE u.status<>'deleted' AND m.status<>'cancelled'
), orders AS MATERIALIZED (
 SELECT brand_id,global_user_id AS user_id,total_points::numeric AS stake,
 consumed_points::numeric-prize_points::numeric AS net,status='placed' AS pending,
 placed_at,settled_at,settled_at AS refunded_at,
 CASE WHEN snapshot->'catalog'->>'funding'='reserve' THEN 0 ELSE refund_points::numeric END AS refund
 FROM betting_financial_facts
), days AS (
 SELECT d::date::text AS day,d AT TIME ZONE timezone AS start_at,
 (d+interval '1 day') AT TIME ZONE timezone AS end_at
 FROM bounds CROSS JOIN LATERAL generate_series(local_day-interval '6 days',local_day,interval '1 day') d
), daily AS (
 SELECT day,
 coalesce(sum(stake) FILTER(WHERE placed_at>=start_at AND placed_at<end_at),0)::text AS stake_points,
 coalesce(sum(net) FILTER(WHERE settled_at>=start_at AND settled_at<end_at),0)::text AS net_points,
 count(*) FILTER(WHERE placed_at>=start_at AND placed_at<end_at)::text AS order_count,
 count(DISTINCT (brand_id,user_id)) FILTER(WHERE placed_at>=start_at AND placed_at<end_at)::text AS bettor_count,
 coalesce(sum(refund) FILTER(WHERE refunded_at>=start_at AND refunded_at<end_at),0)::text AS refund_points,
 (SELECT count(*)::text FROM brand_members m JOIN global_users u ON u.id=m.global_user_id WHERE u.status<>'deleted' AND m.status<>'cancelled' AND m.joined_at>=start_at AND m.joined_at<end_at) AS new_members
 FROM days LEFT JOIN orders ON placed_at>=start_at AND placed_at<end_at OR settled_at>=start_at AND settled_at<end_at OR refunded_at>=start_at AND refunded_at<end_at
 GROUP BY day,start_at,end_at
), brand_totals AS (
 SELECT b.id::text AS brand_id,b.name AS brand_name,
 coalesce(sum(o.stake) FILTER(WHERE placed_at>=local_day AT TIME ZONE bounds.timezone),0) AS stake,
 coalesce(sum(o.net) FILTER(WHERE settled_at>=local_day AT TIME ZONE bounds.timezone),0) AS net
 FROM brands b CROSS JOIN bounds LEFT JOIN orders o ON o.brand_id=b.id
 GROUP BY b.id,b.name
), ranked_brands AS (
 SELECT brand_id,brand_name,stake::text AS stake_points,net::text AS net_points
 FROM brand_totals ORDER BY stake DESC,brand_name,brand_id LIMIT 10
)
SELECT jsonb_build_object(
 'snapshot_at',snapshot_at,'timezone',timezone,'today',local_day::date::text,
 'total_members',(SELECT count(*)::text FROM brand_members m JOIN global_users u ON u.id=m.global_user_id WHERE u.status<>'deleted' AND m.status<>'cancelled'),
 'online_members',(SELECT count(DISTINCT s.member_id)::text FROM session_activity a JOIN sessions s ON s.id=a.session_id
 JOIN global_users u ON u.id=s.user_id JOIN brand_members m ON m.id=s.member_id AND m.brand_id=s.brand_id
 JOIN brands b ON b.id=s.brand_id
 WHERE a.last_seen_at>=snapshot_at-interval '5 minutes' AND s.revoked_at IS NULL AND s.expires_at>snapshot_at
 AND u.status='active' AND m.status IN ('normal','frozen') AND b.status<>'disabled'),
 'unsettled_points',(SELECT coalesce(sum(stake) FILTER(WHERE pending),0)::text FROM orders),
 'available_points',(SELECT coalesce(sum(points) FILTER(WHERE state='available'),0)::text FROM member_balances),
 'frozen_points',(SELECT coalesce(sum(points) FILTER(WHERE state IN ('manual_frozen','system_frozen')),0)::text FROM member_balances),
 'withdrawal_points',(SELECT coalesce(sum(points),0)::text FROM withdrawal_orders WHERE state='reviewing'),
 'withdrawal_count',(SELECT count(*)::text FROM withdrawal_orders WHERE state='reviewing'),
 'yesterday_same_time_stake',(SELECT coalesce(sum(stake),0)::text FROM orders
 WHERE placed_at>=(local_day-interval '1 day') AT TIME ZONE timezone
 AND placed_at<(local_now-interval '1 day') AT TIME ZONE timezone),
 'days',(SELECT jsonb_agg(to_jsonb(daily) ORDER BY day) FROM daily),
 'brands',coalesce((SELECT jsonb_agg(to_jsonb(ranked_brands)) FROM ranked_brands),'[]'::jsonb)
) FROM bounds`

func registerPlatformOverview(handle func(string, string, http.HandlerFunc), d Dependencies) {
	handle("GET", "/overview", func(w http.ResponseWriter, r *http.Request) {
		_, ok := platformAccountActor(w, r, d, "overview", "view")
		if !ok {
			return
		}
		if len(r.URL.Query()) != 0 {
			failure(w, r, 400, "REQUEST_INVALID", "Overview accepts no filters")
			return
		}
		var data json.RawMessage
		if err := d.Admins.DB.QueryRow(r.Context(), platformOverviewSQL).Scan(&data); err != nil {
			slog.Default().Error("platform overview read failed", "request_id", requestID(r), "error", err)
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "Overview unavailable")
			return
		}
		respond(w, r, 200, data)
	})
}
