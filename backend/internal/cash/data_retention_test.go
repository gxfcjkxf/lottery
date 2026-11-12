package cash

import (
	"context"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/dataretention"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/jackc/pgx/v5"
)

func TestRetentionPreservesCashTailAndUSDTIdentity(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	o := usdtOrder(t, f, "100")
	transfer := transferFor(t, f, o)
	credited, err := transact(t, f, func(tx pgx.Tx) (bool, error) { return f.s.CreditUSDT(ctx, tx, transfer) })
	if err != nil || !credited {
		t.Fatal(credited, err)
	}
	var before int64
	if err = f.db.QueryRow(ctx, `SELECT deposited FROM cash_balances WHERE brand_id=$1 AND member_id=$2 AND currency='USDT'`, f.brand, f.member).Scan(&before); err != nil {
		t.Fatal(err)
	}
	cutoff := time.Now().UTC().AddDate(0, 0, 1)
	if _, err = f.db.Exec(ctx, `INSERT INTO data_retention_tasks(id,brand_id,policy_version,timezone,execute_at,cutoff_at) VALUES($1,$2,1,'UTC',$3,$4)`, ids.New(), f.brand, cutoff.AddDate(0, 2, 0), cutoff); err != nil {
		t.Fatal(err)
	}
	s := dataretention.Service{DB: f.db}
	for i := 0; i < 30; i++ {
		worked, e := s.Process(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if !worked {
			break
		}
		if i == 29 {
			t.Fatal("unfinished retention")
		}
	}
	var live int
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM cash_orders WHERE id=$1`, o.ID).Scan(&live); err != nil || live != 0 {
		t.Fatal(live, err)
	}
	credited, err = transact(t, f, func(tx pgx.Tx) (bool, error) { return f.s.CreditUSDT(ctx, tx, transfer) })
	if err != nil || credited {
		t.Fatal("old chain transfer was credited again", credited, err)
	}
	next := usdtOrder(t, f, "100")
	nextTransfer := transferFor(t, f, next)
	nextTransfer.Transaction = transfer.Transaction
	nextTransfer.Index = transfer.Index
	credited, err = transact(t, f, func(tx pgx.Tx) (bool, error) { return f.s.CreditUSDT(ctx, tx, nextTransfer) })
	if err != nil || credited {
		t.Fatal("archived transaction identity reused", credited, err)
	}
	nextTransfer.Transaction = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	credited, err = transact(t, f, func(tx pgx.Tx) (bool, error) { return f.s.CreditUSDT(ctx, tx, nextTransfer) })
	if err != nil || !credited {
		t.Fatal("new credit failed after archived cash tail", credited, err)
	}
	var after int64
	if err = f.db.QueryRow(ctx, `SELECT deposited FROM cash_balances WHERE brand_id=$1 AND member_id=$2 AND currency='USDT'`, f.brand, f.member).Scan(&after); err != nil || after != before*2 {
		t.Fatal(before, after, err)
	}
}
