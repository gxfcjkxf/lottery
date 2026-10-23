package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Exact account-name selection for association forms. The target operation's
// existing permission is required; this does not grant member or agent browsing.
func registerAssociationLookup(handle func(string, string, http.HandlerFunc), d Dependencies) {
	handle("GET", "/association-selection", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for _, values := range q {
			if len(values) != 1 {
				failure(w, r, 400, "REQUEST_INVALID", "关联账号选择参数无效")
				return
			}
		}
		kind, resource, action, username := q.Get("kind"), q.Get("resource"), q.Get("action"), strings.ToLower(strings.TrimSpace(q.Get("username")))
		allowed := resource == "reward" && action == "grant" && kind == "member" || resource == "agent" && action == "write" || resource == "join_code" && action == "write" || resource == "commission" && action == "view" || resource == "report_commission" && action == "view" || resource == "report_attribution" && action == "view"
		if len(q) != 4 || !allowed || (kind != "member" && kind != "agent") || username == "" || len(username) > 100 {
			failure(w, r, 400, "REQUEST_INVALID", "关联账号选择参数无效")
			return
		}
		actor, brand, ok := pointAdminActor(w, r, d, resource, action, resource == "reward")
		if !ok {
			return
		}
		query := `SELECT m.id::text,m.id::text,u.username FROM brand_members m JOIN global_users u ON u.id=m.global_user_id WHERE m.brand_id=$1 AND u.username=$2`
		if kind == "agent" {
			query = `SELECT n.id::text,a.id::text,a.username FROM agent_nodes n JOIN agent_accounts a ON a.id=n.member_id AND a.brand_id=n.brand_id WHERE n.brand_id=$1 AND a.username=$2`
		}
		var id, member, name string
		err := d.Admins.DB.QueryRow(r.Context(), query, brand, username).Scan(&id, &member, &name)
		if errors.Is(err, pgx.ErrNoRows) {
			failure(w, r, 404, "MEMBER_NOT_FOUND", "当前品牌未找到该关联账号")
			return
		}
		if err != nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法选择关联账号")
			return
		}
		if !adminReadAudit(w, r, d, actor, brand, "association.select") {
			return
		}
		respond(w, r, 200, map[string]string{"id": id, "member_id": member, "brand_id": brand, "username": name})
	})
}
