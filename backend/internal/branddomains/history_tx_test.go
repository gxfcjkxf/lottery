package branddomains

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
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	got, err := s.HistoryTx(ctx, tx, brand, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	want, err := s.History(ctx, brand, 20, 0)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("tx history differs: got=%#v want=%#v err=%v", got, want, err)
	}
	if rows, err := s.HistoryTx(ctx, tx, "0199a000-0000-7000-8000-000000000001", 20, 0); err != nil || len(rows) != 0 {
		t.Fatalf("cross-brand rows leaked: %#v err=%v", rows, err)
	}
	for _, tc := range []struct {
		tx            pgx.Tx
		brand         string
		limit, offset int
	}{{nil, brand, 20, 0}, {tx, brand, 0, 0}, {tx, brand, 101, 0}, {tx, brand, 20, -1}} {
		if _, err := s.HistoryTx(ctx, tc.tx, tc.brand, tc.limit, tc.offset); !errors.Is(err, ErrInvalid) {
			t.Errorf("invalid args returned %v", err)
		}
	}
	if _, err := s.HistoryTx(ctx, tx, "0199a000-0000-7000-8000-000000000099", 20, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing brand returned %v", err)
	}
}
