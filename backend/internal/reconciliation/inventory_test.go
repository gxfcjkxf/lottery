package reconciliation

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/finance"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func TestBrandInventoryReadsCompleteEmptyBrandWithoutWriting(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	before := moneyDigest(t, s)
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	out, err := s.InventoryTx(ctx, tx, testBrand)
	if err != nil {
		t.Fatal(err)
	}
	if out.BrandID != testBrand || out.SchemaVersion != 1 || !out.Consistent || out.SourceRowCount != "0" || out.ReferenceCount != "0" || out.IssueCount != "0" || out.IssuesTruncated || len(out.Issues) != 0 || len(out.Coverage) != 41 {
		t.Fatalf("incomplete empty inventory: %+v", out)
	}
	if moneyDigest(t, s) != before {
		t.Fatal("inventory moved funds")
	}
}

func TestBrandInventoryGenuineRechargeAndMissingAccountRemainDiscoverable(t *testing.T) {
	s, actor := fixture(t)
	ctx := context.Background()
	member, account := addMember(t, s, testBrand)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	meta := points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()}
	service := finance.Service{DB: s.DB, Points: points.Store{DB: s.DB}}
	order, err := service.CreateRecharge(ctx, tx, testBrand, member, 20, "owned source evidence", "", "genuine inventory fixture", meta)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err = service.ConfirmRecharge(ctx, tx, testBrand, order.ID, order.Version, "confirm actual inventory recharge", meta); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	read := func() InventorySnapshot {
		t.Helper()
		before := moneyDigest(t, s)
		tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		out, err := s.InventoryTx(ctx, tx, testBrand)
		if err != nil {
			t.Fatal(err)
		}
		if moneyDigest(t, s) != before {
			t.Fatal("source inventory altered money")
		}
		return out
	}
	good := read()
	if !good.Consistent || good.IssueCount != "0" || good.SourceRowCount == "0" {
		t.Fatalf("genuine financial history rejected %+v", good)
	}
	// Only this owned schema loses its account, with all guards restored before
	// commit. Business and ledger rows remain to prove the account-loop blind spot.
	damage, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer damage.Rollback(ctx)
	if _, err = damage.Exec(ctx, `SET LOCAL session_replication_role=replica`); err != nil {
		t.Fatal(err)
	}
	if _, err = damage.Exec(ctx, `DELETE FROM point_accounts WHERE brand_id=$1 AND id=$2`, testBrand, account); err != nil {
		t.Fatal(err)
	}
	if _, err = damage.Exec(ctx, `SET LOCAL session_replication_role=origin`); err != nil {
		t.Fatal(err)
	}
	if err = damage.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	orphan := read()
	if orphan.Consistent || orphan.IssueCount == "0" || orphan.Fingerprint == good.Fingerprint {
		t.Fatalf("orphan source hidden %+v", orphan)
	}
	found := false
	for _, issue := range orphan.Issues {
		if issue.SourceTable == "recharge_orders" && issue.SourceID == order.ID && issue.ParentTable == "point_accounts" {
			found = true
		}
	}
	if !found {
		t.Fatal("recharge missing its account was lost through an inner join")
	}
}
