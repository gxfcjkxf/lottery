package finance

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

// MemberRecharge is a private-field-free current order projection. Confirmation
// records a historical points credit, never an external payment or current balance.
type MemberRecharge struct {
	ID            string     `json:"id"`
	BrandID       string     `json:"brand_id"`
	MemberID      string     `json:"member_id"`
	Points        string     `json:"points"`
	State         string     `json:"state"`
	Version       string     `json:"version"`
	CreatedAt     time.Time  `json:"created_at"`
	ConfirmedAt   *time.Time `json:"confirmed_at"`
	LedgerEntryID *string    `json:"ledger_entry_id"`
}
type MemberRechargePage struct {
	BrandID    string           `json:"brand_id"`
	MemberID   string           `json:"member_id"`
	SnapshotAt time.Time        `json:"snapshot_at"`
	State      *string          `json:"state"`
	Items      []MemberRecharge `json:"items"`
	Limit      int              `json:"limit"`
	Offset     int              `json:"offset"`
	TotalCount string           `json:"total_count"`
}

func validMemberRechargeState(state string) bool {
	return state == "pending" || state == "confirmed" || state == "cancelled"
}

// Reuse the existing member/time index. Each requested page, count, and snapshot
// is read in one statement; the caller owns fresh session checks and audit commit.
func (s Service) ListMemberRechargesTx(ctx context.Context, tx pgx.Tx, brand, member, state string, limit, offset int) (MemberRechargePage, error) {
	brand, member = strings.ToLower(brand), strings.ToLower(member)
	out := MemberRechargePage{BrandID: brand, MemberID: member, Items: []MemberRecharge{}, Limit: limit, Offset: offset}
	if tx == nil || !financeUUIDPattern.MatchString(brand) || !financeUUIDPattern.MatchString(member) || (state != "" && !validMemberRechargeState(state)) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return out, points.ErrInvalid
	}
	if state != "" {
		out.State = &state
	}
	var raw []byte
	var scope bool
	err := tx.QueryRow(ctx, `WITH matched AS (
 SELECT id,brand_id,member_id,points::text,state,version::text,created_at,confirmed_at,ledger_entry_id::text
 FROM recharge_orders WHERE brand_id=$1 AND member_id=$2 AND ($3::text='' OR state=$3)),
 page AS (SELECT * FROM matched ORDER BY created_at DESC,id DESC LIMIT $4 OFFSET $5)
 SELECT statement_timestamp(),EXISTS(SELECT 1 FROM brand_members WHERE brand_id=$1 AND id=$2),
 coalesce((SELECT jsonb_agg(to_jsonb(page) ORDER BY created_at DESC,id DESC) FROM page),'[]'::jsonb),
 (SELECT count(*)::text FROM matched)`, brand, member, state, limit, offset).Scan(&out.SnapshotAt, &scope, &raw, &out.TotalCount)
	if err != nil {
		return out, err
	}
	if !scope {
		return out, points.ErrNotFound
	}
	if err = json.Unmarshal(raw, &out.Items); err != nil {
		return out, err
	}
	out.SnapshotAt = out.SnapshotAt.UTC()
	for i := range out.Items {
		if err = normalizeMemberRecharge(&out.Items[i], brand, member); err != nil {
			return MemberRechargePage{}, err
		}
	}
	return out, nil
}

func (s Service) ReadMemberRechargeTx(ctx context.Context, tx pgx.Tx, brand, member, id string) (MemberRecharge, error) {
	brand, member, id = strings.ToLower(brand), strings.ToLower(member), strings.ToLower(id)
	var out MemberRecharge
	if tx == nil || !financeUUIDPattern.MatchString(brand) || !financeUUIDPattern.MatchString(member) || !financeUUIDPattern.MatchString(id) {
		return out, points.ErrInvalid
	}
	err := tx.QueryRow(ctx, `SELECT id::text,brand_id::text,member_id::text,points::text,state,version::text,created_at,confirmed_at,ledger_entry_id::text
 FROM recharge_orders WHERE brand_id=$1 AND member_id=$2 AND id=$3`, brand, member, id).Scan(&out.ID, &out.BrandID, &out.MemberID, &out.Points, &out.State, &out.Version, &out.CreatedAt, &out.ConfirmedAt, &out.LedgerEntryID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, points.ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if err = normalizeMemberRecharge(&out, brand, member); err != nil {
		return MemberRecharge{}, err
	}
	return out, nil
}

func normalizeMemberRecharge(out *MemberRecharge, brand, member string) error {
	amount, e := strconv.ParseInt(out.Points, 10, 64)
	version, vErr := strconv.ParseInt(out.Version, 10, 64)
	if out.BrandID != brand || out.MemberID != member || !financeUUIDPattern.MatchString(out.ID) || e != nil || amount < 1 || strconv.FormatInt(amount, 10) != out.Points || vErr != nil || version < 1 || strconv.FormatInt(version, 10) != out.Version || !validMemberRechargeState(out.State) || out.CreatedAt.IsZero() || out.CreatedAt.Year() < 1 || out.CreatedAt.Year() > 9999 {
		return points.ErrCorrupt
	}
	if out.State == "confirmed" {
		if out.ConfirmedAt == nil || out.ConfirmedAt.IsZero() || out.ConfirmedAt.Year() < 1 || out.ConfirmedAt.Year() > 9999 || out.LedgerEntryID == nil || !financeUUIDPattern.MatchString(*out.LedgerEntryID) {
			return points.ErrCorrupt
		}
		utc := out.ConfirmedAt.UTC()
		out.ConfirmedAt = &utc
	} else if out.ConfirmedAt != nil || out.LedgerEntryID != nil {
		return points.ErrCorrupt
	}
	out.CreatedAt = out.CreatedAt.UTC()
	return nil
}
