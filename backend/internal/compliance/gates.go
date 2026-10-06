package compliance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

// GateRecord contains no credentials, claimed age/country, raw idempotency key
// or PII. Actor identifiers refer only to previously persisted identities.
type GateRecord struct {
	ID            string    `json:"id"`
	BrandID       string    `json:"brand_id"`
	PolicyVersion int64     `json:"policy_version"`
	Config        Config    `json:"config"`
	Operation     string    `json:"operation"`
	Action        string    `json:"action"`
	Decision      string    `json:"decision"`
	Checks        []Check   `json:"checks"`
	AdapterMode   string    `json:"adapter_mode"`
	ActorType     string    `json:"actor_type"`
	ActorID       *string   `json:"actor_id"`
	MemberID      *string   `json:"member_id"`
	RequestID     string    `json:"request_id"`
	AuditLogID    string    `json:"audit_log_id"`
	CreatedAt     time.Time `json:"created_at"`
}
type GatePage struct {
	BrandID    string       `json:"brand_id"`
	Operation  *string      `json:"operation"`
	Items      []GateRecord `json:"items"`
	Limit      int          `json:"limit"`
	Offset     int          `json:"offset"`
	TotalCount string       `json:"total_count"`
}
type GateSubject struct{ ActorType, ActorID, MemberID, RequestID, IP string }
type Blocked struct {
	Record GateRecord
	ip     string
}

func (b *Blocked) Error() string { return "compliance admission requires verified adapter" }
func ptr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
func validGateSubject(action string, s GateSubject) bool {
	switch action {
	case "register":
		return s.ActorType == "anonymous" && s.ActorID == "" && s.MemberID == ""
	case "operator_join":
		return s.ActorType == "admin" && uuid.MatchString(s.ActorID) && s.MemberID == ""
	case "join":
		return s.ActorType == "user" && uuid.MatchString(s.ActorID) && (s.MemberID == "" || uuid.MatchString(s.MemberID))
	case "bet_preview", "bet_place":
		return s.ActorType == "user" && uuid.MatchString(s.ActorID) && uuid.MatchString(s.MemberID)
	default:
		return false
	}
}

// AssessTx holds policy FOR SHARE until the caller finishes admission. A policy
// update cannot commit while a transaction using the older allow state admits
// business. Disabled checks are skipped, not successful real verification.
func AssessTx(ctx context.Context, tx pgx.Tx, brand, action string, subject GateSubject) error {
	if tx == nil || !uuid.MatchString(brand) || !validGateSubject(action, subject) {
		return ErrInvalid
	}
	p, e := scanPolicy(tx.QueryRow(ctx, policySQL+" FOR SHARE OF p", brand))
	if e != nil {
		return e
	}
	result, checks, e := Evaluate(p.Config)
	if e != nil {
		return e
	}
	if result == "allow" {
		return nil
	}
	if subject.RequestID == "" || len(subject.RequestID) > 80 {
		return ErrInvalid
	}
	operation := "registration"
	if action == "bet_preview" || action == "bet_place" {
		operation = "betting"
	}
	r := GateRecord{ID: ids.New(), BrandID: p.BrandID, PolicyVersion: p.Version, Config: p.Config, Operation: operation, Action: action, Decision: result, Checks: checks, AdapterMode: "stub", ActorType: subject.ActorType, ActorID: ptr(strings.ToLower(subject.ActorID)), MemberID: ptr(strings.ToLower(subject.MemberID)), RequestID: subject.RequestID}
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&r.CreatedAt); e != nil {
		return e
	}
	r.CreatedAt = r.CreatedAt.UTC()
	return &Blocked{Record: r, ip: subject.IP}
}

// Persist runs only after business rollback (or inside a read-only preview's
// dedicated rejection transaction). It binds to immutable historical policy,
// not to a potentially newer projection, and cannot record a forged allow.
func (b *Blocked) Persist(ctx context.Context, tx pgx.Tx) error {
	if b == nil || tx == nil || b.Record.Decision != "review" || b.Record.AdapterMode != "stub" {
		return ErrInvalid
	}
	r := b.Record
	actor := ""
	if r.ActorID != nil {
		actor = *r.ActorID
	}
	auditType := r.ActorType
	if auditType == "anonymous" {
		auditType = "system"
	}
	id, e := audit.Append(ctx, tx, audit.Record{BrandID: r.BrandID, ActorType: auditType, ActorID: actor, Action: "compliance.gate.reject", ResourceType: "compliance_gate", ResourceID: r.ID, Reason: "COMPLIANCE_REVIEW_REQUIRED", RequestID: r.RequestID, IP: b.ip, After: r})
	if e != nil {
		return e
	}
	raw, _ := json.Marshal(r.Config)
	checks, _ := json.Marshal(r.Checks)
	_, e = tx.Exec(ctx, `INSERT INTO compliance_gate_rejections(id,brand_id,policy_version,config,operation,action,decision,checks,adapter_mode,actor_type,actor_id,member_id,request_id,audit_log_id,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, r.ID, r.BrandID, r.PolicyVersion, raw, r.Operation, r.Action, r.Decision, checks, r.AdapterMode, r.ActorType, r.ActorID, r.MemberID, r.RequestID, id, r.CreatedAt)
	return e
}

// Rejection attaches server-owned evidence; mutation commits it after rolling
// back business writes, together with the encrypted negative receipt.
func Rejection(err error) (mutation.Result, bool) {
	var b *Blocked
	if !errors.As(err, &b) {
		return mutation.Result{}, false
	}
	return mutation.WithRejectedEvidence(mutation.Fail(409, "COMPLIANCE_REVIEW_REQUIRED", "合规检查尚未完成，暂不能注册、加入品牌或投注；请联系品牌运营人员"), b.Persist), true
}
func (s Service) Gates(ctx context.Context, brand, operation string, limit, offset int) (GatePage, error) {
	out := GatePage{BrandID: brand, Operation: ptr(operation), Items: []GateRecord{}, Limit: limit, Offset: offset}
	if s.DB == nil || !validPage(brand, limit, offset) || (operation != "" && operation != "registration" && operation != "betting") {
		return out, ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	if _, e = scanPolicy(tx.QueryRow(ctx, policySQL, brand)); e != nil {
		return out, e
	}
	if e = tx.QueryRow(ctx, `SELECT count(*)::text FROM compliance_gate_rejections WHERE brand_id=$1 AND ($2='' OR operation=$2)`, brand, operation).Scan(&out.TotalCount); e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, `SELECT id::text,brand_id::text,policy_version,config,operation,action,decision,checks,adapter_mode,actor_type,actor_id::text,member_id::text,request_id,audit_log_id::text,created_at FROM compliance_gate_rejections WHERE brand_id=$1 AND ($2='' OR operation=$2) ORDER BY created_at DESC,id DESC LIMIT $3 OFFSET $4`, brand, operation, limit, offset)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var r GateRecord
		var raw, checks []byte
		if e = rows.Scan(&r.ID, &r.BrandID, &r.PolicyVersion, &raw, &r.Operation, &r.Action, &r.Decision, &checks, &r.AdapterMode, &r.ActorType, &r.ActorID, &r.MemberID, &r.RequestID, &r.AuditLogID, &r.CreatedAt); e != nil {
			rows.Close()
			return out, e
		}
		if e = json.Unmarshal(raw, &r.Config); e != nil {
			rows.Close()
			return out, e
		}
		if e = json.Unmarshal(checks, &r.Checks); e != nil {
			rows.Close()
			return out, e
		}
		r.CreatedAt = r.CreatedAt.UTC()
		out.Items = append(out.Items, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	return out, tx.Commit(ctx)
}
