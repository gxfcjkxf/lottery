package commission

import (
	"context"
	"github.com/jackc/pgx/v5"
	"time"
)

// ListFilter selects complete saved cycles, never a slice of their earnings.
// From is inclusive and To exclusive, both against the saved cycle end.
type ListFilter struct {
	From    *time.Time
	To      *time.Time
	State   string
	CycleID string
	AgentID string
}

func (f ListFilter) Validate(kind string) error {
	if (f.From == nil) != (f.To == nil) || (f.From != nil && !f.From.Before(*f.To)) {
		return ErrInvalid
	}
	if f.CycleID != "" && !cycleCanonicalUUID.MatchString(f.CycleID) {
		return ErrInvalid
	}
	if f.AgentID != "" && !cycleCanonicalUUID.MatchString(f.AgentID) {
		return ErrInvalid
	}
	states := map[string]bool{"": true}
	switch kind {
	case "cycle":
		for _, s := range []string{"enumerating", "waiting", "calculating", "summarizing", "ready", "failed", "stale"} {
			states[s] = true
		}
	case "payment":
		for _, s := range []string{"awaiting_approval", "paying", "paid", "failed", "stale", "blocked", "needs_attention"} {
			states[s] = true
		}
	case "correction":
		for _, state := range []string{"pending", "processing", "history"} {
			states[state] = true
		}
	case "target":
		states["pending"] = true
		states["paid"] = true
		if f.From != nil || f.CycleID != "" {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	if !states[f.State] {
		return ErrInvalid
	}
	return nil
}

func (f ListFilter) validateScope(ctx context.Context, tx pgx.Tx, brand string) error {
	for _, check := range []struct{ id, query string }{
		{f.CycleID, `SELECT EXISTS(SELECT 1 FROM commission_cycles WHERE brand_id=$1 AND id=$2)`},
		{f.AgentID, `SELECT EXISTS(SELECT 1 FROM agent_nodes WHERE brand_id=$1 AND id=$2)`},
	} {
		if check.id == "" {
			continue
		}
		var exists bool
		if err := tx.QueryRow(ctx, check.query, brand, check.id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}
