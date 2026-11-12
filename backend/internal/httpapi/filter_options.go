package httpapi

import (
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"
)

// Options inherit the target screen's read permission and brand scope.
func registerFilterOptions(handle func(string, string, http.HandlerFunc), d Dependencies) {
	handle("GET", "/filter-options", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		kind, resource := q.Get("kind"), q.Get("resource")
		allowed := map[string]string{
			"recharge": "member", "withdrawal": "member", "bet": "game",
			"report_betting": "game member", "report_ledger": "member", "report_withdrawal": "member",
			"report_commission": "agent member cycle", "report_attribution": "game agent member",
			"report_rewards": "member reward_order", "report_reward_orders": "member reward_order",
			"period": "game period", "draw": "game period", "bet_policy": "game", "withdrawal_policy": "game",
			"audit": "actor audit_resource", "commission": "agent member cycle bet_order", "reward": "reward_order member",
		}
		if !slices.Contains(strings.Fields(allowed[resource]), kind) || kind == "" || allowed[resource] == "" {
			failure(w, r, 400, "REQUEST_INVALID", "筛选项类型无效")
			return
		}
		for key, values := range q {
			if len(values) != 1 || (key != "kind" && key != "resource" && key != "limit" && key != "offset" && key != "search" && key != "action") {
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
		action := q.Get("action")
		if action == "" {
			action = "view"
		}
		if action != "view" && !(resource == "reward" && kind == "member" && action == "grant") {
			failure(w, r, 400, "REQUEST_INVALID", "筛选参数无效")
			return
		}
		actor, brand, ok := pointAdminActor(w, r, d, permissionResource, action, action == "grant")
		if !ok {
			return
		}
		var query string
		switch kind {
		case "game":
			switch resource {
			case "bet":
				query = `SELECT DISTINCT g.id::text,g.name FROM platform_games g JOIN configured_orders o ON o.game_id=g.id WHERE o.brand_id=$1`
			case "bet_policy":
				query = `SELECT g.id::text,g.name FROM platform_games g JOIN brand_platform_games b ON b.game_id=g.id WHERE b.brand_id=$1 AND g.status<>'deleted'`
			case "period", "draw", "withdrawal_policy":
				query = `SELECT g.id::text,g.name FROM platform_games g JOIN brand_platform_games b ON b.game_id=g.id WHERE b.brand_id=$1`
			case "report_betting", "report_attribution":
				query = `SELECT g.id::text,g.name FROM platform_games g JOIN brand_platform_games b ON b.game_id=g.id WHERE b.brand_id=$1`
			}
		case "period":
			query = `SELECT p.id::text,g.name||' · '||p.period_no AS name FROM platform_game_periods p JOIN platform_games g ON g.id=p.game_id JOIN brand_platform_games b ON b.game_id=g.id WHERE b.brand_id=$1`
		case "member":
			query = `SELECT m.id::text,u.username AS name FROM brand_members m JOIN global_users u ON u.id=m.global_user_id WHERE m.brand_id=$1`
		case "agent":
			query = `SELECT n.id::text,a.username AS name FROM agent_nodes n JOIN agent_accounts a ON a.id=n.member_id AND a.brand_id=n.brand_id WHERE n.brand_id=$1`
		case "cycle":
			query = `SELECT id::text,to_char(window_from AT TIME ZONE 'UTC','YYYY-MM-DD HH24:MI')||' → '||to_char(window_to AT TIME ZONE 'UTC','YYYY-MM-DD HH24:MI')||' UTC' AS name FROM commission_cycles WHERE brand_id=$1`
		case "reward_order":
			query = `SELECT o.id::text,u.username||' · '||o.points::text||' · '||to_char(o.created_at AT TIME ZONE 'UTC','YYYY-MM-DD HH24:MI:SS')||' UTC' AS name FROM reward_orders o JOIN brand_members m ON m.id=o.member_id AND m.brand_id=o.brand_id JOIN global_users u ON u.id=m.global_user_id WHERE o.brand_id=$1`
		case "bet_order":
			query = `SELECT DISTINCT o.id::text,g.name||' · '||p.period_no||' · '||u.username||' · '||to_char(o.placed_at AT TIME ZONE 'UTC','YYYY-MM-DD HH24:MI:SS')||' UTC' AS name FROM commission_allocations a JOIN betting_financial_facts o ON o.id=a.order_id AND o.brand_id=a.brand_id JOIN platform_games g ON g.id=o.game_id JOIN platform_game_periods p ON p.id=o.period_id JOIN brand_members m ON m.id=o.brand_member_id AND m.brand_id=o.brand_id JOIN global_users u ON u.id=m.global_user_id WHERE a.brand_id=$1`
		case "actor":
			query = `SELECT DISTINCT a.id::text,a.username AS name FROM retention_audit_logs() l JOIN admin_accounts a ON a.id=l.actor_id WHERE l.brand_id=$1 UNION SELECT DISTINCT u.id::text,u.username AS name FROM retention_audit_logs() l JOIN global_users u ON u.id=l.actor_id WHERE l.brand_id=$1`
		case "audit_resource":
			query = `SELECT DISTINCT ON (l.resource_id) l.resource_id::text AS id, l.resource_type||' · '||l.action||' · '||to_char(l.created_at AT TIME ZONE 'UTC','YYYY-MM-DD HH24:MI:SS')||' UTC' AS name FROM retention_audit_logs() l WHERE l.brand_id=$1 AND l.resource_id IS NOT NULL ORDER BY l.resource_id,l.created_at DESC`
		}
		search := strings.TrimSpace(q.Get("search"))
		if _, supplied := q["search"]; supplied && !slices.Contains([]string{"member", "agent", "actor"}, kind) {
			failure(w, r, 400, "REQUEST_INVALID", "该筛选项不支持账号搜索")
			return
		}
		if slices.Contains([]string{"member", "agent", "actor"}, kind) && utf8.RuneCountInString(search) < 2 {
			failure(w, r, 400, "REQUEST_INVALID", "账号搜索至少输入两个字符")
			return
		}
		rows, err := d.Admins.DB.Query(r.Context(), "SELECT id,name FROM ("+query+") options WHERE starts_with(lower(name),lower($4)) ORDER BY name,id LIMIT $2 OFFSET $3", brand, limit, offset, search)
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
