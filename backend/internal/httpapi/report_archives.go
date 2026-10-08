package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/reportarchive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const reportArchivesPath = "/report-archives"

var errReportArchiveActor = errors.New("report archive actor context changed")

type archiveHTTPInput struct{ reportarchive.Input }

func (in *archiveHTTPInput) UnmarshalJSON(raw []byte) error {
	fields, err := reconciliationClosedObject(raw, map[string]bool{"kind": true, "period_key": true, "expected_revision": true, "reason": true})
	if err != nil || len(fields) != 4 {
		return reportarchive.ErrInvalid
	}
	var next reportarchive.Input
	if json.Unmarshal(fields["kind"], &next.Kind) != nil || json.Unmarshal(fields["period_key"], &next.PeriodKey) != nil || json.Unmarshal(fields["expected_revision"], &next.ExpectedRevision) != nil || json.Unmarshal(fields["reason"], &next.Reason) != nil || next.ExpectedRevision < 0 || next.ExpectedRevision >= 9007199254740991 || next.Reason == "" || len(next.Reason) > 500 || strings.TrimSpace(next.Reason) != next.Reason {
		return reportarchive.ErrInvalid
	}
	for _, c := range next.Reason {
		if unicode.IsControl(c) {
			return reportarchive.ErrInvalid
		}
	}
	if _, err = reportarchive.ResolveWindow(next.Kind, next.PeriodKey, "UTC"); err != nil {
		return reportarchive.ErrInvalid
	}
	in.Input = next
	return nil
}

func archiveHTTPDBError(err error) error {
	var p *pgconn.PgError
	if errors.As(err, &p) && (p.Code == "55P03" || p.Code == "40P01" || p.Code == "40001") {
		return reportarchive.ErrBusy
	}
	return err
}

func archiveHTTPResult(out any, err error) (mutation.Result, error) {
	switch {
	case err == nil:
		return mutation.OK(200, out), nil
	case errors.Is(err, identity.ErrSession):
		return mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
	case errors.Is(err, errReportArchiveActor):
		return mutation.Fail(401, "AUTH_ACTOR_CONTEXT_CHANGED", "管理员身份与确认时不同，请重新核对"), nil
	case errors.Is(err, reportarchive.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "无归档操作权限"), nil
	case errors.Is(err, reportarchive.ErrInvalid):
		return mutation.Fail(400, "REPORT_ARCHIVE_INPUT_INVALID", "归档请求参数不正确"), nil
	case errors.Is(err, reportarchive.ErrNotFound):
		return mutation.Fail(404, "REPORT_ARCHIVE_NOT_FOUND", "当前品牌未找到该归档"), nil
	case errors.Is(err, reportarchive.ErrVersion):
		return mutation.Fail(409, "REPORT_ARCHIVE_VERSION_CONFLICT", "归档版本已变化，请重新读取"), nil
	case errors.Is(err, reportarchive.ErrState):
		return mutation.Fail(409, "REPORT_ARCHIVE_STATE_CONFLICT", "当前品牌或归档周期不允许此操作"), nil
	default:
		return mutation.Result{}, err
	}
}

func archiveHTTPOutput(w http.ResponseWriter, r *http.Request, out any, err error) {
	err = archiveHTTPDBError(err)
	if errors.Is(err, reportarchive.ErrBusy) {
		failure(w, r, 503, "REPORT_ARCHIVE_BUSY", "归档数据正在变化，请稍后重试")
		return
	}
	if errors.Is(err, reportarchive.ErrIntegrity) {
		failure(w, r, 503, "REPORT_ARCHIVE_INTEGRITY_UNAVAILABLE", "归档完整性无法验证，未返回文件或快照")
		return
	}
	result, err := archiveHTTPResult(out, err)
	outputMutation(w, r, result, err)
}

func archiveHTTPAuth(ctx context.Context, tx pgx.Tx, r *http.Request, d Dependencies, initial access.Account, brand, action string) (access.Account, error) {
	a, err := freshAdmin(ctx, tx, r, d, initial, false)
	if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
		return access.Account{}, identity.ErrSession
	}
	if err != nil {
		return a, archiveHTTPDBError(err)
	}
	if action == "create" {
		actor := r.Header.Get("X-Report-Archive-Actor-ID")
		if !cycleCanonicalID(actor) {
			return a, reportarchive.ErrInvalid
		}
		if actor != a.ID {
			return a, errReportArchiveActor
		}
	}
	if !reportarchive.Allowed(a, brand, action) {
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
	if action == "create" && status == "disabled" {
		return a, reportarchive.ErrState
	}
	return a, nil
}

type archiveHTTPDownload struct {
	Record reportarchive.Record
	Body   []byte
	SHA    string
}

func archiveHTTPRead(r *http.Request, d Dependencies, initial access.Account, brand, id, action string, query func(pgx.Tx) (any, error)) (any, error) {
	ctx := r.Context()
	tx, err := d.Admins.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	check := func() (access.Account, error) { return archiveHTTPAuth(ctx, tx, r, d, initial, brand, action) }
	a, err := check()
	if errors.Is(err, reportarchive.ErrDenied) {
		_, err = audit.Append(ctx, tx, audit.Record{ActorType: "admin", ActorID: a.ID, Action: "access.denied", ResourceType: "permission", Reason: "administrative permission denied", RequestID: requestID(r), IP: meta(r).IP, After: map[string]string{"attempted_brand": brand, "permission": "report_archive." + action}})
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
	var after any
	if file, ok := out.(archiveHTTPDownload); ok && queryErr == nil {
		after = map[string]any{"archive_id": id, "revision": file.Record.Revision, "format_version": file.Record.Snapshot.FormatVersion, "payload_sha256": file.SHA, "bytes": len(file.Body)}
	}
	auditAction := "report_archive.read"
	if id == "" {
		auditAction = "report_archive.list"
	} else if action == "download" {
		auditAction = "report_archive.download"
	}
	_, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: auditAction, ResourceType: "query", RequestID: requestID(r), IP: meta(r).IP, After: after})
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

func registerReportArchiveRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := reportarchive.Service{DB: d.Admins.DB}
	for _, path := range []string{reportArchivesPath, reportArchivesPath + "/{id}", reportArchivesPath + "/{id}/download"} {
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
			id := r.PathValue("id")
			limit, offset := 20, 0
			if path == reportArchivesPath {
				limit, offset, _, ok = reconciliationPage(r, false)
			} else {
				ok = cycleCanonicalID(id) && r.URL.RawQuery == ""
			}
			if !ok {
				failure(w, r, 400, "REQUEST_INVALID", "归档编号或查询参数不正确")
				return
			}
			initial, ok := adminAccount(w, r, d)
			if !ok {
				return
			}
			action := "view"
			if strings.HasSuffix(path, "/download") {
				action = "download"
			}
			out, err := archiveHTTPRead(r, d, initial, brand, id, action, func(tx pgx.Tx) (any, error) {
				if id == "" {
					return s.ListTx(r.Context(), tx, brand, limit, offset)
				}
				record, e := s.ReadTx(r.Context(), tx, brand, id)
				if e != nil || action != "download" {
					return record, e
				}
				body, sha, e := s.CanonicalPayloadTx(r.Context(), tx, brand, id)
				return archiveHTTPDownload{record, body, sha}, e
			})
			if err != nil || action != "download" {
				archiveHTTPOutput(w, r, out, err)
				return
			}
			file := out.(archiveHTTPDownload)
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="report-archive-%s-v%d.json"`, id, file.Record.Revision))
			w.Header().Set("Content-Length", strconv.Itoa(len(file.Body)))
			w.Header().Set("X-Content-SHA256", file.SHA)
			w.Header().Set("X-Archive-Revision", strconv.FormatInt(file.Record.Revision, 10))
			w.Header().Set("X-Archive-Format-Version", strconv.Itoa(file.Record.Snapshot.FormatVersion))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(file.Body)
		})
	}
	handle("POST", reportArchivesPath, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			failure(w, r, 400, "REQUEST_INVALID", "归档写入不接受查询参数")
			return
		}
		brand, ok := reconciliationBrand(w, r)
		if !ok {
			return
		}
		var in archiveHTTPInput
		if !decodeBody(w, r, &in) {
			return
		}
		initial, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		raw, _ := json.Marshal(in.Input)
		var fresh access.Account
		check := func(ctx context.Context, tx pgx.Tx) error {
			a, err := archiveHTTPAuth(ctx, tx, r, d, initial, brand, "create")
			if err == nil {
				fresh = a
			}
			return err
		}
		result, err := d.Mutations.ExecuteChecked(r.Context(), brand, initial.ID, "admin.report_archive.create", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(brand+":"+string(raw)), check, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			out, e := s.CreateTx(ctx, tx, brand, fresh, in.Input, pointMeta(r, fresh))
			e = archiveHTTPDBError(e)
			response, e := archiveHTTPResult(out, e)
			if e != nil {
				return mutation.Result{}, e
			}
			if e = check(ctx, tx); e != nil {
				return mutation.Result{}, e
			}
			if response.Error == nil {
				response.Status = http.StatusCreated
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
		finishAdminMutation(w, r, d, initial, brand, "report_archive.create", result, err)
	})
}
