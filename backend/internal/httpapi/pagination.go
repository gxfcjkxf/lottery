package httpapi

import (
	"context"
	"github.com/jackc/pgx/v5"
)

type paginationCounter interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Called only after the list's normal authorization and scope checks succeed.
func countedItems(ctx context.Context, db paginationCounter, items any, query string, args ...any) (map[string]any, error) {
	var total string
	if err := db.QueryRow(ctx, query, args...).Scan(&total); err != nil {
		return nil, err
	}
	return map[string]any{"items": items, "total_count": total}, nil
}
