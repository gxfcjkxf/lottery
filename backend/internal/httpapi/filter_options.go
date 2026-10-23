package httpapi

import (
	"net/http"
	"slices"
	"strings"
)

// Options inherit the target screen's read permission and brand scope.
func registerFilterOptions(handle func(string, string, http.HandlerFunc), d Dependencies) {
	handle("GET", "/filter-options", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		kind, resource := q.Get("kind"), q.Get("resource")
		allowed := map[string]string{
			"report_betting": "game member", "report_ledger": "member", "report_withdrawal": "member",
			"report_commission": "agent member cycle", "report_attribution": "game agent member",
			"report_rewards": "member reward_order", "report_reward_orders": "member reward_order",
			"period": "game period", "draw": "game period",
			"audit": "actor audit_resource", "commission": "agent member cycle bet_order", "reward": "reward_order member",
		}
		if !slices.Contains(strings.Fields(allowed[resource]), kind) || kind == "" || allowed[resource] == "" {
			failure(w, r, 400, "REQUEST_INVALID", "筛选项类型无效")
			return
		}
		for key, values := range q {
			if len(values) != 1 || (key != "kind" && key != "resource" && key != "limit" && key != "offset") {
				failure(w, r, 400, "REQUEST_INVALID", "筛选参数无效")
				return
			}
		}
		limit, offset, valid := pageParams(r)
		if !valid {
			failure(w, r, 400, "REQUEST_INVALID", "分页参数无效")
			return
		}
		permissionResource := resource
		if resource == "report_rewards" || resource == "report_reward_orders" {
			permissionResource = "report_reward"
		}
		actor, brand, ok := managementActor(w, r, d, permissionResource, "view")
		if !ok {
			return
		}
		var query string
		switch kind {
		case "game":
			query = `SELECT id::text,name FROM games WHERE brand_id=$1 UNION SELECT g.id::text,g.name FROM platform_games g JOIN brand_platform_games b ON b.game_id=g.id WHERE b.brand_id=$1`
		case "period":
			query = `SELECT p.id::text,g.name||' · '||p.period_no AS name FROM periods p JOIN games g ON g.id=p.game_id AND g.brand_id=p.brand_id WHERE p.brand_id=$1 UNION SELECT p.id::text,g.name||' · '||p.period_no AS name FROM platform_game_periods p JOIN platform_games g ON g.id=p.game_id JOIN brand_platform_games b ON b.game_id=g.id WHERE b.brand_id=$1`
		case "member":
			query = `SELECT m.id::text,u.username AS name FROM brand_members m JOIN global_users u ON u.id=m.global_user_id WHERE m.brand_id=$1`
		case "agent":
			query = `SELECT n.id::text,u.username AS name FROM agent_nodes n JOIN brand_members m ON m.id=n.member_id AND m.brand_id=n.brand_id JOIN global_users u ON u.id=m.global_user_id WHERE n.brand_id=$1`
		case "cycle":
			query = `SELECT id::text,to_char(window_from AT TIME ZONE 'UTC','YYYY-MM-DD HH24:MI')||' → '||to_char(window_to AT TIME ZONE 'UTC','YYYY-MM-DD HH24:MI')||' UTC' AS name FROM commission_cycles WHERE brand_id=$1`
		case "reward_order":
			query = `SELECT o.id::text,u.username||' · '||o.points::text||' · '||to_char(o.created_at AT TIME ZONE 'UTC','YYYY-MM-DD HH24:MI:SS')||' UTC' AS name FROM reward_orders o JOIN brand_members m ON m.id=o.member_id AND m.brand_id=o.brand_id JOIN global_users u ON u.id=m.global_user_id WHERE o.brand_id=$1`
		case "bet_order":
			query = `SELECT DISTINCT o.id::text,g.name||' · '||p.period_no||' · '||u.username||' · '||to_char(o.placed_at AT TIME ZONE 'UTC','YYYY-MM-DD HH24:MI:SS')||' UTC' AS name FROM commission_allocations a JOIN bet_orders o ON o.id=a.order_id AND o.brand_id=a.brand_id JOIN games g ON g.id=o.game_id AND g.brand_id=o.brand_id JOIN periods p ON p.id=o.period_id AND p.brand_id=o.brand_id JOIN brand_members m ON m.id=o.brand_member_id AND m.brand_id=o.brand_id JOIN global_users u ON u.id=m.global_user_id WHERE a.brand_id=$1`
		case "actor":
			query = `SELECT DISTINCT a.id::text,a.username AS name FROM audit_logs l JOIN admin_accounts a ON a.id=l.actor_id WHERE l.brand_id=$1 UNION SELECT DISTINCT u.id::text,u.username AS name FROM audit_logs l JOIN global_users u ON u.id=l.actor_id WHERE l.brand_id=$1`
		case "audit_resource":
			query = `SELECT DISTINCT ON (l.resource_id) l.resource_id::text AS id, l.resource_type||' · '||l.action||' · '||to_char(l.created_at AT TIME ZONE 'UTC','YYYY-MM-DD HH24:MI:SS')||' UTC' AS name FROM audit_logs l WHERE l.brand_id=$1 AND l.resource_id IS NOT NULL ORDER BY l.resource_id,l.created_at DESC`
		}
		rows, err := d.Admins.DB.Query(r.Context(), "SELECT id,name FROM ("+query+") options ORDER BY name,id LIMIT $2 OFFSET $3", brand, limit, offset)
		if err != nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法加载筛选项")
			return
		}
		defer rows.Close()
		type item struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		items := []item{}
		for rows.Next() {
			var out item
			if err = rows.Scan(&out.ID, &out.Name); err != nil {
				failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法加载筛选项")
				return
			}
			items = append(items, out)
		}
		if rows.Err() != nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法加载筛选项")
			return
		}
		if !adminReadAudit(w, r, d, actor, brand, "filter.options.view") {
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		respond(w, r, 200, map[string]any{"brand_id": brand, "items": items, "limit": limit, "offset": offset})
	})
}
