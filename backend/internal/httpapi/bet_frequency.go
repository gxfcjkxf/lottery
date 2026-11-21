package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
	"github.com/jackc/pgx/v5"
)

// Live orders and retained order receipts are disjoint after each archive
// transaction commits. Read both in one statement to keep that boundary stable.
const betFrequencySQL = `WITH bounds AS (
 SELECT $2::text::date::timestamp AT TIME ZONE timezone AS start_at,
        ($3::text::date+1)::timestamp AT TIME ZONE timezone AS end_at
 FROM platform_settings WHERE id
), orders AS (
 SELECT rule_id,placed_at,result
 FROM configured_orders CROSS JOIN bounds
 WHERE game_id=$1 AND status='settled' AND placed_at>=start_at AND placed_at<end_at
 UNION ALL
 SELECT (payload->>'rule_id')::uuid,(payload->>'placed_at')::timestamptz,payload->'result'
 FROM data_retention_receipts CROSS JOIN bounds
 WHERE family='configured_orders' AND NOT discard
   AND payload->>'game_id'=$1::uuid::text AND payload->>'status'='settled'
   AND (payload->>'placed_at')::timestamptz>=start_at AND (payload->>'placed_at')::timestamptz<end_at
), counts_by_rule AS (
 SELECT line->>'play_id' AS play_id,rule_id,count(*) AS items,max(placed_at) AS last_placed_at
 FROM orders CROSS JOIN LATERAL jsonb_array_elements(result->'lines') AS line
 WHERE (line->>'effective_points')::numeric>0
 GROUP BY line->>'play_id',rule_id
), totals AS (
 SELECT DISTINCT ON (play_id) play_id,rule_id,sum(items) OVER (PARTITION BY play_id) AS item_count
 FROM counts_by_rule
 ORDER BY play_id,last_placed_at DESC,rule_id DESC
), page AS (
 SELECT play_id,rule_id,item_count
 FROM totals ORDER BY item_count DESC,play_id LIMIT $4 OFFSET $5
)
SELECT jsonb_build_object(
 'effective_item_count',(SELECT coalesce(sum(item_count),0)::text FROM totals),
 'play_count',(SELECT count(*)::text FROM totals),
 'items',coalesce((
  SELECT jsonb_agg(jsonb_build_object(
   'play_id',page.play_id,'effective_item_count',page.item_count::text,
   'label',(SELECT play->'label'
            FROM jsonb_array_elements(v.snapshot->'catalog'->'plays') AS play
            WHERE play->>'id'=page.play_id)
  ) ORDER BY page.item_count DESC,page.play_id)
  FROM page JOIN configured_rule_versions v ON v.id=page.rule_id
 ),'[]'::jsonb)
) WHERE EXISTS(SELECT 1 FROM platform_games WHERE id=$1)`

func parseBetFrequencyQuery(raw string) (reporting.Query, error) {
	q := reporting.Query{GroupBy: "game", Limit: 20}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return q, reporting.ErrInvalid
	}
	for key, list := range values {
		if len(list) != 1 {
			return q, reporting.ErrInvalid
		}
		switch key {
		case "game_id", "from", "to":
		case "limit":
			q.Limit, err = strconv.Atoi(list[0])
		case "offset":
			q.Offset, err = strconv.Atoi(list[0])
		default:
			return q, reporting.ErrInvalid
		}
		if err != nil {
			return q, reporting.ErrInvalid
		}
	}
	game := values.Get("game_id")
	q.GameID = &game
	q.From, err = time.Parse(time.DateOnly, values.Get("from"))
	if err != nil {
		return q, reporting.ErrInvalid
	}
	inclusiveEnd, err := time.Parse(time.DateOnly, values.Get("to"))
	if err != nil {
		return q, reporting.ErrInvalid
	}
	q.To = inclusiveEnd.AddDate(0, 0, 1)
	return q, q.Validate("betting")
}

func registerBetFrequencyRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	const path = "/reports/bet-frequency"
	handle("GET", path+"/games", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := platformAccountActor(w, r, d, "report_betting", "view"); !ok {
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
			return
		}
		var data json.RawMessage
		err := d.Admins.DB.QueryRow(r.Context(), `SELECT jsonb_build_object('items',coalesce(
 jsonb_agg(jsonb_build_object('id',id,'name',name) ORDER BY name,id),'[]'::jsonb))
 FROM platform_games`).Scan(&data)
		if err != nil {
			failure(w, r, 503, "REPORT_UNAVAILABLE", "报表暂时不可用，请稍后重试")
			return
		}
		respond(w, r, 200, data)
	})
	handle("GET", path, func(w http.ResponseWriter, r *http.Request) {
		actor, ok := platformAccountActor(w, r, d, "report_betting", "view")
		if !ok {
			return
		}
		q, err := parseBetFrequencyQuery(r.URL.RawQuery)
		if err != nil {
			failure(w, r, 400, "REPORT_QUERY_INVALID", "报表筛选参数不正确，请指定最多93天的起止时间")
			return
		}
		var data json.RawMessage
		err = d.Admins.DB.QueryRow(r.Context(), betFrequencySQL, *q.GameID, q.From.Format(time.DateOnly), q.To.AddDate(0, 0, -1).Format(time.DateOnly), q.Limit, q.Offset).Scan(&data)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				failure(w, r, 404, "REPORT_SCOPE_NOT_FOUND", "未找到对应报表筛选资源")
				return
			}
			slog.Error("bet frequency read failed", "request_id", requestID(r), "error", err)
			failure(w, r, 503, "REPORT_UNAVAILABLE", "报表暂时不可用，请稍后重试")
			return
		}
		if adminReadAudit(w, r, d, actor, "", "report.bet_frequency.view") {
			respond(w, r, 200, data)
		}
	})
}
