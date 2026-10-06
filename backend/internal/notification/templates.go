package notification

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

var ErrTemplateVersion = errors.New("notification template version conflict")

const maxTemplateVersion int64 = 9007199254740991

var templateURL = regexp.MustCompile(`(?i)(https?:|javascript:|data:|www\.)`)

var templateKeys = [...]string{
	"bet.order.abnormal",
	"bet.order.cancelled",
	"bet.order.judged_cancelled",
	"bet.order.placed",
	"bet.order.prize_reversed",
	"bet.order.won",
	"member.joined",
	"recharge.confirmed",
}

type Copy struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type Content struct {
	En   Copy `json:"en"`
	ZhCN Copy `json:"zh-CN"`
}

type Template struct {
	BrandID    string    `json:"brand_id"`
	Key        string    `json:"key"`
	Version    int64     `json:"version"`
	Content    Content   `json:"content"`
	UpdatedAt  time.Time `json:"updated_at"`
	AuditLogID *string   `json:"audit_log_id"`
}

type Revision struct {
	ID         string    `json:"id"`
	BrandID    string    `json:"brand_id"`
	Key        string    `json:"key"`
	Version    int64     `json:"version"`
	Content    Content   `json:"content"`
	ChangedBy  *string   `json:"changed_by"`
	Reason     string    `json:"reason"`
	AuditLogID *string   `json:"audit_log_id"`
	CreatedAt  time.Time `json:"created_at"`
}

type UpdateInput struct {
	Version int64   `json:"version"`
	Content Content `json:"content"`
	Reason  string  `json:"reason"`
}

func ValidTemplateKey(key string) bool {
	i := sortSearchTemplateKey(key)
	return i < len(templateKeys) && templateKeys[i] == key
}

func sortSearchTemplateKey(key string) int {
	lo, hi := 0, len(templateKeys)
	for lo < hi {
		mid := lo + (hi-lo)/2
		if templateKeys[mid] < key {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

func ValidateContent(key string, content Content) error {
	if !ValidTemplateKey(key) {
		return ErrInvalid
	}
	if !validCopyText(content.En.Title, 120, false) || !validCopyText(content.En.Body, 1200, true) ||
		!validCopyText(content.ZhCN.Title, 120, false) || !validCopyText(content.ZhCN.Body, 1200, true) {
		return ErrInvalid
	}
	enPoints := strings.Count(content.En.Body, "{points}")
	zhPoints := strings.Count(content.ZhCN.Body, "{points}")
	if key == "member.joined" {
		if enPoints != 0 || zhPoints != 0 || strings.Contains(content.En.Title, "{points}") || strings.Contains(content.ZhCN.Title, "{points}") {
			return ErrInvalid
		}
	} else if enPoints == 0 || zhPoints == 0 {
		return ErrInvalid
	}
	return nil
}

func validTemplateVersion(version int64) bool {
	return version >= 1 && version <= maxTemplateVersion
}

// validCopyText verifies canonical, safe plain text and supported placeholders.
func validCopyText(value string, maxBytes int, body bool) bool {
	if !utf8.ValidString(value) || value == "" || strings.TrimSpace(value) != value || len(value) > maxBytes ||
		strings.ContainsAny(value, "<>") || templateURL.MatchString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) && !(body && (r == '\n' || r == '\t')) {
			return false
		}
	}
	for i := 0; i < len(value); {
		switch value[i] {
		case '{':
			end := strings.IndexByte(value[i+1:], '}')
			if end < 0 {
				return false
			}
			name := value[i+1 : i+1+end]
			if name != "points" && name != "resource_id" {
				return false
			}
			i += end + 2
		case '}':
			return false
		default:
			_, size := utf8.DecodeRuneInString(value[i:])
			i += size
		}
	}
	return true
}

func AllowedTemplate(a access.Account, brand, action string) bool {
	if !uuid.MatchString(brand) {
		return false
	}
	switch action {
	case "write":
		return !a.SuperAdmin && access.Authorize(a, "notification_template", "write", access.ScopeBrand, brand)
	case "view":
		return access.Authorize(a, "notification_template", "view", access.ScopeBrand, brand) ||
			access.Authorize(a, "notification_template", "view", access.ScopePlatform, "")
	default:
		return false
	}
}

const templateFields = `brand_id::text,template_key,version,content,updated_at,
 COALESCE((SELECT r.audit_log_id::text FROM notification_template_revisions r WHERE r.brand_id=t.brand_id AND r.template_key=t.template_key AND r.version=t.version),'')`

func scanTemplate(row pgx.Row) (Template, error) {
	var out Template
	var raw []byte
	var auditID string
	if err := row.Scan(&out.BrandID, &out.Key, &out.Version, &raw, &out.UpdatedAt, &auditID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Template{}, ErrNotFound
		}
		return Template{}, err
	}
	if err := json.Unmarshal(raw, &out.Content); err != nil {
		return Template{}, err
	}
	if auditID != "" {
		out.AuditLogID = &auditID
	}
	out.UpdatedAt = out.UpdatedAt.UTC()
	return out, nil
}

type templateQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s Service) Templates(ctx context.Context, brand string) ([]Template, error) {
	if s.DB == nil || !uuid.MatchString(brand) {
		return nil, ErrInvalid
	}
	return templates(ctx, s.DB, brand)
}

func (s Service) TemplatesTx(ctx context.Context, tx pgx.Tx, brand string) ([]Template, error) {
	if tx == nil || !uuid.MatchString(brand) {
		return nil, ErrInvalid
	}
	return templates(ctx, tx, brand)
}

func templates(ctx context.Context, q templateQueryer, brand string) ([]Template, error) {
	rows, err := q.Query(ctx, `SELECT `+templateFields+` FROM notification_templates t WHERE brand_id=$1 ORDER BY template_key COLLATE "C"`, brand)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Template, 0, len(templateKeys))
	for rows.Next() {
		item, scanErr := scanTemplate(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		var exists bool
		if err = q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brands WHERE id=$1)`, brand).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrNotFound
		}
	}
	return out, nil
}

func (s Service) Template(ctx context.Context, brand, key string) (Template, error) {
	if s.DB == nil || !uuid.MatchString(brand) || !ValidTemplateKey(key) {
		return Template{}, ErrInvalid
	}
	return scanTemplate(s.DB.QueryRow(ctx, `SELECT `+templateFields+` FROM notification_templates t WHERE brand_id=$1 AND template_key=$2`, brand, key))
}

func (s Service) TemplateTx(ctx context.Context, tx pgx.Tx, brand, key string, lock bool) (Template, error) {
	if tx == nil || !uuid.MatchString(brand) || !ValidTemplateKey(key) {
		return Template{}, ErrInvalid
	}
	query := `SELECT ` + templateFields + ` FROM notification_templates t WHERE brand_id=$1 AND template_key=$2`
	if lock {
		query += ` FOR SHARE OF t`
	}
	return scanTemplate(tx.QueryRow(ctx, query, brand, key))
}

func (s Service) TemplateHistory(ctx context.Context, brand, key string, limit, offset int) ([]Revision, error) {
	if s.DB == nil || !uuid.MatchString(brand) || !ValidTemplateKey(key) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return nil, ErrInvalid
	}
	return templateHistory(ctx, s.DB, brand, key, limit, offset)
}

func (s Service) TemplateHistoryTx(ctx context.Context, tx pgx.Tx, brand, key string, limit, offset int) ([]Revision, error) {
	if tx == nil || !uuid.MatchString(brand) || !ValidTemplateKey(key) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return nil, ErrInvalid
	}
	return templateHistory(ctx, tx, brand, key, limit, offset)
}

func templateHistory(ctx context.Context, q templateQueryer, brand, key string, limit, offset int) ([]Revision, error) {
	if _, err := scanTemplate(q.QueryRow(ctx, `SELECT `+templateFields+` FROM notification_templates t WHERE brand_id=$1 AND template_key=$2`, brand, key)); err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `SELECT id::text,brand_id::text,template_key,version,content,changed_by::text,reason,audit_log_id::text,created_at
 FROM notification_template_revisions WHERE brand_id=$1 AND template_key=$2 ORDER BY version DESC LIMIT $3 OFFSET $4`, brand, key, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Revision, 0, limit)
	for rows.Next() {
		var item Revision
		var raw []byte
		if err = rows.Scan(&item.ID, &item.BrandID, &item.Key, &item.Version, &raw, &item.ChangedBy, &item.Reason, &item.AuditLogID, &item.CreatedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &item.Content); err != nil {
			return nil, err
		}
		item.CreatedAt = item.CreatedAt.UTC()
		out = append(out, item)
	}
	return out, rows.Err()
}

func validTemplateReason(reason string) bool {
	if !utf8.ValidString(reason) || len(strings.TrimSpace(reason)) < 1 || len(strings.TrimSpace(reason)) > 500 {
		return false
	}
	for _, r := range reason {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func (s Service) UpdateTemplate(ctx context.Context, tx pgx.Tx, brand, key string, a access.Account, in UpdateInput, meta points.Metadata) (Template, error) {
	if tx == nil || !uuid.MatchString(brand) || !ValidTemplateKey(key) || !validTemplateVersion(in.Version) || !validTemplateReason(in.Reason) {
		return Template{}, ErrInvalid
	}
	if !AllowedTemplate(a, brand, "write") {
		return Template{}, ErrDenied
	}
	if !uuid.MatchString(a.ID) || meta.ActorType != "admin" || meta.ActorID != a.ID || meta.RequestID == "" {
		return Template{}, ErrInvalid
	}
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE`, brand).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Template{}, ErrNotFound
		}
		return Template{}, err
	}
	if status == "disabled" {
		return Template{}, ErrState
	}
	before, err := scanTemplate(tx.QueryRow(ctx, `SELECT `+templateFields+` FROM notification_templates t WHERE brand_id=$1 AND template_key=$2 FOR UPDATE OF t`, brand, key))
	if err != nil {
		return Template{}, err
	}
	if before.Version != in.Version || before.Version >= maxTemplateVersion {
		return Template{}, ErrTemplateVersion
	}
	if err = ValidateContent(key, in.Content); err != nil {
		return Template{}, err
	}
	raw, err := json.Marshal(in.Content)
	if err != nil {
		return Template{}, err
	}
	var after Template
	var afterRaw []byte
	var updatedAt time.Time
	if err = tx.QueryRow(ctx, `UPDATE notification_templates SET version=version+1,content=$3,updated_at=clock_timestamp()
 WHERE brand_id=$1 AND template_key=$2 RETURNING brand_id::text,template_key,version,content,updated_at`, brand, key, raw).
		Scan(&after.BrandID, &after.Key, &after.Version, &afterRaw, &updatedAt); err != nil {
		return Template{}, err
	}
	if err = json.Unmarshal(afterRaw, &after.Content); err != nil {
		return Template{}, err
	}
	after.UpdatedAt = updatedAt.UTC()
	reason := strings.TrimSpace(in.Reason)
	beforeJSON := map[string]any{"brand_id": before.BrandID, "key": before.Key, "version": before.Version, "content": before.Content}
	afterJSON := map[string]any{"brand_id": after.BrandID, "key": after.Key, "version": after.Version, "content": after.Content}
	auditID, err := audit.Append(ctx, tx, audit.Record{
		BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "notification.template.update",
		ResourceType: "notification_template", ResourceID: brand, Reason: reason,
		RequestID: meta.RequestID, IP: meta.IP, Before: beforeJSON, After: afterJSON,
	})
	if err != nil {
		return Template{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO notification_template_revisions(id,brand_id,template_key,version,content,changed_by,reason,audit_log_id)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New(), brand, key, after.Version, raw, a.ID, reason, auditID); err != nil {
		return Template{}, err
	}
	after.AuditLogID = &auditID
	return after, nil
}
