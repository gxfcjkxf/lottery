// Package platformsettings stores the platform-wide display and time settings.
package platformsettings

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDenied  = errors.New("platform settings access denied")
	ErrInvalid = errors.New("invalid platform settings")
	ErrVersion = errors.New("platform settings version conflict")
)

type Service struct{ DB *pgxpool.Pool }

type Settings struct {
	Version     int64     `json:"version"`
	DisplayName string    `json:"display_name"`
	LogoText    string    `json:"logo_text"`
	LogoURL     string    `json:"logo_url"`
	Timezone    string    `json:"timezone"`
	DateFormat  string    `json:"date_format"`
	TimeFormat  string    `json:"time_format"`
	UpdatedAt   time.Time `json:"updated_at"`
	AuditLogID  string    `json:"audit_log_id,omitempty"`
}

type Input struct {
	Version     int64  `json:"version"`
	DisplayName string `json:"display_name"`
	LogoText    string `json:"logo_text"`
	LogoURL     string `json:"logo_url"`
	Timezone    string `json:"timezone"`
	DateFormat  string `json:"date_format"`
	TimeFormat  string `json:"time_format"`
}

func Allowed(a access.Account, action string) bool {
	if action != "view" && action != "write" {
		return false
	}
	return access.Authorize(a, "platform_settings", action, access.ScopePlatform, "")
}

func validate(in Input) error {
	if in.Version < 1 || !validText(in.DisplayName, 120) || strings.TrimSpace(in.DisplayName) == "" || !validText(in.LogoText, 8) || strings.TrimSpace(in.LogoText) == "" || !validText(in.LogoURL, 512) || !validText(in.Timezone, 100) {
		return ErrInvalid
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return ErrInvalid
	}
	if in.DateFormat != "YYYY-MM-DD" && in.DateFormat != "DD/MM/YYYY" && in.DateFormat != "MM/DD/YYYY" {
		return ErrInvalid
	}
	if in.TimeFormat != "HH:mm:ss" && in.TimeFormat != "hh:mm:ss A" {
		return ErrInvalid
	}
	return nil
}

func validText(value string, max int) bool {
	return utf8.ValidString(value) && len(value) <= max && !strings.ContainsAny(value, "\r\n")
}

const selectSQL = `SELECT version,display_name,logo_text,logo_url,timezone,date_format,time_format,updated_at,COALESCE((SELECT id::text FROM audit_logs WHERE action='platform_settings.update' ORDER BY created_at DESC,id DESC LIMIT 1),'') FROM platform_settings WHERE id=true`

func scan(row pgx.Row) (Settings, error) {
	var out Settings
	err := row.Scan(&out.Version, &out.DisplayName, &out.LogoText, &out.LogoURL, &out.Timezone, &out.DateFormat, &out.TimeFormat, &out.UpdatedAt, &out.AuditLogID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, ErrInvalid
	}
	return out, err
}

func (s Service) Read(ctx context.Context) (Settings, error) {
	if s.DB == nil {
		return Settings{}, ErrInvalid
	}
	return scan(s.DB.QueryRow(ctx, selectSQL))
}

func (s Service) Update(ctx context.Context, tx pgx.Tx, a access.Account, in Input, meta points.Metadata) (Settings, error) {
	if tx == nil || meta.ActorType != "admin" || meta.ActorID != a.ID || meta.RequestID == "" || validate(in) != nil {
		return Settings{}, ErrInvalid
	}
	if !Allowed(a, "write") {
		return Settings{}, ErrDenied
	}
	var before Settings
	var err error
	before, err = scan(tx.QueryRow(ctx, selectSQL+" FOR UPDATE"))
	if err != nil {
		return Settings{}, err
	}
	if before.Version != in.Version {
		return Settings{}, ErrVersion
	}
	var after Settings
	err = tx.QueryRow(ctx, `UPDATE platform_settings SET version=version+1,display_name=$1,logo_text=$2,logo_url=$3,timezone=$4,date_format=$5,time_format=$6,updated_by=$7,updated_at=clock_timestamp() WHERE id=true RETURNING version,display_name,logo_text,logo_url,timezone,date_format,time_format,updated_at`, in.DisplayName, in.LogoText, in.LogoURL, in.Timezone, in.DateFormat, in.TimeFormat, a.ID).Scan(&after.Version, &after.DisplayName, &after.LogoText, &after.LogoURL, &after.Timezone, &after.DateFormat, &after.TimeFormat, &after.UpdatedAt)
	if err != nil {
		return Settings{}, err
	}
	auditID, err := audit.Append(ctx, tx, audit.Record{ActorType: "admin", ActorID: a.ID, Action: "platform_settings.update", ResourceType: "platform_settings", RequestID: meta.RequestID, IP: meta.IP, Before: before, After: after})
	if err != nil {
		return Settings{}, err
	}
	after.AuditLogID = auditID
	return after, nil
}
