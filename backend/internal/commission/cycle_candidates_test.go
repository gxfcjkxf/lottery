package commission

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestCycleCandidatesHistoricalWindowsAndBrandScope(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	// Owned temporary relations isolate directory selection from financial writes.
	_, err = tx.Exec(ctx, `CREATE TEMP TABLE betting_financial_facts(id uuid,brand_id uuid,placed_at timestamptz,commission_rule_snapshot jsonb) ON COMMIT DROP;
CREATE TEMP TABLE commission_cycles(brand_id uuid,window_from timestamptz,window_to timestamptz) ON COMMIT DROP`)
	if err != nil {
		t.Fatal(err)
	}
	weekly := `{"financial_policy":{"config":{"enabled":true,"calendar":{"timezone":"Asia/Singapore","cycle":"weekly","boundary_time":"00:00:00","weekday":1,"month_day":null,"short_month":""}}}}`
	monthly := `{"financial_policy":{"config":{"enabled":true,"calendar":{"timezone":"UTC","cycle":"monthly","boundary_time":"00:00:00","weekday":null,"month_day":1,"short_month":"last_day"}}}}`
	for _, row := range []struct{ id, brand, at, snapshot string }{
		{batchAnchorID, batchBrandID, "2020-01-07T00:00:00Z", weekly},
		{batchAdminID, batchBrandID, "2020-01-08T00:00:00Z", weekly},
		{"dddddddd-dddd-4ddd-8ddd-dddddddddddd", batchBrandID, "2020-02-07T00:00:00Z", monthly},
		{"eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", batchBrandID, "2099-01-07T00:00:00Z", weekly},
		{"ffffffff-ffff-4fff-8fff-ffffffffffff", batchAdminID, "2020-03-07T00:00:00Z", monthly},
		{"11111111-1111-4111-8111-111111111111", batchBrandID, "2020-03-07T00:00:00Z", `{"financial_policy":{"config":{"enabled":false}}}`},
	} {
		if _, err = tx.Exec(ctx, `INSERT INTO betting_financial_facts VALUES($1,$2,$3,$4)`, row.id, row.brand, row.at, row.snapshot); err != nil {
			t.Fatal(err)
		}
	}
	s := Service{DB: db}
	page, err := s.CycleCandidatesTx(ctx, tx, batchBrandID, 100, 0)
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("candidates=%+v err=%v", page, err)
	}
	if page.Items[0].Cycle != "monthly" || page.Items[1].AnchorOrderID != batchAnchorID || page.Items[1].From.UTC().Format("2006-01-02T15:04:05Z") != "2020-01-05T16:00:00Z" {
		t.Fatalf("unexpected historical windows %+v", page)
	}
	first := page.Items[0]
	if _, err = tx.Exec(ctx, `INSERT INTO commission_cycles VALUES($1,$2,$3)`, batchBrandID, first.From, first.To); err != nil {
		t.Fatal(err)
	}
	page, err = s.CycleCandidatesTx(ctx, tx, batchBrandID, 1, 0)
	if err != nil || len(page.Items) != 1 || page.Items[0].Cycle != "weekly" {
		t.Fatalf("registered cycle remained: %+v %v", page, err)
	}
	page, err = s.CycleCandidatesTx(ctx, tx, batchBrandID, 1, 1)
	if err != nil || len(page.Items) != 0 || page.Items == nil {
		t.Fatalf("empty page %+v %v", page, err)
	}
}
