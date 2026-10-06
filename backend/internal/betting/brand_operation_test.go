package betting

import (
	"context"
	"errors"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/brandops"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func setBrandOperationForBetting(t *testing.T, f bettingFixture, status string) {
	t.Helper()
	ctx := context.Background()
	s := brandops.Service{DB: f.db}
	current, err := s.Read(ctx, f.brand)
	if err != nil {
		t.Fatal(err)
	}
	actor := access.Account{ID: f.version.CreatedBy, Type: access.AccountAdmin, BrandIDs: []string{f.brand}, Roles: []access.Role{{BrandID: f.brand, Permissions: []access.Permission{{Resource: "brand_operation", Action: "write", Scope: access.ScopeBrand}}}}}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := s.Update(ctx, tx, f.brand, actor, brandops.Input{Version: current.Version, Status: status, Reason: "integration operational state"}, points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New(), Reason: "integration operational state"})
		return e
	})
}

func TestBrandOperationPauseBlocksNewBetsKeepsSessionAndOriginalRefund(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	ctx := context.Background()
	fundBettingWallet(t, f, 10, 10, 10)
	setUserCancellation(t, f, true)
	f.input.PolicyVersions = ptrPolicyVersions(readBettingPolicyVersions(t, f.service, f.brand, f.game.ID))
	in := f.input
	in.Multiplier = 25
	before := walletBySource(t, f)
	order, err := placeBettingOrder(t, f, in, "brand-operation-existing-bet")
	if err != nil {
		t.Fatal(err)
	}
	setBrandOperationForBetting(t, f, "paused")
	pausedBalance := walletBySource(t, f)
	if _, err = placeBettingOrder(t, f, f.input, "brand-operation-paused-new-bet"); !errors.Is(err, ErrDenied) {
		t.Fatal("paused bet admitted", err)
	}
	if walletBySource(t, f) != pausedBalance {
		t.Fatal("rejected paused bet changed balance")
	}
	u, err := identity.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = u.Authenticate(ctx, f.brand, f.token); err != nil {
		t.Fatal("pause revoked user session", err)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		cancelled, e := f.service.Cancel(ctx, tx, f.brand, f.user, order.ID, order.Version, "cancel existing bet while paused", points.Metadata{RequestID: ids.New()})
		if e == nil && cancelled.Status != "bet_cancelled" {
			t.Fatal(cancelled)
		}
		return e
	})
	if walletBySource(t, f) != before {
		t.Fatal("pause prevented exact source refund", before, walletBySource(t, f))
	}
	var other string
	if err := f.db.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1`, storeTestOtherBrand).Scan(&other); err != nil || other != "active" {
		t.Fatal("other brand changed", other, err)
	}
	setBrandOperationForBetting(t, f, "active")
	if _, err = placeBettingOrder(t, f, f.input, "brand-operation-resumed-new-bet"); err != nil {
		t.Fatal("resumed bet rejected", err)
	}
	var entries int
	if err := f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='bet'`).Scan(&entries); err != nil || entries != 2 {
		t.Fatal("paused attempt posted debit", entries, err)
	}
}

func TestBrandOperationPauseDoesNotStopExistingDrawnSettlement(t *testing.T) {
	f, order := drawnFixtureOrder(t)
	ctx := context.Background()
	setBrandOperationForBetting(t, f, "paused")
	actor := settlementActor(f)
	automatic := "automatic"
	policy := setSettlementMode(t, f, actor, 1, &automatic)
	job := startFixtureSettlement(t, f, actor, policy.Version)
	if n, err := f.service.ProcessSettlements(ctx, 20); err != nil || n == 0 {
		t.Fatal(n, err)
	}
	completed, err := f.service.SettlementJob(ctx, f.brand, job.ID)
	if err != nil || completed.State != "completed" || completed.PaidPoints != "10" {
		t.Fatal(completed, err)
	}
	settled, err := f.service.Order(ctx, f.brand, f.member, order.ID)
	if err != nil || settled.Status != "won" || settled.PrizePoints != 10 {
		t.Fatal(settled, err)
	}
	if wallet := walletBySource(t, f); wallet[0][0] != 99 || wallet[1][0] != 10 {
		t.Fatal(wallet)
	}
	var status string
	if err := f.db.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1`, f.brand).Scan(&status); err != nil || status != "paused" {
		t.Fatal(status, err)
	}
}
