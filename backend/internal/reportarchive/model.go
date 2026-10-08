package reportarchive

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalid   = errors.New("invalid report archive request")
	ErrDenied    = errors.New("report archive permission denied")
	ErrNotFound  = errors.New("report archive not found")
	ErrVersion   = errors.New("report archive revision changed")
	ErrState     = errors.New("report archive period or brand is not ready")
	ErrBusy      = errors.New("report archive creation is busy")
	ErrIntegrity = errors.New("report archive integrity cannot be proven")
	archiveUUID  = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

const maxRevision int64 = 9007199254740991

type Service struct{ DB *pgxpool.Pool }
type Input struct {
	Kind             string `json:"kind"`
	PeriodKey        string `json:"period_key"`
	ExpectedRevision int64  `json:"expected_revision"`
	Reason           string `json:"reason"`
}
type Record struct {
	ID            string                    `json:"id"`
	BrandID       string                    `json:"brand_id"`
	Window        Window                    `json:"window"`
	Revision      int64                     `json:"revision"`
	PreviousID    *string                   `json:"previous_id"`
	SnapshotAt    time.Time                 `json:"snapshot_at"`
	CreatedBy     string                    `json:"created_by"`
	Reason        string                    `json:"reason"`
	PayloadSHA256 string                    `json:"payload_sha256"`
	AuditLogID    string                    `json:"audit_log_id"`
	CreatedAt     time.Time                 `json:"created_at"`
	Snapshot      reporting.ArchiveSnapshot `json:"snapshot"`
}
type Page struct {
	BrandID    string   `json:"brand_id"`
	Items      []Record `json:"items"`
	TotalCount string   `json:"total_count"`
	Limit      int      `json:"limit"`
	Offset     int      `json:"offset"`
}

func Allowed(a access.Account, brand, action string) bool {
	if !archiveUUID.MatchString(brand) {
		return false
	}
	brandView := access.Authorize(a, "report_archive", "view", access.ScopeBrand, brand)
	platformView := access.Authorize(a, "report_archive", "view", access.ScopePlatform, "")
	view := brandView || platformView
	if action == "view" {
		return view
	}
	if action == "download" {
		return view && (access.Authorize(a, "report_archive", "download", access.ScopeBrand, brand) ||
			access.Authorize(a, "report_archive", "download", access.ScopePlatform, ""))
	}
	return action == "create" && view && !a.SuperAdmin && brandView && access.Authorize(a, "report_archive", "create", access.ScopeBrand, brand)
}
func validReason(s string) bool {
	return utf8.ValidString(s) && s != "" && len(s) <= 500 && strings.TrimSpace(s) == s && !strings.ContainsAny(s, "\x00\r\n")
}
