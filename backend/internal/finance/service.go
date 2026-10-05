// Package finance contains the manual financial sidecar. It records recharge
// requests and delegates balance changes to the immutable points ledger.
package finance

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var financeUUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Service persists manual recharge workflow state. Points is the only writer
// of balances and ledger entries.
type Service struct {
	DB     *pgxpool.Pool
	Points points.Store
}

type Recharge struct {
	ID             string     `json:"id"`
	BrandID        string     `json:"brand_id"`
	MemberID       string     `json:"member_id"`
	AccountID      string     `json:"account_id"`
	Points         string     `json:"points"`
	State          string     `json:"state"`
	ProofReference string     `json:"proof_reference"`
	Remark         string     `json:"remark"`
	CreatedBy      string     `json:"created_by"`
	ConfirmedBy    string     `json:"confirmed_by,omitempty"`
	Version        int64      `json:"version"`
	CreatedAt      time.Time  `json:"created_at"`
	ConfirmedAt    *time.Time `json:"confirmed_at,omitempty"`
	LedgerEntryID  string     `json:"ledger_entry_id,omitempty"`
	AuditLogID     string     `json:"audit_log_id,omitempty"`
}

func (s Service) CreateRecharge(ctx context.Context, tx pgx.Tx, brand, member string, amount points.Amount, proof, remark, reason string, meta points.Metadata) (Recharge, error) {
	reason, err := validReason(reason)
	if err != nil || tx == nil || !financeUUIDPattern.MatchString(brand) || !financeUUIDPattern.MatchString(member) || amount <= 0 || !validText(proof, 500) || !validText(remark, 2000) {
		return Recharge{}, points.ErrInvalid
	}
	var accountID string
	err = tx.QueryRow(ctx, `SELECT pa.id::text FROM brand_members bm JOIN point_accounts pa ON pa.brand_id=bm.brand_id AND pa.brand_member_id=bm.id
		WHERE bm.brand_id=$1 AND bm.id=$2`, brand, member).Scan(&accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Recharge{}, points.ErrNotFound
	}
	if err != nil {
		return Recharge{}, err
	}
	record := Recharge{ID: ids.New(), BrandID: brand, MemberID: member, AccountID: accountID, Points: strconv.FormatInt(int64(amount), 10), State: "pending", ProofReference: proof, Remark: remark, CreatedBy: meta.ActorID, Version: 1}
	err = tx.QueryRow(ctx, `INSERT INTO recharge_orders(id,brand_id,member_id,account_id,points,state,proof_reference,remark,created_by,version)
		VALUES($1,$2,$3,$4,$5,'pending',$6,$7,$8,1) RETURNING created_at`, record.ID, brand, member, accountID, int64(amount), proof, remark, meta.ActorID).Scan(&record.CreatedAt)
	if isUnique(err) {
		return Recharge{}, points.ErrConflict
	}
	if err != nil {
		return Recharge{}, err
	}
	record.AuditLogID, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: meta.ActorType, ActorID: meta.ActorID, Action: "finance.recharge.create", ResourceType: "recharge_order", ResourceID: record.ID, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, After: rechargeSnapshot(record)})
	return record, err
}

func (s Service) ConfirmRecharge(ctx context.Context, tx pgx.Tx, brand, id string, version int64, reason string, meta points.Metadata) (Recharge, error) {
	reason, err := validReason(reason)
	if err != nil || tx == nil || !financeUUIDPattern.MatchString(brand) || !financeUUIDPattern.MatchString(id) || version < 1 {
		return Recharge{}, points.ErrInvalid
	}
	record, err := scanRecharge(tx.QueryRow(ctx, `SELECT id::text,brand_id::text,member_id::text,account_id::text,points,state,proof_reference,remark,created_by::text,
		COALESCE(confirmed_by::text,''),version,created_at,confirmed_at,COALESCE(ledger_entry_id::text,'')
		FROM recharge_orders WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Recharge{}, points.ErrNotFound
	}
	if err != nil {
		return Recharge{}, err
	}
	if record.State != "pending" || record.Version != version {
		return Recharge{}, points.ErrConflict
	}
	before := rechargeSnapshot(record)
	amount, err := parseAmount(record.Points)
	if err != nil {
		return Recharge{}, err
	}
	var delta points.Balance
	delta[0][0] = amount
	entry, err := s.Points.Post(ctx, tx, points.Change{
		BrandID: brand, MemberID: record.MemberID, EntryType: "recharge", ReferenceType: "recharge", ReferenceID: record.ID,
		OperationKey: "recharge-confirm:" + record.ID, Reason: reason, ActorType: meta.ActorType, ActorID: meta.ActorID,
		RequestID: meta.RequestID, IP: meta.IP, Delta: delta,
		Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: amount}},
	})
	if err != nil {
		return Recharge{}, err
	}
	command, err := tx.Exec(ctx, `UPDATE recharge_orders SET state='confirmed',confirmed_by=$3,confirmed_at=now(),ledger_entry_id=$4,version=version+1
		WHERE brand_id=$1 AND id=$2 AND state='pending' AND version=$5`, brand, id, meta.ActorID, entry.ID, version)
	if err != nil {
		return Recharge{}, err
	}
	if command.RowsAffected() != 1 {
		return Recharge{}, points.ErrConflict
	}
	var confirmedAt time.Time
	if err := tx.QueryRow(ctx, `SELECT confirmed_at FROM recharge_orders WHERE brand_id=$1 AND id=$2`, brand, id).Scan(&confirmedAt); err != nil {
		return Recharge{}, err
	}
	record.State, record.ConfirmedBy, record.ConfirmedAt, record.LedgerEntryID, record.Version = "confirmed", meta.ActorID, &confirmedAt, entry.ID, version+1
	auditID, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: meta.ActorType, ActorID: meta.ActorID, Action: "finance.recharge.confirm", ResourceType: "recharge_order", ResourceID: record.ID, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, Before: before, After: rechargeSnapshot(record)})
	if err != nil {
		return Recharge{}, err
	}
	record.AuditLogID = auditID
	return record, nil
}

func (s Service) ListRecharges(ctx context.Context, brand, member string, limit, offset int) ([]Recharge, error) {
	if !financeUUIDPattern.MatchString(brand) || !financeUUIDPattern.MatchString(member) || limit < 1 || limit > 100 || offset < 0 {
		return nil, points.ErrInvalid
	}
	var exists bool
	err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brand_members WHERE brand_id=$1 AND id=$2)`, brand, member).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, points.ErrNotFound
	}
	rows, err := s.DB.Query(ctx, `SELECT id::text,brand_id::text,member_id::text,account_id::text,points,state,proof_reference,remark,created_by::text,
		COALESCE(confirmed_by::text,''),version,created_at,confirmed_at,COALESCE(ledger_entry_id::text,'')
		FROM recharge_orders WHERE brand_id=$1 AND member_id=$2 ORDER BY created_at DESC,id DESC LIMIT $3 OFFSET $4`, brand, member, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Recharge, 0)
	for rows.Next() {
		record, err := scanRecharge(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

type rechargeScanner interface{ Scan(...any) error }

func scanRecharge(row rechargeScanner) (Recharge, error) {
	var record Recharge
	var amount int64
	err := row.Scan(&record.ID, &record.BrandID, &record.MemberID, &record.AccountID, &amount, &record.State,
		&record.ProofReference, &record.Remark, &record.CreatedBy, &record.ConfirmedBy, &record.Version,
		&record.CreatedAt, &record.ConfirmedAt, &record.LedgerEntryID)
	record.Points = strconv.FormatInt(amount, 10)
	return record, err
}

func rechargeSnapshot(record Recharge) map[string]any {
	return map[string]any{"id": record.ID, "brand_id": record.BrandID, "member_id": record.MemberID, "account_id": record.AccountID,
		"points": record.Points, "state": record.State, "proof_reference": record.ProofReference, "remark": record.Remark,
		"created_by": record.CreatedBy, "confirmed_by": record.ConfirmedBy, "version": record.Version,
		"created_at": record.CreatedAt, "confirmed_at": record.ConfirmedAt, "ledger_entry_id": record.LedgerEntryID}
}

func validReason(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", points.ErrInvalid
	}
	value = strings.TrimSpace(value)
	if len(value) < 1 || len(value) > 500 {
		return "", points.ErrInvalid
	}
	return value, nil
}

func validText(value string, max int) bool { return utf8.ValidString(value) && len(value) <= max }

func parseAmount(value string) (points.Amount, error) {
	var amount int64
	var err error
	amount, err = strconv.ParseInt(value, 10, 64)
	if err != nil || amount <= 0 {
		return 0, points.ErrCorrupt
	}
	return points.Amount(amount), nil
}

func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
