package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net"
	"strings"
)

var ErrNotFound = errors.New("brand not found")

type Brand struct {
	ID            string          `json:"id"`
	Code          string          `json:"code"`
	Name          string          `json:"name"`
	Status        string          `json:"status"`
	DefaultLocale string          `json:"default_locale"`
	Timezone      string          `json:"timezone"`
	Theme         json.RawMessage `json:"theme"`
	ConfigVersion int64           `json:"config_version"`
}
type Resolver interface {
	Resolve(context.Context, string, string) (Brand, error)
}
type Store struct{ DB *pgxpool.Pool }

// Host is the request authority. Forwarded host headers are never trusted.
func NormalizeHost(raw string) string {
	if h, _, err := net.SplitHostPort(raw); err == nil {
		raw = h
	}
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
}
func (s Store) Resolve(ctx context.Context, host, code string) (Brand, error) {
	var b Brand
	query := `SELECT b.id::text,b.code,b.name,b.status,b.default_locale,b.timezone,b.theme,b.config_version
 FROM brands b JOIN brand_domains d ON d.brand_id=b.id WHERE d.domain=$1 AND d.enabled AND b.status<>'disabled'`
	args := []any{NormalizeHost(host)}
	if code != "" {
		query = `SELECT b.id::text,b.code,b.name,b.status,b.default_locale,b.timezone,b.theme,b.config_version
   FROM brands b WHERE b.code=$2 AND b.status<>'disabled'
   AND EXISTS(SELECT 1 FROM platform_domains WHERE domain=$1 AND enabled)`
		args = append(args, code)
	}
	err := s.DB.QueryRow(ctx, query, args...).Scan(&b.ID, &b.Code, &b.Name, &b.Status, &b.DefaultLocale, &b.Timezone, &b.Theme, &b.ConfigVersion)
	if err == pgx.ErrNoRows {
		return b, ErrNotFound
	}
	return b, err
}
