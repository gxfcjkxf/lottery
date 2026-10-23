package platformlottery

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"testing"
	"time"
)

func TestGameLifecycleDeletionAvailabilityAndIsolation(t *testing.T) {
	db := testdb.New(t)
	s := Service{DB: db}
	ctx := context.Background()
	brand := "0199a000-0000-7000-8000-000000000001"
	other := "0199a000-0000-7000-8000-000000000002"
	a := access.Account{ID: ids.New(), Type: access.AccountAdmin, SuperAdmin: true, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "platform_game", Action: "write", Scope: access.ScopePlatform}}}}}
	if _, e := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test')`, a.ID, "life_"+a.ID); e != nil {
		t.Fatal(e)
	}
	b := access.Account{ID: a.ID, Type: access.AccountAdmin, BrandIDs: []string{brand}, Roles: []access.Role{{BrandID: brand, Permissions: []access.Permission{{Resource: "platform_game", Action: "sell", Scope: access.ScopeBrand}, {Resource: "platform_game", Action: "connect", Scope: access.ScopeBrand}}}}}
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	g, e := s.Create(ctx, tx, a, GameInput{Code: "delete_trial", Name: "Delete trial", Model: rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true}, Timezone: "UTC"}, points.Metadata{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SetLifecycle(ctx, tx, b, g.ID, LifecycleInput{ExpectedVersion: 1, Status: "deleted", Reason: "test"}, points.Metadata{}); !errors.Is(e, ErrDenied) {
		t.Fatal("brand deleted shared game", e)
	}
	if _, e = s.SetLifecycle(ctx, tx, a, g.ID, LifecycleInput{ExpectedVersion: 2, Status: "deleted", Reason: "test"}, points.Metadata{}); !errors.Is(e, ErrVersionConflict) {
		t.Fatal("stale delete", e)
	}
	if _, e = s.SetLifecycle(ctx, tx, a, g.ID, LifecycleInput{ExpectedVersion: 1, Status: "deleted", Reason: "test"}, points.Metadata{}); e != nil {
		t.Fatal(e)
	}
	if _, e = readGame(ctx, tx, g.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("deleted game still reachable", e)
	}
	g, e = s.Create(ctx, tx, a, GameInput{Code: "offline_trial", Name: "Offline trial", Model: rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true}, Timezone: "UTC"}, points.Metadata{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Connect(ctx, tx, b, brand, g.ID, "test", points.Metadata{}); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `INSERT INTO brand_platform_games(brand_id,game_id,connected_by) VALUES($1,$2,$3)`, other, g.ID, a.ID); e != nil {
		t.Fatal(e)
	}
	p, e := s.CreatePeriod(ctx, tx, a, g.ID, PeriodInput{PeriodNo: "1", BetStartAt: time.Now().Add(-time.Hour), BetEndAt: time.Now().Add(time.Hour), DrawAt: time.Now().Add(time.Hour)}, points.Metadata{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SetLifecycle(ctx, tx, a, g.ID, LifecycleInput{ExpectedVersion: 1, Status: "deleted", Reason: "test"}, points.Metadata{}); !errors.Is(e, ErrGameInUse) {
		t.Fatal("used game deleted", e)
	}
	no := false
	yes := true
	if _, e = s.SetAvailability(ctx, tx, b, other, g.ID, AvailabilityInput{ExpectedVersion: 1, Enabled: &no, Reason: "test"}, points.Metadata{}); !errors.Is(e, ErrDenied) {
		t.Fatal("brand isolation", e)
	}
	if _, e = s.SetAvailability(ctx, tx, b, brand, g.ID, AvailabilityInput{ExpectedVersion: 1, Enabled: &no, Reason: "test"}, points.Metadata{}); e != nil {
		t.Fatal(e)
	}
	gate := func(brand string, want bool) {
		t.Helper()
		var got bool
		if e := tx.QueryRow(ctx, `SELECT shared_game_accepts($1,$2)`, brand, g.ID).Scan(&got); e != nil || got != want {
			t.Fatal("admission", brand, got, want, e)
		}
	}
	gate(brand, false)
	gate(other, true)
	if _, e = s.SetAvailability(ctx, tx, b, brand, g.ID, AvailabilityInput{ExpectedVersion: 1, Enabled: &yes, Reason: "test"}, points.Metadata{}); !errors.Is(e, ErrVersionConflict) {
		t.Fatal("stale brand update", e)
	}
	if _, e = s.SetAvailability(ctx, tx, b, brand, g.ID, AvailabilityInput{ExpectedVersion: 2, Enabled: &yes, Reason: "test"}, points.Metadata{}); e != nil {
		t.Fatal(e)
	}
	gate(brand, true)
	if _, e = s.SetLifecycle(ctx, tx, a, g.ID, LifecycleInput{ExpectedVersion: 1, Status: "offline", Reason: "test"}, points.Metadata{}); e != nil {
		t.Fatal(e)
	}
	gate(brand, false)
	gate(other, false)
	var count int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM platform_game_periods WHERE id=$1`, p.ID).Scan(&count); e != nil || count != 1 {
		t.Fatal("period history removed", e)
	}
	if _, e = s.SetLifecycle(ctx, tx, a, g.ID, LifecycleInput{ExpectedVersion: 2, Status: "active", Reason: "test"}, points.Metadata{}); e != nil {
		t.Fatal(e)
	}
	gate(brand, true)
	// Completed periods without orders no longer prevent catalogue deletion.
	completed, e := s.Create(ctx, tx, a, GameInput{Code: "completed_trial", Name: "Completed trial", Model: rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true}, Timezone: "UTC"}, points.Metadata{})
	if e != nil {
		t.Fatal(e)
	}
	past, e := s.CreatePeriod(ctx, tx, a, completed.ID, PeriodInput{PeriodNo: "completed-1", BetStartAt: time.Now().Add(-3 * time.Hour), BetEndAt: time.Now().Add(-2 * time.Hour), DrawAt: time.Now().Add(-time.Hour)}, points.Metadata{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Publish(ctx, tx, a, past.ID, rules.Draw{Digits: []int{1, 2, 3}}, "", points.Metadata{}); e != nil {
		t.Fatal(e)
	}
	eligible, e := readGame(ctx, tx, completed.ID)
	if e != nil || !eligible.Deletable {
		t.Fatal("completed period blocks deletion", eligible, e)
	}
	if _, e = s.SetLifecycle(ctx, tx, a, completed.ID, LifecycleInput{ExpectedVersion: 1, Status: "deleted"}, points.Metadata{}); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	history, e := s.Detail(ctx, "", completed.ID, 50, 0)
	if e != nil || history.Game.Status != "deleted" || len(history.Periods) != 1 || history.Periods[0].Result == nil {
		t.Fatal("deleted game history unavailable", history, e)
	}
	if _, e = s.Detail(ctx, other, completed.ID, 50, 0); !errors.Is(e, ErrNotFound) {
		t.Fatal("foreign history exposed", e)
	}
}
