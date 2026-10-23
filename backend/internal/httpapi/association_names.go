package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Associations are a presentation projection of identifiers already present in an
// authorized response. There is deliberately no identifier-lookup HTTP endpoint.
type associationName struct {
	Name    string `json:"name"`
	BuiltIn bool   `json:"built_in,omitempty"`
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
}

var associationFields = map[string]string{
	"cycle_id":        "cycle",
	"owner_member_id": "member", "target_member_id": "member", "actor_id": "actor",
	"brand_id": "brand", "brand_ids": "brand", "game_id": "game", "game_ids": "game",
	"period_id": "period", "play_id": "play", "role_ids": "role",
	"member_id": "member", "member_ids": "member", "brand_member_id": "member",
	"bettor_member_id": "member", "beneficiary_member_id": "member", "inviter_member_id": "member",
	"agent_id": "agent", "agent_ids": "agent", "mode_source_agent_id": "agent",
	"parent_agent_id": "agent", "beneficiary_agent_id": "agent", "inviter_agent_id": "agent",
	"global_user_id": "user", "created_by": "admin", "updated_by": "admin",
	"marked_by": "admin", "changed_by": "admin", "reviewed_by": "admin", "judged_by": "admin",
	"approved_by": "admin", "published_by": "admin", "connected_by": "admin",
}

// Do not interpret customer content, audit payloads, rule definitions or frozen
// requests as authoritative references. Those may contain arbitrary JSON keys.
var opaqueAssociationFields = map[string]bool{
	"before_json": true, "after_json": true, "before": true, "after": true,
	"definition": true, "selection": true, "config": true, "snapshot": true,
	"profile_snapshot": true, "attribution_snapshot": true, "body": true,
	"request_body": true, "raw_result": true, "payload": true,
}

func collectAssociationIDs(data any) map[string][]string {
	result := map[string][]string{}
	seen := map[string]bool{}
	add := func(kind string, value any) {
		values := []any{value}
		if list, ok := value.([]any); ok {
			values = list
		}
		for _, item := range values {
			id, ok := item.(string)
			if ok && uuidPattern.MatchString(id) && !seen[kind+":"+id] {
				seen[kind+":"+id] = true
				result[kind] = append(result[kind], id)
			}
		}
	}
	var walk func(any)
	walk = func(value any) {
		switch item := value.(type) {
		case []any:
			for _, child := range item {
				walk(child)
			}
		case map[string]any:
			if query, ok := item["query"].(map[string]any); ok {
				group, _ := query["group_by"].(string)
				if group == "member" || group == "agent" || group == "game" || group == "cycle" {
					if groups, ok := item["items"].([]any); ok {
						for _, entry := range groups {
							if row, ok := entry.(map[string]any); ok {
								add(group, row["key"])
							}
						}
					}
				}
			}
			if id, ok := item["id"].(string); ok {
				switch {
				case item["anchor_order_id"] != nil && item["window_from"] != nil:
					add("cycle", id)
				case item["is_bootstrap"] != nil:
					add("role", id)
				case item["role_ids"] != nil:
					add("admin", id)
				case item["global_user_id"] != nil:
					add("member", id)
				case item["period_no"] != nil:
					add("period", id)
				case item["agent_account_id"] != nil && item["path"] != nil:
					add("agent", id)
				case item["definition"] != nil && item["game_id"] != nil:
					add("play", id)
				case item["model"] != nil || item["timezone"] != nil && item["default_locale"] == nil && item["name"] != nil:
					add("game", id)
				case item["name"] != nil && item["code"] != nil:
					add("brand", id)
				case item["username"] != nil:
					add("user", id)
				}
			}
			for field, child := range item {
				if opaqueAssociationFields[field] {
					continue
				}
				if kind := associationFields[field]; kind != "" {
					add(kind, child)
				}
				if field == "parent_id" || field == "path" {
					if _, node := item["agent_account_id"]; node {
						add("agent", child)
					}
				}
				if field == "actor_id" {
					if item["actor_type"] == "admin" {
						add("admin", child)
					}
				}
				walk(child)
			}
		}
	}
	walk(data)
	return result
}

var associationQueries = map[string]string{
	"actor":  `SELECT id::text,username,false FROM admin_accounts WHERE id=ANY($1::uuid[]) UNION ALL SELECT id::text,COALESCE(username,''),false FROM global_users WHERE id=ANY($1::uuid[])`,
	"brand":  `SELECT id::text,name,false FROM brands WHERE id=ANY($1::uuid[])`,
	"role":   `SELECT id::text,name,is_bootstrap FROM roles WHERE id=ANY($1::uuid[])`,
	"member": `SELECT m.id::text,COALESCE(u.username,''),false FROM brand_members m JOIN global_users u ON u.id=m.global_user_id WHERE m.id=ANY($1::uuid[])`,
	"user":   `SELECT id::text,COALESCE(username,''),false FROM global_users WHERE id=ANY($1::uuid[])`,
	"admin":  `SELECT id::text,username,false FROM admin_accounts WHERE id=ANY($1::uuid[])`,
	"agent":  `SELECT n.id::text,a.username,false FROM agent_nodes n JOIN agent_accounts a ON a.id=n.member_id AND a.brand_id=n.brand_id WHERE n.id=ANY($1::uuid[])`,
	"game":   `SELECT id::text,name,false FROM games WHERE id=ANY($1::uuid[]) UNION ALL SELECT id::text,name,false FROM platform_games WHERE id=ANY($1::uuid[])`,
	"period": `SELECT id::text,period_no,false FROM periods WHERE id=ANY($1::uuid[]) UNION ALL SELECT id::text,period_no,false FROM platform_game_periods WHERE id=ANY($1::uuid[])`,
	"play":   `SELECT id::text,name,false FROM play_definitions WHERE id=ANY($1::uuid[]) UNION ALL SELECT id::text,name,false FROM platform_game_plays WHERE id=ANY($1::uuid[])`,
}

func responseAssociationNames(r *http.Request, data any) (map[string]associationName, bool) {
	out := map[string]associationName{}
	raw, err := json.Marshal(data)
	if err != nil {
		return out, true
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return out, true
	}
	ids := collectAssociationIDs(value)
	if len(ids) == 0 {
		return out, false
	}
	d, ok := r.Context().Value(sessionIdentityKey{}).(Dependencies)
	if !ok || d.Admins.DB == nil {
		return out, true
	}
	for kind, values := range ids {
		if kind == "cycle" {
			rows, err := d.Admins.DB.Query(r.Context(), `SELECT id::text,window_from,window_to FROM commission_cycles WHERE id=ANY($1::uuid[])`, values)
			if err != nil {
				return out, true
			}
			for rows.Next() {
				var id string
				var from, to time.Time
				if rows.Scan(&id, &from, &to) != nil {
					rows.Close()
					return out, true
				}
				out[kind+":"+id] = associationName{From: from.UTC().Format(time.RFC3339Nano), To: to.UTC().Format(time.RFC3339Nano)}
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return out, true
			}
			continue
		}
		rows, err := d.Admins.DB.Query(r.Context(), associationQueries[kind], values)
		if err != nil {
			return out, true
		}
		for rows.Next() {
			var id string
			var name associationName
			if rows.Scan(&id, &name.Name, &name.BuiltIn) != nil {
				rows.Close()
				return out, true
			}
			name.Name = strings.TrimSpace(name.Name)
			out[kind+":"+id] = name
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, true
		}
	}
	return out, false
}
