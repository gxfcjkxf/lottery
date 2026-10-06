package compliance

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

func TestHistoryTxReadonlyPagingAndValidation(t *testing.T) {
	ctx := context.Background()
	s := Service{DB: testdb.New(t)}
	a := actor(t, s)
	cfg := DefaultConfig()
	cfg.IdentityEnabled = true
	if _, err := update(t, s, a, Input{Version: 1, Config: cfg, Reason: "history tx fixture"}); err != nil {
		t.Fatal(err)
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	got, err := s.HistoryTx(ctx, tx, testBrand, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	want, err := s.History(ctx, testBrand, 1, 0)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("tx history differs: got=%#v want=%#v err=%v", got, want, err)
	}
	if got.TotalCount == "0" || len(got.Items) != 1 || got.Items[0].Version != 2 {
		t.Fatalf("unexpected count and latest page: %#v", got)
	}
	second, err := s.HistoryTx(ctx, tx, testBrand, 1, 1)
	if err != nil || second.TotalCount != got.TotalCount || len(second.Items) > 1 {
		t.Fatalf("count/page mismatch across offset: first=%#v second=%#v err=%v", got, second, err)
	}
	if page, err := s.HistoryTx(ctx, tx, "0199a000-0000-7000-8000-000000000002", 20, 0); err != nil || page.BrandID != "0199a000-0000-7000-8000-000000000002" {
		t.Fatalf("cross-brand response mismatch: %#v err=%v", page, err)
	} else {
		for _, item := range page.Items {
			if item.BrandID != page.BrandID {
				t.Fatalf("history leaked across brands: %#v", item)
			}
		}
	}
	for _, tc := range []struct {
		tx            pgx.Tx
		brand         string
		limit, offset int
	}{{nil, testBrand, 20, 0}, {tx, testBrand, 0, 0}, {tx, testBrand, 101, 0}, {tx, testBrand, 20, -1}} {
		if _, err := s.HistoryTx(ctx, tc.tx, tc.brand, tc.limit, tc.offset); !errors.Is(err, ErrInvalid) {
			t.Errorf("invalid args returned %v", err)
		}
	}
	if _, err := s.HistoryTx(ctx, tx, "0199a000-0000-7000-8000-000000000099", 20, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing brand returned %v", err)
	}
}
