package reporting

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrHistoricalRange = errors.New("historical reports require complete days in the archive timezone")

type historyRow struct{ pgx.Row }

func (r historyRow) Scan(dest ...any) error {
	err := r.Row.Scan(dest...)
	var p *pgconn.PgError
	if errors.As(err, &p) && p.Code == "P7401" {
		return ErrHistoricalRange
	}
	return err
}
func historyQueryRow(q rowQuerier, ctx context.Context, sql string, args ...any) pgx.Row {
	return historyRow{q.QueryRow(ctx, sql, args...)}
}
