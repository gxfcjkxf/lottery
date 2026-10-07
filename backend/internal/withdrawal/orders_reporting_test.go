package withdrawal

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
)

func TestWithdrawalReportsRealOrdersExactLargeSumsAndCurrentProjection(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	g := f
	g.user, g.member = ids.New(), ids.New()
	g.actor = points.Metadata{ActorType: "user", ActorID: g.user, RequestID: ids.New()}
	ctx := context.Background()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO global_users(id,username) VALUES($1,$2)`, []any{g.user, "report_" + g.user}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','dev-1','dev-1')`, []any{g.member, orderTestBrand, g.user}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{ids.New(), orderTestBrand, g.member}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,id,s,t FROM point_accounts CROSS JOIN unnest(ARRAY['recharge','winning','gift']) s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) t WHERE brand_id=$1 AND brand_member_id=$2`, []any{orderTestBrand, g.member}},
	} {
		if _, err := f.db.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	const amount points.Amount = 6000000000000000000
	f.fund(t, oneRechargeAllocation(amount))
	g.fund(t, oneRechargeAllocation(amount))
	a := createOrder(t, f, amount, "large-report-a", oneRechargeAllocation(amount))
	b := createOrder(t, g, amount, "large-report-b", oneRechargeAllocation(amount))
	meta := points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()}
	if _, err := orderAction(t, f, a.ID, "approve", "report-approve", f.admin, ActionInput{Version: 1, ClientKey: "report-approve", Reason: "review"}, meta); err != nil {
		t.Fatal(err)
	}
	if _, err := orderAction(t, f, a.ID, "mark_paid", "report-paid", f.admin, ActionInput{Version: 2, ClientKey: "report-paid", Reason: "internal"}, meta); err != nil {
		t.Fatal(err)
	}
	s := reporting.Service{DB: f.db}
	q := reporting.Query{From: a.CreatedAt.Add(-time.Nanosecond), To: b.CreatedAt.Add(time.Nanosecond), GroupBy: "state", Limit: 20}
	r, err := s.Withdrawal(ctx, orderTestBrand, q)
	if err != nil || r.Summary.OrderCount != "2" || r.Summary.RequestedPoints != "12000000000000000000" || r.Summary.PaidPoints != "6000000000000000000" || r.Summary.ReviewingPoints != "6000000000000000000" || r.TotalGroups != "2" {
		t.Fatal(r, err)
	}
	q.Offset = 1
	page, err := s.Withdrawal(ctx, orderTestBrand, q)
	if err != nil || len(page.Items) != 1 || page.Summary != r.Summary {
		t.Fatal(page, err)
	}
	q.Offset = 20
	empty, err := s.Withdrawal(ctx, orderTestBrand, q)
	if err != nil || len(empty.Items) != 0 || empty.Summary != r.Summary {
		t.Fatal(empty, err)
	}
	q.Offset = 0
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	export, err := s.WithdrawalExport(ctx, tx, orderTestBrand, q)
	if err != nil {
		t.Fatal(err)
	}
	csv, err := reporting.WithdrawalCSV(export)
	if err != nil || !strings.Contains(string(csv), "12000000000000000000") {
		t.Fatal("CSV lost exact total", err)
	}
	_ = tx.Rollback(ctx)
	if _, err := orderAction(t, f, b.ID, "reject", "report-reject", f.admin, ActionInput{Version: 1, ClientKey: "report-reject", Reason: "review"}, meta); err != nil {
		t.Fatal(err)
	}
	r, err = s.Withdrawal(ctx, orderTestBrand, q)
	if err != nil || r.Summary.RejectedPoints != "6000000000000000000" || r.Summary.ReviewingCount != "0" || r.Summary.RequestedPoints != "12000000000000000000" {
		t.Fatal(r, err)
	}
	// Submitted-time grouping remains unchanged by a later terminal transition.
	q.MemberID = &g.member
	if _, err = s.Withdrawal(ctx, orderOtherBrand, q); !errors.Is(err, reporting.ErrNotFound) {
		t.Fatal("foreign member broadened scope", err)
	}
}
