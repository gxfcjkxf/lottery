// Package branddomains manages registered HTTP authorities, not DNS or TLS.
package branddomains

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalid   = errors.New("invalid brand domain input")
	ErrDenied    = errors.New("brand domain permission denied")
	ErrNotFound  = errors.New("brand domain not found")
	ErrVersion   = errors.New("brand domain version conflict")
	ErrState     = errors.New("brand domain state conflict")
	ErrConflict  = errors.New("domain already reserved or binding limit reached")
	uuidPattern  = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	labelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)
	numericTail  = regexp.MustCompile(`^(?:[0-9]+|0x[0-9a-f]+)$`)
	letterTail   = regexp.MustCompile(`[a-z]`)
)

type Domain struct {
	ID      string `json:"id"`
	Domain  string `json:"domain"`
	Enabled bool   `json:"enabled"`
	Primary bool   `json:"is_primary"`
}
type Record struct {
	BrandID    string    `json:"brand_id"`
	Version    int64     `json:"version"`
	Status     string    `json:"status"`
	Domains    []Domain  `json:"domains"`
	UpdatedAt  time.Time `json:"updated_at"`
	AuditLogID string    `json:"audit_log_id,omitempty"`
}
type Revision struct {
	ID            string    `json:"id"`
	BrandID       string    `json:"brand_id"`
	Version       int64     `json:"version"`
	ChangedBy     string    `json:"changed_by"`
	Reason        string    `json:"reason"`
	AuditLogID    string    `json:"audit_log_id"`
	CreatedAt     time.Time `json:"created_at"`
	BeforeDomains []Domain  `json:"before_domains"`
	Domains       []Domain  `json:"domains"`
}
type Input struct {
	Version int64  `json:"version"`
	Domain  string `json:"domain,omitempty"`
	Enabled bool   `json:"enabled"`
	Primary bool   `json:"is_primary"`
	Reason  string `json:"reason"`
}

func (in *Input) UnmarshalJSON(raw []byte) error {
	if in == nil || !utf8.Valid(raw) {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, e := d.Token()
	if e != nil || token != json.Delim('{') {
		return ErrInvalid
	}
	seen := map[string]bool{}
	var next Input
	for d.More() {
		token, e = d.Token()
		key, ok := token.(string)
		if e != nil || !ok || seen[key] {
			return ErrInvalid
		}
		seen[key] = true
		var value json.RawMessage
		if d.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return ErrInvalid
		}
		switch key {
		case "version":
			e = json.Unmarshal(value, &next.Version)
		case "domain":
			e = json.Unmarshal(value, &next.Domain)
			if next.Domain == "" {
				return ErrInvalid
			}
		case "enabled":
			e = json.Unmarshal(value, &next.Enabled)
		case "is_primary":
			e = json.Unmarshal(value, &next.Primary)
		case "reason":
			e = json.Unmarshal(value, &next.Reason)
		default:
			return ErrInvalid
		}
		if e != nil {
			return ErrInvalid
		}
	}
	if token, e = d.Token(); e != nil || token != json.Delim('}') || !seen["version"] || !seen["enabled"] || !seen["is_primary"] || !seen["reason"] {
		return ErrInvalid
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		return ErrInvalid
	}
	*in = next
	return nil
}
func ValidDomain(host string) bool {
	if host == "" || len(host) > 253 || strings.ToLower(host) != host || net.ParseIP(host) != nil {
		return false
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 || numericTail.MatchString(labels[len(labels)-1]) || !letterTail.MatchString(labels[len(labels)-1]) {
		return false
	}
	for _, label := range labels {
		if len(label) < 1 || len(label) > 63 || !labelPattern.MatchString(label) {
			return false
		}
	}
	switch labels[len(labels)-1] {
	case "localhost", "local", "localdomain", "internal":
		return false
	}
	return true
}
func Allowed(a access.Account, brand, action string) bool {
	return uuidPattern.MatchString(brand) && (action == "view" || action == "write") && (access.Authorize(a, "brand_domains", action, access.ScopeBrand, brand) || access.Authorize(a, "brand_domains", action, access.ScopePlatform, ""))
}

type Service struct{ DB *pgxpool.Pool }

const domainProjection = `COALESCE((SELECT jsonb_agg(jsonb_build_object('id',d.id,'domain',d.domain,'enabled',d.enabled,'is_primary',d.is_primary) ORDER BY d.domain,d.id) FROM brand_domains d WHERE d.brand_id=b.id),'[]'::jsonb)`
const recordSQL = `SELECT b.id::text,b.config_version,b.status,` + domainProjection + `,b.updated_at,
 COALESCE((SELECT h.audit_log_id::text FROM brand_domain_revisions h WHERE h.brand_id=b.id AND h.version=b.config_version),'') FROM brands b WHERE b.id=$1`

func read(row pgx.Row) (Record, error) {
	var r Record
	var raw []byte
	e := row.Scan(&r.BrandID, &r.Version, &r.Status, &raw, &r.UpdatedAt, &r.AuditLogID)
	if errors.Is(e, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	if e != nil {
		return r, e
	}
	e = json.Unmarshal(raw, &r.Domains)
	r.UpdatedAt = r.UpdatedAt.UTC()
	return r, e
}
func (s Service) Read(ctx context.Context, brand string) (Record, error) {
	if s.DB == nil || !uuidPattern.MatchString(brand) {
		return Record{}, ErrInvalid
	}
	return read(s.DB.QueryRow(ctx, recordSQL, brand))
}
func (s Service) Change(ctx context.Context, tx pgx.Tx, brand string, a access.Account, target string, in Input, meta points.Metadata) (Record, error) {
	if tx == nil || !uuidPattern.MatchString(brand) || in.Version < 1 || !utf8.ValidString(in.Reason) || len(in.Reason) > 500 || strings.TrimSpace(in.Reason) == "" || in.Primary && !in.Enabled {
		return Record{}, ErrInvalid
	}
	if target == "" && !ValidDomain(in.Domain) || target != "" && (!uuidPattern.MatchString(target) || in.Domain != "") {
		return Record{}, ErrInvalid
	}
	if !Allowed(a, brand, "write") {
		return Record{}, ErrDenied
	}
	if !uuidPattern.MatchString(a.ID) || meta.ActorType != "admin" || meta.ActorID != a.ID || meta.RequestID == "" {
		return Record{}, ErrInvalid
	}
	before, e := read(tx.QueryRow(ctx, recordSQL+` FOR UPDATE OF b`, brand))
	if e != nil {
		return Record{}, e
	}
	if before.Version != in.Version || before.Version == math.MaxInt64 {
		return Record{}, ErrVersion
	}
	if before.Status == "disabled" {
		return Record{}, ErrState
	}
	if target == "" {
		if len(before.Domains) >= 100 {
			return Record{}, ErrConflict
		}
		// Serialize a global namespace reservation across different brand locks.
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,790031))`, in.Domain); e != nil {
			return Record{}, e
		}
		var reserved bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brand_domains WHERE domain=$1) OR EXISTS(SELECT 1 FROM platform_domains WHERE domain=$1)`, in.Domain).Scan(&reserved); e != nil {
			return Record{}, e
		}
		if reserved {
			return Record{}, ErrConflict
		}
		target = ids.New()
		var inserted string
		e = tx.QueryRow(ctx, `INSERT INTO brand_domains(id,brand_id,domain,enabled,is_primary)VALUES($1,$2,$3,$4,false) ON CONFLICT DO NOTHING RETURNING id::text`, target, brand, in.Domain, in.Enabled).Scan(&inserted)
		if errors.Is(e, pgx.ErrNoRows) {
			return Record{}, ErrConflict
		}
		if e != nil {
			return Record{}, e
		}
	} else {
		var found bool
		for _, d := range before.Domains {
			if d.ID == strings.ToLower(target) {
				found = true
				if d.Enabled == in.Enabled && d.Primary == in.Primary {
					return Record{}, ErrState
				}
			}
		}
		if !found {
			return Record{}, ErrNotFound
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE brands SET config_version=config_version+1,updated_at=clock_timestamp() WHERE id=$1`, brand); e != nil {
		return Record{}, e
	}
	if in.Primary {
		if _, e = tx.Exec(ctx, `UPDATE brand_domains SET is_primary=false WHERE brand_id=$1 AND is_primary`, brand); e != nil {
			return Record{}, e
		}
	}
	_, e = tx.Exec(ctx, `UPDATE brand_domains SET enabled=$3,is_primary=$4 WHERE brand_id=$1 AND id=$2`, brand, target, in.Enabled, in.Primary)
	if e != nil {
		return Record{}, e
	}
	after, e := read(tx.QueryRow(ctx, recordSQL, brand))
	if e != nil {
		return Record{}, e
	}
	auditID, e := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "brand_domains.update", ResourceType: "brand_domains", ResourceID: brand, Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, Before: before, After: after})
	if e != nil {
		return Record{}, e
	}
	prev, e := json.Marshal(before.Domains)
	if e != nil {
		return Record{}, e
	}
	next, e := json.Marshal(after.Domains)
	if e != nil {
		return Record{}, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO brand_domain_revisions(id,brand_id,version,changed_by,reason,audit_log_id,before_domains,domains)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New(), brand, after.Version, a.ID, in.Reason, auditID, prev, next); e != nil {
		return Record{}, e
	}
	after.AuditLogID = auditID
	return after, nil
}
func (s Service) History(ctx context.Context, brand string, limit, offset int) ([]Revision, error) {
	if s.DB == nil || !uuidPattern.MatchString(brand) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return nil, ErrInvalid
	}
	if _, e := s.Read(ctx, brand); e != nil {
		return nil, e
	}
	rows, e := s.DB.Query(ctx, `SELECT id::text,brand_id::text,version,changed_by::text,reason,audit_log_id::text,created_at,before_domains,domains FROM brand_domain_revisions WHERE brand_id=$1 ORDER BY version DESC LIMIT $2 OFFSET $3`, brand, limit, offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Revision{}
	for rows.Next() {
		var r Revision
		var before, after []byte
		if e = rows.Scan(&r.ID, &r.BrandID, &r.Version, &r.ChangedBy, &r.Reason, &r.AuditLogID, &r.CreatedAt, &before, &after); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(before, &r.BeforeDomains); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(after, &r.Domains); e != nil {
			return nil, e
		}
		r.CreatedAt = r.CreatedAt.UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}
