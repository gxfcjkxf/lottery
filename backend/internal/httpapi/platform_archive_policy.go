package httpapi

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/reportarchive"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func registerPlatformArchivePolicyRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := reportarchive.Service{DB: d.Admins.DB}
	valid := func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.RawQuery != "" || r.URL.ForceQuery || r.Header.Get("X-Brand-ID") != "" {
			failure(w, r, 400, "REQUEST_INVALID", "平台归档配置不接受品牌或查询参数")
			return false
		}
		return true
	}
	handle("GET", "/report-archive-policy", func(w http.ResponseWriter, r *http.Request) {
		if !valid(w, r) {
			return
		}
		a, ok := platformAccountActor(w, r, d, "platform_settings", "view")
		if !ok {
			return
		}
		tx, err := d.Admins.DB.Begin(r.Context())
		if err != nil {
			archiveHTTPOutput(w, r, nil, err)
			return
		}
		defer tx.Rollback(r.Context())
		out, err := s.PlatformPolicyTx(r.Context(), tx)
		if err == nil {
			_, err = audit.Append(r.Context(), tx, audit.Record{ActorType: "admin", ActorID: a.ID, Action: "report_archive.platform_policy.read", ResourceType: "query", RequestID: requestID(r), IP: meta(r).IP})
		}
		if err == nil {
			_, err = freshAdmin(r.Context(), tx, r, d, a, true)
		}
		if err == nil {
			err = tx.Commit(r.Context())
		}
		archiveHTTPOutput(w, r, out, err)
	})
	handle("PUT", "/report-archive-policy", func(w http.ResponseWriter, r *http.Request) {
		if !valid(w, r) {
			return
		}
		a, ok := platformAccountActor(w, r, d, "platform_settings", "write")
		if !ok {
			return
		}
		var in archivePolicyActivationHTTPInput
		if !decodeBody(w, r, &in) {
			return
		}
		raw, _ := json.Marshal(in)
		var fresh access.Account
		check := func(ctx context.Context, tx pgx.Tx) error {
			var err error
			fresh, err = freshAdmin(ctx, tx, r, d, a, true)
			if err != nil {
				return err
			}
			if r.Header.Get("X-Report-Archive-Actor-ID") != fresh.ID {
				return identity.ErrSession
			}
			if !access.Authorize(fresh, "platform_settings", "write", access.ScopePlatform, "") {
				return reportarchive.ErrDenied
			}
			return nil
		}
		result, err := d.Mutations.ExecuteChecked(r.Context(), "", a.ID, "platform.report_archive.policy.update", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(string(raw)), check, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			out, e := s.UpdatePlatformPolicyTx(ctx, tx, fresh, reportarchive.AutomaticActivationInput(in), pointMeta(r, fresh))
			return archiveHTTPResult(out, e)
		})
		if err != nil {
			result, err = archiveHTTPResult(nil, err)
		}
		finishAdminMutation(w, r, d, a, "", "report_archive.platform_policy.update", result, err)
	})
}
