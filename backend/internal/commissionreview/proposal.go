package commissionreview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

var (
	ErrDenied        = errors.New("commission history proposal access denied")
	ErrSourceChanged = errors.New("commission history proposal source changed")
	ErrProposalState = errors.New("commission history proposal state conflict")
	digestPattern    = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

const proposalResource = "commission_history_recovery"
const proposeAction = "commission.history_recovery.propose"
const reviewAction = "commission.history_recovery.review"

type ProposalInput struct {
	CycleID      string `json:"cycle_id"`
	SourceDigest string `json:"source_digest"`
	Reason       string `json:"reason"`
}
type ReviewInput struct {
	Version      int64  `json:"version"`
	SourceDigest string `json:"source_digest"`
	Reason       string `json:"reason"`
}

// A reviewed proposal confirms the evidence only. It cannot authorize an
// account mutation, payment state repair, service admission, or compensation.
type Proposal struct {
	ID                 string     `json:"id"`
	BrandID            string     `json:"brand_id"`
	CycleID            string     `json:"cycle_id"`
	Version            int64      `json:"version"`
	State              string     `json:"state"`
	SourceDigest       string     `json:"source_digest"`
	ExecutionAvailable bool       `json:"execution_available"`
	ProposedBy         string     `json:"proposed_by"`
	ReviewedBy         *string    `json:"reviewed_by"`
	ProposedAt         time.Time  `json:"proposed_at"`
	ReviewedAt         *time.Time `json:"reviewed_at"`
	Reason             string     `json:"reason"`
	ReviewReason       *string    `json:"review_reason"`
	Snapshot           Report     `json:"snapshot"`
	AuditLogID         string     `json:"audit_log_id"`
}

func validReason(s string) bool { return strings.TrimSpace(s) != "" && len(s) <= 500 }

// AuthorizeProposalTx reloads actual grants under the same shared ACL gate
// used by normal management revocations. Caller-supplied roles never suffice.
func AuthorizeProposalTx(ctx context.Context, tx pgx.Tx, actor access.Account, brand, action string) (access.Account, error) {
	if tx == nil || !canonicalUUID.MatchString(brand) || !canonicalUUID.MatchString(actor.ID) || actor.Type != access.AccountAdmin {
		return access.Account{}, ErrInvalid
	}
	actual, err := (adminsys.Store{}).LockAdminAccess(ctx, tx, actor.ID, false)
	if err != nil {
		return access.Account{}, err
	}
	if actor.Version != actual.Version {
		return access.Account{}, ErrDenied
	}
	view := (access.Authorize(actual, "commission", "view", access.ScopeBrand, brand) || access.Authorize(actual, "commission", "view", access.ScopePlatform, "")) &&
		(access.Authorize(actual, "report_commission", "view", access.ScopeBrand, brand) || access.Authorize(actual, "report_commission", "view", access.ScopePlatform, ""))
	if !view {
		return access.Account{}, ErrDenied
	}
	if action == "view" {
		return actual, nil
	}
	permission := ""
	if action == "propose" {
		permission = "retry"
	} else if action == "review" {
		permission = "approve"
	} else {
		return access.Account{}, ErrInvalid
	}
	if actual.SuperAdmin || !access.Authorize(actual, "commission_correction", permission, access.ScopeBrand, brand) {
		return access.Account{}, ErrDenied
	}
	return actual, nil
}

func lockProposalCycle(ctx context.Context, tx pgx.Tx, brand, cycle string) error {
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE NOWAIT`, brand).Scan(&status); err != nil {
		return err
	}
	if status == "disabled" {
		return ErrProposalState
	}
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM commission_cycles WHERE brand_id=$1 AND id=$2 FOR UPDATE NOWAIT`, brand, cycle).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func proposalMetadata(actor access.Account, meta points.Metadata) bool {
	return meta.ActorType == "admin" && meta.ActorID == actor.ID && meta.RequestID != "" && len(meta.RequestID) <= 80
}

func ProposeTx(ctx context.Context, tx pgx.Tx, actor access.Account, brand string, in ProposalInput, meta points.Metadata) (Proposal, error) {
	if !canonicalUUID.MatchString(in.CycleID) || !digestPattern.MatchString(in.SourceDigest) || !validReason(in.Reason) || !proposalMetadata(actor, meta) {
		return Proposal{}, ErrInvalid
	}
	actual, err := AuthorizeProposalTx(ctx, tx, actor, brand, "propose")
	if err != nil {
		return Proposal{}, err
	}
	if err = lockProposalCycle(ctx, tx, brand, in.CycleID); err != nil {
		return Proposal{}, err
	}
	snapshot, err := SnapshotTx(ctx, tx, brand, in.CycleID)
	if err != nil {
		return Proposal{}, err
	}
	if snapshot.SourceDigest != in.SourceDigest {
		return Proposal{}, ErrSourceChanged
	}
	if snapshot.Report.StaleMoneyPaymentCount == "0" || !snapshot.Report.Analysis.Summary.CalculationComplete || !snapshot.Report.Analysis.Summary.EffectiveTargetComplete {
		return Proposal{}, ErrProposalState
	}
	p := Proposal{ID: ids.New(), BrandID: brand, CycleID: in.CycleID, Version: 1, State: "awaiting_review", SourceDigest: in.SourceDigest, ProposedBy: actual.ID, ProposedAt: snapshot.Report.SnapshotAt, Reason: in.Reason, Snapshot: snapshot.Report}
	id, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: actual.ID, Action: proposeAction, ResourceType: proposalResource, ResourceID: p.ID, Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, After: p})
	if err != nil {
		return Proposal{}, err
	}
	p.AuditLogID = id
	return p, nil
}

// loadProposal validates the immutable journal chain, not only its last JSON.
// There may be one proposal and at most one review; malformed or extra events
// never become an actionable object. Future execution must revalidate sources.
func loadProposal(ctx context.Context, tx pgx.Tx, brand, id string) (Proposal, error) {
	rows, err := tx.Query(ctx, `SELECT id::text,action,actor_type,actor_id::text,reason,before_json,after_json FROM audit_logs WHERE brand_id=$1 AND resource_type=$2 AND resource_id=$3 AND action IN($4,$5) ORDER BY created_at,id LIMIT 3`, brand, proposalResource, id, proposeAction, reviewAction)
	if err != nil {
		return Proposal{}, err
	}
	defer rows.Close()
	var prior Proposal
	count := 0
	for rows.Next() {
		var auditID, action, actorType, actorID, reason string
		var before, after []byte
		if err = rows.Scan(&auditID, &action, &actorType, &actorID, &reason, &before, &after); err != nil {
			return Proposal{}, err
		}
		var p Proposal
		d := json.NewDecoder(bytes.NewReader(after))
		d.DisallowUnknownFields()
		if err = d.Decode(&p); err != nil {
			return Proposal{}, ErrProposalState
		}
		var trailing any
		if err = d.Decode(&trailing); !errors.Is(err, io.EOF) {
			return Proposal{}, ErrProposalState
		}
		if actorType != "admin" || !canonicalUUID.MatchString(actorID) || p.ID != id || p.BrandID != brand || !canonicalUUID.MatchString(p.CycleID) || !digestPattern.MatchString(p.SourceDigest) || p.ExecutionAvailable || p.AuditLogID != "" || !validReason(p.Reason) || p.ProposedAt.IsZero() || p.Snapshot.BrandID != brand || p.Snapshot.CycleID != p.CycleID || !p.Snapshot.ReviewOnly || p.Snapshot.ExecutionAuthorized {
			return Proposal{}, ErrProposalState
		}
		if count == 0 {
			if action != proposeAction || p.Version != 1 || p.State != "awaiting_review" || p.ProposedBy != actorID || p.ReviewedBy != nil || p.ReviewedAt != nil || p.ReviewReason != nil || reason != p.Reason || !bytes.Equal(bytes.TrimSpace(before), []byte("null")) {
				return Proposal{}, ErrProposalState
			}
		} else if count == 1 {
			if action != reviewAction || p.Version != 2 || p.State != "reviewed" || p.ReviewedBy == nil || *p.ReviewedBy != actorID || p.ReviewReason == nil || *p.ReviewReason != reason || p.ReviewedAt == nil || p.ReviewedAt.Before(p.ProposedAt) {
				return Proposal{}, ErrProposalState
			}
			expected, _ := json.Marshal(map[string]any{"id": prior.ID, "version": prior.Version, "state": prior.State, "source_digest": prior.SourceDigest, "audit_log_id": prior.AuditLogID})
			b1, e1 := canonicalJSON(before)
			b2, e2 := canonicalJSON(expected)
			if e1 != nil || e2 != nil || !bytes.Equal(b1, b2) {
				return Proposal{}, ErrProposalState
			}
			copy := p
			copy.Version = 1
			copy.State = "awaiting_review"
			copy.ReviewedBy = nil
			copy.ReviewedAt = nil
			copy.ReviewReason = nil
			priorCopy := prior
			priorCopy.AuditLogID = ""
			a, _ := json.Marshal(copy)
			b, _ := json.Marshal(priorCopy)
			if !bytes.Equal(a, b) {
				return Proposal{}, ErrProposalState
			}
		} else {
			return Proposal{}, ErrProposalState
		}
		p.AuditLogID = auditID
		prior = p
		count++
	}
	if err = rows.Err(); err != nil {
		return Proposal{}, err
	}
	if count == 0 {
		return Proposal{}, ErrNotFound
	}
	return prior, nil
}

func ReadProposalTx(ctx context.Context, tx pgx.Tx, actor access.Account, brand, id string) (Proposal, error) {
	if !canonicalUUID.MatchString(id) {
		return Proposal{}, ErrInvalid
	}
	if _, err := AuthorizeProposalTx(ctx, tx, actor, brand, "view"); err != nil {
		return Proposal{}, err
	}
	return loadProposal(ctx, tx, brand, id)
}

func ReviewTx(ctx context.Context, tx pgx.Tx, actor access.Account, brand, id string, in ReviewInput, meta points.Metadata) (Proposal, error) {
	if !canonicalUUID.MatchString(id) || in.Version < 1 || !digestPattern.MatchString(in.SourceDigest) || !validReason(in.Reason) || !proposalMetadata(actor, meta) {
		return Proposal{}, ErrInvalid
	}
	actual, err := AuthorizeProposalTx(ctx, tx, actor, brand, "review")
	if err != nil {
		return Proposal{}, err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "commission-history-proposal:"+brand+":"+id); err != nil {
		return Proposal{}, err
	}
	p, err := loadProposal(ctx, tx, brand, id)
	if err != nil {
		return Proposal{}, err
	}
	if p.Version != in.Version || p.State != "awaiting_review" {
		return Proposal{}, ErrProposalState
	}
	if p.SourceDigest != in.SourceDigest {
		return Proposal{}, ErrSourceChanged
	}
	if err = lockProposalCycle(ctx, tx, brand, p.CycleID); err != nil {
		return Proposal{}, err
	}
	snapshot, err := SnapshotTx(ctx, tx, brand, p.CycleID)
	if err != nil {
		return Proposal{}, err
	}
	if snapshot.SourceDigest != p.SourceDigest {
		return Proposal{}, ErrSourceChanged
	}
	oldReport, _ := json.Marshal(p.Snapshot)
	freshReport, _ := json.Marshal(snapshot.Report)
	oldProjection, e1 := canonicalObjectWithout(oldReport, "snapshot_at")
	freshProjection, e2 := canonicalObjectWithout(freshReport, "snapshot_at")
	if e1 != nil || e2 != nil || !bytes.Equal(oldProjection, freshProjection) {
		return Proposal{}, ErrSourceChanged
	}
	before := map[string]any{"id": p.ID, "version": p.Version, "state": p.State, "source_digest": p.SourceDigest, "audit_log_id": p.AuditLogID}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return Proposal{}, err
	}
	p.Version = 2
	p.State = "reviewed"
	p.ReviewedBy = &actual.ID
	p.ReviewReason = &in.Reason
	p.ReviewedAt = &now
	p.AuditLogID = ""
	auditID, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: actual.ID, Action: reviewAction, ResourceType: proposalResource, ResourceID: p.ID, Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, Before: before, After: p})
	if err != nil {
		return Proposal{}, err
	}
	p.AuditLogID = auditID
	return p, nil
}
