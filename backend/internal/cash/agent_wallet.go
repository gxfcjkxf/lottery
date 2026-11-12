package cash

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"regexp"
	"time"
)

var agentMemberID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type AgentTransferInput struct {
	MemberID string `json:"member_id"`
	Amount   string `json:"amount"`
}

// Agents do not bet and have no member turnover cycle. Their available points
// can be converted directly; frozen points are never consumed.
func (s Service) convertAgent(ctx context.Context, tx pgx.Tx, brand, agent, order string, amount points.Amount, meta points.Metadata) (points.Entry, json.RawMessage, error) {
	wallet, e := s.Points.LockedSnapshot(ctx, tx, brand, agent)
	if e != nil {
		return points.Entry{}, nil, e
	}
	allocation, e := wallet.BySource.Allocate(amount, "available")
	if e != nil {
		return points.Entry{}, nil, e
	}
	delta, e := points.AllocationDelta(allocation, "available", "")
	if e != nil {
		return points.Entry{}, nil, e
	}
	entry, e := s.Points.Post(ctx, tx, points.Change{BrandID: brand, MemberID: agent, EntryType: "currency_exchange_debit", ReferenceType: "cash_order", ReferenceID: order, OperationKey: "cash-exchange:" + order, ActorType: meta.ActorType, ActorID: meta.ActorID, RequestID: meta.RequestID, IP: meta.IP, Delta: delta, Allocation: allocation})
	evidence, marshalErr := json.Marshal(map[string]any{"source_allocation": allocation, "agent_account_id": agent})
	if e != nil {
		return entry, nil, e
	}
	return entry, evidence, marshalErr
}

func (s Service) TransferAgent(ctx context.Context, tx pgx.Tx, brand, agent string, in AgentTransferInput, meta points.Metadata) (any, error) {
	amount, e := points.ParseAmount(in.Amount)
	if e != nil || amount <= 0 || !agentMemberID.MatchString(in.MemberID) {
		return nil, ErrInvalid
	}
	if e = checkOwner(ctx, tx, brand, agent, meta); e != nil {
		return nil, e
	}
	var allowed bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brand_members m JOIN global_users u ON u.id=m.global_user_id JOIN agent_nodes n ON n.brand_id=m.brand_id AND n.id::text=m.attribution_snapshot->>'agent_id' JOIN agent_nodes a ON a.brand_id=n.brand_id AND a.id=ANY(n.path) WHERE m.brand_id=$1 AND m.id=$2 AND a.member_id=$3 AND m.status='normal' AND u.status='active')`, brand, in.MemberID, agent).Scan(&allowed)
	if e != nil {
		return nil, e
	}
	if !allowed {
		return nil, ErrInvalid
	}
	if _, e = s.Points.LockedPolicy(ctx, tx, brand); e != nil {
		return nil, e
	}
	wallet, e := s.Points.LockedSnapshot(ctx, tx, brand, agent)
	if e != nil {
		return nil, e
	}
	allocation, e := wallet.BySource.Allocate(amount, "available")
	if e != nil {
		return nil, e
	}
	delta, e := points.AllocationDelta(allocation, "available", "")
	if e != nil {
		return nil, e
	}
	id := ids.New()
	change := points.Change{BrandID: brand, MemberID: agent, EntryType: "agent_transfer_debit", ReferenceType: "agent_transfer", ReferenceID: id, OperationKey: "agent-transfer:" + id + ":debit", ActorType: meta.ActorType, ActorID: meta.ActorID, RequestID: meta.RequestID, IP: meta.IP, Delta: delta, Allocation: allocation}
	if _, e = s.Points.Post(ctx, tx, change); e != nil {
		return nil, e
	}
	// Member receipts are recharge points, so transferring never bypasses the
	// member's withdrawal source and turnover requirements.
	change.MemberID = in.MemberID
	change.EntryType = "agent_transfer_credit"
	change.OperationKey = "agent-transfer:" + id + ":credit"
	change.Delta = points.Balance{}
	change.Delta[0][0] = amount
	change.Allocation = []points.Allocation{{Source: "recharge", State: "available", Points: amount}}
	if _, e = s.Points.Post(ctx, tx, change); e != nil {
		return nil, e
	}
	out := map[string]any{"id": id, "member_id": in.MemberID, "amount": amount}
	return out, record(ctx, tx, brand, "agent.transfer", id, nil, out, meta)
}

// Both sides are immutable ledger entries linked by the transfer reference.
// Scope the debit to the authenticated agent before reading its recipient.
func AgentTransfers(ctx context.Context, tx pgx.Tx, brand, agent string, limit, offset int) (json.RawMessage, error) {
	var out json.RawMessage
	e := tx.QueryRow(ctx, `WITH transfers AS (
 SELECT d.id,c.member_id,u.username AS member_account,c.delta_snapshot->'recharge'->>'available' AS amount,d.created_at
 FROM point_ledger_entries d
 JOIN point_ledger_entries c ON c.brand_id=d.brand_id AND c.reference_type='agent_transfer' AND c.reference_id=d.reference_id AND c.entry_type='agent_transfer_credit'
 LEFT JOIN brand_members m ON m.brand_id=c.brand_id AND m.id=c.member_id
 LEFT JOIN global_users u ON u.id=m.global_user_id
 WHERE d.brand_id=$1 AND d.member_id=$2 AND d.entry_type='agent_transfer_debit' AND d.reference_type='agent_transfer'
 ), page AS (SELECT * FROM transfers ORDER BY created_at DESC,id DESC LIMIT $3 OFFSET $4)
 SELECT jsonb_build_object('items',coalesce((SELECT jsonb_agg(to_jsonb(page) ORDER BY created_at DESC,id DESC) FROM page),'[]'::jsonb),'total_count',(SELECT count(*)::text FROM transfers),'limit',$3::int,'offset',$4::int)`, brand, agent, limit, offset).Scan(&out)
	return out, e
}

type AgentTransferRecord struct {
	ID            string    `json:"id"`
	MemberID      string    `json:"member_id"`
	MemberAccount *string   `json:"member_account"`
	Amount        string    `json:"amount"`
	CreatedAt     time.Time `json:"created_at"`
}
type AgentWalletRecord struct {
	ID        string               `json:"id"`
	Kind      string               `json:"kind"`
	CreatedAt time.Time            `json:"created_at"`
	Order     *Order               `json:"order"`
	Transfer  *AgentTransferRecord `json:"transfer"`
}
type AgentWalletRecordPage struct {
	Items      []AgentWalletRecord `json:"items"`
	TotalCount string              `json:"total_count"`
	Limit      int                 `json:"limit"`
	Offset     int                 `json:"offset"`
}

// Page the combined timeline before loading currency-order details.
func AgentWalletRecords(ctx context.Context, tx pgx.Tx, brand, agent string, limit, offset int) (AgentWalletRecordPage, error) {
	var raw []byte
	var out AgentWalletRecordPage
	e := tx.QueryRow(ctx, `WITH records AS (
 SELECT id,kind,created_at,NULL::jsonb AS transfer FROM cash_orders WHERE brand_id=$1 AND member_id=$2
 UNION ALL
 SELECT d.id,'transfer',d.created_at,jsonb_build_object('id',d.id,'member_id',c.member_id,'member_account',u.username,'amount',c.delta_snapshot->'recharge'->>'available','created_at',d.created_at)
 FROM point_ledger_entries d JOIN point_ledger_entries c ON c.brand_id=d.brand_id AND c.reference_type='agent_transfer' AND c.reference_id=d.reference_id AND c.entry_type='agent_transfer_credit'
 LEFT JOIN brand_members m ON m.brand_id=c.brand_id AND m.id=c.member_id LEFT JOIN global_users u ON u.id=m.global_user_id
 WHERE d.brand_id=$1 AND d.member_id=$2 AND d.entry_type='agent_transfer_debit' AND d.reference_type='agent_transfer'
 ),page AS (SELECT * FROM records ORDER BY created_at DESC,id DESC LIMIT $3 OFFSET $4)
 SELECT jsonb_build_object('items',coalesce((SELECT jsonb_agg(to_jsonb(page) ORDER BY created_at DESC,id DESC) FROM page),'[]'::jsonb),'total_count',(SELECT count(*)::text FROM records),'limit',$3::int,'offset',$4::int)`, brand, agent, limit, offset).Scan(&raw)
	if e != nil {
		return out, e
	}
	if e = json.Unmarshal(raw, &out); e != nil {
		return out, e
	}
	for i := range out.Items {
		if out.Items[i].Kind != "transfer" {
			order, err := ReadOrder(ctx, tx, brand, agent, out.Items[i].ID)
			if err != nil {
				return out, err
			}
			out.Items[i].Order = &order
		}
	}
	return out, nil
}
