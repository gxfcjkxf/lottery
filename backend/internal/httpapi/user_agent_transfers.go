package httpapi

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type userAgentTransfer struct {
	AgentID      string        `json:"agent_id"`
	AgentAccount *string       `json:"agent_account"`
	Amount       points.Amount `json:"amount"`
	CreatedAt    time.Time     `json:"created_at"`
}
type userLedgerEntry struct {
	points.Entry
	AgentTransfer *userAgentTransfer `json:"agent_transfer,omitempty"`
}

func userLedgerEntries(ctx context.Context, db *pgxpool.Pool, brand, member string, entries []points.Entry) ([]userLedgerEntry, error) {
	out := make([]userLedgerEntry, len(entries))
	ids := []string{}
	indexes := map[string]int{}
	for i, entry := range entries {
		out[i].Entry = entry
		if entry.EntryType == "agent_transfer_credit" {
			ids = append(ids, entry.ID)
			indexes[entry.ID] = i
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := db.Query(ctx, `SELECT c.id::text,d.member_id::text,a.username,(c.delta_snapshot->'recharge'->>'available')::numeric,c.created_at
 FROM point_ledger_entries c JOIN point_ledger_entries d ON d.brand_id=c.brand_id AND d.reference_type='agent_transfer' AND d.reference_id=c.reference_id AND d.entry_type='agent_transfer_debit'
 LEFT JOIN agent_accounts a ON a.brand_id=d.brand_id AND a.id=d.member_id
 WHERE c.brand_id=$1 AND c.member_id=$2 AND c.id=ANY($3::uuid[]) AND c.entry_type='agent_transfer_credit' AND c.reference_type='agent_transfer'`, brand, member, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var receipt userAgentTransfer
		if err = rows.Scan(&id, &receipt.AgentID, &receipt.AgentAccount, &receipt.Amount, &receipt.CreatedAt); err != nil {
			return nil, err
		}
		out[indexes[id]].AgentTransfer = &receipt
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for _, index := range indexes {
		if out[index].AgentTransfer == nil {
			return nil, errors.New("agent transfer credit has no matching debit")
		}
	}
	return out, nil
}
