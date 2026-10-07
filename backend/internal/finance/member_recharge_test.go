package finance

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func TestMemberRechargeProjectionExactPrivateAndReadOnly(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	create := func(amount points.Amount) Recharge {
		t.Helper()
		tx := financeTx(t, f.pool)
		v, e := f.service.CreateRecharge(ctx, tx, f.brandID, f.memberID, amount, "confidential-proof", "staff-only-remark", "staff-only-reason", financeMeta(f.adminID))
		if e != nil {
			t.Fatal(e)
		}
		commitFinanceTx(t, tx)
		return v
	}
	pending := create(9007199254740993)
	confirmed := create(7)
	cancelled := create(11)
	tx := financeTx(t, f.pool)
	var e error
	confirmed, e = f.service.ConfirmRecharge(ctx, tx, f.brandID, confirmed.ID, confirmed.Version, "private-confirmation", financeMeta(f.adminID))
	if e != nil {
		t.Fatal(e)
	}
	commitFinanceTx(t, tx)
	tx = financeTx(t, f.pool)
	cancelled, e = f.service.CancelRecharge(ctx, tx, f.brandID, cancelled.ID, cancelled.Version, "private-cancel", financeMeta(f.adminID))
	if e != nil {
		t.Fatal(e)
	}
	commitFinanceTx(t, tx)
	fingerprint := func() string {
		var v string
		e := f.pool.QueryRow(ctx, `SELECT md5(jsonb_build_object('orders',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM recharge_orders r),'wallets',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM point_accounts a),'buckets',(SELECT jsonb_agg(to_jsonb(b) ORDER BY account_id,source,state) FROM point_buckets b),'ledger',(SELECT jsonb_agg(to_jsonb(l) ORDER BY id) FROM point_ledger_entries l),'audit',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM audit_logs a))::text)`).Scan(&v)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	before := fingerprint()
	tx = financeTx(t, f.pool)
	page, e := f.service.ListMemberRechargesTx(ctx, tx, f.brandID, f.memberID, "", 2, 0)
	if e != nil {
		t.Fatal(e)
	}
	if page.BrandID != f.brandID || page.MemberID != f.memberID || page.TotalCount != "3" || page.State != nil || page.Limit != 2 || page.Offset != 0 || len(page.Items) != 2 || page.SnapshotAt.IsZero() {
		t.Fatalf("unexpected page %+v", page)
	}
	last, e := f.service.ListMemberRechargesTx(ctx, tx, f.brandID, f.memberID, "", 2, 2)
	if e != nil {
		t.Fatal(e)
	}
	if len(last.Items) != 1 || last.Items[0].ID != pending.ID || last.Items[0].Points != "9007199254740993" {
		t.Fatalf("large pending points lost: %+v", last)
	}
	for _, want := range []Recharge{pending, confirmed, cancelled} {
		v, e := f.service.ReadMemberRechargeTx(ctx, tx, f.brandID, f.memberID, want.ID)
		if e != nil {
			t.Fatal(e)
		}
		if v.ID != want.ID || v.State != want.State || v.Points != want.Points {
			t.Fatalf("wrong projection %+v", v)
		}
		if v.State == "confirmed" {
			if v.ConfirmedAt == nil || v.LedgerEntryID == nil || *v.LedgerEntryID != confirmed.LedgerEntryID || v.Version != "2" {
				t.Fatalf("missing confirmation %+v", v)
			}
		} else if v.ConfirmedAt != nil || v.LedgerEntryID != nil {
			t.Fatalf("nonconfirmed credit %+v", v)
		}
		raw, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		var shape map[string]any
		if e = json.Unmarshal(raw, &shape); e != nil {
			t.Fatal(e)
		}
		if len(shape) != 9 {
			t.Fatalf("private fields in DTO %s", raw)
		}
		for _, key := range []string{"id", "brand_id", "member_id", "points", "state", "version", "created_at", "confirmed_at", "ledger_entry_id"} {
			if _, ok := shape[key]; !ok {
				t.Fatalf("missing field %s", key)
			}
		}
		filtered, e := f.service.ListMemberRechargesTx(ctx, tx, f.brandID, f.memberID, v.State, 20, 0)
		if e != nil {
			t.Fatal(e)
		}
		if filtered.State == nil || *filtered.State != v.State || filtered.TotalCount != "1" || len(filtered.Items) != 1 || !reflect.DeepEqual(filtered.Items[0], v) {
			t.Fatalf("filter mismatch %+v", filtered)
		}
	}
	commitFinanceTx(t, tx)
	if got := fingerprint(); got != before {
		t.Fatal("member reads mutated money, orders or audit")
	}
}

func TestMemberRechargeScopeQueriesAndEmptyPages(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	tx := financeTx(t, f.pool)
	empty, e := f.service.ListMemberRechargesTx(ctx, tx, f.brandID, f.memberID, "pending", 20, 40)
	if e != nil {
		t.Fatal(e)
	}
	if empty.TotalCount != "0" || empty.Items == nil || len(empty.Items) != 0 || empty.Offset != 40 {
		t.Fatalf("bad empty %+v", empty)
	}
	for _, query := range []struct {
		state         string
		limit, offset int
	}{{"wrong", 20, 0}, {"", 0, 0}, {"", 101, 0}, {"", 20, -1}, {"", 20, 1000001}} {
		if _, e := f.service.ListMemberRechargesTx(ctx, tx, f.brandID, f.memberID, query.state, query.limit, query.offset); !errors.Is(e, points.ErrInvalid) {
			t.Fatalf("accepted query %+v %v", query, e)
		}
	}
	for _, scope := range [][2]string{{f.otherBrand, f.memberID}, {f.brandID, ids.New()}} {
		if _, e := f.service.ListMemberRechargesTx(ctx, tx, scope[0], scope[1], "", 20, 0); !errors.Is(e, points.ErrNotFound) {
			t.Fatalf("foreign list %v", e)
		}
	}
	if _, e := f.service.ReadMemberRechargeTx(ctx, tx, f.brandID, f.memberID, ids.New()); !errors.Is(e, points.ErrNotFound) {
		t.Fatal(e)
	}
	if _, e := f.service.ReadMemberRechargeTx(ctx, tx, f.brandID, f.memberID, "not-a-uuid"); !errors.Is(e, points.ErrInvalid) {
		t.Fatal(e)
	}
	if _, e := f.service.ListMemberRechargesTx(ctx, nil, f.brandID, f.memberID, "", 20, 0); !errors.Is(e, points.ErrInvalid) {
		t.Fatal(e)
	}
}

func TestMemberRechargeStableOrderingAndForeignRecordIsolation(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	tx := financeTx(t, f.pool)
	var orderedIDs []string
	for i := 0; i < 4; i++ {
		r, err := f.service.CreateRecharge(ctx, tx, f.brandID, f.memberID, 1, "private", "private", "same-transaction ordering", financeMeta(f.adminID))
		if err != nil {
			t.Fatal(err)
		}
		orderedIDs = append(orderedIDs, r.ID)
	}
	commitFinanceTx(t, tx)
	sort.Sort(sort.Reverse(sort.StringSlice(orderedIDs)))
	foreignMember, foreignUser, foreignAccount := ids.New(), ids.New(), ids.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'not-a-login-secret');`, foreignUser, "foreign_finance_"+foreignUser); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,display_name,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'Other member','operator','1','1')`, foreignMember, f.brandID, foreignUser); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, foreignAccount, f.brandID, foreignMember); err != nil {
		t.Fatal(err)
	}
	tx = financeTx(t, f.pool)
	foreign, err := f.service.CreateRecharge(ctx, tx, f.brandID, foreignMember, 3, "foreign proof", "foreign remark", "foreign fixture", financeMeta(f.adminID))
	if err != nil {
		t.Fatal(err)
	}
	commitFinanceTx(t, tx)
	tx = financeTx(t, f.pool)
	var gotIDs []string
	for _, offset := range []int{0, 2} {
		page, err := f.service.ListMemberRechargesTx(ctx, tx, strings.ToUpper(f.brandID), strings.ToUpper(f.memberID), "", 2, offset)
		if err != nil {
			t.Fatal(err)
		}
		if page.TotalCount != "4" {
			t.Fatal("foreign order affected member count")
		}
		for _, item := range page.Items {
			gotIDs = append(gotIDs, item.ID)
		}
	}
	if !reflect.DeepEqual(gotIDs, orderedIDs) {
		t.Fatalf("unstable tied-time pagination %v != %v", gotIDs, orderedIDs)
	}
	if _, err := f.service.ReadMemberRechargeTx(ctx, tx, f.brandID, f.memberID, foreign.ID); !errors.Is(err, points.ErrNotFound) {
		t.Fatalf("foreign record visible: %v", err)
	}
	page, err := f.service.ListMemberRechargesTx(ctx, tx, f.brandID, f.memberID, "", 20, 100)
	if err != nil || len(page.Items) != 0 || page.TotalCount != "4" {
		t.Fatalf("empty page beyond last group: %+v %v", page, err)
	}
}

func TestMemberRechargeNormalizationFailsClosed(t *testing.T) {
	created := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	valid := MemberRecharge{ID: ids.New(), BrandID: financeBrandA, MemberID: ids.New(), Points: "1", State: "pending", Version: "1", CreatedAt: created}
	for name, mutate := range map[string]func(*MemberRecharge){
		"amount syntax":             func(v *MemberRecharge) { v.Points = "01" },
		"amount overflow":           func(v *MemberRecharge) { v.Points = "9223372036854775808" },
		"version syntax":            func(v *MemberRecharge) { v.Version = "+1" },
		"version overflow":          func(v *MemberRecharge) { v.Version = "9223372036854775808" },
		"wrong state":               func(v *MemberRecharge) { v.State = "paid" },
		"zero created":              func(v *MemberRecharge) { v.CreatedAt = time.Time{} },
		"confirmation absent":       func(v *MemberRecharge) { v.State = "confirmed" },
		"pending with confirmation": func(v *MemberRecharge) { v.ConfirmedAt = &created },
		"foreign identity":          func(v *MemberRecharge) { v.MemberID = ids.New() },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := normalizeMemberRecharge(&candidate, valid.BrandID, valid.MemberID); !errors.Is(err, points.ErrCorrupt) {
				t.Fatal("accepted malformed projection", err)
			}
		})
	}
}
