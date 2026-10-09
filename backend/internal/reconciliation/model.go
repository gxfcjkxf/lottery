// Package reconciliation runs durable, brand-scoped wallet integrity checks.
// It never changes economic entries, wallet buckets or account versions.
package reconciliation

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalid  = errors.New("invalid reconciliation input")
	ErrDenied   = errors.New("reconciliation denied")
	ErrNotFound = errors.New("reconciliation not found")
	ErrState    = errors.New("reconciliation state conflict")
	ErrVersion  = errors.New("reconciliation version conflict")
	ErrTooLarge = errors.New("reconciliation scope exceeds limit")
	uuid        = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

const MaxTargets = 100000
const maxVersion int64 = 9007199254740991

const ScopeWallet = "wallet"
const ScopeWalletAndBusiness = "wallet_and_business"

type BusinessIssue struct {
	Code          string  `json:"code"`
	EntryType     *string `json:"entry_type"`
	LedgerEntryID *string `json:"ledger_entry_id"`
	ResourceType  string  `json:"resource_type"`
	ResourceID    *string `json:"resource_id"`
}
type BusinessCoverage struct {
	Family                 string `json:"family"`
	LedgerEntryCount       string `json:"ledger_entry_count"`
	BusinessReferenceCount string `json:"business_reference_count"`
	IssueCount             string `json:"issue_count"`
}
type BusinessPreview struct {
	AccountID              string             `json:"account_id"`
	MemberID               string             `json:"member_id"`
	AccountVersion         int64              `json:"account_version"`
	LedgerEntryCount       string             `json:"ledger_entry_count"`
	BusinessReferenceCount string             `json:"business_reference_count"`
	IssueCount             string             `json:"issue_count"`
	IssuesTruncated        bool               `json:"issues_truncated"`
	Consistent             bool               `json:"consistent"`
	Fingerprint            string             `json:"fingerprint"`
	Issues                 []BusinessIssue    `json:"issues"`
	Coverage               []BusinessCoverage `json:"coverage"`
}

func ValidScope(scope string) bool { return scope == ScopeWallet || scope == ScopeWalletAndBusiness }

type Service struct{ DB *pgxpool.Pool }
type Job struct {
	CheckScope         string     `json:"check_scope"`
	ID                 string     `json:"id"`
	BrandID            string     `json:"brand_id"`
	State              string     `json:"state"`
	Version            int64      `json:"version"`
	TargetCount        string     `json:"target_count"`
	CheckedCount       string     `json:"checked_count"`
	ConsistentCount    string     `json:"consistent_count"`
	RepairableCount    string     `json:"repairable_count"`
	CorruptCount       string     `json:"corrupt_count"`
	FailedCount        string     `json:"failed_count"`
	PendingCount       string     `json:"pending_count"`
	CreatedBy          string     `json:"created_by"`
	Reason             string     `json:"reason"`
	CreatedAt          time.Time  `json:"created_at"`
	StartedAt          *time.Time `json:"started_at"`
	CompletedAt        *time.Time `json:"completed_at"`
	LastErrorCode      *string    `json:"last_error_code"`
	CanRetry           bool       `json:"can_retry"`
	CreationAuditLogID string     `json:"creation_audit_log_id"`
}
type JobPage struct {
	BrandID    string `json:"brand_id"`
	Items      []Job  `json:"items"`
	TotalCount string `json:"total_count"`
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
}
type Target struct {
	CheckScope      string                `json:"check_scope"`
	BusinessPreview *BusinessPreview      `json:"business_preview"`
	ID              string                `json:"id"`
	BrandID         string                `json:"brand_id"`
	JobID           string                `json:"job_id"`
	AccountID       string                `json:"account_id"`
	MemberID        string                `json:"member_id"`
	State           string                `json:"state"`
	Outcome         *string               `json:"outcome"`
	Preview         *points.RepairPreview `json:"preview"`
	AttemptCount    int                   `json:"attempt_count"`
	ErrorCode       *string               `json:"error_code"`
	CheckedAt       *time.Time            `json:"checked_at"`
	AuditLogID      *string               `json:"audit_log_id"`
}
type TargetPage struct {
	BrandID    string   `json:"brand_id"`
	JobID      string   `json:"job_id"`
	Items      []Target `json:"items"`
	TotalCount string   `json:"total_count"`
	Limit      int      `json:"limit"`
	Offset     int      `json:"offset"`
	Outcome    *string  `json:"outcome"`
}

func Allowed(a access.Account, brand, action string) bool {
	if !uuid.MatchString(brand) {
		return false
	}
	view := access.Authorize(a, "wallet", "view", access.ScopeBrand, brand) || access.Authorize(a, "wallet", "view", access.ScopePlatform, "")
	if action == "view" {
		return view
	}
	return (action == "run" || action == "retry") && view && !a.SuperAdmin && access.Authorize(a, "wallet", "reconcile", access.ScopeBrand, brand)
}
func validReason(s string) bool {
	return utf8.ValidString(s) && strings.TrimSpace(s) == s && s != "" && len(s) <= 500 && !strings.ContainsAny(s, "\x00\r\n")
}
func validPage(brand string, limit, offset int) bool {
	return uuid.MatchString(brand) && limit >= 1 && limit <= 100 && offset >= 0 && offset <= 1000000
}
func validMeta(a access.Account, meta points.Metadata) bool {
	return a.Type == access.AccountAdmin && uuid.MatchString(a.ID) && meta.ActorType == "admin" && meta.ActorID == a.ID && meta.RequestID != "" && len(meta.RequestID) <= 80
}
func ValidFilter(s string) bool {
	switch s {
	case "", "pending", "failed", "consistent", "repairable", "corrupt":
		return true
	}
	return false
}
