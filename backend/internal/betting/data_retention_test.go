package betting

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/dataretention"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
	"github.com/gxfcjkxf/lottery/backend/internal/withdrawal"
	"github.com/jackc/pgx/v5"
)

func TestRetentionKeepsSettlementCommissionAndTurnoverFacts(t *testing.T) {
	c := newConfiguredFixture(t)
	ctx := context.Background()
	brand := ids.New()
	if _, err := c.f.db.Exec(ctx, `INSERT INTO brands(id,code,name,status,allow_custom_games) VALUES($1,$2,'Retention betting isolated test','active',true)`, brand, "retention_"+brand); err != nil {
		t.Fatal(err)
	}
	c = configuredMember(t, c, brand)
	f := c.f
	fundBettingWallet(t, f, 10000)
	out, err := configuredPlace(t, c, ids.New(), c.input)
	if err != nil {
		t.Fatal(err)
	}
	var order ConfiguredOrder
	if err = json.Unmarshal(out.Data, &order); err != nil {
		t.Fatal(err)
	}
	configuredPublishDraw(t, c, 8)
	if _, err = f.service.ProcessConfiguredSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	notifier := notification.Service{DB: f.db}
	if _, err = notifier.Process(ctx, 100); err != nil {
		t.Fatal(err)
	}
	var notificationsBefore int
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE brand_id=$1`, f.brand).Scan(&notificationsBefore); err != nil || notificationsBefore == 0 {
		t.Fatal("notification fixture missing", notificationsBefore, err)
	}
	before := walletBySource(t, f)
	var zone string
	if err = f.db.QueryRow(ctx, `SELECT timezone FROM brands WHERE id=$1`, f.brand).Scan(&zone); err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(loc)
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	to := from.AddDate(0, 0, 1)
	rs := reporting.Service{DB: f.db}
	q := reporting.Query{From: from, To: to, GroupBy: "game", Limit: 20}
	original, err := rs.Betting(ctx, f.brand, q)
	if err != nil {
		t.Fatal(err)
	}
	var proofBefore, proofAfter string
	check := func(target *string) {
		bettingTx(t, f.db, func(tx pgx.Tx) error {
			resolution, e := (commission.Source{}).FinalOrderTx(ctx, tx, f.brand, order.ID)
			if e != nil {
				return e
			}
			if resolution.Fact == nil {
				t.Fatal("missing archived economic fact")
			}
			if _, e = (commission.Source{}).PolicySnapshotTx(ctx, tx, f.brand, order.ID); e != nil {
				return e
			}
			w, e := (points.Store{DB: f.db}).Snapshot(ctx, tx, f.brand, f.member)
			if e != nil {
				return e
			}
			base := withdrawal.TurnoverBaseSnapshot{RechargeAvailable: w.BySource[0][0], GiftAvailable: w.BySource[2][0], Points: w.BySource[0][0] + w.BySource[2][0], WalletVersion: w.Version}
			d, e := (withdrawal.TurnoverChecker{}).Check(ctx, tx, withdrawal.EligibilityInput{BrandID: f.brand, MemberID: f.member, Wallet: w, TurnoverBase: base, CutoffAt: time.Now(), CutoffVersion: w.Version})
			*target = string(d.Evidence)
			return e
		})
	}
	check(&proofBefore)
	if _, err = f.db.Exec(ctx, `INSERT INTO data_retention_tasks(id,brand_id,policy_version,timezone,execute_at,cutoff_at) VALUES($1,$2,1,$3,$4,$5)`, ids.New(), f.brand, zone, to.AddDate(0, 2, 0), to); err != nil {
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
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM configured_orders WHERE id=$1`, order.ID).Scan(&live); err != nil || live != 0 {
		t.Fatal("bet detail retained", live, err)
	}
	var payload json.RawMessage
	if err = f.db.QueryRow(ctx, `SELECT payload FROM data_retention_receipts WHERE family='configured_orders' AND id=$1`, order.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var receipt map[string]json.RawMessage
	if err = json.Unmarshal(payload, &receipt); err != nil {
		t.Fatal(err)
	}
	if string(receipt["sales_snapshot"]) != "{}" {
		t.Fatal("raw sales configuration retained")
	}
	current, err := rs.Betting(ctx, f.brand, q)
	if err != nil || current.Summary != original.Summary {
		t.Fatal(original.Summary, current.Summary, err)
	}
	attr, err := rs.Attribution(ctx, f.brand, reporting.AttributionQuery{From: from, To: to, GroupBy: "game", Limit: 20, AgentScope: "direct"})
	if err != nil || attr.Summary.BettingTotals != original.Summary {
		t.Fatal(attr, err)
	}
	check(&proofAfter)
	if proofBefore != proofAfter {
		t.Fatal("turnover changed after retention", proofBefore, proofAfter)
	}
	if after := walletBySource(t, f); after != before {
		t.Fatal("wallet changed", before, after)
	}
	var messages, deliveries int
	if err = f.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM notifications WHERE brand_id=$1),(SELECT count(*) FROM notification_deliveries WHERE brand_id=$1)`, f.brand).Scan(&messages, &deliveries); err != nil || messages != 0 || deliveries != 0 {
		t.Fatal("completed message details retained", messages, deliveries, err)
	}
	if n, e := notifier.Process(ctx, 100); e != nil || n != 0 {
		t.Fatal("old notifications delivered again", n, e)
	}
	if n, e := f.service.ProcessConfiguredSettlements(ctx, 20); e != nil || n != 0 {
		t.Fatal("settlement repeated", n, e)
	}
}
