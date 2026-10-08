package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/reportarchive"
	"github.com/jackc/pgx/v5"
)

const archivePolicyRoutePath = "/report-archive-policy"
const archiveTasksPath = "/report-archive-tasks"

type archiveTaskRetryHTTPInput struct {
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}

func (in *archiveTaskRetryHTTPInput) UnmarshalJSON(raw []byte) error {
	fields, err := reconciliationClosedObject(raw, map[string]bool{"version": true, "reason": true})
	if err != nil || len(fields) != 2 {
		return reportarchive.ErrInvalid
	}
	var next archiveTaskRetryHTTPInput
	if json.Unmarshal(fields["version"], &next.Version) != nil || json.Unmarshal(fields["reason"], &next.Reason) != nil || next.Version < 1 || next.Version >= 9007199254740991 || next.Reason == "" || len(next.Reason) > 500 || strings.TrimSpace(next.Reason) != next.Reason {
		return reportarchive.ErrInvalid
	}
	for _, c := range next.Reason {
		if unicode.IsControl(c) {
			return reportarchive.ErrInvalid
		}
	}
	*in = next
	return nil
}

func archiveTaskHTTPAuth(ctx context.Context, tx pgx.Tx, r *http.Request, d Dependencies, initial access.Account, brand, action string) (access.Account, error) {
	a, err := freshAdmin(ctx, tx, r, d, initial, false)
	if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
		return access.Account{}, identity.ErrSession
	}
	if err != nil {
		return a, archiveHTTPDBError(err)
	}
	if action == "retry" {
		actor := r.Header.Get("X-Report-Archive-Actor-ID")
		if !cycleCanonicalID(actor) {
			return a, reportarchive.ErrInvalid
		}
		if actor != a.ID {
			return a, errReportArchiveActor
		}
	}
	if !reportarchive.AllowedAutomatic(a, brand, action) {
		return a, reportarchive.ErrDenied
	}
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE NOWAIT`, brand).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, reportarchive.ErrNotFound
	}
	if err != nil {
		return a, archiveHTTPDBError(err)
	}
	if action == "retry" && status == "disabled" {
		return a, reportarchive.ErrState
	}
	return a, nil
}

func archiveTaskHTTPRead(r *http.Request, d Dependencies, initial access.Account, brand, auditAction string, query func(pgx.Tx) (any, error)) (any, error) {
	ctx := r.Context()
	tx, err := d.Admins.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	check := func() (access.Account, error) { return archiveTaskHTTPAuth(ctx, tx, r, d, initial, brand, "view") }
	a, err := check()
	if errors.Is(err, reportarchive.ErrDenied) {
		_, err = audit.Append(ctx, tx, audit.Record{ActorType: "admin", ActorID: a.ID, Action: "access.denied", ResourceType: "permission", Reason: "administrative permission denied", RequestID: requestID(r), IP: meta(r).IP, After: map[string]string{"attempted_brand": brand, "permission": "report_archive.view"}})
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			return nil, err
		}
		return nil, reportarchive.ErrDenied
	}
	if err != nil {
		return nil, err
	}
	out, queryErr := query(tx)
	if _, err = check(); err != nil {
		return nil, err
	}
	_, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: auditAction, ResourceType: "query", RequestID: requestID(r), IP: meta(r).IP})
	if err == nil {
		_, err = check()
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		return nil, archiveHTTPDBError(err)
	}
	return out, queryErr
}

// Activation is deliberately not registered until its business start rule is
// agreed. These reads and explicit failure retries do not enable a policy.
func registerReportArchiveTaskRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := reportarchive.Service{DB: d.Admins.DB}
	for _, path := range []string{archivePolicyRoutePath, archiveTasksPath, archiveTasksPath + "/{id}"} {
		handle("GET", path, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.ForceQuery || !reconciliationGetHasNoBody(w, r) {
				if r.URL.ForceQuery {
					failure(w, r, 400, "REQUEST_INVALID", "不接受空查询串")
				}
				return
			}
			brand, ok := reconciliationBrand(w, r)
			if !ok {
				return
			}
			id, limit, offset := r.PathValue("id"), 20, 0
			if path == archiveTasksPath {
				limit, offset, _, ok = reconciliationPage(r, false)
			} else {
				ok = r.URL.RawQuery == "" && (path == archivePolicyRoutePath || cycleCanonicalID(id))
			}
			if !ok {
				failure(w, r, 400, "REQUEST_INVALID", "归档任务编号或查询参数不正确")
				return
			}
			initial, ok := adminAccount(w, r, d)
			if !ok {
				return
			}
			action := "report_archive.task.read"
			if path == archivePolicyRoutePath {
				action = "report_archive.policy.read"
			} else if path == archiveTasksPath {
				action = "report_archive.task.list"
			}
			out, err := archiveTaskHTTPRead(r, d, initial, brand, action, func(tx pgx.Tx) (any, error) {
				if path == archivePolicyRoutePath {
					return s.AutomaticPolicyTx(r.Context(), tx, brand)
				}
				if path == archiveTasksPath {
					return s.AutomaticTasksTx(r.Context(), tx, brand, limit, offset)
				}
				return s.AutomaticTaskTx(r.Context(), tx, brand, id)
			})
			archiveHTTPOutput(w, r, out, err)
		})
	}
	handle("POST", archiveTasksPath+"/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" || r.URL.ForceQuery || !cycleCanonicalID(r.PathValue("id")) {
			failure(w, r, 400, "REQUEST_INVALID", "归档任务重试不接受查询参数且须规范编号")
			return
		}
		brand, ok := reconciliationBrand(w, r)
		if !ok {
			return
		}
		var in archiveTaskRetryHTTPInput
		if !decodeBody(w, r, &in) {
			return
		}
		initial, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		id := r.PathValue("id")
		raw, _ := json.Marshal(in)
		var fresh access.Account
		check := func(ctx context.Context, tx pgx.Tx) error {
			a, err := archiveTaskHTTPAuth(ctx, tx, r, d, initial, brand, "retry")
			if err == nil {
				fresh = a
			}
			return err
		}
		result, err := d.Mutations.ExecuteChecked(r.Context(), brand, initial.ID, "admin.report_archive.task.retry", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(brand+":"+id+":"+string(raw)), check, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			out, e := s.RetryAutomaticTaskTx(ctx, tx, brand, id, fresh, in.Version, in.Reason, pointMeta(r, fresh))
			response, e := archiveHTTPResult(out, archiveHTTPDBError(e))
			if e != nil {
				return mutation.Result{}, e
			}
			if e = check(ctx, tx); e != nil {
				return mutation.Result{}, e
			}
			return response, nil
		})
		err = archiveHTTPDBError(err)
		if errors.Is(err, reportarchive.ErrBusy) || errors.Is(err, reportarchive.ErrIntegrity) {
			archiveHTTPOutput(w, r, nil, err)
			return
		}
		if err != nil {
			result, err = archiveHTTPResult(nil, err)
		}
		finishAdminMutation(w, r, d, initial, brand, "report_archive_task.retry", result, err)
	})
}
