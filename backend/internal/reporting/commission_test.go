package reporting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestCommissionQueryRequiresPostingWindowAndClosedGroups(t *testing.T) {
	now := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	query := CommissionQuery{From: now.Add(-24 * time.Hour), To: now, GroupBy: "day", Limit: 20}
	if query.Validate() != nil {
		t.Fatal("valid posting-window query rejected")
	}
	for _, change := range []func(*CommissionQuery){
		func(q *CommissionQuery) { q.From = time.Time{} }, func(q *CommissionQuery) { q.To = q.From },
		func(q *CommissionQuery) { q.To = q.From.Add(93*24*time.Hour + time.Nanosecond) },
		func(q *CommissionQuery) { q.GroupBy = "game" }, func(q *CommissionQuery) { q.GroupBy = "member" },
		func(q *CommissionQuery) { q.Limit = 0 }, func(q *CommissionQuery) { q.Limit = 101 },
		func(q *CommissionQuery) { q.Offset = -1 }, func(q *CommissionQuery) { q.Offset = 1000001 },
		func(q *CommissionQuery) { v := "not-a-cycle"; q.CycleID = &v },
	} {
		q := query
		change(&q)
		if !errors.Is(q.Validate(), ErrInvalid) {
			t.Fatalf("invalid query accepted: %+v", q)
		}
	}
}

func TestCommissionEmptyReportIsReadOnlyExactAndDoesNotRequirePayoutGate(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	brand := "0199a000-0000-7000-8000-000000000001"
	s := Service{DB: db}
	now := time.Now().UTC()
	q := CommissionQuery{From: now.Add(-24 * time.Hour), To: now, GroupBy: "agent", Limit: 20}
	out, err := s.Commission(ctx, brand, q)
	if err != nil {
		t.Fatal(err)
	}
	if out.BrandID != brand || out.SnapshotAt.IsZero() || out.Timezone == "" || out.TotalGroups != "0" || len(out.Items) != 0 || out.Items == nil || out.Summary.EntryCount != "0" || out.Summary.NetPoints != "0" {
		t.Fatalf("empty report=%+v", out)
	}
	var enabled bool
	var entries, roles, audits int
	if err = db.QueryRow(ctx, `SELECT enabled FROM brand_commission_payment_policies WHERE brand_id=$1`, brand).Scan(&enabled); err != nil || enabled {
		t.Fatalf("read enabled financial gate: %t %v", enabled, err)
	}
	if err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM point_ledger_entries),(SELECT count(*) FROM commission_payments),(SELECT count(*) FROM audit_logs)`).Scan(&entries, &roles, &audits); err != nil || entries != 0 || roles != 0 || audits != 0 {
		t.Fatalf("empty report wrote records: %d %d %d %v", entries, roles, audits, err)
	}
	foreign := "ffffffff-ffff-4fff-8fff-ffffffffffff"
	q.AgentID = &foreign
	if _, err = s.Commission(ctx, brand, q); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign filter=%v", err)
	}
}
