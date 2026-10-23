package platformlottery

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"reflect"
	"testing"
	"time"
)

func TestPlatformAuthorityDoesNotInheritBrandGrants(t *testing.T) {
	brand := "0199a000-0000-7000-8000-000000000001"
	a := access.Account{Type: access.AccountAdmin, BrandIDs: []string{brand}, Roles: []access.Role{{BrandID: brand, Permissions: []access.Permission{{Resource: "platform_game", Action: "connect", Scope: access.ScopeBrand}, {Resource: "platform_game", Action: "write", Scope: access.ScopeBrand}}}}}
	if !Allowed(a, brand, "connect") || Allowed(a, "", "write") || Allowed(a, "0199a000-0000-7000-8000-000000000002", "connect") {
		t.Fatal("brand authority leaked")
	}
	a.SuperAdmin = true
	if Allowed(a, brand, "connect") {
		t.Fatal("platform account must not connect on behalf of a brand")
	}
}

func TestPlatformModelWithPlaysCannotBeOverwritten(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	s := Service{DB: db}
	a := access.Account{ID: ids.New(), Type: access.AccountAdmin, SuperAdmin: true, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "platform_game", Action: "write", Scope: access.ScopePlatform}}}}}
	if _, e := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, a.ID, "model_"+a.ID); e != nil {
		t.Fatal(e)
	}
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	g, e := s.Create(ctx, tx, a, GameInput{Code: "model_guard", Name: "Model guard", Model: rules.Model{Type: "DIGITS_0_9", Length: 3, Ordered: true}, Timezone: "UTC", Reason: "test"}, points.Metadata{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `INSERT INTO platform_game_plays(id,game_id,code,name,definition,definition_hash,created_by) VALUES($1,$2,'existing_play','Existing play','{}','test',$3)`, ids.New(), g.ID, a.ID); e != nil {
		t.Fatal(e)
	}
	_, e = s.UpdateModel(ctx, tx, a, g.ID, ModelInput{Model: rules.Model{Type: "DIGITS_0_9", Length: 4, Ordered: true}, ExpectedVersion: 1, Reason: "edit"}, points.Metadata{})
	if !errors.Is(e, ErrModelLocked) {
		t.Fatalf("used model overwrite: %v", e)
	}
	after, e := readGame(ctx, tx, g.ID)
	if e != nil {
		t.Fatal(e)
	}
	if after.Version != 1 || after.Model.Length != 3 {
		t.Fatal("used model changed")
	}
}

func TestTwoBrandsReadTheSamePeriodAndResult(t *testing.T) {
	db := testdb.New(t)
	s := Service{DB: db}
	ctx := context.Background()
	brands := []string{"0199a000-0000-7000-8000-000000000001", "0199a000-0000-7000-8000-000000000002"}
	a := access.Account{ID: ids.New(), Type: access.AccountAdmin, SuperAdmin: true, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "platform_game", Action: "write", Scope: access.ScopePlatform}}}}}
	b := access.Account{ID: ids.New(), Type: access.AccountAdmin, BrandIDs: brands, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "platform_game", Action: "connect", Scope: access.ScopeBrand}}}}}
	for _, x := range []access.Account{a, b} {
		if _, e := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, x.ID, "platform_"+x.ID); e != nil {
			t.Fatal(e)
		}
	}
	transact := func(fn func(pgx.Tx) error) {
		t.Helper()
		tx, e := db.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		if e = fn(tx); e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
	}
	var g Game
	var p Period
	transact(func(tx pgx.Tx) error {
		var e error
		g, e = s.Create(ctx, tx, a, GameInput{Code: "shared_digits", Name: "Shared digits", Model: rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true}, Timezone: "UTC"}, points.Metadata{})
		return e
	})
	if _, e := s.Detail(ctx, brands[0], g.ID, 50, 0); !errors.Is(e, ErrNotFound) {
		t.Fatalf("unconnected read: %v", e)
	}
	for _, brand := range brands {
		transact(func(tx pgx.Tx) error {
			_, e := s.Connect(ctx, tx, b, brand, g.ID, "test connection", points.Metadata{})
			return e
		})
	}
	now := time.Now().UTC()
	transact(func(tx pgx.Tx) error {
		var e error
		p, e = s.CreatePeriod(ctx, tx, a, g.ID, PeriodInput{PeriodNo: "shared-001", BetStartAt: now.Add(-3 * time.Hour), BetEndAt: now.Add(-2 * time.Hour), DrawAt: now.Add(-time.Hour)}, points.Metadata{})
		return e
	})
	transact(func(tx pgx.Tx) error {
		var e error
		p, e = s.Publish(ctx, tx, a, p.ID, rules.Draw{Digits: []int{1, 2, 3}}, "shared result", points.Metadata{})
		return e
	})
	one, e := s.Detail(ctx, brands[0], g.ID, 50, 0)
	if e != nil {
		t.Fatal(e)
	}
	two, e := s.Detail(ctx, brands[1], g.ID, 50, 0)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(one.Periods, two.Periods) || len(one.Periods) != 1 || one.Periods[0].ID != p.ID || one.Periods[0].Result == nil {
		t.Fatalf("brands did not share canonical records: %#v %#v", one, two)
	}
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = s.Publish(ctx, tx, b, p.ID, rules.Draw{Digits: []int{9, 9, 9}}, "brand overwrite", points.Metadata{}); !errors.Is(e, ErrDenied) {
		t.Fatalf("brand write: %v", e)
	}
	if _, e = s.Publish(ctx, tx, a, p.ID, rules.Draw{Digits: []int{9, 9, 9}}, "duplicate result", points.Metadata{}); !errors.Is(e, ErrConflict) {
		t.Fatalf("duplicate publish: %v", e)
	}
	if _, e = s.Connect(ctx, tx, b, brands[0], g.ID, "duplicate connect", points.Metadata{}); !errors.Is(e, ErrConflict) {
		t.Fatalf("duplicate connection: %v", e)
	}
	if _, e = tx.Exec(ctx, `UPDATE platform_game_periods SET result='{"digits":[9,9,9]}' WHERE id=$1`, p.ID); e == nil {
		t.Fatal("database allowed published result rewrite")
	}
}
