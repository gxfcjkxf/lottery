package betting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestBetCannotCommitWithoutWithdrawalRuleSnapshot(t *testing.T) {
	f := newBettingFixtureWithWindow(t, storeTestBrand, 5*time.Second, 5100*time.Millisecond)
	fundBettingWallet(t, f, 37)
	before := walletBySource(t, f)
	if _, err := f.db.Exec(context.Background(), `ALTER TABLE bet_orders DISABLE TRIGGER capture_bet_withdrawal_snapshot`); err != nil {
		t.Fatal(err)
	}
	_, err := placeBettingOrder(t, f, f.input, "required-rule-snapshot")
	var sqlErr *pgconn.PgError
	if !errors.As(err, &sqlErr) || sqlErr.Code != "23502" || sqlErr.ColumnName != "withdrawal_rule_snapshot" {
		t.Fatalf("missing required rule snapshot was not rejected by the database: %v", err)
	}
	if walletBySource(t, f) != before {
		t.Fatal("failed bet changed points")
	}
	var orders int
	if err := f.db.QueryRow(context.Background(), `SELECT count(*) FROM bet_orders WHERE brand_id=$1`, f.brand).Scan(&orders); err != nil || orders != 0 {
		t.Fatal("failed bet committed an order", orders, err)
	}
}
