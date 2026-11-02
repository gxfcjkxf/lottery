package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/finance"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func registerRechargeQuotaRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := finance.Service{DB: d.Admins.DB}
	for _, suffix := range []string{"", "/entries"} {
		handle("GET", "/recharge-quota"+suffix, func(w http.ResponseWriter, r *http.Request) {
			a, brand, ok := pointAdminActor(w, r, d, "recharge", "view", false)
			if !ok {
				return
			}
			var out any
			var err error
			if suffix == "" {
				out, err = s.ReadRechargeQuota(r.Context(), brand)
			} else {
				limit, offset, valid := pageParams(r)
				if !valid {
					failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
					return
				}
				items, e := s.ListRechargeQuotaEntries(r.Context(), brand, limit, offset)
				err = e
				if err == nil {
					out, err = countedItems(r.Context(), d.Admins.DB, items, `SELECT count(*)::text FROM brand_recharge_quota_entries e JOIN admin_accounts a ON a.id=e.actor_id WHERE e.brand_id=$1`, brand)
				}
			}
			if err == nil && !adminReadAudit(w, r, d, a, brand, "recharge_quota.view") {
				return
			}
			pointOutput(w, r, out, err)
		})
	}
	for _, operation := range []struct{ path, name string }{{"grants", "grant"}, {"reclaims", "reclaim"}} {
		handle("POST", "/recharge-quota/"+operation.path, func(w http.ResponseWriter, r *http.Request) {
			a, brand, ok := managementActor(w, r, d, "recharge_quota", "write")
			if !ok {
				return
			}
			if !a.SuperAdmin || !access.Authorize(a, "recharge_quota", "write", access.ScopePlatform, "") {
				rejectAdmin(w, r, d, a, brand, "recharge_quota.write.platform")
				return
			}
			var in struct {
				Points points.Amount `json:"points"`
			}
			if !decodeBody(w, r, &in) {
				return
			}
			raw, _ := json.Marshal(in)
			var fresh access.Account
			result, err := d.Mutations.ExecuteChecked(r.Context(), brand, a.ID, "platform.recharge_quota."+operation.name, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(string(raw)), func(ctx context.Context, tx pgx.Tx) error {
				var e error
				fresh, e = freshAdmin(ctx, tx, r, d, a, true)
				if e != nil {
					return e
				}
				if !fresh.SuperAdmin || !access.Authorize(fresh, "recharge_quota", "write", access.ScopePlatform, "") {
					return adminsys.ErrDenied
				}
				return nil
			}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
				var out finance.RechargeQuota
				var e error
				if operation.name == "reclaim" {
					out, e = s.ReclaimRechargeQuota(ctx, tx, brand, in.Points, pointMeta(r, fresh))
				} else {
					out, e = s.GrantRechargeQuota(ctx, tx, brand, in.Points, pointMeta(r, fresh))
				}
				if operation.name == "reclaim" && errors.Is(e, finance.ErrRechargeQuota) {
					return mutation.Fail(409, "RECHARGE_QUOTA_RECLAIM_EXCEEDED", "只能收回品牌剩余未使用的充值额度"), nil
				}
				return pointResult(out, e)
			})
			if errors.Is(err, identity.ErrSession) {
				result, err = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
			}
			if errors.Is(err, adminsys.ErrDenied) {
				result, err = mutation.Fail(403, "PERMISSION_DENIED", "无平台充值额度授权权限"), nil
			}
			finishAdminMutation(w, r, d, a, brand, "recharge_quota.write.platform", result, err)
		})
	}
}
