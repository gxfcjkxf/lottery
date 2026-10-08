package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/finance"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func registerPointSafetyRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	registerReconciliationRoutes(handle, d)
	registerJoinCodeAdmin(handle, d)
	registerAgentAdminRoutes(handle, d)
	registerWithdrawalPolicyRoutes(handle, d)
	registerCommissionPolicyRoutes(handle, d)
	registerCommissionCycleRoutes(handle, d)
	registerCommissionDiscoveryRoutes(handle, d)
	registerCommissionPaymentRoutes(handle, d)
	registerCommissionAdjustmentRoutes(handle, d)
	registerCommissionCorrectionRoutes(handle, d)
	registerRewardRoutes(handle, d)
	registerNotificationAdminRoutes(handle, d)
	s := points.Store{DB: d.Admins.DB}
	f := finance.Service{DB: d.Admins.DB, Points: s}
	handle("GET", "/point-policy", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := pointAdminActor(w, r, d, "point_policy", "view", false)
		if !ok {
			return
		}
		out, err := s.ReadPolicy(r.Context(), brand)
		if err == nil && !adminReadAudit(w, r, d, a, brand, "point_policy.view") {
			return
		}
		pointOutput(w, r, out, err)
	})
	handle("PUT", "/point-policy", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := pointAdminActor(w, r, d, "point_policy", "write", true)
		if !ok {
			return
		}
		var in points.PolicyInput
		if !decodeBody(w, r, &in) {
			return
		}
		managementWrite(w, r, d, a, brand, "point_policy", "write", "admin.point_policy.update", brand, in, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			if fresh.SuperAdmin {
				return mutation.Fail(403, "PERMISSION_DENIED", "无资金操作权限"), nil
			}
			out, err := s.UpdatePolicy(ctx, tx, brand, in, pointMeta(r, fresh))
			return pointResult(out, err)
		})
	})
	handle("POST", "/recharges/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := pointAdminActor(w, r, d, "recharge", "write", true)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !uuidPattern.MatchString(id) {
			failure(w, r, 400, "REQUEST_INVALID", "充值编号不正确")
			return
		}
		var in struct {
			Version int64  `json:"version"`
			Reason  string `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		managementWrite(w, r, d, a, brand, "recharge", "write", "admin.recharge.cancel", id, in, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			if fresh.SuperAdmin {
				return mutation.Fail(403, "PERMISSION_DENIED", "无资金操作权限"), nil
			}
			out, err := f.CancelRecharge(ctx, tx, brand, id, in.Version, in.Reason, pointMeta(r, fresh))
			return pointResult(out, err)
		})
	})
	handle("GET", "/wallets/{memberID}/repair-preview", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := pointAdminActor(w, r, d, "wallet", "view", false)
		if !ok {
			return
		}
		out, err := s.PreviewRepair(r.Context(), brand, r.PathValue("memberID"))
		if err == nil && !adminReadAudit(w, r, d, a, brand, "wallet.repair_preview") {
			return
		}
		pointOutput(w, r, out, err)
	})
	handle("POST", "/wallets/{memberID}/repair", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := pointAdminActor(w, r, d, "wallet", "repair", true)
		if !ok {
			return
		}
		member := r.PathValue("memberID")
		var in struct {
			Version int64  `json:"version"`
			Token   string `json:"token"`
			Reason  string `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		managementWrite(w, r, d, a, brand, "wallet", "repair", "admin.wallet.repair", member, in, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			if fresh.SuperAdmin {
				return mutation.Fail(403, "PERMISSION_DENIED", "无资金操作权限"), nil
			}
			out, err := s.RepairBalance(ctx, tx, brand, member, in.Version, in.Token, in.Reason, pointMeta(r, fresh))
			return pointResult(out, err)
		})
	})
	handle("GET", "/wallets/{memberID}/repairs", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := pointAdminActor(w, r, d, "wallet", "view", false)
		if !ok {
			return
		}
		limit, offset, valid := pageParams(r)
		if !valid {
			failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
			return
		}
		out, err := s.ListRepairs(r.Context(), brand, r.PathValue("memberID"), limit, offset)
		if err == nil && !adminReadAudit(w, r, d, a, brand, "wallet.repairs.view") {
			return
		}
		pointOutput(w, r, map[string]any{"items": out}, err)
	})
}
