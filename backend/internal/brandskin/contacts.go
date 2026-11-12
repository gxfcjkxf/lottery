package brandskin

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"regexp"
	"strings"
)

type Contact struct {
	ID       string  `json:"id"`
	Kind     string  `json:"kind"`
	Value    string  `json:"value"`
	ImageURL *string `json:"image_url"`
}
type ContactConfig struct {
	Items     []Contact `json:"items"`
	DefaultID *string   `json:"default_id"`
}
type ContactRecord struct {
	BrandID    string        `json:"brand_id"`
	Version    int64         `json:"version"`
	Status     string        `json:"status"`
	Config     ContactConfig `json:"config"`
	AuditLogID string        `json:"audit_log_id,omitempty"`
}
type ContactInput struct {
	Version int64         `json:"version"`
	Config  ContactConfig `json:"config"`
}

var contactImage = regexp.MustCompile(`^/api/v1/brand-assets/([a-f0-9]{64})\.(png|jpg)$`)

func (c ContactConfig) Validate() error {
	if c.Items == nil {
		return ErrInvalid
	}
	ids := map[string]bool{}
	for _, item := range c.Items {
		if !uuidPattern.MatchString(item.ID) || ids[item.ID] || (item.Kind != "wechat" && item.Kind != "telegram" && item.Kind != "whatsapp") || !validText(item.Value, 120, true) || strings.TrimSpace(item.Value) != item.Value {
			return ErrInvalid
		}
		ids[item.ID] = true
		if item.ImageURL != nil && !contactImage.MatchString(*item.ImageURL) {
			return ErrInvalid
		}
		if item.Value == "" && item.ImageURL == nil {
			return ErrInvalid
		}
	}
	if len(c.Items) == 0 {
		if c.DefaultID != nil {
			return ErrInvalid
		}
	} else if c.DefaultID == nil || !ids[*c.DefaultID] {
		return ErrInvalid
	}
	return nil
}

const contactRecordSQL = `SELECT id::text,contact_version,status,contact_config FROM brands WHERE id=$1`

func readContacts(row pgx.Row) (ContactRecord, error) {
	var out ContactRecord
	var raw []byte
	err := row.Scan(&out.BrandID, &out.Version, &out.Status, &raw)
	if err == pgx.ErrNoRows {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out.Config); err != nil {
		return out, err
	}
	return out, out.Config.Validate()
}
func (s Service) ReadContacts(ctx context.Context, brand string) (ContactRecord, error) {
	return readContacts(s.DB.QueryRow(ctx, contactRecordSQL, brand))
}
func (s Service) UpdateContacts(ctx context.Context, tx pgx.Tx, brand string, a access.Account, in ContactInput, meta points.Metadata) (ContactRecord, error) {
	if !Allowed(a, brand, "write") {
		return ContactRecord{}, ErrDenied
	}
	if in.Version < 1 || in.Config.Validate() != nil {
		return ContactRecord{}, ErrInvalid
	}
	before, err := readContacts(tx.QueryRow(ctx, contactRecordSQL+" FOR UPDATE", brand))
	if err != nil {
		return ContactRecord{}, err
	}
	if before.Status == "disabled" {
		return ContactRecord{}, ErrState
	}
	if before.Version != in.Version {
		return ContactRecord{}, ErrVersion
	}
	for _, item := range in.Config.Items {
		if item.ImageURL == nil {
			continue
		}
		parts := contactImage.FindStringSubmatch(*item.ImageURL)
		var owned bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brand_logo_assets WHERE id=$1 AND brand_id=$2 AND media_type=$3)`, parts[1], brand, map[string]string{"png": "image/png", "jpg": "image/jpeg"}[parts[2]]).Scan(&owned)
		if err != nil {
			return ContactRecord{}, err
		}
		if !owned {
			return ContactRecord{}, ErrInvalid
		}
	}
	raw, err := json.Marshal(in.Config)
	if err != nil {
		return ContactRecord{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE brands SET contact_version=contact_version+1,contact_config=$2,updated_at=clock_timestamp() WHERE id=$1`, brand, raw)
	if err != nil {
		return ContactRecord{}, err
	}
	after, err := readContacts(tx.QueryRow(ctx, contactRecordSQL, brand))
	if err != nil {
		return after, err
	}
	after.AuditLogID, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "brand_contacts.update", ResourceType: "brand_contacts", ResourceID: brand, RequestID: meta.RequestID, IP: meta.IP, Before: before.Config.auditView(), After: after.Config.auditView()})
	return after, err
}

func (c ContactConfig) auditView() map[string]any {
	items := make([]map[string]any, 0, len(c.Items))
	for _, item := range c.Items {
		items = append(items, map[string]any{"contact_kind": item.Kind, "contact_value": item.Value, "contact_image": item.ImageURL != nil, "contact_default": c.DefaultID != nil && *c.DefaultID == item.ID})
	}
	return map[string]any{"contacts": items}
}
