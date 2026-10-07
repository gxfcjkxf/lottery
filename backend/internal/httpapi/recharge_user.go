package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/finance"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

var canonicalRechargePageNumber = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

type memberRechargeQuery struct {
	Limit  int
	Offset int
	State  string
}

func parseMemberRechargeQuery(r *http.Request) (memberRechargeQuery, bool) {
	out := memberRechargeQuery{Limit: 20}
	if r.URL.ForceQuery {
		return out, false
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return out, false
	}
	for key, entries := range values {
		if len(entries) != 1 || entries[0] == "" {
			return out, false
		}
		switch key {
		case "limit":
			if !canonicalRechargePageNumber.MatchString(entries[0]) {
				return out, false
			}
			out.Limit, err = strconv.Atoi(entries[0])
		case "offset":
			if !canonicalRechargePageNumber.MatchString(entries[0]) {
				return out, false
			}
			out.Offset, err = strconv.Atoi(entries[0])
		case "state":
			out.State = entries[0]
		default:
			return out, false
		}
		if err != nil {
			return out, false
		}
	}
	if out.State != "" && out.State != "pending" && out.State != "confirmed" && out.State != "cancelled" {
		return out, false
	}
	return out, out.Limit >= 1 && out.Limit <= 100 && out.Offset >= 0 && out.Offset <= 1_000_000
}

func rechargeUserNoBody(r *http.Request) bool {
	if r.ContentLength > 0 || len(r.TransferEncoding) != 0 {
		return false
	}
	if r.Body == nil || r.Body == http.NoBody {
		return true
	}
	var one [1]byte
	n, err := r.Body.Read(one[:])
	return n == 0 && err == io.EOF
}

func rechargeUserUnavailable(w http.ResponseWriter, r *http.Request, d Dependencies) bool {
	if d.Identity == nil || d.Admins.DB == nil || d.Brands == nil {
		failure(w, r, 503, "RECHARGE_UNAVAILABLE", "充值记录查询暂不可用")
		return true
	}
	return false
}

func rechargeUserError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case err == nil:
		return
	case errors.Is(err, identity.ErrSession):
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "请先登录")
	case errors.Is(err, points.ErrInvalid):
		failure(w, r, 400, "RECHARGE_INPUT_INVALID", "充值记录查询参数不正确")
	case errors.Is(err, points.ErrNotFound):
		failure(w, r, 404, "RECHARGE_NOT_FOUND", "当前账号没有对应充值记录")
	default:
		failure(w, r, 503, "RECHARGE_UNAVAILABLE", "充值记录查询暂不可用")
	}
}

func rechargeUserAuditRecord(r *http.Request, brand, member, action, resourceType, resourceID, outcome string, query map[string]any) audit.Record {
	return audit.Record{
		BrandID: brand, ActorType: "user", ActorID: "", Action: action,
		ResourceType: resourceType, ResourceID: resourceID,
		RequestID: requestID(r), IP: meta(r).IP,
		After: map[string]any{"member_id": member, "query": query, "outcome": outcome},
	}
}

func readMemberRechargeUserTx(r *http.Request, d Dependencies, brand string, action string, query map[string]any, run func(pgx.Tx, identity.Session) (any, string, error)) (any, error) {
	tx, err := d.Admins.DB.Begin(r.Context())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(r.Context())

	session, err := d.Identity.AuthenticateTx(r.Context(), tx, brand, requestToken(r, userCookieName(brand)))
	if err != nil {
		return nil, err
	}
	out, resourceID, readErr := run(tx, session)
	outcome := "success"
	if errors.Is(readErr, points.ErrNotFound) {
		outcome = "not_found"
	}
	if readErr != nil && outcome != "not_found" {
		return nil, readErr
	}
	resourceType := "query"
	if resourceID != "" {
		resourceType = "recharge_order"
	}
	record := rechargeUserAuditRecord(r, brand, session.Member.ID, action, resourceType, resourceID, outcome, query)
	record.ActorID = session.User.ID
	if _, err = audit.Append(r.Context(), tx, record); err != nil {
		return nil, err
	}
	// Audit storage can wait independently of the projection. Recheck expiry
	// after that wait as well; an expired read rolls its audit back with no DTO.
	if _, err = d.Identity.AuthenticateTx(r.Context(), tx, brand, requestToken(r, userCookieName(brand))); err != nil {
		return nil, err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return nil, err
	}
	return out, readErr
}

func registerRechargeUserRoutes(mux routeRegistrar, d Dependencies) {
	service := finance.Service{DB: d.Admins.DB}
	for _, prefix := range []string{"/api/v1", "/api/v1/b/{brandCode}"} {
		for _, path := range []string{"/recharges", "/recharges/{id}"} {
			mux.HandleFunc("GET "+prefix+path, func(w http.ResponseWriter, r *http.Request) {
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				defer cancel()
				r = r.WithContext(ctx)
				if rechargeUserUnavailable(w, r, d) {
					return
				}
				brand, ok := resolveBrand(w, r, d)
				if !ok {
					return
				}
				if !rechargeUserNoBody(r) {
					failure(w, r, 400, "REQUEST_INVALID", "充值记录查询不接受请求正文")
					return
				}

				var out any
				var err error
				if path == "/recharges" {
					q, valid := parseMemberRechargeQuery(r)
					if !valid {
						failure(w, r, 400, "REQUEST_INVALID", "充值记录分页参数不正确")
						return
					}
					query := map[string]any{"limit": q.Limit, "offset": q.Offset, "state": nil}
					if q.State != "" {
						query["state"] = q.State
					}
					out, err = readMemberRechargeUserTx(r, d, brand.ID, "finance.recharge.user.view", query, func(tx pgx.Tx, session identity.Session) (any, string, error) {
						page, e := service.ListMemberRechargesTx(r.Context(), tx, brand.ID, session.Member.ID, q.State, q.Limit, q.Offset)
						return page, "", e
					})
				} else {
					if r.URL.RawQuery != "" || r.URL.ForceQuery {
						failure(w, r, 400, "REQUEST_INVALID", "充值记录详情不接受查询参数")
						return
					}
					id := strings.ToLower(r.PathValue("id"))
					if !uuidPattern.MatchString(id) {
						failure(w, r, 400, "REQUEST_INVALID", "充值记录编号不正确")
						return
					}
					out, err = readMemberRechargeUserTx(r, d, brand.ID, "finance.recharge.user.detail", map[string]any{}, func(tx pgx.Tx, session identity.Session) (any, string, error) {
						record, e := service.ReadMemberRechargeTx(r.Context(), tx, brand.ID, session.Member.ID, id)
						if e != nil {
							return nil, "", e
						}
						return record, record.ID, nil
					})
				}
				rechargeUserError(w, r, err)
				if err == nil {
					respond(w, r, 200, out)
				}
			})
		}
	}
}
