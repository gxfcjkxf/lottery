package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/jackc/pgx/v5"
)

const auditExportRows = 10000
const auditExportBytes = 4 * 1024 * 1024

var errAuditExportLarge = errors.New("audit export exceeds bound")

type auditQuery struct {
	From         *time.Time `json:"from,omitempty"`
	To           *time.Time `json:"to,omitempty"`
	Action       string     `json:"action,omitempty"`
	ActorID      string     `json:"actor_id,omitempty"`
	ResourceType string     `json:"resource_type,omitempty"`
	ResourceID   string     `json:"resource_id,omitempty"`
	RequestID    string     `json:"request_id,omitempty"`
	Limit        int        `json:"limit"`
	Offset       int        `json:"offset"`
}

func parseAuditQuery(r *http.Request, export bool) (auditQuery, error) {
	q := auditQuery{Limit: 100}
	v, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		return q, e
	}
	for key, values := range v {
		if len(values) != 1 || values[0] == "" || !utf8.ValidString(values[0]) || strings.ContainsRune(values[0], 0) {
			return q, adminsys.ErrInvalid
		}
		switch key {
		case "from", "to", "action", "actor_id", "resource_type", "resource_id", "request_id":
		case "limit", "offset":
			if export {
				return q, adminsys.ErrInvalid
			}
		default:
			return q, adminsys.ErrInvalid
		}
	}
	if export || v.Has("from") || v.Has("to") {
		from, e1 := time.Parse(time.RFC3339Nano, v.Get("from"))
		to, e2 := time.Parse(time.RFC3339Nano, v.Get("to"))
		if e1 != nil || e2 != nil || !strings.HasSuffix(v.Get("from"), "Z") || !strings.HasSuffix(v.Get("to"), "Z") || !from.Before(to) || to.Sub(from) > 31*24*time.Hour || from.Year() < 1 || to.Year() > 9999 {
			return q, adminsys.ErrInvalid
		}
		from, to = from.UTC(), to.UTC()
		q.From, q.To = &from, &to
	}
	for key, dst := range map[string]*string{"actor_id": &q.ActorID, "resource_id": &q.ResourceID} {
		*dst = strings.ToLower(v.Get(key))
		if *dst != "" && !uuidPattern.MatchString(*dst) {
			return q, adminsys.ErrInvalid
		}
	}
	for key, dst := range map[string]*string{"action": &q.Action, "resource_type": &q.ResourceType, "request_id": &q.RequestID} {
		*dst = v.Get(key)
		if len(*dst) > 128 || strings.TrimSpace(*dst) != *dst {
			return q, adminsys.ErrInvalid
		}
		for _, char := range *dst {
			if unicode.IsControl(char) {
				return q, adminsys.ErrInvalid
			}
		}
	}
	if v.Has("limit") {
		q.Limit, e = strconv.Atoi(v.Get("limit"))
		if e != nil {
			return q, e
		}
	}
	if v.Has("offset") {
		q.Offset, e = strconv.Atoi(v.Get("offset"))
		if e != nil {
			return q, e
		}
	}
	if q.Limit < 1 || q.Limit > 200 || q.Offset < 0 || q.Offset > 100000 {
		return q, adminsys.ErrInvalid
	}
	if export {
		q.Limit = auditExportRows + 1
	}
	return q, nil
}

// AdminAuditRecord is the actual read projection shared with contract examples.
// It contains stored, already-redacted audit facts, never live credentials.
type AdminAuditRecord struct {
	ID           string          `json:"id"`
	BrandID      *string         `json:"brand_id"`
	Action       string          `json:"action"`
	ActorType    string          `json:"actor_type"`
	ActorID      string          `json:"actor_id"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	Reason       string          `json:"reason"`
	RequestID    string          `json:"request_id"`
	CreatedAt    time.Time       `json:"created_at"`
	IP           string          `json:"ip_address"`
	Before       json.RawMessage `json:"before_json"`
	After        json.RawMessage `json:"after_json"`
}

type auditItem = AdminAuditRecord

// One SELECT snapshot, not stitched pagination. Bound JSON text in the SELECT
// before transfer so a pathological audit record cannot allocate an unbounded
// export body. Oversized records fail the whole request, never get truncated.
func auditItems(r *http.Request, tx pgx.Tx, brand string, q auditQuery, export bool) ([]auditItem, time.Time, error) {
	rows, e := tx.Query(r.Context(), `WITH bounds AS (SELECT statement_timestamp() AS at)
 SELECT b.at,a.id::text,a.brand_id::text,a.action,a.actor_type,coalesce(a.actor_id::text,''),a.resource_type,coalesce(a.resource_id::text,''),a.reason,a.request_id,a.created_at,coalesce(a.ip_address,''),
 CASE WHEN octet_length(coalesce(a.before_json,'null'::jsonb)::text)<=$11 THEN coalesce(a.before_json,'null'::jsonb) ELSE NULL END,
 CASE WHEN octet_length(coalesce(a.after_json,'null'::jsonb)::text)<=$11 THEN coalesce(a.after_json,'null'::jsonb) ELSE NULL END
 FROM bounds b LEFT JOIN LATERAL (
 SELECT * FROM audit_logs WHERE ($1='' OR brand_id=NULLIF($1,'')::uuid)
 AND ($2::timestamptz IS NULL OR created_at >= $2) AND ($3::timestamptz IS NULL OR created_at < $3)
 AND created_at <= b.at AND ($4='' OR action=$4) AND ($5='' OR actor_id=NULLIF($5,'')::uuid)
 AND ($6='' OR resource_type=$6) AND ($7='' OR resource_id=NULLIF($7,'')::uuid) AND ($8='' OR request_id=$8)
 ORDER BY created_at DESC,id DESC LIMIT $9 OFFSET $10
 ) a ON TRUE ORDER BY a.created_at DESC,a.id DESC`, brand, q.From, q.To, q.Action, q.ActorID, q.ResourceType, q.ResourceID, q.RequestID, q.Limit, q.Offset, auditExportBytes)
	if e != nil {
		return nil, time.Time{}, e
	}
	defer rows.Close()
	items := []auditItem{}
	var snapshot time.Time
	bytesUsed := 0
	for rows.Next() {
		var raw []byte
		// The left join's empty row supplies a snapshot even for zero matches.
		var id *string
		var item auditItem
		var action, actorType, actorID, resource, resourceID, reason, requestID, ip *string
		var created *time.Time
		var before, after []byte
		e = rows.Scan(&snapshot, &id, &item.BrandID, &action, &actorType, &actorID, &resource, &resourceID, &reason, &requestID, &created, &ip, &before, &after)
		if e != nil {
			return nil, snapshot, e
		}
		if id == nil {
			continue
		}
		if before == nil || after == nil {
			return nil, snapshot, errAuditExportLarge
		}
		item.ID, item.Action, item.ActorType, item.ActorID, item.ResourceType, item.ResourceID, item.Reason, item.RequestID, item.CreatedAt, item.IP, item.Before, item.After = *id, *action, *actorType, *actorID, *resource, *resourceID, *reason, *requestID, created.UTC(), *ip, before, after
		raw, e = json.Marshal(item)
		if e != nil {
			return nil, snapshot, e
		}
		bytesUsed += len(raw)
		if bytesUsed > auditExportBytes || (export && len(items) >= auditExportRows) {
			return nil, snapshot, errAuditExportLarge
		}
		items = append(items, item)
	}
	return items, snapshot.UTC(), rows.Err()
}

func auditAllowed(a access.Account, brand string, export bool) bool {
	grant := func(action string) bool {
		return access.Authorize(a, "audit", action, access.ScopePlatform, "") || access.Authorize(a, "audit", action, access.ScopeBrand, brand)
	}
	return grant("view") && (!export || grant("export"))
}

type auditCSVBuffer struct{ bytes.Buffer }

func (b *auditCSVBuffer) Write(p []byte) (int, error) {
	if len(p) > auditExportBytes-b.Len() {
		return 0, errAuditExportLarge
	}
	return b.Buffer.Write(p)
}
func auditCSVCell(s string) string {
	lead := strings.TrimLeftFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) || r == '\uFEFF' })
	if strings.ContainsAny(s, "\t\r\n") || (lead != "" && strings.ContainsRune("=+-@", rune(lead[0]))) {
		return "'" + s
	}
	return s
}
func auditCSV(items []auditItem) ([]byte, error) {
	b := &auditCSVBuffer{}
	_, _ = b.Write([]byte("\xef\xbb\xbf"))
	w := csv.NewWriter(b)
	w.UseCRLF = true
	if e := w.Write([]string{"id", "brand_id", "action", "actor_type", "actor_id", "resource_type", "resource_id", "reason", "request_id", "created_at", "ip_address", "before_json", "after_json"}); e != nil {
		return nil, e
	}
	for _, i := range items {
		brand := ""
		if i.BrandID != nil {
			brand = *i.BrandID
		}
		cells := []string{i.ID, brand, i.Action, i.ActorType, i.ActorID, i.ResourceType, i.ResourceID, i.Reason, i.RequestID, i.CreatedAt.UTC().Format(time.RFC3339Nano), i.IP, string(i.Before), string(i.After)}
		for j, s := range cells {
			cells[j] = auditCSVCell(s)
		}
		if e := w.Write(cells); e != nil {
			return nil, e
		}
	}
	w.Flush()
	if e := w.Error(); e != nil {
		return nil, e
	}
	return b.Bytes(), nil
}

func auditExportFailure(w http.ResponseWriter, r *http.Request, e error) {
	if historyFailure(w, r, e) {
		return
	}
	if errors.Is(e, identity.ErrSession) || errors.Is(e, adminsys.ErrDenied) || errors.Is(e, adminsys.ErrNotFound) {
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "管理会话或授权已变化")
		return
	}
	if errors.Is(e, errAuditExportLarge) {
		failure(w, r, 422, "AUDIT_EXPORT_TOO_LARGE", "导出超过10000条或4MiB，请缩小范围；未生成截断文件")
		return
	}
	failure(w, r, 503, "AUDIT_EXPORT_UNAVAILABLE", "无法完整生成或审计导出")
}

func registerAuditRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	handle("GET", "/audit", func(w http.ResponseWriter, r *http.Request) {
		a, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		brand := strings.ToLower(r.Header.Get("X-Brand-ID"))
		if (brand != "" && !uuidPattern.MatchString(brand)) || !auditAllowed(a, brand, false) {
			rejectAdmin(w, r, d, a, brand, "audit.view")
			return
		}
		q, e := parseAuditQuery(r, false)
		if e != nil {
			failure(w, r, 400, "REQUEST_INVALID", "审计筛选参数无效")
			return
		}
		out, e := auditedHistoryRecord(w, r, d, a, func(fresh access.Account) bool { return auditAllowed(fresh, brand, false) }, audit.Record{BrandID: brand, Action: "audit.view", ResourceType: "audit", After: q}, func(tx pgx.Tx) (any, error) {
			items, _, e := auditItems(r, tx, brand, q, false)
			return map[string]any{"items": items}, e
		})
		if historyFailure(w, r, e) {
			return
		}
		if errors.Is(e, errAuditExportLarge) {
			failure(w, r, 422, "AUDIT_QUERY_TOO_LARGE", "审计查询超过4MiB，请缩小筛选范围或分页条数；未返回截断数据")
			return
		}
		if e != nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "审计查询或记录失败")
			return
		}
		respond(w, r, 200, out)
	})
	handle("GET", "/audit/export", func(w http.ResponseWriter, r *http.Request) {
		a, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		brand := strings.ToLower(r.Header.Get("X-Brand-ID"))
		if !uuidPattern.MatchString(brand) {
			failure(w, r, 400, "REQUEST_INVALID", "导出必须选择有效品牌")
			return
		}
		if !auditAllowed(a, brand, true) {
			rejectAdmin(w, r, d, a, brand, "audit.export")
			return
		}
		q, e := parseAuditQuery(r, true)
		if e != nil {
			failure(w, r, 400, "REQUEST_INVALID", "导出须指定不超过31天的时间范围，不接受分页参数")
			return
		}
		tx, e := d.Admins.DB.Begin(r.Context())
		if e != nil {
			auditExportFailure(w, r, e)
			return
		}
		defer tx.Rollback(r.Context())
		check := func() error {
			fresh, e := freshAdmin(r.Context(), tx, r, d, a, false)
			if e != nil {
				return e
			}
			if !auditAllowed(fresh, brand, true) {
				return errHistoryPermission
			}
			return nil
		}
		if e = check(); e != nil {
			auditExportFailure(w, r, e)
			return
		}
		items, snapshot, e := auditItems(r, tx, brand, q, true)
		if e != nil {
			auditExportFailure(w, r, e)
			return
		}
		body, e := auditCSV(items)
		if e != nil {
			auditExportFailure(w, r, e)
			return
		}
		if e = check(); e != nil {
			auditExportFailure(w, r, e)
			return
		}
		sum := sha256.Sum256(body)
		digest := hex.EncodeToString(sum[:])
		id, e := audit.Append(r.Context(), tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "audit.export", ResourceType: "audit_export", RequestID: requestID(r), IP: meta(r).IP, After: map[string]any{"query": q, "snapshot_at": snapshot, "row_count": len(items), "byte_count": len(body), "sha256": digest, "format_version": 1}})
		if e == nil {
			e = check()
		}
		if e == nil {
			e = tx.Commit(r.Context())
		}
		if e != nil {
			auditExportFailure(w, r, e)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="audit-`+brand+`-v1.csv"`)
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Audit-Brand-ID", brand)
		w.Header().Set("X-Audit-Snapshot-At", snapshot.Format(time.RFC3339Nano))
		w.Header().Set("X-Audit-Row-Count", fmt.Sprint(len(items)))
		w.Header().Set("X-Audit-SHA256", digest)
		w.Header().Set("X-Audit-Format-Version", "1")
		w.Header().Set("X-Audit-Export-ID", id)
		w.WriteHeader(200)
		_, _ = w.Write(body)
	})
}
