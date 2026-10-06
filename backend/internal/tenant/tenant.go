package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/brandskin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net"
	"strings"
)

var ErrNotFound = errors.New("brand not found")

type Brand struct {
	ID               string          `json:"id"`
	Code             string          `json:"code"`
	Name             string          `json:"name"`
	Status           string          `json:"status"`
	DefaultLocale    string          `json:"default_locale"`
	Timezone         string          `json:"timezone"`
	Theme            json.RawMessage `json:"theme"`
	ConfigVersion    int64           `json:"config_version"`
	AvailableLocales []string        `json:"-"`
}
type Resolver interface {
	Resolve(context.Context, string, string) (Brand, error)
}
type Store struct{ DB *pgxpool.Pool }

// PlatformEntry admits only explicitly configured platform hosts; it grants no
// account or brand permission and does not expose public user brand context.
func (s Store) PlatformEntry(ctx context.Context, host string) (bool, error) {
	var allowed bool
	err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform_domains WHERE domain=$1 AND enabled)`, NormalizeHost(host)).Scan(&allowed)
	return allowed, err
}

// Host is the request authority. Forwarded host headers are never trusted.
func NormalizeHost(raw string) string {
	if h, _, err := net.SplitHostPort(raw); err == nil {
		raw = h
	}
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
}
func (s Store) Resolve(ctx context.Context, host, code string) (Brand, error) {
	var b Brand
	query := `SELECT b.id::text,b.code,b.name,b.status,b.default_locale,b.timezone,b.theme,b.config_version,p.config
 FROM brands b JOIN brand_domains d ON d.brand_id=b.id JOIN brand_presentations p ON p.brand_id=b.id WHERE d.domain=$1 AND d.enabled AND b.status<>'disabled'`
	args := []any{NormalizeHost(host)}
	if code != "" {
		query = `SELECT b.id::text,b.code,b.name,b.status,b.default_locale,b.timezone,b.theme,b.config_version,p.config
   FROM brands b JOIN brand_presentations p ON p.brand_id=b.id WHERE b.code=$2 AND b.status<>'disabled'
   AND EXISTS(SELECT 1 FROM platform_domains WHERE domain=$1 AND enabled)`
		args = append(args, code)
	}
	var raw []byte
	err := s.DB.QueryRow(ctx, query, args...).Scan(&b.ID, &b.Code, &b.Name, &b.Status, &b.DefaultLocale, &b.Timezone, &b.Theme, &b.ConfigVersion, &raw)
	if err == pgx.ErrNoRows {
		return b, ErrNotFound
	}
	if err != nil {
		return b, err
	}
	var config brandskin.Config
	if err = json.Unmarshal(raw, &config); err != nil {
		return b, err
	}
	effective, err := brandskin.Resolve(b.Name, config)
	if err != nil {
		return b, err
	}
	b.Name = effective.DisplayName
	b.DefaultLocale = effective.DefaultLocale
	b.AvailableLocales = effective.AvailableLocales
	b.Theme, err = json.Marshal(effective)
	return b, err
}
