package betting

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"testing"
)

func TestDecimalPrizeWalletAndReplay(t *testing.T) {
	c := newConfiguredFixture(t)
	c.input.Requests[0].Points = 101
	fundBettingWallet(t, c.f, points.Amount(1000)) // 10.00 points
	placed, err := configuredPlace(t, c, "decimal-place", c.input)
	if err != nil {
		t.Fatal(err)
	}
	var order ConfiguredOrder
	if err = json.Unmarshal(placed.Data, &order); err != nil {
		t.Fatal(err)
	}
	configuredPublishDraw(t, c, 7)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := c.f.service.ProcessConfiguredSettlements(ctx, 10); err != nil {
			t.Fatal(err)
		}
	}
	wallet, err := (points.Store{DB: c.f.db}).Read(ctx, c.f.brand, c.f.member)
	if err != nil {
		t.Fatal(err)
	}
	if wallet.WinningPoints.String() != "2.53" || wallet.AvailablePoints.String() != "11.52" {
		t.Fatalf("wrong fractional wallet: %+v", wallet)
	}
	var count int
	if err = c.f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE reference_id=$1 AND entry_type='prize'`, order.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("prize count=%d err=%v", count, err)
	}
}
