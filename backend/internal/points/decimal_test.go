package points

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"testing"
)

func TestDecimalWalletTransferReversalAndNoOverdraft(t *testing.T) {
	p, s, member := fixture(t)
	credit(t, p, s, member, 1, 123)
	wallet, err := s.Read(context.Background(), testBrand, member)
	if err != nil || wallet.AvailablePoints.String() != "1.23" {
		t.Fatal(wallet, err)
	}
	allocation := []Allocation{{Source: "winning", State: "available", Points: 23}}
	delta, err := AllocationDelta(allocation, "available", "manual_frozen")
	if err != nil {
		t.Fatal(err)
	}
	input := change(member, "decimal-freeze", delta, allocation)
	input.EntryType = "freeze"
	entry := post(t, p, s, input)
	if replay := post(t, p, s, input); replay.ID != entry.ID {
		t.Fatal("duplicate decimal freeze")
	}
	wallet, err = s.Read(context.Background(), testBrand, member)
	if err != nil || wallet.AvailablePoints.String() != "1" || wallet.FrozenPoints.String() != "0.23" {
		t.Fatal(wallet, err)
	}
	if _, err = wallet.BySource.Allocate(101, "available"); !errors.Is(err, ErrInsufficient) {
		t.Fatal("decimal overdraft", err)
	}
	tx, err := p.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = s.Reverse(context.Background(), tx, testBrand, member, entry.ID, "decimal-unfreeze", "", Metadata{ActorType: "system", RequestID: ids.New()}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	wallet, err = s.Read(context.Background(), testBrand, member)
	if err != nil || wallet.AvailablePoints.String() != "1.23" || wallet.FrozenPoints != 0 {
		t.Fatal(wallet, err)
	}
}
