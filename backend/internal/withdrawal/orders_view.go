package withdrawal

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

// OrderView intentionally excludes raw eligibility evidence, policy snapshots,
// actor identities and original idempotency keys. Ledger cursors remain exact.
type OrderView struct {
	ID               string              `json:"id"`
	BrandID          string              `json:"brand_id"`
	MemberID         string              `json:"member_id"`
	AccountID        string              `json:"account_id"`
	Points           points.Amount       `json:"points"`
	State            string              `json:"state"`
	Version          int64               `json:"version"`
	SourceAllocation []points.Allocation `json:"source_allocation"`
	ReserveEntryID   string              `json:"reserve_entry_id"`
	ReleaseEntryID   *string             `json:"release_entry_id"`
	PaidEntryID      *string             `json:"paid_entry_id"`
	CycleFromAt      *time.Time          `json:"cycle_from_at"`
	CycleFromVersion string              `json:"cycle_from_version"`
	ReserveVersion   string              `json:"reserve_version"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
	ReviewedAt       *time.Time          `json:"reviewed_at"`
	CompletedAt      *time.Time          `json:"completed_at"`
	DecisionReason   string              `json:"decision_reason"`
	AuditLogID       string              `json:"audit_log_id"`
}

func optionalID(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
func ToOrderView(o Order) OrderView {
	return OrderView{ID: o.ID, BrandID: o.BrandID, MemberID: o.MemberID, AccountID: o.AccountID, Points: o.Points, State: o.State, Version: o.Version, SourceAllocation: o.SourceAllocation, ReserveEntryID: o.ReserveEntryID, ReleaseEntryID: optionalID(o.ReleaseEntryID), PaidEntryID: optionalID(o.PaidEntryID), CycleFromAt: o.CycleFromAt, CycleFromVersion: strconv.FormatInt(o.CycleFromVersion, 10), ReserveVersion: strconv.FormatInt(o.ReserveVersion, 10), CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt, ReviewedAt: o.ReviewedAt, CompletedAt: o.CompletedAt, DecisionReason: o.DecisionReason, AuditLogID: o.AuditLogID}
}
func ToUserOrderView(v OrderView) OrderView {
	if v.State != "rejected" && v.State != "failed" && v.State != "cancelled" {
		v.DecisionReason = ""
	}
	return v
}

type OrderPage struct {
	BrandID string      `json:"brand_id"`
	Items   []OrderView `json:"items"`
	Limit   int         `json:"limit"`
	Offset  int         `json:"offset"`
	HasMore bool        `json:"has_more"`
}
type TransitionView struct {
	ID         string    `json:"id"`
	Version    int64     `json:"version"`
	FromState  string    `json:"from_state"`
	ToState    string    `json:"to_state"`
	Reason     string    `json:"reason"`
	ActorType  string    `json:"actor_type"`
	CreatedAt  time.Time `json:"created_at"`
	AuditLogID string    `json:"audit_log_id"`
}
type HistoryView struct {
	BrandID string           `json:"brand_id"`
	OrderID string           `json:"order_id"`
	Items   []TransitionView `json:"items"`
}
type AvailabilityView struct {
	BrandID               string         `json:"brand_id"`
	MemberID              string         `json:"member_id"`
	PolicyEnabled         bool           `json:"policy_enabled"`
	EligibilityConfigured bool           `json:"eligibility_configured"`
	CanApply              bool           `json:"can_apply"`
	ReasonCode            string         `json:"reason_code"`
	MinPoints             points.Amount  `json:"min_points"`
	MaxPoints             *points.Amount `json:"max_points"`
	AllowedSources        []string       `json:"allowed_sources"`
	RealPayments          bool           `json:"real_payments"`
	ActorContext          string         `json:"actor_context"`
}
type CreateRequest struct {
	Points           points.Amount       `json:"points"`
	SourceAllocation []points.Allocation `json:"source_allocation"`
}

func (c *CreateRequest) UnmarshalJSON(raw []byte) error {
	f, err := fields(raw, "points", "source_allocation")
	if err != nil {
		return err
	}
	var out CreateRequest
	if decodeField(f["points"], &out.Points, false) != nil {
		return ErrInvalid
	}
	var items []json.RawMessage
	if decodeField(f["source_allocation"], &items, false) != nil || len(items) < 1 || len(items) > 4 {
		return ErrInvalid
	}
	for _, item := range items {
		entry, e := fields(item, "source", "state", "points")
		if e != nil {
			return e
		}
		var a points.Allocation
		if decodeField(entry["source"], &a.Source, false) != nil || decodeField(entry["state"], &a.State, false) != nil || decodeField(entry["points"], &a.Points, false) != nil {
			return ErrInvalid
		}
		out.SourceAllocation = append(out.SourceAllocation, a)
	}
	if ValidateOrderInput(OrderInput{Points: out.Points, SourceAllocation: out.SourceAllocation, ClientKey: "validate-body"}) != nil {
		return ErrInvalid
	}
	*c = out
	return nil
}

type ActionRequest struct {
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}

func (c *ActionRequest) UnmarshalJSON(raw []byte) error {
	f, err := fields(raw, "version", "reason")
	if err != nil {
		return err
	}
	var out ActionRequest
	if decodeField(f["version"], &out.Version, false) != nil || decodeField(f["reason"], &out.Reason, false) != nil || ValidateActionInput(ActionInput{Version: out.Version, Reason: out.Reason, ClientKey: "validate-body"}) != nil {
		return ErrInvalid
	}
	*c = out
	return nil
}

type orderAuditRow struct {
	source row
	audit  *string
}

func (r orderAuditRow) Scan(dest ...any) error { return r.source.Scan(append(dest, r.audit)...) }

const viewColumns = orderColumns + `,(SELECT audit_log_id::text FROM withdrawal_order_transitions WHERE order_id=withdrawal_orders.id AND version=withdrawal_orders.version)`

func (s OrderService) ReadViewTx(ctx context.Context, tx pgx.Tx, brand, id, member string) (OrderView, error) {
	if tx == nil || !validIDs(brand, id) || member != "" && !validIDs(member) {
		return OrderView{}, ErrInvalid
	}
	var audit string
	o, err := scanOrder(orderAuditRow{tx.QueryRow(ctx, `SELECT `+viewColumns+` FROM withdrawal_orders WHERE brand_id=$1 AND id=$2 AND ($3::text='' OR member_id=NULLIF($3,'')::uuid)`, brand, id, member), &audit})
	if err != nil {
		return OrderView{}, err
	}
	o.AuditLogID = audit
	out := ToOrderView(o)
	if member != "" {
		out = ToUserOrderView(out)
	}
	return out, nil
}
func (s OrderService) ListViewTx(ctx context.Context, tx pgx.Tx, brand, member, state string, limit, offset int) (OrderPage, error) {
	out := OrderPage{BrandID: brand, Items: []OrderView{}, Limit: limit, Offset: offset}
	if tx == nil || !validIDs(brand) || member != "" && !validIDs(member) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 || !validStateFilter(state) {
		return out, ErrInvalid
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brands WHERE id=$1) AND ($2::text='' OR EXISTS(SELECT 1 FROM brand_members WHERE brand_id=$1 AND id=NULLIF($2,'')::uuid))`, brand, member).Scan(&exists); err != nil {
		return out, err
	}
	if !exists {
		return out, ErrNotFound
	}
	rows, err := tx.Query(ctx, `SELECT `+viewColumns+` FROM withdrawal_orders WHERE brand_id=$1 AND ($2::text='' OR member_id=NULLIF($2,'')::uuid) AND ($3::text='' OR state=$3) ORDER BY created_at DESC,id DESC LIMIT $4 OFFSET $5`, brand, member, state, limit+1, offset)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var audit string
		o, e := scanOrder(orderAuditRow{rows, &audit})
		if e != nil {
			return out, e
		}
		o.AuditLogID = audit
		out.Items = append(out.Items, ToOrderView(o))
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.HasMore = true
		out.Items = out.Items[:limit]
	}
	return out, nil
}
func validStateFilter(s string) bool {
	switch s {
	case "", "reviewing", "processing", "paid", "rejected", "failed", "cancelled":
		return true
	}
	return false
}
func (s OrderService) HistoryViewTx(ctx context.Context, tx pgx.Tx, brand, id, member string) (HistoryView, error) {
	out := HistoryView{BrandID: brand, OrderID: id, Items: []TransitionView{}}
	if _, err := s.ReadViewTx(ctx, tx, brand, id, member); err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT id::text,version,from_state,to_state,reason,actor_type,created_at,audit_log_id::text FROM withdrawal_order_transitions WHERE brand_id=$1 AND order_id=$2 ORDER BY version`, brand, id)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v TransitionView
		if err = rows.Scan(&v.ID, &v.Version, &v.FromState, &v.ToState, &v.Reason, &v.ActorType, &v.CreatedAt, &v.AuditLogID); err != nil {
			return out, err
		}
		v.CreatedAt = v.CreatedAt.UTC()
		if member != "" && v.ToState != "rejected" && v.ToState != "failed" && v.ToState != "cancelled" {
			v.Reason = ""
		}
		out.Items = append(out.Items, v)
	}
	return out, rows.Err()
}
func (s OrderService) AvailabilityTx(ctx context.Context, tx pgx.Tx, brand, member string) (AvailabilityView, error) {
	out := AvailabilityView{BrandID: brand, MemberID: member, AllowedSources: []string{}}
	if tx == nil || !validIDs(brand, member) {
		return out, ErrInvalid
	}
	policy, err := scanBrand(tx.QueryRow(ctx, `SELECT `+brandFields+` FROM brand_withdrawal_policies WHERE brand_id=$1 FOR SHARE`, brand))
	if err != nil {
		return out, err
	}
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM brand_members WHERE brand_id=$1 AND id=$2`, brand, member).Scan(&status); err != nil {
		return out, err
	}
	out.PolicyEnabled = policy.Config.Enabled
	out.EligibilityConfigured = s.Eligibility != nil
	out.MinPoints = policy.Config.MinPoints
	out.MaxPoints = policy.Config.MaxPoints
	out.AllowedSources = policy.Config.AllowedSources
	switch {
	case status != "normal":
		out.ReasonCode = "WITHDRAWAL_ACCOUNT_RESTRICTED"
	case !out.PolicyEnabled:
		out.ReasonCode = "WITHDRAWAL_DISABLED"
	case !out.EligibilityConfigured:
		out.ReasonCode = "WITHDRAWAL_ELIGIBILITY_NOT_CONFIGURED"
	default:
		out.CanApply = true
		out.ReasonCode = "AVAILABLE"
	}
	return out, nil
}
