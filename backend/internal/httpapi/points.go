package httpapi

import (
	"context"
	"errors"
	"math"
	"net/http"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/finance"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func pointResult(data any, err error) (mutation.Result, error) {
	switch {
	case err == nil:
		return mutation.OK(200, data), nil
	case errors.Is(err, points.ErrInvalid):
		return mutation.Fail(400, "POINTS_INPUT_INVALID", "积分或操作资料不正确"), nil
	case errors.Is(err, points.ErrOverflow):
		return mutation.Fail(400, "POINTS_LIMIT_EXCEEDED", "积分超出整数上限"), nil
	case errors.Is(err, points.ErrPolicyLimit):
		return mutation.Fail(409, "POINTS_POLICY_LIMIT_EXCEEDED", "超出当前品牌积分限额"), nil
	case errors.Is(err, points.ErrInsufficient):
		return mutation.Fail(409, "POINTS_INSUFFICIENT", "对应来源或状态积分不足"), nil
	case errors.Is(err, points.ErrConflict):
		return mutation.Fail(409, "POINTS_OPERATION_CONFLICT", "业务记录已处理或版本已变化"), nil
	case errors.Is(err, points.ErrNotFound):
		return mutation.Fail(404, "POINTS_RECORD_NOT_FOUND", "当前品牌未找到对应积分记录"), nil
	case errors.Is(err, points.ErrCorrupt):
		return mutation.Fail(409, "POINTS_RECONCILIATION_REQUIRED", "余额异常，需人工对账处理"), nil
	default:
		return mutation.Result{}, err
	}
}
func pointOutput(w http.ResponseWriter, r *http.Request, data any, err error) {
	result, e := pointResult(data, err)
	outputMutation(w, r, result, e)
}
func pointMeta(r *http.Request, a access.Account) points.Metadata {
	m := meta(r)
	return points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: m.RequestID, IP: m.IP}
}
func ledgerKey(d Dependencies, r *http.Request, a access.Account, member, kind string) string {
	return kind + ":" + d.Mutations.Fingerprint(a.ID+":"+member+":"+r.Header.Get("Idempotency-Key"))
}
func registerPointRoutes(mux routeRegistrar, d Dependencies) {
	registerAgentUserRoutes(mux, d)
	registerJoinCodeUser(mux, d)
	for _, prefix := range []string{"/api/v1", "/api/v1/b/{brandCode}"} {
		for _, path := range []string{"/wallet", "/wallet/ledger"} {
			mux.HandleFunc("GET "+prefix+path, func(w http.ResponseWriter, r *http.Request) {
				if d.Identity == nil || d.Admins.DB == nil {
					failure(w, r, 503, "POINTS_UNAVAILABLE", "积分服务尚未配置")
					return
				}
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				defer cancel()
				r = r.WithContext(ctx)
				b, ok := resolveBrand(w, r, d)
				if !ok {
					return
				}
				v, err := d.Identity.Authenticate(ctx, b.ID, requestToken(r, userCookieName(b.ID)))
				if errors.Is(err, identity.ErrSession) {
					failure(w, r, 401, "AUTH_SESSION_REVOKED", "请重新登录")
					return
				}
				if err != nil {
					pointOutput(w, r, nil, err)
					return
				}
				s := points.Store{DB: d.Admins.DB}
				wallet, err := s.Read(ctx, b.ID, v.View.Member.ID)
				if err != nil {
					pointOutput(w, r, nil, err)
					return
				}
				if path == "/wallet" {
					respond(w, r, 200, wallet)
					return
				}
				limit, offset, ok := pageParams(r)
				if !ok {
					failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
					return
				}
				items, err := s.List(ctx, b.ID, v.View.Member.ID, limit, offset)
				pointOutput(w, r, map[string]any{"items": items}, err)
			})
		}
	}
}
func pointAdminActor(w http.ResponseWriter, r *http.Request, d Dependencies, resource, action string, write bool) (access.Account, string, bool) {
	a, brand, ok := managementActor(w, r, d, resource, action)
	if !ok {
		return a, brand, false
	}
	if write && (a.SuperAdmin || !access.Authorize(a, resource, action, access.ScopeBrand, brand)) {
		rejectAdmin(w, r, d, a, brand, resource+"."+action)
		return a, brand, false
	}
	return a, brand, true
}
func registerPointAdminRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	registerWorkbenchRoutes(handle, d)
	registerReportAdminRoutes(handle, d)
	registerPointSafetyRoutes(handle, d)
	s := points.Store{DB: d.Admins.DB}
	f := finance.Service{DB: d.Admins.DB, Points: s}
	for _, suffix := range []string{"", "/ledger", "/reconciliation"} {
		handle("GET", "/wallets/{memberID}"+suffix, func(w http.ResponseWriter, r *http.Request) {
			a, brand, ok := pointAdminActor(w, r, d, "wallet", "view", false)
			if !ok {
				return
			}
			member := r.PathValue("memberID")
			if !uuidPattern.MatchString(member) {
				failure(w, r, 400, "REQUEST_INVALID", "成员编号不正确")
				return
			}
			var data any
			var err error
			switch suffix {
			case "":
				data, err = s.Read(r.Context(), brand, member)
			case "/reconciliation":
				data, err = s.Reconcile(r.Context(), brand, member)
			default:
				_, err = s.Read(r.Context(), brand, member)
				if err == nil {
					limit, offset, valid := pageParams(r)
					if !valid {
						failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
						return
					}
					var items []points.Entry
					items, err = s.List(r.Context(), brand, member, limit, offset)
					data = map[string]any{"items": items}
				}
			}
			if err == nil && !adminReadAudit(w, r, d, a, brand, "wallet.view") {
				return
			}
			pointOutput(w, r, data, err)
		})
	}
	handle("GET", "/recharges", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := pointAdminActor(w, r, d, "recharge", "view", false)
		if !ok {
			return
		}
		member := r.URL.Query().Get("member_id")
		if member != "" && !uuidPattern.MatchString(member) {
			failure(w, r, 400, "REQUEST_INVALID", "成员编号不正确")
			return
		}
		limit, offset, ok := pageParams(r)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
			return
		}
		items, err := f.ListRecharges(r.Context(), brand, member, limit, offset)
		if err == nil && !adminReadAudit(w, r, d, a, brand, "recharge.view") {
			return
		}
		pointOutput(w, r, map[string]any{"items": items}, err)
	})
	handle("POST", "/recharges", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := pointAdminActor(w, r, d, "recharge", "write", true)
		if !ok {
			return
		}
		var in struct {
			MemberID string        `json:"member_id"`
			Points   points.Amount `json:"points"`
			Proof    string        `json:"proof_reference"`
			Remark   string        `json:"remark"`
			Reason   string        `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		managementWrite(w, r, d, a, brand, "recharge", "write", "admin.recharge.create", in.MemberID, in, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			if fresh.SuperAdmin {
				return mutation.Fail(403, "PERMISSION_DENIED", "超级管理员不可操作用户积分"), nil
			}
			record, err := f.CreateRecharge(ctx, tx, brand, in.MemberID, in.Points, in.Proof, in.Remark, in.Reason, pointMeta(r, fresh))
			result, e := pointResult(record, err)
			if err == nil {
				result.Status = 201
			}
			return result, e
		})
	})
	handle("POST", "/recharges/{id}/confirm", func(w http.ResponseWriter, r *http.Request) {
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
		managementWrite(w, r, d, a, brand, "recharge", "write", "admin.recharge.confirm", id, in, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			if fresh.SuperAdmin {
				return mutation.Fail(403, "PERMISSION_DENIED", "超级管理员不可操作用户积分"), nil
			}
			record, err := f.ConfirmRecharge(ctx, tx, brand, id, in.Version, in.Reason, pointMeta(r, fresh))
			return pointResult(record, err)
		})
	})
	for _, kind := range []string{"freeze", "unfreeze", "adjust"} {
		handle("POST", "/wallets/{memberID}/"+kind, func(w http.ResponseWriter, r *http.Request) {
			action := "freeze"
			if kind == "adjust" {
				action = "adjust"
			}
			a, brand, ok := pointAdminActor(w, r, d, "wallet", action, true)
			if !ok {
				return
			}
			member := r.PathValue("memberID")
			if !uuidPattern.MatchString(member) {
				failure(w, r, 400, "REQUEST_INVALID", "成员编号不正确")
				return
			}
			// Separate closed schemas: clients cannot add a system state, arbitrary delta
			// matrix or an original debit that is not an eligible manual freeze.
			var body any
			var amount points.Amount
			var source, entryID, reason string
			switch kind {
			case "freeze":
				var in struct {
					Points points.Amount `json:"points"`
					Reason string        `json:"reason"`
				}
				if !decodeBody(w, r, &in) {
					return
				}
				body = in
				amount, reason = in.Points, in.Reason
			case "unfreeze":
				var in struct {
					EntryID string `json:"entry_id"`
					Reason  string `json:"reason"`
				}
				if !decodeBody(w, r, &in) {
					return
				}
				body = in
				entryID, reason = in.EntryID, in.Reason
			default:
				var in struct {
					Source string        `json:"source"`
					Delta  points.Amount `json:"delta"`
					Reason string        `json:"reason"`
				}
				if !decodeBody(w, r, &in) {
					return
				}
				body = in
				source, amount, reason = in.Source, in.Delta, in.Reason
			}
			managementWrite(w, r, d, a, brand, "wallet", action, "admin.wallet."+kind, member, body, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
				if fresh.SuperAdmin {
					return mutation.Fail(403, "PERMISSION_DENIED", "超级管理员不可操作用户积分"), nil
				}
				m := pointMeta(r, fresh)
				key := ledgerKey(d, r, fresh, member, kind)
				var delta points.Balance
				var allocation []points.Allocation
				var err error
				reversal := ""
				switch kind {
				case "freeze":
					if amount <= 0 {
						return pointResult(nil, points.ErrInvalid)
					}
					var wallet points.Wallet
					wallet, err = s.LockedSnapshot(ctx, tx, brand, member)
					if err == nil {
						allocation, err = wallet.BySource.Allocate(amount, "available")
					}
					if err == nil {
						delta, err = points.AllocationDelta(allocation, "available", "manual_frozen")
					}
				case "unfreeze":
					var original points.Entry
					original, err = s.Entry(ctx, tx, brand, member, entryID)
					if err == nil {
						valid := original.EntryType == "freeze"
						for _, row := range original.Delta {
							valid = valid && row[0] <= 0 && row[1] >= 0 && row[0] == -row[1] && row[2] == 0 && row[3] == 0
						}
						if !valid {
							return pointResult(nil, points.ErrInvalid)
						}
						delta, err = points.Negate(original.Delta)
						allocation = original.Allocation
						reversal = original.ID
					}
				default:
					var si int
					si, err = points.SourceIndex(source)
					if err == nil {
						if amount == 0 {
							return pointResult(nil, points.ErrInvalid)
						}
						if amount == points.Amount(math.MinInt64) {
							return pointResult(nil, points.ErrOverflow)
						}
						delta[si][0] = amount
						positive := amount
						if positive < 0 {
							positive = -positive
						}
						allocation = []points.Allocation{{Source: source, State: "available", Points: positive}}
					}
				}
				if err != nil {
					return pointResult(nil, err)
				}
				if kind == "adjust" {
					policy, e := s.LockedPolicy(ctx, tx, brand)
					if e != nil {
						return pointResult(nil, e)
					}
					if e = policy.CheckAdjustment(delta); e != nil {
						return pointResult(nil, e)
					}
				}
				entry, err := s.Post(ctx, tx, points.Change{BrandID: brand, MemberID: member, EntryType: kind, ReferenceType: "manual", ReferenceID: member, OperationKey: key, Reason: reason, ActorType: m.ActorType, ActorID: m.ActorID, RequestID: m.RequestID, IP: m.IP, Delta: delta, Allocation: allocation, ReversalOf: reversal})
				return pointResult(entry, err)
			})
		})
	}
}
