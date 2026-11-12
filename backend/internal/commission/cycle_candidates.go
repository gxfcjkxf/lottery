package commission

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// The representative order stays internal to creation; operators choose windows.
type CycleCandidate struct {
	AnchorOrderID string    `json:"anchor_order_id"`
	From          time.Time `json:"window_from"`
	To            time.Time `json:"window_to"`
	Cycle         string    `json:"cycle"`
}

type CycleCandidatePage struct {
	BrandID string           `json:"brand_id"`
	Items   []CycleCandidate `json:"items"`
	Limit   int              `json:"limit"`
	Offset  int              `json:"offset"`
}

func (s Service) CycleCandidatesTx(ctx context.Context, tx pgx.Tx, brand string, limit, offset int) (CycleCandidatePage, error) {
	out := CycleCandidatePage{BrandID: brand, Items: []CycleCandidate{}, Limit: limit, Offset: offset}
	rows, err := tx.Query(ctx, `WITH windows AS (
 SELECT o.id, o.placed_at, w.window_from, w.window_to,
 o.commission_rule_snapshot->'financial_policy'->'config'->'calendar'->>'cycle' AS cycle
 FROM betting_financial_facts o
 CROSS JOIN LATERAL commission_calendar_window(
 o.commission_rule_snapshot->'financial_policy'->'config'->'calendar', o.placed_at) w
 WHERE o.brand_id=$1 AND o.commission_rule_snapshot->'financial_policy'->'config'->>'enabled'='true'
), candidates AS (
 SELECT DISTINCT ON (window_from,window_to) id,window_from,window_to,cycle
 FROM windows w WHERE window_to<=clock_timestamp()
 AND NOT EXISTS (SELECT 1 FROM commission_cycles c WHERE c.brand_id=$1 AND c.window_from=w.window_from AND c.window_to=w.window_to)
 ORDER BY window_from,window_to,placed_at,id
)
SELECT id::text,window_from,window_to,cycle FROM candidates
ORDER BY window_from DESC,window_to DESC LIMIT $2 OFFSET $3`, brand, limit, offset)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var item CycleCandidate
		if err := rows.Scan(&item.AnchorOrderID, &item.From, &item.To, &item.Cycle); err != nil {
			return out, err
		}
		out.Items = append(out.Items, item)
	}
	return out, rows.Err()
}
