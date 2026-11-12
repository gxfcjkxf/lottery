package betting

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/attribution"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"os"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/withdrawal"
	"github.com/jackc/pgx/v5"
)

func TestConfiguredFinancialFactsAndTurnover(t *testing.T) {
	for _, tc := range []struct {
		name        string
		draw        int
		valid, loss points.Amount
	}{{"lost", 8, 1000, 1000}, {"won", 7, 1000, 0}, {"push", 49, 0, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			c := newConfiguredFixture(t)
			f := c.f
			ctx := context.Background()
			fundBettingWallet(t, f, 10000)
			placed, err := configuredPlace(t, c, ids.New(), c.input)
			if err != nil {
				t.Fatal(err)
			}
			var order ConfiguredOrder
			if err = json.Unmarshal(placed.Data, &order); err != nil {
				t.Fatal(err)
			}
			bettingTx(t, f.db, func(tx pgx.Tx) error {
				r, e := (commission.Source{}).FinalOrderTx(ctx, tx, f.brand, order.ID)
				if e == nil && r.Reason != "not_final" {
					t.Fatal(r)
				}
				return e
			})
			configuredPublishDraw(t, c, tc.draw)
			if _, err = f.service.ProcessConfiguredSettlements(ctx, 20); err != nil {
				t.Fatal(err)
			}
			bettingTx(t, f.db, func(tx pgx.Tx) error {
				r, e := (commission.Source{}).FinalOrderTx(ctx, tx, f.brand, order.ID)
				if e != nil {
					return e
				}
				if r.Fact == nil || r.Fact.Basis.TurnoverPoints != tc.valid || r.Fact.Basis.LossPoints != tc.loss {
					t.Fatalf("wrong financial fact: %+v", r.Fact)
				}
				if _, e = (commission.Source{}).PolicySnapshotTx(ctx, tx, f.brand, order.ID); e != nil {
					return e
				}
				w, e := (points.Store{DB: f.db}).Snapshot(ctx, tx, f.brand, f.member)
				if e != nil {
					return e
				}
				b := withdrawal.TurnoverBaseSnapshot{RechargeAvailable: w.BySource[0][0], GiftAvailable: w.BySource[2][0], Points: w.BySource[0][0] + w.BySource[2][0], WalletVersion: w.Version}
				d, e := (withdrawal.TurnoverChecker{}).Check(ctx, tx, withdrawal.EligibilityInput{BrandID: f.brand, MemberID: f.member, Wallet: w, TurnoverBase: b, CutoffAt: time.Now(), CutoffVersion: w.Version})
				if e != nil {
					return e
				}
				var proof struct {
					Qualification struct {
						Points string `json:"valid_points"`
					} `json:"turnover_qualification"`
				}
				if e = json.Unmarshal(d.Evidence, &proof); e != nil {
					return e
				}
				if proof.Qualification.Points != tc.valid.String() {
					t.Fatalf("turnover %s want %s", proof.Qualification.Points, tc.valid.String())
				}
				return nil
			})
			reports := reporting.Service{DB: f.db}
			q := reporting.Query{From: order.PlacedAt.Add(-time.Minute), To: time.Now().Add(time.Minute), GroupBy: "game", Limit: 20}
			report, e := reports.Betting(ctx, f.brand, q)
			if e != nil || report.Summary.OrderCount != "1" || report.Summary.StakePoints != "10" {
				t.Fatal("configured report", report.Summary, e)
			}
			attributed, e := reports.Attribution(ctx, f.brand, reporting.AttributionQuery{From: q.From, To: q.To, GroupBy: "game", Limit: 20, AgentScope: "direct"})
			lostStake := "0"
			if tc.name == "lost" {
				lostStake = "10"
			}
			if e != nil || attributed.Summary.BettingTotals != report.Summary || attributed.Summary.FinalLostStakePoints != lostStake {
				t.Fatal("configured attribution", attributed.Summary, e)
			}

			bettingTx(t, f.db, func(tx pgx.Tx) error {
				archived, e := reports.CaptureArchive(ctx, tx, f.brand, q.From, q.To)
				if e != nil {
					return e
				}
				if archived.Betting != report.Summary {
					t.Fatalf("archive differs from current report: %+v %+v", archived.Betting, report.Summary)
				}
				return nil
			})

			before := walletBySource(t, f)
			if _, err = f.service.ProcessConfiguredSettlements(ctx, 20); err != nil {
				t.Fatal(err)
			}
			if before != walletBySource(t, f) {
				t.Fatal("settlement replay changed money")
			}
		})
	}
}

func TestCommissionConfiguredDiscoveryAndCycleUseConfiguredOrders(t *testing.T) {
	c := newConfiguredFixture(t)
	f := c.f
	ctx := context.Background()
	finance := commission.Service{DB: f.db}
	actor := access.Account{ID: c.a.ID, Type: access.AccountAdmin, BrandIDs: []string{f.brand}, Roles: []access.Role{{BrandID: f.brand, Permissions: []access.Permission{{Resource: "commission_policy", Action: "write", Scope: access.ScopeBrand}, {Resource: "commission", Action: "run", Scope: access.ScopeBrand}, {Resource: "commission", Action: "view", Scope: access.ScopeBrand}}}}}
	boundary := time.Now().UTC().Truncate(time.Second).Add(10 * time.Second)
	weekday := int(boundary.Weekday())
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := finance.Update(ctx, tx, f.brand, actor, commission.PolicyInput{Version: 1, Config: commission.PolicyConfig{Enabled: true, Calendar: &commission.Calendar{Timezone: "UTC", Cycle: "weekly", BoundaryTime: boundary.Format("15:04:05"), Weekday: &weekday}, PayoutMode: commission.PayoutManual}}, points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()})
		return e
	})
	fundBettingWallet(t, f, 10000)
	out, err := configuredPlace(t, c, ids.New(), c.input)
	if err != nil {
		t.Fatal(err)
	}
	var order ConfiguredOrder
	json.Unmarshal(out.Data, &order)
	configuredPublishDraw(t, c, 7)
	if _, err = f.service.ProcessConfiguredSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if wait := time.Until(boundary); wait > 0 {
		time.Sleep(wait)
	}
	var cycle commission.Cycle
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		cycle, e = finance.CreateCycleTx(ctx, tx, f.brand, actor, commission.CreateCycleInput{AnchorOrderID: order.ID}, points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()})
		return e
	})
	for i := 0; i < 10; i++ {
		if _, err = finance.ProcessCycles(ctx, 20); err != nil {
			t.Fatal(err)
		}
	}
	var ready bool
	if err = f.db.QueryRow(ctx, `SELECT commission_payment_evidence_current(brand_id,id,current_run_id,evidence_epoch) AND commission_analysis_run_valid(brand_id,id,current_run_id) FROM commission_cycles WHERE id=$1`, cycle.ID).Scan(&ready); err != nil || !ready {
		t.Fatal("configured cycle proof", ready, err)
	}
}

func TestCommissionConfiguredAttributedLossPaysOnce(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	ctx := context.Background()
	a := access.Account{ID: f.adminID, Type: access.AccountAdmin, BrandIDs: []string{f.brand}, Roles: []access.Role{{BrandID: f.brand}}}
	for _, v := range []struct{ r, a string }{{"agent_policy", "write"}, {"agent", "write"}, {"join_code", "write"}, {"commission_policy", "write"}, {"commission", "view"}, {"commission", "run"}, {"commission_payment_policy", "write"}} {
		a.Roles[0].Permissions = append(a.Roles[0].Permissions, access.Permission{Resource: v.r, Action: v.a, Scope: access.ScopeBrand})
	}
	agencyService := agency.Service{DB: f.db}
	var policy agency.Policy
	var node agency.Node
	var code attribution.Code
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		policy, e = agencyService.SavePolicy(ctx, tx, f.brand, a, agency.PolicyInput{Version: 1, Config: agency.PolicyConfig{Enabled: true, MaxDepth: 3, LossRatioCap: "0.3", TurnoverRatioCap: "0.3", Mode: "loss", Cycle: "weekly"}}, points.Metadata{RequestID: ids.New()})
		return e
	})
	mode := "loss"
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		node, e = agencyService.Create(ctx, tx, f.brand, a, agency.CreateInput{PolicyVersion: policy.Version, Username: testdb.AgentUsername(f.member), Password: "agent123", Config: agency.NodeConfig{Mode: &mode, Ratio: "0.3", Status: "active", CanCreateChildren: true}}, points.Metadata{RequestID: ids.New()})
		return e
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		code, e = (attribution.Service{DB: f.db}).Create(ctx, tx, f.brand, a, attribution.CreateInput{Kind: "agent", OwnerMemberID: node.MemberID, AgentID: &node.ID}, points.Metadata{RequestID: ids.New()})
		return e
	})
	users, e := identity.New(f.db)
	if e != nil {
		t.Fatal(e)
	}
	var auth identity.Authentication
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		r, e := users.Register(ctx, tx, f.brand, identity.RegisterInput{Username: "configured_finance_user", Password: "configured-finance-password", AgentCode: code.Code}, identity.Metadata{Domain: "aurora.localhost"})
		if e != nil {
			return e
		}
		if r.Status != 201 {
			t.Fatal(r)
		}
		return json.Unmarshal(r.Data, &auth)
	})
	f.user, e = users.Authenticate(ctx, f.brand, auth.AccessToken)
	if e != nil {
		t.Fatal(e)
	}
	f.member = auth.Member.ID
	f.token = auth.AccessToken
	raw, e := os.ReadFile("../rules/config/examples/lucky_group_preview.json")
	if e != nil {
		t.Fatal(e)
	}
	c := configuredFixtureFrom(t, raw, []platformlottery.ConfiguredCase{{Requests: []rules.ConfiguredRequest{{PlayID: "operator_new_play", Options: []string{"lucky"}, Plate: "A", Points: 1000, Multiplier: 1}}, Draw: rules.Draw{Regular: []int{1, 3, 7, 4, 5, 6}, Special: []int{2}}, Bet: 1000, Prize: 2500}}, f)
	finance := commission.Service{DB: f.db}
	boundary := time.Now().UTC().Truncate(time.Second).Add(10 * time.Second)
	weekday := int(boundary.Weekday())
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := finance.Update(ctx, tx, f.brand, a, commission.PolicyInput{Version: 1, Config: commission.PolicyConfig{Enabled: true, Calendar: &commission.Calendar{Timezone: "UTC", Cycle: "weekly", BoundaryTime: boundary.Format("15:04:05"), Weekday: &weekday}, PayoutMode: commission.PayoutAutomatic}}, points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()})
		return e
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := finance.UpdatePaymentPolicyTx(ctx, tx, f.brand, a, commission.PaymentPolicyInput{Version: 1, Enabled: true}, points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()})
		return e
	})
	fundBettingWallet(t, f, 10000)
	out, e := configuredPlace(t, c, ids.New(), c.input)
	if e != nil {
		t.Fatal(e)
	}
	var order ConfiguredOrder
	json.Unmarshal(out.Data, &order)
	configuredPublishDraw(t, c, 8)
	if _, e = f.service.ProcessConfiguredSettlements(ctx, 20); e != nil {
		t.Fatal(e)
	}
	if wait := time.Until(boundary); wait > 0 {
		time.Sleep(wait)
	}
	var cycle commission.Cycle
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		cycle, e = finance.CreateCycleTx(ctx, tx, f.brand, a, commission.CreateCycleInput{AnchorOrderID: order.ID}, points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()})
		return e
	})
	for i := 0; i < 10; i++ {
		if _, e = finance.ProcessCycles(ctx, 20); e != nil {
			t.Fatal(e)
		}
		if _, e = finance.ProcessPayments(ctx, 20); e != nil {
			t.Fatal(e)
		}
	}
	var credits, total string
	if e = f.db.QueryRow(ctx, `SELECT count(*)::text,coalesce(sum((l.delta_snapshot->'commission'->>'available')::bigint),0)::text FROM point_ledger_entries l JOIN commission_payment_targets t ON t.ledger_entry_id=l.id JOIN commission_payments p ON p.id=t.payment_id WHERE p.cycle_id=$1`, cycle.ID).Scan(&credits, &total); e != nil || credits != "1" || total != "3" {
		var debug []byte
		f.db.QueryRow(ctx, `SELECT jsonb_build_object('cycle',(SELECT to_jsonb(c) FROM commission_cycles c WHERE id=$1),'calculations',(SELECT jsonb_agg(to_jsonb(x)) FROM commission_calculations x WHERE cycle_id=$1),'payments',(SELECT jsonb_agg(to_jsonb(p)) FROM commission_payments p WHERE cycle_id=$1))`, cycle.ID).Scan(&debug)
		t.Fatal("commission credit", credits, total, e, string(debug))
	}
}
