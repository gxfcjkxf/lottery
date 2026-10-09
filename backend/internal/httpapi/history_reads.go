package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/jackc/pgx/v5"
)

var errHistoryPermission = errors.New("history permission changed")

func historyFailure(w http.ResponseWriter, r *http.Request, e error) bool {
	if errors.Is(e, identity.ErrSession) {
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录")
		return true
	}
	if errors.Is(e, errHistoryPermission) {
		failure(w, r, 403, "PERMISSION_DENIED", "无历史查询权限")
		return true
	}
	return false
}

func auditedHistory(w http.ResponseWriter, r *http.Request, d Dependencies, a access.Account, brand, action string, allowed func(access.Account) bool, run func(pgx.Tx) (any, error)) (any, error) {
	return auditedHistoryRecord(w, r, d, a, allowed, audit.Record{BrandID: brand, Action: action, ResourceType: "query"}, run)
}

func auditedHistoryRecord(w http.ResponseWriter, r *http.Request, d Dependencies, a access.Account, allowed func(access.Account) bool, record audit.Record, run func(pgx.Tx) (any, error)) (any, error) {
	ctx := r.Context()
	tx, e := d.Admins.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	check := func() error {
		fresh, err := freshAdmin(ctx, tx, r, d, a, false)
		if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
			return identity.ErrSession
		}
		if err != nil {
			return err
		}
		if !allowed(fresh) {
			return errHistoryPermission
		}
		return nil
	}
	if e = check(); e != nil {
		return nil, e
	}
	out, source, e := d.HistoryReads.Read(ctx, tx, database.HistoryRoute(record.Action), run)
	if e == nil {
		e = check()
	}
	if e == nil {
		record.ActorType = "admin"
		record.ActorID = a.ID
		record.RequestID = requestID(r)
		record.IP = meta(r).IP
		_, e = audit.Append(ctx, tx, record)
	}
	if e == nil {
		e = check() // A session can expire while the audit insert is blocked.
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		return nil, e
	} // Never release history data before audit commit.
	w.Header().Set("X-Read-Source", source.Name())
	w.Header().Set("X-Read-Reason", source.Reason)
	if source.Replica > 0 {
		w.Header().Set("X-Read-Replica", strconv.Itoa(source.Replica))
	}
	return out, nil
}
